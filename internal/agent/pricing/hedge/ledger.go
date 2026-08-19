package hedge

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// ContractType defines whether a hedge is a Forward or Call Option.
type ContractType string

const (
	TypeForward    ContractType = "forward"
	TypeCallOption ContractType = "call_option"
)

// ContractStatus defines the lifecycle state of a derivative contract.
type ContractStatus string

const (
	StatusActive    ContractStatus = "active"
	StatusExercised ContractStatus = "exercised"
	StatusExpired   ContractStatus = "expired"
	StatusClosed    ContractStatus = "closed"
)

// HedgeContract represents an active, stateful derivative contract position.
type HedgeContract struct {
	ID               string         `json:"contract_id"`
	Asset            string         `json:"asset"`
	Type             ContractType   `json:"contract_type"`
	Status           ContractStatus `json:"status"`
	StrikeRateUSD    float64        `json:"strike_rate_usd_hr"` // Locked rate (Forward) or Ceiling rate (Call Strike)
	PremiumPaidUSD   float64        `json:"premium_paid_usd"`   // Upfront Black-76 option premium
	AllocatedHours   float64        `json:"allocated_hours"`
	ConsumedHours    float64        `json:"consumed_hours"`
	RemainingHours   float64        `json:"remaining_hours"`
	TotalSavingsUSD  float64        `json:"total_savings_usd"`  // Realized downside protection savings
	EnteredAt        time.Time      `json:"entered_at"`
	ExpiresAt        time.Time      `json:"expires_at"`
}

// HedgeLedger is a thread-safe repository tracking derivative contracts and positions.
type HedgeLedger struct {
	mu        sync.RWMutex
	contracts map[string]*HedgeContract
}

var (
	globalLedgerOnce sync.Once
	globalLedger     *HedgeLedger
)

// GetGlobalHedgeLedger returns the shared singleton derivative ledger.
func GetGlobalHedgeLedger() *HedgeLedger {
	globalLedgerOnce.Do(func() {
		globalLedger = NewHedgeLedger()
	})
	return globalLedger
}

// NewHedgeLedger initializes an in-memory hedge book.
func NewHedgeLedger() *HedgeLedger {
	return &HedgeLedger{
		contracts: make(map[string]*HedgeContract),
	}
}

