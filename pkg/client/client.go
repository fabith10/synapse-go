package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Orchestration mode constants.
const (
	ModeAutonomous    = "autonomous"
	ModeSteered       = "steered"
	ModeCollaborative = "collaborative"

	DecisionApprove    = "APPROVED"
	DecisionReject     = "REJECTED"
	DecisionApproveAll = "APPROVE_ALL"
)

// TaskRequest defines parameters for dispatching a swarm task.
type TaskRequest struct {
	Prompt         string            `json:"prompt,omitempty"`
	Content        string            `json:"content,omitempty"`
	Mode           string            `json:"mode,omitempty"`
	CorrelationID  string            `json:"correlation_id,omitempty"`
	Wait           bool              `json:"wait,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// TaskResponse represents the API response for task dispatch and synchronous execution.
type TaskResponse struct {
	Success  bool              `json:"success"`
	TaskID   string            `json:"task_id"`
	Status   string            `json:"status,omitempty"` // "COMPLETED", "TIMEOUT", "DISPATCHED"
	Sender   string            `json:"sender,omitempty"`
	Response string            `json:"response,omitempty"`
	Message  string            `json:"message,omitempty"`
	Error    string            `json:"error,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// TelemetryEvent represents a typed, structured framework event from the swarm event stream.
type TelemetryEvent struct {
	Time       string            `json:"time"`
	Sender     string            `json:"sender"`
	Recipient  string            `json:"recipient"`
	EventType  string            `json:"event_type"` // TASK_DISPATCH, TOOL_CALL, SUBTASK_COMPLETE, TASK_COMPLETE, QA_EVAL, PROGRESS, INFO
	Category   string            `json:"category"`   // dispatch, tool_call, outcome, supervisor, general
	ActiveTool string            `json:"active_tool,omitempty"`
	Model      string            `json:"model,omitempty"`
	Content    string            `json:"content"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// PendingApproval represents a Human-in-the-Loop decision gate.
type PendingApproval struct {
	ID            string            `json:"id"`
	CorrelationID string            `json:"correlation_id"`
	AgentID       string            `json:"agent_id"`
	Action        string            `json:"action"`
	Details       string            `json:"details"`
	Status        string            `json:"status"`
	CreatedAt     string            `json:"created_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Client is an HTTP SDK client for communicating with the agent framework service.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// Option customizes client behavior.
type Option func(*Client)

// WithToken configures the Bearer / API token for machine authentication.
func WithToken(token string) Option {
	return func(c *Client) {
		c.token = strings.TrimSpace(token)
	}
}

// WithHTTPClient configures a custom http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// New creates an initialized Synapse SDK Client.
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("X-API-Key", c.token)
	}
}

// Execute submits a task synchronously and blocks until the agent swarm completes its plan or ctx expires.
func (c *Client) Execute(ctx context.Context, req TaskRequest) (*TaskResponse, error) {
	req.Wait = true
	return c.dispatchInternal(ctx, req)
}

// Dispatch submits a task asynchronously and immediately returns the correlation TaskID.
func (c *Client) Dispatch(ctx context.Context, req TaskRequest) (*TaskResponse, error) {
	req.Wait = false
	return c.dispatchInternal(ctx, req)
}

func (c *Client) dispatchInternal(ctx context.Context, req TaskRequest) (*TaskResponse, error) {
	if req.Content == "" && req.Prompt != "" {
		req.Content = req.Prompt
	}
	if req.Mode == "" {
		req.Mode = ModeAutonomous
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("client: marshal task request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/task", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("client: new request: %w", err)
	}
	c.applyHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("client: dispatch request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("client: read response body: %w", err)
	}

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusGatewayTimeout {
		return nil, fmt.Errorf("client: server returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var taskResp TaskResponse
	if err := json.Unmarshal(respBytes, &taskResp); err != nil {
		return nil, fmt.Errorf("client: unmarshal task response: %w (body: %s)", err, string(respBytes))
	}

	return &taskResp, nil
}

// Stream connects to the JSON Server-Sent Events stream (/stream/events) and yields structured telemetry events.
func (c *Client) Stream(ctx context.Context) (<-chan TelemetryEvent, <-chan error, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/stream/events", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("client: stream request: %w", err)
	}
	c.applyHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	// Use an un-timeouted client for continuous SSE streaming
	streamClient := &http.Client{Timeout: 0}
	if c.httpClient != nil && c.httpClient.Transport != nil {
		streamClient.Transport = c.httpClient.Transport
	}

	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("client: connect stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("client: stream returned HTTP %d", resp.StatusCode)
	}

	eventsCh := make(chan TelemetryEvent, 64)
	errCh := make(chan error, 1)

	go func() {
		defer resp.Body.Close()
		defer close(eventsCh)
		defer close(errCh)

		reader := bufio.NewReader(resp.Body)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if ctx.Err() == nil && err != io.EOF {
					errCh <- err
				}
				return
			}

			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}

			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}

			var event TelemetryEvent
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				select {
				case eventsCh <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return eventsCh, errCh, nil
}

// ListPendingApprovals queries any active Human-in-the-Loop decision checkpoints.
func (c *Client) ListPendingApprovals(ctx context.Context) ([]PendingApproval, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/hitl/pending", nil)
	if err != nil {
		return nil, fmt.Errorf("client: new request: %w", err)
	}
	c.applyHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("client: list pending approvals: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("client: server returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var list []PendingApproval
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("client: decode pending approvals: %w", err)
	}
	return list, nil
}

// ResolveApproval submits an approval or rejection decision for a gated action.
func (c *Client) ResolveApproval(ctx context.Context, correlationID string, decision string) error {
	payload := map[string]string{
		"correlation_id": correlationID,
		"decision":       decision,
	}
	bodyBytes, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/hitl/respond", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("client: new request: %w", err)
	}
	c.applyHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("client: resolve approval: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("client: server returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CheckHealth queries the server health endpoint.
func (c *Client) CheckHealth(ctx context.Context) (map[string]interface{}, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/system/health", nil)
	if err != nil {
		return nil, fmt.Errorf("client: new request: %w", err)
	}
	c.applyHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("client: health check: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("client: decode health response: %w", err)
	}
	return result, nil
}
