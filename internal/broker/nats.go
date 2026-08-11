package broker

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fabith10/synapse-go/pkg/logger"
	"github.com/nats-io/nats.go"
)

// DefaultNATSURL is the default connection string for local NATS server.
const DefaultNATSURL = "nats://localhost:4222"

// StreamName is the JetStream stream name for agent event persistence.
const StreamName = "AGENTS_EVENTS"

// NATSBroker manages distributed NATS connections, JetStream streams, and subscriptions.
type NATSBroker struct {
	nc        *nats.Conn
	js        nats.JetStreamContext
	subs      map[string]*nats.Subscription
	mu        sync.RWMutex
	connected bool
}

// GlobalNATSBroker is the global instance of the NATS broker.
var (
	GlobalNATSBroker *NATSBroker
	globalBrokerMu   sync.RWMutex
)

// InitGlobalNATSBroker connects to NATS using NATS_URL environment variable or default URL.
// Returns nil if NATS is unavailable or not configured, allowing fallback to in-memory broker.
func InitGlobalNATSBroker() *NATSBroker {
	globalBrokerMu.Lock()
	defer globalBrokerMu.Unlock()

	url := os.Getenv("NATS_URL")
	if url == "" {
		url = DefaultNATSURL
	}

	broker, err := NewNATSBroker(url)
	if err != nil {
		logger.WithComponent("broker").Info("NATS server unconfigured or unavailable; using in-memory broker fallback", "url", url, "error", err)
		return nil
	}

	GlobalNATSBroker = broker
	logger.WithComponent("broker").Info("Successfully connected to NATS JetStream broker", "url", url)
	return broker
}

// GetGlobalNATSBroker returns the active global NATS broker instance if connected.
func GetGlobalNATSBroker() *NATSBroker {
	globalBrokerMu.RLock()
	defer globalBrokerMu.RUnlock()
	if GlobalNATSBroker != nil && GlobalNATSBroker.IsConnected() {
		return GlobalNATSBroker
	}
	return nil
}

// NewNATSBroker initializes a new connection to NATS and configures JetStream.
func NewNATSBroker(url string) (*NATSBroker, error) {
	opts := []nats.Option{
		nats.Name("Synapse Agent Framework"),
		nats.Timeout(2 * time.Second),
		nats.MaxReconnects(3),
		nats.ReconnectWait(1 * time.Second),
	}

	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats jetstream: %w", err)
	}

	// Ensure JetStream stream exists
	_, err = js.StreamInfo(StreamName)
	if err != nil {
		_, err = js.AddStream(&nats.StreamConfig{
			Name:      StreamName,
			Subjects:  []string{"agents.>"},
			Retention: nats.LimitsPolicy,
			MaxAge:    24 * time.Hour,
			Storage:   nats.FileStorage,
		})
		if err != nil {
			logger.WithComponent("broker").Warn("Failed to create JetStream stream (running in core NATS pub/sub mode)", "error", err)
		}
	}

	b := &NATSBroker{
		nc:        nc,
		js:        js,
		subs:      make(map[string]*nats.Subscription),
		connected: true,
	}

	return b, nil
}

// IsConnected checks if the NATS connection is active.
func (b *NATSBroker) IsConnected() bool {
	if b == nil || b.nc == nil {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected && b.nc.IsConnected()
}

// Publish publishes a JSON payload to a NATS subject.
func (b *NATSBroker) Publish(subject string, payload interface{}) error {
	if !b.IsConnected() {
		return fmt.Errorf("nats broker not connected")
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal nats payload: %w", err)
	}

	// Clean subject string
	cleanSubject := strings.ReplaceAll(subject, "/", ".")
	return b.nc.Publish(cleanSubject, data)
}

// Subscribe subscribes to a NATS subject and dispatches incoming messages to handler.
func (b *NATSBroker) Subscribe(subject string, handler func(msg []byte)) error {
	if !b.IsConnected() {
		return fmt.Errorf("nats broker not connected")
	}

	cleanSubject := strings.ReplaceAll(subject, "/", ".")

	b.mu.Lock()
	defer b.mu.Unlock()

	sub, err := b.nc.Subscribe(cleanSubject, func(m *nats.Msg) {
		handler(m.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to nats subject %s: %w", cleanSubject, err)
	}

	b.subs[cleanSubject] = sub
	return nil
}

// Close closes all subscriptions and the NATS connection.
func (b *NATSBroker) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, sub := range b.subs {
		_ = sub.Unsubscribe()
	}
	if b.nc != nil {
		b.nc.Close()
	}
	b.connected = false
}
