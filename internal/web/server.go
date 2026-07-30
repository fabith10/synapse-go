package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/agent"
	"github.com/fabith10/synapse-go/internal/memory"
	"github.com/fabith10/synapse-go/pkg/logger"
)

//go:embed static/*
var staticFS embed.FS

// LogBroker manages subscribers for the Server-Sent Events (SSE) logs.
type LogBroker struct {
	mu          sync.Mutex
	subscribers map[chan string]struct{}
	history     []string
}

func NewLogBroker() *LogBroker {
	return &LogBroker{
		subscribers: make(map[chan string]struct{}),
		history:     make([]string, 0, 200),
	}
}

func (lb *LogBroker) Subscribe() (chan string, []string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	ch := make(chan string, 128)
	lb.subscribers[ch] = struct{}{}
	histCopy := make([]string, len(lb.history))
	copy(histCopy, lb.history)
	return ch, histCopy
}

func (lb *LogBroker) Unsubscribe(ch chan string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	delete(lb.subscribers, ch)
	close(ch)
}

func (lb *LogBroker) Broadcast(msg string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.history = append(lb.history, msg)
	if len(lb.history) > 200 {
		lb.history = lb.history[len(lb.history)-200:]
	}
	for ch := range lb.subscribers {
		select {
		case ch <- msg:
		default:
			// Buffer full, skip
		}
	}
}

// ConvTurn is one entry in the rolling conversation history.
type ConvTurn struct {
	Role    string // "USER" or the agent ID
	Content string
}

// Server acts as the web backend serving dashboard templates and HTMX requests.
type Server struct {
	runtime     *adk.Runtime
	logBroker   *LogBroker
	approvals   map[string]adk.Message
	approvalsMu sync.Mutex

	// convHistory keeps the last maxHistoryTurns turns so every new task
	// dispatch arrives at triage with full conversation context.
	convHistory []ConvTurn
	historyMu   sync.Mutex

	mockTaskStore   *MockTaskStore
	mockTaskStoreMu sync.Once
}

const maxHistoryTurns = 10


func NewServer(rt *adk.Runtime) *Server {
	s := &Server{
		runtime:       rt,
		logBroker:     NewLogBroker(),
		approvals:     make(map[string]adk.Message),
		mockTaskStore: NewMockTaskStore(),
	}
	if agent.GlobalScheduler != nil {
		agent.GlobalScheduler.SetLogger(func(sender, recipient, content string) {
			s.LogEvent(sender, recipient, content)
		})
	}

	// Broadcast initial mock & system warnings to log broker & terminal stdout
	go func() {
		time.Sleep(100 * time.Millisecond)
		for _, w := range s.checkWarnings() {
			logger.WithComponent("bootstrap").Warn("System Warning", "warning", w)
			s.LogEvent("SYSTEM", "WARNING", w)
		}
	}()

	return s
}

// LogEvent helper to broadcast formatted logs.
func (s *Server) LogEvent(sender, recipient, content string) {
	logger.WithComponent("web").Info("LogEvent", "sender", sender, "recipient", recipient, "content", content)

	// RawJSON is a JSON-encoded string literal so it can be safely assigned
	// to a JS variable inside an inline <script> without HTML/newline issues.
	rawJSONBytes, _ := json.Marshal(content)

	var buf bytes.Buffer
	LogSnippetTemplate.Execute(&buf, map[string]interface{}{
		"Time":      time.Now().Format("15:04:05"),
		"Sender":    sender,
		"Recipient": recipient,
		"Content":   content,
		"RawJSON":   template.JS(rawJSONBytes), // safe JS literal
	})
	cleanHTML := strings.ReplaceAll(buf.String(), "\n", " ")
	cleanHTML = strings.ReplaceAll(cleanHTML, "\r", " ")
	sseMsg := fmt.Sprintf("event: log-message\ndata: %s\n\n", cleanHTML)
	logger.WithComponent("web").Debug("SSE Broadcast payload", "payload", sseMsg)
	s.logBroker.Broadcast(sseMsg)
}

// addToHistory appends a turn to the rolling conversation window (capped at maxHistoryTurns).
func (s *Server) addToHistory(role, content string) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.convHistory = append(s.convHistory, ConvTurn{Role: role, Content: content})
	if len(s.convHistory) > maxHistoryTurns {
		s.convHistory = s.convHistory[len(s.convHistory)-maxHistoryTurns:]
	}
}

