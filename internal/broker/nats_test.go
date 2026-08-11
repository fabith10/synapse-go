package broker

import (
	"testing"
)

func TestNATSBrokerFallback(t *testing.T) {
	// Attempt connecting to unroutable NATS URL to test clean fallback
	broker, err := NewNATSBroker("nats://127.0.0.1:59999")
	if err == nil {
		t.Fatalf("expected connection error for unroutable URL, got nil")
	}
	if broker.IsConnected() {
		t.Fatalf("expected IsConnected to return false")
	}
}
