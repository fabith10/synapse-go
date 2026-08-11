package hitl

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/fabith10/synapse-go/pkg/logger"
	_ "github.com/mattn/go-sqlite3"
)

// PendingApproval represents a persistent HITL approval request.
type PendingApproval struct {
	ID            string            `json:"id"`
	CorrelationID string            `json:"correlation_id"`
	Sender        string            `json:"sender"`
	Action        string            `json:"action"`
	Content       string            `json:"content"`
	Status        string            `json:"status"` // PENDING, APPROVED, REJECTED, EXPIRED
	Metadata      map[string]string `json:"metadata"`
	CreatedAt     time.Time         `json:"created_at"`
	RespondedAt   *time.Time        `json:"responded_at,omitempty"`
	Decision      string            `json:"decision,omitempty"`
	Feedback      string            `json:"feedback,omitempty"`
}

// HITLGateway manages database-persisted human-in-the-loop approvals.
type HITLGateway struct {
	db *sql.DB
	mu sync.RWMutex
}

var (
	GlobalHITLGateway *HITLGateway
	gatewayOnce       sync.Once
)

// InitGlobalHITLGateway initializes the SQLite database table for HITL approvals.
func InitGlobalHITLGateway(db *sql.DB) (*HITLGateway, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
	CREATE TABLE IF NOT EXISTS pending_approvals (
		id             TEXT PRIMARY KEY,
		correlation_id TEXT NOT NULL,
		sender         TEXT NOT NULL,
		action         TEXT,
		content        TEXT NOT NULL,
		status         TEXT NOT NULL,
		metadata_json  TEXT,
		created_at     DATETIME NOT NULL,
		responded_at   DATETIME,
		decision       TEXT,
		feedback       TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_approvals_corr ON pending_approvals (correlation_id);
	CREATE INDEX IF NOT EXISTS idx_approvals_status ON pending_approvals (status);
	`

	if _, err := db.Exec(query); err != nil {
		return nil, fmt.Errorf("failed to initialize pending_approvals table: %w", err)
	}

	gateway := &HITLGateway{db: db}
	gatewayOnce.Do(func() {
		GlobalHITLGateway = gateway
	})

	logger.WithComponent("hitl_gateway").Info("Successfully initialized persistent HITL approval gateway")
	return gateway, nil
}

// GetGlobalHITLGateway returns the global HITL gateway.
func GetGlobalHITLGateway() *HITLGateway {
	return GlobalHITLGateway
}

// CreateApproval creates and persists a new pending HITL approval.
func (g *HITLGateway) CreateApproval(corrID, sender, action, content string, meta map[string]string) (*PendingApproval, error) {
	if g == nil || g.db == nil {
		return nil, fmt.Errorf("hitl gateway not initialized")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	id := fmt.Sprintf("approval-%s-%d", corrID, time.Now().UnixNano())
	now := time.Now().UTC()

	metaJSON, _ := json.Marshal(meta)

	query := `
	INSERT INTO pending_approvals (id, correlation_id, sender, action, content, status, metadata_json, created_at)
	VALUES (?, ?, ?, ?, ?, 'PENDING', ?, ?);
	`

	_, err := g.db.Exec(query, id, corrID, sender, action, content, string(metaJSON), now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert pending approval: %w", err)
	}

	app := &PendingApproval{
		ID:            id,
		CorrelationID: corrID,
		Sender:        sender,
		Action:        action,
		Content:       content,
		Status:        "PENDING",
		Metadata:      meta,
		CreatedAt:     now,
	}

	logger.WithComponent("hitl_gateway").Info("Created persistent pending approval", "id", id, "correlation_id", corrID, "sender", sender)
	return app, nil
}

// SubmitDecision records a human response (APPROVED or REJECTED) for a correlation_id.
func (g *HITLGateway) SubmitDecision(corrID, decision, feedback string) (*PendingApproval, error) {
	if g == nil || g.db == nil {
		return nil, fmt.Errorf("hitl gateway not initialized")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UTC()
	status := "APPROVED"
	if decision == "reject" || decision == "REJECTED" {
		status = "REJECTED"
	}

	query := `
	UPDATE pending_approvals
	SET status = ?, responded_at = ?, decision = ?, feedback = ?
	WHERE correlation_id = ? AND status = 'PENDING';
	`

	res, err := g.db.Exec(query, status, now, decision, feedback, corrID)
	if err != nil {
		return nil, fmt.Errorf("failed to update approval decision: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, fmt.Errorf("no pending approval found for correlation_id: %s", corrID)
	}

	logger.WithComponent("hitl_gateway").Info("Resolved pending approval", "correlation_id", corrID, "status", status, "decision", decision)

	// Fetch resolved approval
	return g.GetApprovalByCorrelationID(corrID)
}

// GetApprovalByCorrelationID retrieves the most recent approval for a correlation ID.
func (g *HITLGateway) GetApprovalByCorrelationID(corrID string) (*PendingApproval, error) {
	if g == nil || g.db == nil {
		return nil, fmt.Errorf("hitl gateway not initialized")
	}

	query := `
	SELECT id, correlation_id, sender, action, content, status, metadata_json, created_at, responded_at, decision, feedback
	FROM pending_approvals
	WHERE correlation_id = ?
	ORDER BY created_at DESC LIMIT 1;
	`

	row := g.db.QueryRow(query, corrID)

	var app PendingApproval
	var metaJSON sql.NullString
	var respondedAt sql.NullTime
	var decision, feedback sql.NullString

	err := row.Scan(&app.ID, &app.CorrelationID, &app.Sender, &app.Action, &app.Content, &app.Status, &metaJSON, &app.CreatedAt, &respondedAt, &decision, &feedback)
	if err != nil {
		return nil, err
	}

	if metaJSON.Valid {
		_ = json.Unmarshal([]byte(metaJSON.String), &app.Metadata)
	}
	if respondedAt.Valid {
		app.RespondedAt = &respondedAt.Time
	}
	if decision.Valid {
		app.Decision = decision.String
	}
	if feedback.Valid {
		app.Feedback = feedback.String
	}

	return &app, nil
}

// ListPendingApprovals returns all active pending approvals.
func (g *HITLGateway) ListPendingApprovals() ([]PendingApproval, error) {
	if g == nil || g.db == nil {
		return nil, nil
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	query := `
	SELECT id, correlation_id, sender, action, content, status, metadata_json, created_at
	FROM pending_approvals
	WHERE status = 'PENDING'
	ORDER BY created_at DESC;
	`

	rows, err := g.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PendingApproval
	for rows.Next() {
		var app PendingApproval
		var metaJSON sql.NullString
		if err := rows.Scan(&app.ID, &app.CorrelationID, &app.Sender, &app.Action, &app.Content, &app.Status, &metaJSON, &app.CreatedAt); err == nil {
			if metaJSON.Valid {
				_ = json.Unmarshal([]byte(metaJSON.String), &app.Metadata)
			}
			result = append(result, app)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pending approvals: %w", err)
	}

	return result, nil
}
