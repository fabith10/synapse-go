package agent

import (
	"sync"

	"github.com/fabith10/agent-framework/adk"
)

var (
	pendingResponsesMu sync.Mutex
	pendingResponses   = make(map[string]chan adk.Message)
)

func init() {
	adk.ResponseDispatcher = DispatchResponse
}

// RegisterPendingResponse registers a channel to receive a response for the given correlationID.
func RegisterPendingResponse(correlationID string, ch chan adk.Message) {
	pendingResponsesMu.Lock()
	defer pendingResponsesMu.Unlock()
	pendingResponses[correlationID] = ch
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
	if ch, ok := pendingResponses[correlationID]; ok {
		select {
		case ch <- msg:
			return true
		default:
			return false
		}
	}
	return false
}