// buildHistoryBlock returns a <conversation_history> block for injection into task messages.
// Returns an empty string when there is no prior conversation.
func (s *Server) buildHistoryBlock() string {
	s.historyMu.Lock()
	snapshot := make([]ConvTurn, len(s.convHistory))
	copy(snapshot, s.convHistory)
	s.historyMu.Unlock()

	if len(snapshot) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<conversation_history>\n")
	for _, t := range snapshot {
		// Truncate very long entries so the LLM context doesn't explode.
		body := t.Content
		if len(body) > 800 {
			body = body[:800] + "…[truncated]"
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", t.Role, body))
	}
	sb.WriteString("</conversation_history>\n\n")
	return sb.String()
}

// StartHITLListener polls HumanApprovalChan to feed approvals and final responses to the web.
func (s *Server) StartHITLListener(ctx context.Context) {
	approvalCh := s.runtime.Orchestrator().HumanApprovalChan()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-approvalCh:
				if msg.Metadata != nil && msg.Metadata["Type"] == "HITL_APPROVAL" {
					// Store approval
					s.approvalsMu.Lock()
					corrID := msg.Metadata["correlation_id"]
					if corrID == "" {
						corrID = "unknown"
					}
					s.approvals[corrID] = msg
					s.approvalsMu.Unlock()

					// Broadcast approval alert to front-end SSE channel
					var buf bytes.Buffer
					PendingApprovalTemplate.Execute(&buf, struct {
						Sender        string
						Content       string
						CorrelationID string
						Metadata      map[string]string
					}{
						Sender:        msg.Sender,
						Content:       msg.Content,
						CorrelationID: corrID,
						Metadata:      msg.Metadata,
					})
					cleanHTML := strings.ReplaceAll(buf.String(), "\n", " ")
					cleanHTML = strings.ReplaceAll(cleanHTML, "\r", " ")
					sseMsg := fmt.Sprintf("event: hitl-request\ndata: %s\n\n", cleanHTML)
					s.logBroker.Broadcast(sseMsg)
					s.LogEvent("SYSTEM", "WEB", fmt.Sprintf("Approval request received from %s (action: %s)", msg.Sender, msg.Metadata["action"]))
				} else {
					// Record agent final output into conversation history for follow-up tasks.
					s.addToHistory(msg.Sender, msg.Content)

					// Broadcast final outcome to event log
					s.LogEvent(msg.Sender, "USER", fmt.Sprintf("[Final Outcome] %s", msg.Content))

					// Check if message carries an artifact or references a generated file path
					artifactPath := ""
					if msg.Metadata != nil && msg.Metadata["artifact_path"] != "" {
						artifactPath = msg.Metadata["artifact_path"]
					} else {
						for _, dir := range []string{"emails/", "reports/", "workbooks/", "scripts/", "scratch/", "output/"} {
							if idx := strings.Index(msg.Content, dir); idx != -1 {
								rest := msg.Content[idx:]
								end := strings.IndexAny(rest, " \t\n\r\"')}]")
								if end == -1 {
									artifactPath = rest
								} else {
									artifactPath = rest[:end]
								}
								break
							}
						}
						if artifactPath == "" && strings.Contains(msg.Content, "file://") {
							idx := strings.Index(msg.Content, "file://")
							rest := msg.Content[idx+7:]
							end := strings.IndexAny(rest, " \t\n\r\"')}]")
							if end == -1 {
								artifactPath = rest
							} else {
								artifactPath = rest[:end]
							}
						}
					}

					if artifactPath != "" {
						artifactPath = strings.TrimPrefix(artifactPath, "file://")
						artifactPath = strings.TrimPrefix(artifactPath, "file:")
						artifactPath = strings.TrimRight(artifactPath, ".,;:\t\n\r\"')}]")

						filename := filepath.Base(artifactPath)
						artifactType := "file"
						extLower := strings.ToLower(filepath.Ext(artifactPath))
						switch extLower {
						case ".pdf":
							artifactType = "pdf"
						case ".xlsx", ".xls":
							artifactType = "excel"
						case ".py", ".sh", ".js", ".go", ".css", ".html":
							artifactType = "code"
						case ".json":
							artifactType = "json"
						case ".txt":
							if strings.Contains(artifactPath, "email") {
								artifactType = "email"
							} else {
								artifactType = "text"
							}
						case ".md":
							artifactType = "doc"
						case ".png", ".jpg", ".jpeg", ".webp", ".svg":
							artifactType = "image"
						}

						agentName := msg.Sender
						if msg.Metadata != nil && msg.Metadata["artifact_agent"] != "" {
							agentName = msg.Metadata["artifact_agent"]
						} else if strings.Contains(artifactPath, "email") {
							agentName = "email-agent"
						}

						var cardBuf bytes.Buffer
						ArtifactCardTemplate.Execute(&cardBuf, map[string]string{
							"ArtifactType": artifactType,
							"Filename":     filename,
							"Agent":        agentName,
							"Time":         time.Now().Format("15:04:05"),
							"Path":         artifactPath,
						})
						cleanCard := strings.ReplaceAll(cardBuf.String(), "\n", " ")
						cleanCard = strings.ReplaceAll(cleanCard, "\r", " ")
						s.logBroker.Broadcast(fmt.Sprintf("event: artifact-ready\ndata: %s\n\n", cleanCard))
					}
				}
			}
		}
	}()
}