// BookForward creates and activates a forward hedge contract locking compute costs.
func (l *HedgeLedger) BookForward(asset string, forwardRate float64, durationHours int, expiryDays float64) (*HedgeContract, error) {
	if asset == "" {
		return nil, errors.New("asset cannot be empty")
	}
	if forwardRate <= 0 {
		return nil, errors.New("forward_rate must be positive")
	}
	if durationHours <= 0 {
		durationHours = 4
	}
	if expiryDays <= 0 {
		expiryDays = 30.0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().UTC()
	contract := &HedgeContract{
		ID:              generateContractID("fwd", asset),
		Asset:           asset,
		Type:            TypeForward,
		Status:          StatusActive,
		StrikeRateUSD:   roundTo4(forwardRate),
		PremiumPaidUSD:  0.0,
		AllocatedHours:  float64(durationHours),
		ConsumedHours:   0.0,
		RemainingHours:  float64(durationHours),
		TotalSavingsUSD: 0.0,
		EnteredAt:       now,
		ExpiresAt:       now.Add(time.Duration(expiryDays*24) * time.Hour),
	}

	l.contracts[contract.ID] = contract
	return contract, nil
}

// BuyCallOption books a European call option buffer providing a rate ceiling against spot spikes.
func (l *HedgeLedger) BuyCallOption(asset string, strikeRate float64, premiumUSD float64, durationHours int, expiryDays float64) (*HedgeContract, error) {
	if asset == "" {
		return nil, errors.New("asset cannot be empty")
	}
	if strikeRate <= 0 {
		return nil, errors.New("strike_rate must be positive")
	}
	if durationHours <= 0 {
		durationHours = 4
	}
	if expiryDays <= 0 {
		expiryDays = 30.0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().UTC()
	contract := &HedgeContract{
		ID:              generateContractID("opt", asset),
		Asset:           asset,
		Type:            TypeCallOption,
		Status:          StatusActive,
		StrikeRateUSD:   roundTo4(strikeRate),
		PremiumPaidUSD:  roundTo4(premiumUSD),
		AllocatedHours:  float64(durationHours),
		ConsumedHours:   0.0,
		RemainingHours:  float64(durationHours),
		TotalSavingsUSD: 0.0,
		EnteredAt:       now,
		ExpiresAt:       now.Add(time.Duration(expiryDays*24) * time.Hour),
	}

	l.contracts[contract.ID] = contract
	return contract, nil
}

// ConsumeHedge is called by the Compute Broker when provisioning spot compute.
// If an active contract covers the asset, it computes the effective billing rate and realizes savings.
func (l *HedgeLedger) ConsumeHedge(asset string, requestedHours float64, currentSpotRate float64) (effectiveRate float64, savingsUSD float64, contractID string) {
	if requestedHours <= 0 {
		requestedHours = 1.0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().UTC()

	// Find the best active contract for this asset (lowest strike)
	var bestContract *HedgeContract
	for _, c := range l.contracts {
		if c.Asset != asset || c.Status != StatusActive {
			continue
		}
		if now.After(c.ExpiresAt) {
			c.Status = StatusExpired
			continue
		}
		if c.RemainingHours <= 0 {
			c.Status = StatusClosed
			continue
		}
		if bestContract == nil || c.StrikeRateUSD < bestContract.StrikeRateUSD {
			bestContract = c
		}
	}

	if bestContract == nil {
		// No active hedge position -> pay raw spot rate
		return currentSpotRate, 0.0, ""
	}

	hoursToConsume := math.Min(requestedHours, bestContract.RemainingHours)
	bestContract.ConsumedHours += hoursToConsume
	bestContract.RemainingHours -= hoursToConsume

	switch bestContract.Type {
	case TypeForward:
		// Forward locks the price strictly at StrikeRateUSD
		effectiveRate = bestContract.StrikeRateUSD
		if currentSpotRate > effectiveRate {
			savingsUSD = (currentSpotRate - effectiveRate) * hoursToConsume
		}
	case TypeCallOption:
		// Call Option acts as a ceiling: effectiveRate = min(Spot, Strike)
		if currentSpotRate > bestContract.StrikeRateUSD {
			effectiveRate = bestContract.StrikeRateUSD
			savingsUSD = (currentSpotRate - bestContract.StrikeRateUSD) * hoursToConsume
			bestContract.Status = StatusExercised
		} else {
			effectiveRate = currentSpotRate
			savingsUSD = 0.0
		}
	}

	if bestContract.RemainingHours <= 0 {
		bestContract.Status = StatusClosed
	}

	bestContract.TotalSavingsUSD += savingsUSD
	return roundTo4(effectiveRate), roundTo4(savingsUSD), bestContract.ID
}

// ListPositions returns all contracts in the ledger.
func (l *HedgeLedger) ListPositions() []HedgeContract {
	l.mu.RLock()
	defer l.mu.RUnlock()

	now := time.Now().UTC()
	res := make([]HedgeContract, 0, len(l.contracts))
	for _, c := range l.contracts {
		copyC := *c
		if copyC.Status == StatusActive && now.After(copyC.ExpiresAt) {
			copyC.Status = StatusExpired
		}
		res = append(res, copyC)
	}
	return res
}

// ClosePosition terminates an open contract.
func (l *HedgeLedger) ClosePosition(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	c, ok := l.contracts[id]
	if !ok {
		return fmt.Errorf("contract %q not found", id)
	}
	c.Status = StatusClosed
	return nil
}

func generateContractID(prefix, asset string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("hedge-%s-%s-%s", prefix, strings.ToLower(asset), hex.EncodeToString(b))
}

func roundTo4(v float64) float64 {
	return math.Round(v*10000.0) / 10000.0
}
