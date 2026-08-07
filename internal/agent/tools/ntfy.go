package agenttools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/pkg/logger"
)

// NtfyNotification represents the payload parameters for sending an ntfy notification.
type NtfyNotification struct {
	Topic     string   `json:"topic,omitempty"`
	Message   string   `json:"message"`
	Title     string   `json:"title,omitempty"`
	Priority  int      `json:"priority,omitempty"` // 1 (min) to 5 (max)
	Tags      []string `json:"tags,omitempty"`
	ClickURL  string   `json:"click,omitempty"`
	ServerURL string   `json:"server_url,omitempty"`
	AuthToken string   `json:"auth_token,omitempty"`
}

// NtfyIncomingMessage represents an incoming message from the ntfy JSON stream endpoint.
type NtfyIncomingMessage struct {
	ID            string   `json:"id"`
	Time          int64    `json:"time"`
	Event         string   `json:"event"` // "open", "keepalive", "message"
	Topic         string   `json:"topic"`
	Title         string   `json:"title,omitempty"`
	Message       string   `json:"message"`
	Priority      int      `json:"priority,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Click         string   `json:"click,omitempty"`
	Attachment    interface{} `json:"attachment,omitempty"`
	CorrelationID string   `json:"correlation_id,omitempty"`
}

// GetSendNtfyNotificationTool returns the Tier 1 native tool for publishing notifications over ntfy.
func GetSendNtfyNotificationTool() adk.Tool {
	return adk.Tool{
		Name:        "send_ntfy_notification",
		Description: "Publishes a push notification or message to an ntfy topic (e.g. ntfy.sh or self-hosted ntfy server).",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"topic": map[string]interface{}{
					"type":        "string",
					"description": "Target ntfy topic name. If omitted, uses default configured NTFY_TOPIC.",
				},
				"message": map[string]interface{}{
					"type":        "string",
					"description": "Notification content body.",
				},
				"title": map[string]interface{}{
					"type":        "string",
					"description": "Optional title for the notification.",
				},
				"priority": map[string]interface{}{
					"type":        "integer",
					"description": "Notification priority: 1 (min), 2 (low), 3 (default), 4 (high), 5 (urgent/max).",
				},
				"tags": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "List of emoji tags or keywords (e.g. [\"warning\", \"robot\"]).",
				},
				"click_url": map[string]interface{}{
					"type":        "string",
					"description": "URL to open when the user clicks the notification.",
				},
				"server_url": map[string]interface{}{
					"type":        "string",
					"description": "Custom ntfy server base URL (e.g. https://ntfy.sh or http://localhost:8080). Defaults to NTFY_SERVER env var or https://ntfy.sh.",
				},
				"auth_token": map[string]interface{}{
					"type":        "string",
					"description": "Optional bearer token for protected ntfy topics.",
				},
			},
			"required": []string{"message"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var notif NtfyNotification
			if err := json.Unmarshal(args, &notif); err != nil {
				return "", fmt.Errorf("failed to parse ntfy parameters: %w", err)
			}

			if notif.Message == "" {
				return "", fmt.Errorf("notification message cannot be empty")
			}

			topic := strings.TrimSpace(notif.Topic)
			if topic == "" {
				topic = strings.TrimSpace(os.Getenv("NTFY_TOPIC"))
			}
			if topic == "" {
				return "", fmt.Errorf("no ntfy topic specified and NTFY_TOPIC environment variable is unset")
			}

			serverURL := strings.TrimSpace(notif.ServerURL)
			if serverURL == "" {
				serverURL = strings.TrimSpace(os.Getenv("NTFY_SERVER"))
			}
			if serverURL == "" {
				serverURL = "https://ntfy.sh"
			}
			serverURL = strings.TrimRight(serverURL, "/")

			authToken := strings.TrimSpace(notif.AuthToken)
			if authToken == "" {
				authToken = strings.TrimSpace(os.Getenv("NTFY_AUTH_TOKEN"))
			}

			reqBody := map[string]interface{}{
				"topic":   topic,
				"message": notif.Message,
			}
			if notif.Title != "" {
				reqBody["title"] = notif.Title
			}
			if notif.Priority > 0 {
				reqBody["priority"] = notif.Priority
			}
			if len(notif.Tags) > 0 {
				reqBody["tags"] = notif.Tags
			}
			if notif.ClickURL != "" {
				reqBody["click"] = notif.ClickURL
			}

			jsonBytes, err := json.Marshal(reqBody)
			if err != nil {
				return "", fmt.Errorf("failed to marshal ntfy request payload: %w", err)
			}

			postURL := serverURL + "/"
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewBuffer(jsonBytes))
			if err != nil {
				return "", fmt.Errorf("failed to create ntfy HTTP request: %w", err)
			}

			req.Header.Set("Content-Type", "application/json")
			if authToken != "" {
				req.Header.Set("Authorization", "Bearer "+authToken)
			}

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("failed to send ntfy notification HTTP request: %w", err)
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return "", fmt.Errorf("ntfy server returned error (status %d): %s", resp.StatusCode, string(respBody))
			}

			return fmt.Sprintf("Successfully sent ntfy notification to topic %q on server %q", topic, serverURL), nil
		},
	}
}

// NtfyConfig defines setup options for the background inbound ntfy listener.
type NtfyConfig struct {
	ServerURL     string
	Topic         string
	AuthToken     string
	TargetAgentID string // Default recipient agent (e.g. "triage-agent")
}

// NtfyListener listens to an ntfy topic JSON stream and routes incoming messages to agents/HITL.
type NtfyListener struct {
	cfg   NtfyConfig
	orch  *adk.Orchestrator
	mu    sync.Mutex
	stop  chan struct{}
	done  chan struct{}
}

// NewNtfyListener constructs a new background listener for ntfy topics.
func NewNtfyListener(orch *adk.Orchestrator, cfg NtfyConfig) *NtfyListener {
	if cfg.ServerURL == "" {
		cfg.ServerURL = os.Getenv("NTFY_SERVER")
	}
	if cfg.ServerURL == "" {
		cfg.ServerURL = "https://ntfy.sh"
	}
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")

	if cfg.Topic == "" {
		cfg.Topic = os.Getenv("NTFY_TOPIC")
	}
	if cfg.AuthToken == "" {
		cfg.AuthToken = os.Getenv("NTFY_AUTH_TOKEN")
	}
	if cfg.TargetAgentID == "" {
		cfg.TargetAgentID = "triage-agent"
	}

	return &NtfyListener{
		cfg:  cfg,
		orch: orch,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// Start launches the inbound streaming reader loop in a background goroutine.
func (l *NtfyListener) Start(ctx context.Context) error {
	if l.cfg.Topic == "" {
		return fmt.Errorf("ntfy listener start failed: topic is empty")
	}

	go l.loop(ctx)
	return nil
}

// Stop cleanly terminates the background listener.
func (l *NtfyListener) Stop() {
	l.mu.Lock()
	select {
	case <-l.stop:
		l.mu.Unlock()
		return
	default:
		close(l.stop)
	}
	l.mu.Unlock()

	<-l.done
}

func (l *NtfyListener) loop(ctx context.Context) {
	defer close(l.done)

	// ponytail: Naive backoff algorithm with stdlib timers; upgrade to exponential jitter backoff for production network resiliency.
	backoff := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stop:
			return
		default:
		}

		err := l.stream(ctx)
		if err != nil && ctx.Err() == nil {
			select {
			case <-l.stop:
				return
			case <-time.After(backoff):
				if backoff < 30*time.Second {
					backoff *= 2
				}
			}
		} else {
			backoff = 1 * time.Second
		}
	}
}

func (l *NtfyListener) stream(ctx context.Context) error {
	streamURL := fmt.Sprintf("%s/%s/json", l.cfg.ServerURL, l.cfg.Topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return err
	}

	if l.cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+l.cfg.AuthToken)
	}

	client := &http.Client{Timeout: 0} // Long-lived HTTP stream reader
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ntfy stream HTTP error %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.stop:
			return nil
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg NtfyIncomingMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		if msg.Event != "message" || strings.TrimSpace(msg.Message) == "" {
			continue
		}

		l.handleMessage(msg)
	}

	return scanner.Err()
}

func (l *NtfyListener) handleMessage(msg NtfyIncomingMessage) {
	// Extract correlation ID if embedded in title, tags, or message
	corrID := msg.CorrelationID
	if corrID == "" {
		for _, tag := range msg.Tags {
			if strings.HasPrefix(tag, "corr-") || strings.HasPrefix(tag, "corr_") {
				corrID = tag
				break
			}
		}
	}
	if corrID == "" && strings.Contains(msg.Message, "correlation_id:") {
		parts := strings.Split(msg.Message, "correlation_id:")
		if len(parts) > 1 {
			corrID = strings.TrimSpace(strings.Fields(parts[1])[0])
		}
	}

	adkMsg := adk.Message{
		Sender:    fmt.Sprintf("ntfy:%s", msg.Topic),
		Recipient: l.cfg.TargetAgentID,
		Content:   msg.Message,
		Metadata: map[string]string{
			"Channel":     "ntfy",
			"ntfy_id":     msg.ID,
			"ntfy_topic":  msg.Topic,
			"ntfy_title":  msg.Title,
			"ntfy_click":  msg.Click,
		},
	}

	if corrID != "" {
		adkMsg.Metadata["correlation_id"] = corrID
		// Try routing directly to pending HITL checkpoint if registered
		if adk.ResponseDispatcher != nil {
			dispatched := adk.ResponseDispatcher(corrID, adkMsg)
			if dispatched {
				logger.Info("Dispatched ntfy response to waiting HITL checkpoint", "correlation_id", corrID, "topic", msg.Topic)
				return
			}
		}
	}

	// Fallback route to target agent mailbox via Orchestrator bus
	if l.orch != nil {
		l.orch.Send(adkMsg)
		logger.Info("Routed incoming ntfy message to agent mailbox", "recipient", l.cfg.TargetAgentID, "topic", msg.Topic)
	}
}

// StartNtfyListener launches a managed background ntfy stream listener if configured.
func StartNtfyListener(ctx context.Context, orch *adk.Orchestrator, cfg NtfyConfig) (*NtfyListener, error) {
	listener := NewNtfyListener(orch, cfg)
	if err := listener.Start(ctx); err != nil {
		return nil, err
	}
	return listener, nil
}