// ServeHTTP implements http.Handler routing.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/stream/logs", s.handleSSE)
	mux.HandleFunc("/api/task", s.handleTaskDispatch)
	mux.HandleFunc("/api/hitl/respond", s.handleHITLResponse)
	mux.HandleFunc("/api/config/save", s.handleConfigSave)
	mux.HandleFunc("/api/config/pricing-provider", s.handlePricingProviderConfig)
	mux.HandleFunc("/api/artifact", s.handleArtifact)
	mux.HandleFunc("/api/schedules", s.handleSchedules)
	mux.HandleFunc("/api/schedules/", s.handleSchedules)

	// User-friendly Agent Studio & System Diagnostics API Endpoints
	mux.HandleFunc("/api/system/health", s.handleSystemHealth)
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/agents/export", s.handleExportAgentBlueprint)
	mux.HandleFunc("/api/agents/import", s.handleImportAgentBlueprint)
	mux.HandleFunc("/api/settings/docker-toggle", s.handleDockerToolsToggle)
	mux.HandleFunc("/api/models", s.handleModelsConfig)

	// Audit Explorer & Cost Telemetry API Endpoints
	mux.HandleFunc("/api/audit/records", s.handleAuditRecords)
	mux.HandleFunc("/api/audit/summary", s.handleAuditSummary)
	mux.HandleFunc("/api/audit/export", s.handleAuditExport)
	mux.HandleFunc("/api/audit/prune", s.handleAuditPrune)

	// Agent Network Topology Visualizer Endpoint
	mux.HandleFunc("/api/network/topology", s.handleNetworkTopology)

	// Mock APIs for testing framework end-to-end
	mux.HandleFunc("/api/mock/compute/prices", s.handleMockComputePrices)
	mux.HandleFunc("/api/mock/compute/forward-curves", s.handleMockForwardCurves)
	mux.HandleFunc("/api/mock/compute/options", s.handleMockOptions)
	mux.HandleFunc("/api/mock/compute/vol-surface", s.handleMockVolSurface)
	mux.HandleFunc("/api/mock/tasks", s.handleMockTasks)
	mux.HandleFunc("/api/mock/tasks/", s.handleMockTasks)

	subFS, err := fs.Sub(staticFS, "static")
	if err == nil {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(subFS))))
	}

	mux.ServeHTTP(w, r)
}

func (s *Server) checkWarnings() []string {
	return agent.CheckSystemWarnings()
}

type ScheduleWithNext struct {
	ID       string
	Name     string
	Cron     string
	Task     string
	Enabled  bool
	Created  string
	NextFire string
}

type DashboardData struct {
	Warnings                []string
	AgentsConfig            map[string]string
	ModelsConfig            string
	CriticalActions         agent.CriticalActionsConfig
	RiskyBashKeywordsStr    string
	RiskyPythonKeywordsStr  string
	RequireApprovalToolsStr string
	Schedules               []*ScheduleWithNext
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	warnings := s.checkWarnings()

	// Load agents config
	agentsBytes, err := os.ReadFile("agents.json")
	var agentsMap map[string]struct {
		SystemPrompt string `json:"system_prompt"`
	}
	if err == nil {
		_ = json.Unmarshal(agentsBytes, &agentsMap)
	}

	agents := make(map[string]string)
	for k, v := range agentsMap {
		agents[k] = v.SystemPrompt
	}

	// Fallback to active dynamic prompts if not present
	for id := range agent.GetLoadedAgentConfigs() {
		if agents[id] == "" {
			agents[id] = agent.GetPromptByID(id)
		}
	}

	// Load models config
	modelsBytes, err := os.ReadFile("models.json")
	var modelsConfig string
	if err != nil {
		modelsConfig = "{}"
	} else {
		modelsConfig = string(modelsBytes)
	}

	var schedulesWithNext []*ScheduleWithNext
	if agent.GlobalScheduler != nil {
		scheds, nextTimes := agent.GlobalScheduler.ListSchedules()
		for _, s := range scheds {
			nfStr := "disabled"
			if t, ok := nextTimes[s.ID]; ok {
				duration := time.Until(t)
				nfStr = fmt.Sprintf("in %s (%s)", agent.HumanDuration(duration), t.Format("15:04:05"))
			}
			schedulesWithNext = append(schedulesWithNext, &ScheduleWithNext{
				ID:       s.ID,
				Name:     s.Name,
				Cron:     s.Cron,
				Task:     s.Task,
				Enabled:  s.Enabled,
				Created:  s.Created,
				NextFire: nfStr,
			})
		}
	}

	data := DashboardData{
		Warnings:                warnings,
		AgentsConfig:            agents,
		ModelsConfig:            modelsConfig,
		CriticalActions:         agent.CriticalActions,
		RiskyBashKeywordsStr:    strings.Join(agent.CriticalActions.RiskyBashKeywords, ", "),
		RiskyPythonKeywordsStr:  strings.Join(agent.CriticalActions.RiskyPythonKeywords, ", "),
		RequireApprovalToolsStr: strings.Join(agent.CriticalActions.RequireApprovalTools, ", "),
		Schedules:               schedulesWithNext,
	}

	DashboardPage.Execute(w, data)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	logger.WithComponent("web").Debug("SSE client connected", "remote_addr", r.RemoteAddr)
	defer logger.WithComponent("web").Debug("SSE client disconnected", "remote_addr", r.RemoteAddr)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	rc := http.NewResponseController(w)

	// Immediately flush headers and an initial comment to transition EventSource readyState to OPEN
	logChan, pastLogs := s.logBroker.Subscribe()
	defer s.logBroker.Unsubscribe(logChan)

	// Immediately replay all past log history to newly connected client
	for _, pastLine := range pastLogs {
		_, _ = fmt.Fprint(w, pastLine)
	}
	_ = rc.Flush()

	// Flush logs as they arrive
	for {
		select {
		case <-r.Context().Done():
			return
		case logLine, ok := <-logChan:
			if !ok {
				return
			}
			logger.WithComponent("web").Debug("SSE writing message to client", "line", logLine)
			_, _ = fmt.Fprint(w, logLine)
			_ = rc.Flush()
		}
	}
}

