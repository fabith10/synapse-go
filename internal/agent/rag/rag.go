package rag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fabith10/synapse-go/internal/memory"
	"github.com/fabith10/synapse-go/pkg/logger"
	"github.com/ollama/ollama/api"
)

// ---------------------------------------------------------------------------
// Embedding model configuration
// ---------------------------------------------------------------------------

const (
	// defaultEmbeddingModel is the Ollama model used for real embeddings.
	// nomic-embed-text produces 768-dim vectors; small, fast, and good quality.
	defaultEmbeddingModel = "nomic-embed-text"

	// hashEmbeddingDim is the fixed dimension for the deterministic hash-based
	// fallback embedder when Ollama is unavailable.
	hashEmbeddingDim = 384
)

var (
	embeddingModelMu sync.RWMutex
	embeddingModel   = defaultEmbeddingModel

	// ollamaAvailable caches whether we can reach Ollama.
	// Checked once; retried if initially unavailable.
	ollamaCheckOnce sync.Once
	ollamaReachable bool
)

// SetEmbeddingModel overrides the Ollama model used for embeddings.
func SetEmbeddingModel(model string) {
	embeddingModelMu.Lock()
	defer embeddingModelMu.Unlock()
	if trimmed := strings.TrimSpace(model); trimmed != "" {
		embeddingModel = trimmed
	}
}

// GetEmbeddingModel returns the configured embedding model name.
// Priority: OLLAMA_EMBEDDING_MODEL env > EMBEDDING_MODEL env > configured default.
func GetEmbeddingModel() string {
	if m := os.Getenv("OLLAMA_EMBEDDING_MODEL"); strings.TrimSpace(m) != "" {
		return strings.TrimSpace(m)
	}
	if m := os.Getenv("EMBEDDING_MODEL"); strings.TrimSpace(m) != "" {
		return strings.TrimSpace(m)
	}
	embeddingModelMu.RLock()
	defer embeddingModelMu.RUnlock()
	return embeddingModel
}

// checkOllama probes whether the Ollama API is reachable.
func checkOllama() bool {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Try a minimal embedding call to confirm the model is loaded.
	req := &api.EmbedRequest{Model: GetEmbeddingModel(), Input: "test"}
	_, err = client.Embed(ctx, req)
	return err == nil
}

// ---------------------------------------------------------------------------
// ComputeEmbedding — real embeddings via Ollama, hash-based fallback
// ---------------------------------------------------------------------------

// ComputeEmbedding returns a fixed-dimension embedding vector for the given text.
// Uses the Ollama embedding API when available (producing real semantic vectors),
// falling back to a deterministic hash-based projection when Ollama is unreachable.
// Both paths produce fixed-dimension vectors so cosine similarity always works.
func ComputeEmbedding(text string) []float64 {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// Skip real embeddings in test mode for speed.
	if os.Getenv("AGENT_FRAMEWORK_TESTING") == "true" {
		return hashEmbedding(text)
	}

	ollamaCheckOnce.Do(func() {
		ollamaReachable = checkOllama()
		if ollamaReachable {
			logger.Info("RAG embedding provider online", "model", GetEmbeddingModel())
		} else {
			logger.Warn("Ollama embedding model unavailable, using hash-based fallback", "model", GetEmbeddingModel())
		}
	})

	if ollamaReachable {
		if vec, err := ollamaEmbed(text); err == nil {
			return vec
		}
		// On transient failure, fall through to hash fallback.
	}

	return hashEmbedding(text)
}

// ollamaEmbed calls the Ollama embedding API and returns a normalised float64 vector.
func ollamaEmbed(text string) ([]float64, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &api.EmbedRequest{
		Model: GetEmbeddingModel(),
		Input: text,
	}

	resp, err := client.Embed(ctx, req)
	if err != nil {
		logger.Warn("Ollama embed call failed, using fallback", "error", err)
		return nil, err
	}

	if len(resp.Embeddings) == 0 || len(resp.Embeddings[0]) == 0 {
		return nil, err
	}

	// Convert []float32 → []float64 and normalise.
	vec := make([]float64, len(resp.Embeddings[0]))
	for i, f := range resp.Embeddings[0] {
		vec[i] = float64(f)
	}
	return normalise(vec), nil
}

