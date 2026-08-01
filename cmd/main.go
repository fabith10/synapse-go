package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/fabith10/synapse-go/adk"
	"github.com/fabith10/synapse-go/internal/agent"
	agenttools "github.com/fabith10/synapse-go/internal/agent/tools"
	"github.com/fabith10/synapse-go/internal/broker"
	"github.com/fabith10/synapse-go/internal/orchestrator"
	"github.com/fabith10/synapse-go/internal/web"
	"github.com/fabith10/synapse-go/pkg/logger"
	"github.com/joho/godotenv"
	"github.com/ollama/ollama/api"
	"github.com/pkg/browser"
)

// mockLLMProvider satisfies broker.LLMProvider and returns dummy text.
type mockLLMProvider struct{}

func (m *mockLLMProvider) FormatPrompt(msgs []adk.Message, ts []broker.Tool) (interface{}, error) {
	return msgs, nil
}

func (m *mockLLMProvider) GenerateResponse(_ context.Context, payload interface{}) (adk.Message, error) {
	msgs, ok := payload.([]adk.Message)
	if ok && len(msgs) > 0 {
		lastMsg := msgs[len(msgs)-1]
		if lastMsg.Sender == "SYSTEM" && len(msgs) > 1 {
			lastMsg = msgs[len(msgs)-2]
		}
		if strings.Contains(lastMsg.Content, "Code Output:") {
			return adk.Message{
				Sender:  "mock-provider",
				Content: fmt.Sprintf("documented code: [start]\n%s\n[end]", lastMsg.Content),
			}, nil
		}
		return adk.Message{
			Sender:  "mock-provider",
			Content: "func main() { fmt.Println(\"Hello, World!\") }",
		}, nil
	}
	return adk.Message{
		Sender:  "mock-provider",
		Content: "generic mock answer",
	}, nil
}

// ollamaLLMProvider satisfies broker.LLMProvider and queries local Ollama models.
type ollamaLLMProvider struct {
	model string
}

func (o *ollamaLLMProvider) FormatPrompt(msgs []adk.Message, ts []broker.Tool) (interface{}, error) {
	var builder strings.Builder
	for _, msg := range msgs {
		builder.WriteString(fmt.Sprintf("%s: %s\n", msg.Sender, msg.Content))
	}
	return builder.String(), nil
}

func (o *ollamaLLMProvider) GenerateResponse(ctx context.Context, payload interface{}) (adk.Message, error) {
	promptStr, _ := payload.(string)
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return adk.Message{}, fmt.Errorf("ollama: client error: %w", err)
	}

	stream := false
	req := &api.GenerateRequest{
		Model:  o.model,
		Prompt: promptStr,
		Stream: &stream,
		Format: json.RawMessage([]byte(`"json"`)),
	}

	var fullResponse string
	respFunc := func(resp api.GenerateResponse) error {
		fullResponse += resp.Response
		return nil
	}

	err = client.Generate(ctx, req, respFunc)
	if err != nil {
		return adk.Message{}, fmt.Errorf("ollama: generate error: %w", err)
	}

	return adk.Message{
		Sender:  "ollama",
		Content: fullResponse,
	}, nil
}

// openAILLMProvider queries any OpenAI-compatible API (e.g. OpenAI, OpenRouter, Groq, local vLLM).
type openAILLMProvider struct {
	model     string
	apiURL    string
	apiKeyEnv string
}

func (p *openAILLMProvider) FormatPrompt(msgs []adk.Message, ts []broker.Tool) (interface{}, error) {
	type openAIMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	var chatMsgs []openAIMessage
	for i, msg := range msgs {
		senderLower := strings.ToLower(strings.TrimSpace(msg.Sender))
		role := "user"
		// ONLY the very first message is allowed to have role "system" for Gemini/OpenAI API compliance
		if i == 0 && senderLower == "system" {
			role = "system"
		} else if senderLower == "assistant" || senderLower == "ollama" || senderLower == "openai" || strings.HasPrefix(senderLower, "gemini-") {
			role = "assistant"
		}

		if len(chatMsgs) > 0 && chatMsgs[len(chatMsgs)-1].Role == role {
			// Combine consecutive turns of the same role to strictly enforce turn interleaving
			chatMsgs[len(chatMsgs)-1].Content += "\n\n" + msg.Content
		} else {
			chatMsgs = append(chatMsgs, openAIMessage{
				Role:    role,
				Content: msg.Content,
			})
		}
	}

	// Ensure the first non-system turn is a "user" turn for Gemini API compliance
	firstNonSystemIdx := -1
	for i, m := range chatMsgs {
		if m.Role != "system" {
			firstNonSystemIdx = i
			break
		}
	}
	if firstNonSystemIdx != -1 && chatMsgs[firstNonSystemIdx].Role == "assistant" {
		chatMsgs[firstNonSystemIdx].Role = "user"
	}

	// Gemini and OpenAI API compliance: ensure payload array never ends with an assistant turn.
	if len(chatMsgs) > 0 && chatMsgs[len(chatMsgs)-1].Role == "assistant" {
		chatMsgs = append(chatMsgs, openAIMessage{
			Role:    "user",
			Content: "Continue and execute your next action.",
		})
	}

	return chatMsgs, nil
}