func (s *Server) handleTaskDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Parse Multipart Form (10MB limit)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		_ = r.ParseForm()
	}

	content := r.FormValue("content")
	if content == "" {
		http.Error(w, "Missing task content", http.StatusBadRequest)
		return
	}

	mode := r.FormValue("orchestration_mode")

	corrID := fmt.Sprintf("web-%d", time.Now().UnixNano())
	meta := map[string]string{
		"correlation_id": corrID,
	}
	if mode != "" {
		meta["orchestration_mode"] = mode
	}

	// 2. Handle File Upload if present
	file, header, err := r.FormFile("file")
	if err == nil {
		defer file.Close()

		uploadsDir := "uploads"
		if err := os.MkdirAll(uploadsDir, 0755); err == nil {
			filePath := filepath.Join(uploadsDir, filepath.Base(header.Filename))
			out, err := os.Create(filePath)
			if err == nil {
				_, _ = io.Copy(out, file)
				out.Close()

				ext := strings.ToLower(filepath.Ext(header.Filename))
				switch ext {
				case ".txt", ".md":
					textBytes, err := os.ReadFile(filePath)
					if err == nil {
						content = fmt.Sprintf("%s\n\n--- Attachment: %s ---\n%s\n--- End Attachment ---", content, header.Filename, string(textBytes))
					}
				case ".csv":
					textBytes, err := os.ReadFile(filePath)
					if err == nil {
						formatted := formatCSVAsMarkdownTable(string(textBytes))
						content = fmt.Sprintf("%s\n\n--- Attachment Table: %s ---\n%s\n--- End Attachment ---", content, header.Filename, formatted)
					}
				default:
					content = fmt.Sprintf("%s\n\n[Attachment uploaded and saved to: %s]", content, filePath)
				}

				s.LogEvent("SYSTEM", "triage-agent", fmt.Sprintf("[Uploaded File: %s]", filePath))
			}
		}
	}

	// 3. Inject rolling conversation history so triage & specialists have context.
	historyBlock := s.buildHistoryBlock()

	// 4. Record this user turn into history before dispatching.
	s.addToHistory("USER", content)

	// Prepend history to the content so triage sees the full thread.
	enrichedContent := historyBlock + content

	s.LogEvent("USER", "triage-agent", fmt.Sprintf("Triggering new task: %s", content))

	s.runtime.Orchestrator().Send(adk.Message{
		Sender:    "USER",
		Recipient: "triage-agent",
		Content:   enrichedContent,
		Metadata:  meta,
	})

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleHITLResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	corrID := r.FormValue("correlation_id")
	decision := r.FormValue("decision") // APPROVED or REJECTED

	if corrID == "" || decision == "" {
		http.Error(w, "Missing correlation_id or decision", http.StatusBadRequest)
		return
	}

	s.approvalsMu.Lock()
	originalMsg, ok := s.approvals[corrID]
	if ok {
		delete(s.approvals, corrID)
	}
	s.approvalsMu.Unlock()

	if !ok {
		http.Error(w, "Approval request not found or already resolved", http.StatusNotFound)
		return
	}

	replyTo := originalMsg.Metadata["reply_to"]
	if replyTo == "" {
		replyTo = originalMsg.Sender
	}

	s.LogEvent("USER", replyTo, fmt.Sprintf("HITL Steering response submitted: %s", decision))

	// Resolve and unblock the agent
	s.runtime.Orchestrator().Send(adk.Message{
		Sender:    "USER",
		Recipient: replyTo,
		Content:   decision,
		Metadata: map[string]string{
			"correlation_id": corrID,
			"Type":           "HITL_RESPONSE",
			"reason":         "operator decision submitted via web interface",
		},
	})

	// Return swapped badge html replacing the action card
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ActionTakenTemplate.Execute(w, map[string]string{
		"Decision": decision,
	})
}

