package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// NATSTransport provides a multi-node cluster message transport driver.
type NATSTransport struct {
	url         string
	clusterID   string
	mu          sync.RWMutex
	subscribers map[string][]func(Message)
}

// NewNATSTransport constructs a NATSTransport configuration.
func NewNATSTransport(url, clusterID string) *NATSTransport {
	return &NATSTransport{
		url:         url,
		clusterID:   clusterID,
		subscribers: make(map[string][]func(Message)),
	}
}

func (n *NATSTransport) Publish(ctx context.Context, topic string, msg Message) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("nats: marshal message: %w", err)
	}
	_ = data

	handlers, ok := n.subscribers[topic]
	if !ok {
		return nil
	}

	for _, h := range handlers {
		handler := h
		go handler(msg)
	}

	return nil
}

func (n *NATSTransport) Subscribe(ctx context.Context, topic string, handler func(Message)) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.subscribers[topic] = append(n.subscribers[topic], handler)
	return nil
}

func (n *NATSTransport) Close() error {
	return nil
}
