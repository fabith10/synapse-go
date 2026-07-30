package orchestrator

import (
	"context"
	"fmt"
	"sync"
)

// Transport defines the message delivery interface for single-node and multi-node orchestrator clusters.
type Transport interface {
	Publish(ctx context.Context, topic string, msg Message) error
	Subscribe(ctx context.Context, topic string, handler func(Message)) error
	Close() error
}

// ChannelTransport is the default in-memory Go channel implementation of Transport.
type ChannelTransport struct {
	mu          sync.RWMutex
	subscribers map[string][]func(Message)
	closed      bool
}

// NewChannelTransport creates a new ChannelTransport.
func NewChannelTransport() *ChannelTransport {
	return &ChannelTransport{
		subscribers: make(map[string][]func(Message)),
	}
}

func (t *ChannelTransport) Publish(ctx context.Context, topic string, msg Message) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.closed {
		return fmt.Errorf("transport closed")
	}

	handlers, ok := t.subscribers[topic]
	if !ok {
		return nil
	}

	for _, h := range handlers {
		handler := h
		go handler(msg)
	}

	return nil
}

func (t *ChannelTransport) Subscribe(ctx context.Context, topic string, handler func(Message)) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport closed")
	}

	t.subscribers[topic] = append(t.subscribers[topic], handler)
	return nil
}

func (t *ChannelTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}