func (p *openAILLMProvider) GenerateResponse(ctx context.Context, payload interface{}) (adk.Message, error) {
	apiKey := ""
	if p.apiKeyEnv != "" {
		apiKey = os.Getenv(p.apiKeyEnv)
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	apiURL := p.apiURL
	if apiURL == "" {
		apiURL = "https://api.openai.com/v1/chat/completions"
	}

	requestBody := map[string]interface{}{
		"model":    p.model,
		"messages": payload,
	}
	if !strings.Contains(apiURL, "generativelanguage.googleapis.com") && !strings.Contains(p.model, "gemini") {
		requestBody["response_format"] = map[string]string{"type": "json_object"}
	}
	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		return adk.Message{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return adk.Message{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return adk.Message{}, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return adk.Message{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return adk.Message{}, fmt.Errorf("openai provider error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var responseData struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(bodyBytes, &responseData); err != nil {
		return adk.Message{}, err
	}

	if len(responseData.Choices) == 0 {
		return adk.Message{}, fmt.Errorf("empty choices returned from OpenAI completion API")
	}

	return adk.Message{
		Sender:  "openai",
		Content: responseData.Choices[0].Message.Content,
	}, nil
}

type modelJSON struct {
	Name          string  `json:"name"`
	Provider      string  `json:"provider"`
	Tier          string  `json:"tier"`
	InputRateUSD  float64 `json:"input_rate_usd"`
	OutputRateUSD float64 `json:"output_rate_usd"`
	APIURL        string  `json:"api_url,omitempty"`
	APIKeyEnv     string  `json:"api_key_env,omitempty"`
}

type configJSON struct {
	LLMProviders []modelJSON `json:"llm_providers"`
}

func loadModelsConfig(path string) ([]adk.LLMNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rawConfig configJSON
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return nil, err
	}

	var nodes []adk.LLMNode
	for _, m := range rawConfig.LLMProviders {
		var provider broker.LLMProvider
		switch m.Provider {
		case "mock":
			provider = &mockLLMProvider{}
		case "ollama":
			provider = &ollamaLLMProvider{model: m.Name}
		case "openai", "commercial", "anthropic", "gemini":
			provider = &openAILLMProvider{
				model:     m.Name,
				apiURL:    m.APIURL,
				apiKeyEnv: m.APIKeyEnv,
			}
		default:
			provider = &mockLLMProvider{}
		}

		var tier broker.ProviderTier
		if m.Tier == "commercial" {
			tier = broker.TierCommercial
		} else {
			tier = broker.TierLocal
		}

		nodes = append(nodes, adk.LLMNode{
			Name:          m.Name,
			Tier:          tier,
			Provider:      provider,
			InputRateUSD:  m.InputRateUSD,
			OutputRateUSD: m.OutputRateUSD,
		})
	}
	return nodes, nil
}

func main() {
	fs := flag.NewFlagSet("agent-framework", flag.ExitOnError)
	cliPrompt := fs.String("prompt", "", "Run a single prompt task in CLI mode")
	cliLogFile := fs.String("log-file", "", "Path to write agent interaction and audit logs")
	cliTimeoutSecs := fs.Int("timeout", 60, "Timeout in seconds for CLI mode prompt execution")
	_ = fs.Parse(os.Args[1:])

	// Load environment variables from .env file if present
	_ = godotenv.Load(findConfigFile(".env"))

	if *cliLogFile != "" && os.Getenv("LOG_FILE") == "" {
		os.Setenv("LOG_FILE", *cliLogFile)
	}

	logger.InitFromEnv()
	logger.Info("--- Starting Agent Framework Control Center ---")

	// 1. Config loading from models.json if present
	llmNodes, err := loadModelsConfig(findConfigFile("models.json"))
	if err != nil {
		logger.Warn("Could not load models.json, falling back to demo default mock LLM", "error", err)
		llmNodes = []adk.LLMNode{
			{
				Name:     "demo-mock-llm",
				Tier:     broker.TierLocal,
				Provider: &mockLLMProvider{},
			},
		}
	} else {
		fmt.Printf("Successfully loaded %d LLM providers from models.json\n", len(llmNodes))
	}

	// Load critical actions configuration if present
	if err := agenttools.LoadCriticalActionsConfig(findConfigFile("critical_actions.json")); err != nil {
		fmt.Printf("Warning: could not load critical_actions.json: %v (using defaults)\n", err)
	}

	sqliteDSN := os.Getenv("SQLITE_DSN")
	if sqliteDSN == "" {
		sqliteDSN = "agent_framework.db"
	}

	cfg := adk.Config{
		SQLiteDSN:                sqliteDSN,
		LLMProviders:             llmNodes,
		DefaultStrategy:          adk.StrategyBalanced,
		DisableDefaultMiddleware: false,
		MaxProcessMemoryMB:       1024, // 1 GB soft cap on the framework process
		ContainerMemoryMB:        2048, // 2 GB per ephemeral Docker container
		GlobalMaxCostUSD:         1.00, // $1.00 hard ceiling per routing decision
	}

	// Apply framework process memory soft-cap via Go runtime.
	// This makes the GC more aggressive before the OS is forced to OOM-kill.
	if cfg.MaxProcessMemoryMB > 0 {
		limitBytes := cfg.MaxProcessMemoryMB * 1024 * 1024
		debug.SetMemoryLimit(limitBytes)
		fmt.Printf("[Memory Cap] Framework process soft-capped at %d MB (Go runtime GC limit)\n", cfg.MaxProcessMemoryMB)
	}

	// 2. Bootstrap agents and tools
	runtime, err := agent.Bootstrap(cfg)
	if err != nil {
		fmt.Printf("Error bootstrapping framework: %v\n", err)
		return
	}
	defer runtime.Shutdown()

	if *cliPrompt != "" {
		var logWriter io.Writer = os.Stdout
		if *cliLogFile != "" {
			f, err := os.OpenFile(*cliLogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				fmt.Printf("Error opening log file %s: %v\n", *cliLogFile, err)
				return
			}
			defer f.Close()
			logWriter = io.MultiWriter(os.Stdout, f)
		}

		doneChan := make(chan struct{})
		var finalResponse string

		// Setup GlobalPlannerScheduler with Send callback
		orchestrator.GlobalPlannerScheduler.Initialize(runtime.Orchestrator().Send)

		// Inject CLI interaction logging & completion interception middleware
		runtime.Orchestrator().Use(func(ctx context.Context, msg adk.Message, next func(adk.Message)) {
			sender := msg.Sender
			recipient := msg.Recipient
			content := msg.Content

			broker.LastUsedModel.RLock()
			model := broker.LastUsedModel.M[sender]
			broker.LastUsedModel.RUnlock()

			if model != "" {
				content = fmt.Sprintf("[%s] %s", model, content)
			}

			timestamp := time.Now().Format("15:04:05")
			if strings.Contains(msg.Content, "<conversation_history>") {
				fmt.Fprintf(logWriter, "[%s] [RAG CONTEXT INJECTED] Historical memory retrieved and attached to turn: %s ➔ %s\n", timestamp, sender, recipient)
			}
			fmt.Fprintf(logWriter, "[%s] %s ➔ %s: %s\n", timestamp, sender, recipient, content)

			// Detect final user turn
			isSubtask := msg.Metadata != nil && msg.Metadata["is_subtask"] == "true"
			isInitialPlan := msg.Sender == "planner-agent" && (msg.Metadata == nil || msg.Metadata["phase"] != "synthesis")
			isProgressUpdate := strings.Contains(msg.Content, "[Plan Execution]")

			if recipient == "USER" && !isSubtask && !isInitialPlan && !isProgressUpdate {
				finalResponse = msg.Content
				select {
				case <-doneChan:
				default:
					close(doneChan)
				}
			}
			next(msg)
		})

		// Inject PlannerScheduler middleware
		runtime.Orchestrator().Use(orchestrator.GlobalPlannerScheduler.Middleware())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runtime.Start(ctx)

		fmt.Fprintf(logWriter, "--- CLI Headless Prompt Execution: %q ---\n", *cliPrompt)
		runtime.Orchestrator().Send(adk.Message{
			Sender:    "USER",
			Recipient: "triage-agent",
			Content:   *cliPrompt,
			Metadata: map[string]string{
				"correlation_id": fmt.Sprintf("cli-%d", time.Now().UnixNano()),
			},
		})

		select {
		case <-doneChan:
			fmt.Fprintf(logWriter, "\n--- CLI Prompt Task Completed Successfully ---\n")
			fmt.Fprintf(logWriter, "Final Response:\n%s\n", finalResponse)
		case <-time.After(time.Duration(*cliTimeoutSecs) * time.Second):
			fmt.Fprintf(logWriter, "\n--- CLI Prompt Task Timeout (%d seconds exceeded) ---\n", *cliTimeoutSecs)
			os.Exit(1)
		}
		return
	}

	// 3. Instantiate web control panel server
	webServer := web.NewServer(runtime)

	// Initialize GlobalPlannerScheduler with Send callback
	orchestrator.GlobalPlannerScheduler.Initialize(runtime.Orchestrator().Send)

	// 4. Inject web log logging middleware to intercept A2A events
	runtime.Orchestrator().Use(func(ctx context.Context, msg adk.Message, next func(adk.Message)) {
		// Filter out raw JSON plan from log stream
		if msg.Sender == "planner-agent" && msg.Recipient == "USER" && (msg.Metadata == nil || msg.Metadata["phase"] != "synthesis") {
			next(msg)
			return
		}
		if msg.Metadata == nil || msg.Metadata["Type"] != "HITL_RESPONSE" {
			content := msg.Content
			broker.LastUsedModel.RLock()
			model := broker.LastUsedModel.M[msg.Sender]
			broker.LastUsedModel.RUnlock()

			if model != "" {
				content = fmt.Sprintf("[%s] %s", model, content)
			}
			
			recipient := msg.Recipient
			if msg.Recipient == "USER" && msg.Metadata != nil && msg.Metadata["is_subtask"] == "true" {
				recipient = "SUBTASK"
			}
			webServer.LogEvent(msg.Sender, recipient, content)
		}
		next(msg)
	})

	// Inject PlannerScheduler middleware
	runtime.Orchestrator().Use(orchestrator.GlobalPlannerScheduler.Middleware())

	// 5. Start event loops and web logs receiver
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime.Start(ctx)

	webServer.StartHITLListener(ctx)
	fmt.Println("Agents registered and event bus online.")

	// 6. Spawn web server
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}

		// Security (C-2): Bind to localhost only by default to prevent
		// unauthenticated access from the local network. Set BIND_ADDR=0.0.0.0
		// explicitly only when intentional remote/container access is required.
		bindAddr := os.Getenv("BIND_ADDR")
		if bindAddr == "" {
			bindAddr = "127.0.0.1"
		}
		listenAddr := bindAddr + ":" + port

		fmt.Println("====================================================")
		fmt.Printf("  Control Panel running at http://localhost:%s\n", port)
		if bindAddr != "127.0.0.1" {
			fmt.Printf("  WARNING: Server bound to %s (all interfaces)\n", bindAddr)
		}
		fmt.Println("====================================================")

		// Auto-open frontend browser link locally (disabled in headless or test modes)
		if os.Getenv("HEADLESS") != "true" && os.Getenv("AGENT_FRAMEWORK_TESTING") != "true" {
			go func(p string) {
				time.Sleep(300 * time.Millisecond) // Allow server setup
				_ = openBrowser(fmt.Sprintf("http://localhost:%s", p))
			}(port)
		}

		if err := http.ListenAndServe(listenAddr, webServer); err != nil {
			fmt.Printf("Web server error: %v\n", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	fmt.Printf("\n[Signal Received: %v] Shutting down agent framework gracefully...\n", sig)
	cancel()
	unloadOllamaModels()
	fmt.Println("Framework shutdown complete.")
}

func unloadOllamaModels() {
	path := findConfigFile("models.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var rawConfig configJSON
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return
	}

	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://127.0.0.1:11434"
	}
	if !strings.HasPrefix(ollamaHost, "http://") && !strings.HasPrefix(ollamaHost, "https://") {
		ollamaHost = "http://" + ollamaHost
	}
	url := fmt.Sprintf("%s/api/generate", strings.TrimSuffix(ollamaHost, "/"))

	client := &http.Client{Timeout: 3 * time.Second}
	for _, m := range rawConfig.LLMProviders {
		if m.Provider == "ollama" {
			fmt.Printf("Unloading Ollama model %q from GPU/system memory...\n", m.Name)
			payload := map[string]interface{}{
				"model":      m.Name,
				"keep_alive": 0,
			}
			jsonData, err := json.Marshal(payload)
			if err != nil {
				continue
			}

			req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("Notice: could not unload model %q (Ollama daemon offline or unreachable)\n", m.Name)
				continue
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			fmt.Printf("Successfully unloaded Ollama model %q.\n", m.Name)
		}
	}
}

func findConfigFile(name string) string {
	wd, err := os.Getwd()
	if err != nil {
		return name
	}
	curr := wd
	for {
		candidate := filepath.Join(curr, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return name
}

func openBrowser(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "open", url)
	if err := cmd.Run(); err != nil {
		return browser.OpenURL(url)
	}
	return nil
}