func (s *Server) handleConfigSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Reconstruct agents.json content from individual form values
	type AgentJSONConfig struct {
		SystemPrompt string `json:"system_prompt"`
	}
	agentsConfig := make(map[string]agent.AgentJSONConfig)
	for id, cfg := range agent.GetLoadedAgentConfigs() {
		formKey := strings.ReplaceAll(id, "-", "_") + "_prompt"
		if val := r.FormValue(formKey); val != "" {
			cfg.SystemPrompt = val
		}
		agentsConfig[id] = cfg
	}

	agentsBytes, err := json.MarshalIndent(agentsConfig, "", "  ")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to marshal agents config: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. Validate models.json syntax
	modelsJSON := r.FormValue("models_json")
	var temp interface{}
	if err := json.Unmarshal([]byte(modelsJSON), &temp); err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(fmt.Appendf(nil, `<div class="p-3 bg-red-50 border border-red-200 text-red-700 text-xs rounded">Invalid models.json format: %v</div>`, err))
		return
	}

	// 3. Save CriticalActions HITL command & tool approval config
	splitList := func(raw string) []string {
		var res []string
		lines := strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r'
		})
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" {
				res = append(res, l)
			}
		}
		return res
	}

	critConfig := agent.CriticalActionsConfig{
		RiskyBashKeywords:       splitList(r.FormValue("risky_bash_keywords")),
		RiskyPythonKeywords:     splitList(r.FormValue("risky_python_keywords")),
		RequireApprovalTools:    splitList(r.FormValue("require_approval_tools")),
		AutoApproveAll:          r.FormValue("auto_approve_all") == "on" || r.FormValue("auto_approve_all") == "true",
		ProductionExcelPatterns: agent.CriticalActions.ProductionExcelPatterns,
		InternalEmailDomains:    agent.CriticalActions.InternalEmailDomains,
	}

	_ = os.MkdirAll(".agents", 0755)
	critBytes, _ := json.MarshalIndent(critConfig, "", "  ")
	_ = os.WriteFile(".agents/critical_actions.json", critBytes, 0644)
	agent.CriticalActions = critConfig

	// 4. Write configs to disk
	if err := os.WriteFile("agents.json", agentsBytes, 0644); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write agents.json: %v", err), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile("models.json", []byte(modelsJSON), 0644); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write models.json: %v", err), http.StatusInternalServerError)
		return
	}

	// 5. Reload agents in-memory dynamically
	if err := agent.LoadAgentsConfig("agents.json"); err != nil {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(fmt.Appendf(nil, `<div class="p-3 bg-yellow-50 border border-yellow-200 text-yellow-700 text-xs rounded">Config saved to disk, but reload failed: %v</div>`, err))
		return
	}

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<div class="p-3 bg-green-50 border border-green-200 text-green-700 text-xs rounded font-medium flex flex-col gap-1">
        <span>✓ Configuration saved successfully!</span>
        <span class="text-[10px] text-green-600 font-normal">Command approval blocklists and agent system prompts have been updated live in memory and written to .agents/critical_actions.json.</span>
    </div>`))
}

// handleArtifact serves agent-produced files (reports, email drafts, workbooks).
// Only files within the allowlisted directories or workspace are served to prevent path traversal.
func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		http.Error(w, "Missing path parameter", http.StatusBadRequest)
		return
	}

	// 1. Clean path prefixes and trailing punctuation
	rawPath = strings.TrimPrefix(rawPath, "file://")
	rawPath = strings.TrimPrefix(rawPath, "file:")
	rawPath = strings.TrimRight(rawPath, ".,;:\t\n\r\"')}]")

	wd, _ := os.Getwd()
	absWd, _ := filepath.Abs(wd)
	homeDir, _ := os.UserHomeDir()
	geminiBrainDir := filepath.Join(homeDir, ".gemini", "antigravity-ide", "brain")

	targetPath := rawPath
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(absWd, targetPath)
	}

	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// 2. Fallback resolution if file does not exist directly at absPath
	if _, statErr := os.Stat(absPath); os.IsNotExist(statErr) {
		baseName := filepath.Base(rawPath)
		candidates := []string{
			filepath.Join(absWd, rawPath),
			filepath.Join(absWd, baseName),
			filepath.Join(absWd, "reports", baseName),
			filepath.Join(absWd, "emails", baseName),
			filepath.Join(absWd, "workbooks", baseName),
			filepath.Join(absWd, "scripts", baseName),
			filepath.Join(absWd, "scratch", baseName),
			filepath.Join(absWd, "artifacts", baseName),
			filepath.Join(absWd, "output", baseName),
			filepath.Join(absWd, "testdata", "reports", baseName),
			filepath.Join(geminiBrainDir, baseName),
		}

		if matches, err := filepath.Glob(filepath.Join(geminiBrainDir, "*", baseName)); err == nil {
			candidates = append(candidates, matches...)
		}
		if matches, err := filepath.Glob(filepath.Join(geminiBrainDir, "*", "scratch", baseName)); err == nil {
			candidates = append(candidates, matches...)
		}

		for _, cand := range candidates {
			if cand == "" {
				continue
			}
			if _, err := os.Stat(cand); err == nil {
				absPath = cand
				break
			}
		}
	}

	// Double check file existence
	if _, statErr := os.Stat(absPath); os.IsNotExist(statErr) {
		http.Error(w, fmt.Sprintf("Artifact file not found: %s", filepath.Base(rawPath)), http.StatusNotFound)
		return
	}

	// 3. Security bounds: workspace root, allowed directories, or .gemini brain dir
	allowed := false
	if strings.HasPrefix(absPath, absWd+string(filepath.Separator)) || absPath == absWd {
		allowed = true
	} else if strings.HasPrefix(absPath, geminiBrainDir+string(filepath.Separator)) || absPath == geminiBrainDir || strings.HasPrefix(absPath, filepath.Dir(geminiBrainDir)) {
		allowed = true
	} else {
		allowedDirs := []string{"reports", "emails", "workbooks", "scripts", "models", "scratch", "agents", "internal", "adk", "cmd", "output", "artifacts", "testdata"}
		for _, dir := range allowedDirs {
			absDir, _ := filepath.Abs(dir)
			if strings.HasPrefix(absPath, absDir+string(filepath.Separator)) || absPath == absDir {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		http.Error(w, "Forbidden: path is outside allowed artifact directories", http.StatusForbidden)
		return
	}

	// Infer Content-Type from extension
	ext := strings.ToLower(filepath.Ext(absPath))
	switch ext {
	case ".pdf":
		w.Header().Set("Content-Type", "application/pdf")
	case ".xlsx":
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case ".py":
		w.Header().Set("Content-Type", "text/x-python; charset=utf-8")
	case ".sh":
		w.Header().Set("Content-Type", "text/x-sh; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".go":
		w.Header().Set("Content-Type", "text/x-go; charset=utf-8")
	case ".txt", ".md", ".csv", ".yml", ".yaml":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(absPath)))
	http.ServeFile(w, r, absPath)
}

// handleSchedules handles creation, listing and deletion of active cron schedules.
func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		name := r.FormValue("name")
		cron := r.FormValue("cron")
		task := r.FormValue("task")

		if name == "" || cron == "" || task == "" {
			http.Error(w, "Missing fields", http.StatusBadRequest)
			return
		}

		sched := &agent.Schedule{
			Name:    name,
			Cron:    cron,
			Task:    task,
			Enabled: true,
		}
		if err := agent.GlobalScheduler.AddSchedule(sched); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	case http.MethodDelete:
		parts := strings.Split(r.URL.Path, "/")
		id := parts[len(parts)-1]
		if id == "" || id == "schedules" {
			http.Error(w, "Missing schedule ID", http.StatusBadRequest)
			return
		}
		if err := agent.GlobalScheduler.RemoveSchedule(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	var list []*ScheduleWithNext
	if agent.GlobalScheduler != nil {
		scheds, nextTimes := agent.GlobalScheduler.ListSchedules()
		for _, s := range scheds {
			nfStr := "disabled"
			if t, ok := nextTimes[s.ID]; ok {
				duration := time.Until(t)
				nfStr = fmt.Sprintf("in %s (%s)", agent.HumanDuration(duration), t.Format("15:04:05"))
			}
			list = append(list, &ScheduleWithNext{
				ID:       s.ID,
				Name:     s.Name,
				Cron:     s.Cron,
				Task:     s.Task,
				Enabled:  s.Enabled,
				Created:  s.Created,
				NextFire: nfStr,
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	DashboardPage.ExecuteTemplate(w, "schedules-list-tmpl", list)
}

func formatCSVAsMarkdownTable(csvContent string) string {
	var sb strings.Builder
	lines := strings.Split(csvContent, "\n")
	first := true
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		sb.WriteString("|")
		for _, f := range fields {
			sb.WriteString(" ")
			sb.WriteString(strings.TrimSpace(f))
			sb.WriteString(" |")
		}
		sb.WriteString("\n")

		if first {
			sb.WriteString("|")
			for range fields {
				sb.WriteString(" --- |")
			}
			sb.WriteString("\n")
			first = false
		}
	}
	return sb.String()
}

func (s *Server) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check Docker daemon connection
	dockerOnline := false
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err == nil && cli != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 300*time.Millisecond)
		defer cancel()
		if _, pingErr := cli.Ping(ctx); pingErr == nil {
			dockerOnline = true
		}
	}

	// Check Ollama classifier daemon connection
	ollamaOnline := false
	ollamaURL := os.Getenv("OLLAMA_HOST")
	if ollamaURL == "" {
		ollamaURL = "http://127.0.0.1:11434"
	}
	resp, err := http.Get(ollamaURL + "/api/tags")
	if err == nil && resp != nil {
		if resp.StatusCode == 200 {
			ollamaOnline = true
		}
		resp.Body.Close()
	}

	sqliteOnline := (s.runtime != nil && s.runtime.Store() != nil)
	activeAgentsCount := len(agent.GetLoadedAgentConfigs())

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"docker_online":       dockerOnline,
		"ollama_online":       ollamaOnline,
		"sqlite_online":       sqliteOnline,
		"docker_tools_gated":  !agent.IsDockerToolsEnabled(),
		"active_agents_count": activeAgentsCount,
	})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		configs := agent.GetLoadedAgentConfigs()
		availableTools := agent.GetAvailableToolsList()
		dockerEnabled := agent.IsDockerToolsEnabled()

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"agents":               configs,
			"available_tools":      availableTools,
			"docker_tools_enabled": dockerEnabled,
		})
		return
	}

	if r.Method == http.MethodPost {
		var payload struct {
			ID           string                `json:"id"`
			Config       agent.AgentJSONConfig `json:"config"`
			SystemPrompt string                `json:"system_prompt"`
			Description  string                `json:"description"`
			Tools        []string              `json:"tools"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Invalid JSON payload: " + err.Error(),
			})
			return
		}

		// Support both nested config or flat fields
		if payload.SystemPrompt != "" && payload.Config.SystemPrompt == "" {
			payload.Config.SystemPrompt = payload.SystemPrompt
		}
		if payload.Description != "" && payload.Config.Description == "" {
			payload.Config.Description = payload.Description
		}
		if len(payload.Tools) > 0 && len(payload.Config.Tools) == 0 {
			payload.Config.Tools = payload.Tools
		}

		err := agent.RegisterDynamicAgent(s.runtime, payload.ID, payload.Config)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "restricted") || strings.Contains(err.Error(), "administrator policy") {
				status = http.StatusForbidden
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		s.LogEvent("SYSTEM", "WEB", fmt.Sprintf("Registered new agent '%s' dynamically via UI Studio", payload.ID))

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Agent '%s' created and registered successfully", payload.ID),
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleDockerToolsToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var payload struct {
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	agent.SetDockerToolsEnabled(payload.Enabled)
	s.LogEvent("SYSTEM", "ADMIN", fmt.Sprintf("Tier 3 Docker tools policy toggle updated: enabled=%v", payload.Enabled))

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":              true,
		"docker_tools_enabled": payload.Enabled,
	})
}

