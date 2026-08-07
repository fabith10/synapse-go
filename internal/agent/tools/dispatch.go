package agenttools

import (
	"sync"
	"time"

	"github.com/fabith10/synapse-go/adk"
)

type pendingEntry struct {
	ch        chan adk.Message
	createdAt time.Time
}

var (
	// ponytail: In-memory map and channel storage with stdlib TTL auto-pruning for response correlation; upgrade to distributed message broker (e.g. NATS/Redis PubSub) for multi-node agent dispatching.
	pendingResponsesMu sync.Mutex
	pendingResponses   = make(map[string]pendingEntry)
)

func init() {
	adk.ResponseDispatcher = DispatchResponse
}

// RegisterPendingResponse registers a channel to receive a response for the given correlationID.
func RegisterPendingResponse(correlationID string, ch chan adk.Message) {
	pendingResponsesMu.Lock()
	defer pendingResponsesMu.Unlock()

	// Prune abandoned correlation entries older than 30 minutes
	now := time.Now()
	for id, entry := range pendingResponses {
		if now.Sub(entry.createdAt) > 30*time.Minute {
			delete(pendingResponses, id)
		}
	}

	pendingResponses[correlationID] = pendingEntry{
		ch:        ch,
		createdAt: now,
	}
}

// UnregisterPendingResponse unregisters the channel for the given correlationID.
func UnregisterPendingResponse(correlationID string) {
	pendingResponsesMu.Lock()
	defer pendingResponsesMu.Unlock()
	delete(pendingResponses, correlationID)
}

// DispatchResponse routes a message to a registered waiting channel if one exists.
func DispatchResponse(correlationID string, msg adk.Message) bool {
	pendingResponsesMu.Lock()
	defer pendingResponsesMu.Unlock()
	if entry, ok := pendingResponses[correlationID]; ok {
		select {
		case entry.ch <- msg:
			return true
		default:
			return false
		}
	}
	return false
}

