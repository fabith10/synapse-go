package adk

import (
	"context"

	"github.com/fabith10/synapse-go/internal/broker"
	"github.com/fabith10/synapse-go/internal/tools"
)

// ---------------------------------------------------------------------------
// LLMClient — adk.LLMClient standard library component (ADK_Standard.md §3.3)
// ---------------------------------------------------------------------------

// LLMClient is the hybrid routing client that wraps broker.Broker behind a
// single Generate method. Agents call Generate with their TaskRequest and
// receive a response; the broker silently handles provider selection, penalty
// scoring, circuit breaking, failover, and tier escalation.
//
// Agent code must never import or reference broker.Broker directly — always
// obtain an LLMClient from Runtime.LLMClient() and call Generate.
type LLMClient interface {
	// Generate submits messages to the Hybrid Compute Broker and blocks until
	// a provider returns a response or ctx expires. The broker applies the
	// penalty-score algorithm and failover chain defined in PDR-003.
	Generate(
		ctx context.Context,
		req TaskRequest,
		messages []Message,
	) (Message, error)
}

// ---------------------------------------------------------------------------
// brokerClient — concrete LLMClient backed by broker.Broker
// ---------------------------------------------------------------------------

type brokerClient struct {
	b        *broker.Broker
	registry *tools.Registry
}

// newLLMClient constructs an LLMClient wired to the given Broker and Registry.
// Called internally by NewRuntime; agent code accesses it via Runtime.LLMClient().
func newLLMClient(b *broker.Broker, registry *tools.Registry) LLMClient {
	return &brokerClient{b: b, registry: registry}
}

// Generate converts the registry's MCP schemas to broker.Tool entries, routes
// the task through the Hybrid Compute Broker, and returns the winning provider's
// response. The caller's context deadline is forwarded verbatim to the broker.
func (c *brokerClient) Generate(
	ctx context.Context,
	req TaskRequest,
	messages []Message,
) (Message, error) {
	// Pull all registered MCP schemas and convert to broker.Tool descriptors.
	// These are passed to LLMProvider.FormatPrompt so the model knows which
	// tools are available without coupling agent code to provider SDKs.
	schemas := c.registry.AllSchemas()
	brokerTools := make([]broker.Tool, len(schemas))
	for i, s := range schemas {
		brokerTools[i] = broker.Tool{
			Name:        s.Name,
			Description: s.Description,
			Parameters:  s.InputSchema.Properties,
		}
	}

	result, err := c.b.Route(ctx, req, messages, brokerTools)
	if err != nil {
		return Message{}, err
	}
	return result.Response, nil
}