func (s *Server) handleExportAgentBlueprint(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing 'id' parameter", http.StatusBadRequest)
		return
	}

	data, err := agent.ExportAgentBlueprint(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-blueprint.json"`, id))
	_, _ = w.Write(data)
}

func (s *Server) handleImportAgentBlueprint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	id, cfg, err := agent.ImportAgentBlueprint(bodyBytes)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	err = agent.RegisterDynamicAgent(s.runtime, id, cfg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	s.LogEvent("SYSTEM", "WEB", fmt.Sprintf("Imported and registered blueprint agent '%s'", id))

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"agent_id": id,
		"message":  fmt.Sprintf("Successfully imported agent '%s'", id),
	})
}

func (s *Server) handleModelsConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	modelsFile := "models.json"

	if r.Method == http.MethodGet {
		data, err := os.ReadFile(modelsFile)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"llm_providers": []interface{}{},
			})
			return
		}
		_, _ = w.Write(data)
		return
	}

	if r.Method == http.MethodPost {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}

		var check map[string]interface{}
		if err := json.Unmarshal(data, &check); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON syntax: " + err.Error()})
			return
		}

		if err := os.WriteFile(modelsFile, data, 0644); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}

		s.LogEvent("SYSTEM", "WEB", "Updated models.json configuration")

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Models configuration updated successfully",
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleAuditRecords(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.runtime == nil || s.runtime.Store() == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"records": []interface{}{}})
		return
	}

	filter := memory.AuditFilter{
		AgentID:   r.URL.Query().Get("agent_id"),
		ModelName: r.URL.Query().Get("model_name"),
		Status:    r.URL.Query().Get("status"),
		Search:    r.URL.Query().Get("search"),
	}

	records, err := s.runtime.Store().QueryAuditRecords(r.Context(), filter)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"records": records})
}