// ---------------------------------------------------------------------------
// Hash-based fallback embedder (deterministic, fixed-dimension)
// ---------------------------------------------------------------------------

// hashEmbedding produces a fixed-dimension pseudo-embedding by hashing
// overlapping word n-grams into a dense vector. Unlike the old TF-IDF approach,
// all vectors have the same dimension so cosine similarity always works.
// ponytail: Not semantic — upgrade path is Ollama real embeddings (above).
func hashEmbedding(text string) []float64 {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return make([]float64, hashEmbeddingDim)
	}

	vec := make([]float64, hashEmbeddingDim)

	// Hash individual tokens and bigrams into fixed-dim buckets.
	for i, tok := range tokens {
		hashInto(vec, tok)
		if i+1 < len(tokens) {
			hashInto(vec, tok+" "+tokens[i+1])
		}
	}

	return normalise(vec)
}

// hashInto adds a deterministic signal for s into the vector.
func hashInto(vec []float64, s string) {
	h := sha256.Sum256([]byte(s))
	// Use first 8 bytes to pick a bucket (unsigned mod), next byte for sign.
	idx := binary.LittleEndian.Uint64(h[:8]) % uint64(len(vec))
	sign := float64(1)
	if h[8]&1 == 1 {
		sign = -1
	}
	vec[idx] += sign
}

// tokenize splits text into lowercase word tokens, stripping punctuation and
// common English stop words.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := fields[:0]
	for _, f := range fields {
		if !stopWord[f] && len(f) > 1 {
			out = append(out, f)
		}
	}
	return out
}

var stopWord = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true,
	"at": true, "be": true, "by": true, "for": true, "from": true,
	"has": true, "he": true, "in": true, "is": true, "it": true,
	"its": true, "of": true, "on": true, "or": true, "that": true,
	"the": true, "this": true, "to": true, "was": true, "with": true,
}

func normalise(v []float64) []float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 {
		return v
	}
	norm := math.Sqrt(sum)
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

// SerializeEmbedding encodes []float64 as little-endian IEEE 754 binary.
func SerializeEmbedding(v []float64) []byte {
	if len(v) == 0 {
		return nil
	}
	buf := new(bytes.Buffer)
	for _, f := range v {
		_ = binary.Write(buf, binary.LittleEndian, math.Float64bits(f))
	}
	return buf.Bytes()
}

// DeserializeEmbedding decodes a BLOB back into []float64.
func DeserializeEmbedding(b []byte) []float64 {
	if len(b) == 0 || len(b)%8 != 0 {
		return nil
	}
	v := make([]float64, len(b)/8)
	for i := range v {
		bits := binary.LittleEndian.Uint64(b[i*8 : i*8+8])
		v[i] = math.Float64frombits(bits)
	}
	return v
}

// CosineSimilarity computes the dot product of two L2-normalised vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	var dot float64
	for i, v := range shorter {
		dot += v * longer[i]
	}
	return dot
}

// RankLogs scores each log entry against a query embedding and returns the
// top-N entries ordered by descending relevance.
func RankLogs(query []float64, logs []memory.ContextLog, topN int) []memory.ContextLog {
	type scored struct {
		log   memory.ContextLog
		score float64
	}
	results := make([]scored, len(logs))
	for i, l := range logs {
		emb := DeserializeEmbedding(l.Embedding)
		results[i] = scored{log: l, score: CosineSimilarity(query, emb)}
	}
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].score > results[j-1].score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
	if topN > len(results) {
		topN = len(results)
	}
	out := make([]memory.ContextLog, topN)
	for i := range out {
		out[i] = results[i].log
	}
	return out
}

// FormatContextBlock renders ContextLog entries as a Markdown block ready to
// prepend to an agent's system prompt. Returns "" if logs is empty.
func FormatContextBlock(logs []memory.ContextLog) string {
	if len(logs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<conversation_history>\n")
	for _, l := range logs {
		sb.WriteString("[")
		sb.WriteString(string(l.Role))
		sb.WriteString("|agent:")
		sb.WriteString(l.AgentID)
		sb.WriteString("] ")
		sb.WriteString(l.Content)
		sb.WriteString("\n")
	}
	sb.WriteString("</conversation_history>\n")
	return sb.String()
}
