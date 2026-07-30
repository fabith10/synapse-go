package main

import (
	"os"
	"testing"
)

func TestLoadModelsConfig(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "models_test_*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `{
		"llm_providers": [
			{
				"name": "test-mock-llm",
				"provider": "mock",
				"tier": "local",
				"input_rate_usd": 0.0,
				"output_rate_usd": 0.0
			},
			{
				"name": "test-ollama-llm",
				"provider": "ollama",
				"tier": "local",
				"input_rate_usd": 0.0,
				"output_rate_usd": 0.0
			},
			{
				"name": "gpt-4o",
				"provider": "openai",
				"tier": "commercial",
				"input_rate_usd": 0.000005,
				"output_rate_usd": 0.000015,
				"api_url": "https://api.openai.com/v1/chat/completions",
				"api_key_env": "OPENAI_API_KEY"
			}
		]
	}`

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	nodes, err := loadModelsConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("loadModelsConfig failed: %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	if nodes[0].Name != "test-mock-llm" {
		t.Errorf("expected nodes[0].Name to be 'test-mock-llm', got %q", nodes[0].Name)
	}

	if _, ok := nodes[0].Provider.(*mockLLMProvider); !ok {
		t.Errorf("expected nodes[0].Provider to be *mockLLMProvider, got %T", nodes[0].Provider)
	}

	if nodes[1].Name != "test-ollama-llm" {
		t.Errorf("expected nodes[1].Name to be 'test-ollama-llm', got %q", nodes[1].Name)
	}

	if _, ok := nodes[1].Provider.(*ollamaLLMProvider); !ok {
		t.Errorf("expected nodes[1].Provider to be *ollamaLLMProvider, got %T", nodes[1].Provider)
	}

	if nodes[2].Name != "gpt-4o" {
		t.Errorf("expected nodes[2].Name to be 'gpt-4o', got %q", nodes[2].Name)
	}

	if p, ok := nodes[2].Provider.(*openAILLMProvider); !ok {
		t.Errorf("expected nodes[2].Provider to be *openAILLMProvider, got %T", nodes[2].Provider)
	} else {
		if p.apiURL != "https://api.openai.com/v1/chat/completions" {
			t.Errorf("expected apiURL to be set, got %q", p.apiURL)
		}
		if p.apiKeyEnv != "OPENAI_API_KEY" {
			t.Errorf("expected apiKeyEnv to be set, got %q", p.apiKeyEnv)
		}
	}
}