func (s *Server) handleAuditSummary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.runtime == nil || s.runtime.Store() == nil {
		_ = json.NewEncoder(w).Encode(memory.AuditSummary{
			CostByAgent: make(map[string]float64),
			CostByModel: make(map[string]float64),
		})
		return
	}

	summary, err := s.runtime.Store().GetAuditSummary(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(summary)
}

func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil || s.runtime.Store() == nil {
		http.Error(w, "Database store not available", http.StatusServiceUnavailable)
		return
	}

	records, err := s.runtime.Store().QueryAuditRecords(r.Context(), memory.AuditFilter{Limit: 1000})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("audit-report-%s.csv", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	var buf bytes.Buffer
	buf.WriteString("ID,CorrelationID,AgentID,ModelName,PromptTokens,CompletionTokens,TotalCostUSD,ExecutionDurationMS,ToolCallsCount,Status,CreatedAt\n")
	for _, rec := range records {
		buf.WriteString(fmt.Sprintf("%s,%s,%s,%s,%d,%d,%.6f,%d,%d,%s,%s\n",
			rec.ID, rec.CorrelationID, rec.AgentID, rec.ModelName,
			rec.PromptTokens, rec.CompletionTokens, rec.TotalCostUSD,
			rec.ExecutionDurationMS, rec.ToolCallsCount, rec.Status,
			rec.CreatedAt.Format(time.RFC3339)))
	}

	_, _ = w.Write(buf.Bytes())
}

func (s *Server) handleAuditPrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if s.runtime == nil || s.runtime.Store() == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Database unavailable"})
		return
	}

	n, err := s.runtime.Store().PruneAuditRecords(r.Context(), 30)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	s.LogEvent("SYSTEM", "ADMIN", fmt.Sprintf("Pruned %d audit records older than 30 days", n))

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"pruned_count": n,
		"message":      fmt.Sprintf("Pruned %d historical audit records older than 30 days", n),
	})
}

func (s *Server) handleNetworkTopology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type Node struct {
		ID       string   `json:"id"`
		Label    string   `json:"label"`
		Category string   `json:"category"` // user, orchestrator, specialist, tool
		Role     string   `json:"role"`
		Tools    []string `json:"tools,omitempty"`
	}

	type Edge struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Weight int    `json:"weight"`
	}

	nodes := []Node{
		{ID: "USER", Label: "User / Client", Category: "user", Role: "Task Dispatcher"},
		{ID: "triage-agent", Label: "Triage Agent", Category: "orchestrator", Role: "Intent Router & Gatekeeper", Tools: []string{"route_task"}},
		{ID: "planner-agent", Label: "Planner Agent", Category: "orchestrator", Role: "Task Decomposer & DAG Planner", Tools: []string{"create_plan"}},
		{ID: "supervisor-agent", Label: "Supervisor Agent", Category: "orchestrator", Role: "Quality Assurance & Verifier", Tools: []string{"verify_output"}},
	}

	loaded := agent.GetLoadedAgentConfigs()
	knownAgents := map[string]bool{
		"USER": true, "triage-agent": true, "planner-agent": true, "supervisor-agent": true,
	}

	for id, cfg := range loaded {
		if !knownAgents[id] {
			knownAgents[id] = true
			label := id
			if parts := strings.Split(id, "-"); len(parts) > 0 {
				label = strings.Title(parts[0]) + " Agent"
			}
			nodes = append(nodes, Node{
				ID:       id,
				Label:    label,
				Category: "specialist",
				Role:     cfg.Description,
				Tools:    cfg.Tools,
			})
		}
	}

	defaultSpecialists := []struct {
		ID, Label, Role string
		Tools           []string
	}{
		{"developer-agent", "Developer Agent", "Code generation, debugging, & bash execution", []string{"read_file", "write_file", "execute_python_docker", "execute_bash_docker"}},
		{"researcher-agent", "Researcher Agent", "Deep web search, HTML fetch & PDF synthesis", []string{"web_search_and_extract", "fetch_html", "generate_pdf_report"}},
		{"quant-agent", "Quant Agent", "Options pricing, forward curves & Monte Carlo simulation", []string{"query_compute_prices", "query_forward_curves", "query_options_chain"}},
		{"writer-agent", "Writer Agent", "Document synthesis & technical documentation", []string{"write_file", "read_file"}},
		{"email-agent", "Email Assistant", "Email drafting & formatting", []string{"write_email"}},
		{"excel-agent", "Excel Analyst", "Workbook generation & financial model audit", []string{"read_file", "write_file"}},
	}

	for _, spec := range defaultSpecialists {
		if !knownAgents[spec.ID] {
			knownAgents[spec.ID] = true
			nodes = append(nodes, Node{
				ID:       spec.ID,
				Label:    spec.Label,
				Category: "specialist",
				Role:     spec.Role,
				Tools:    spec.Tools,
			})
		}
	}

	toolsList := agent.GetAvailableToolsList()
	for _, t := range toolsList {
		nodes = append(nodes, Node{
			ID:       "tool:" + t.Name,
			Label:    t.Name,
			Category: "tool",
			Role:     t.Description,
		})
	}

	edges := []Edge{
		{Source: "USER", Target: "triage-agent", Weight: 10},
		{Source: "triage-agent", Target: "planner-agent", Weight: 5},
		{Source: "triage-agent", Target: "developer-agent", Weight: 8},
		{Source: "triage-agent", Target: "researcher-agent", Weight: 8},
		{Source: "triage-agent", Target: "quant-agent", Weight: 6},
		{Source: "triage-agent", Target: "writer-agent", Weight: 6},
		{Source: "triage-agent", Target: "email-agent", Weight: 4},
		{Source: "triage-agent", Target: "excel-agent", Weight: 4},
		{Source: "supervisor-agent", Target: "triage-agent", Weight: 3},
	}

	for _, n := range nodes {
		if n.Category == "specialist" {
			for _, toolName := range n.Tools {
				edges = append(edges, Edge{
					Source: n.ID,
					Target: "tool:" + toolName,
					Weight: 2,
				})
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	})
}






