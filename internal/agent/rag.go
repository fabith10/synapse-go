// Package agent — rag.go implements a lightweight Retrieval-Augmented Generation
// (RAG) context injection pipeline using pure-Go TF-IDF cosine similarity.
//
// PDR-002 §4: when an agent needs historical context, the framework converts
// the current task to a vector embedding, queries context_logs for the top N
// most relevant prior interactions, and injects only those entries into the
// prompt — not the full table dump.
package agent

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"unicode"

	"github.com/fabith10/synapse-go/internal/memory"
)

// ---------------------------------------------------------------------------
// Tokenization
// ---------------------------------------------------------------------------

// tokenize splits text into lowercase word tokens, stripping punctuation and
// common English stop words to reduce vector noise.
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

// ---------------------------------------------------------------------------
// TF-IDF Embedding
// ---------------------------------------------------------------------------

// ComputeEmbedding returns a normalised TF-IDF vector for the given text.
// The vocabulary is derived from the text itself (single-document TF).
func ComputeEmbedding(text string) []float64 {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return nil
	}
	tf := make(map[string]float64)
	for _, t := range tokens {
		tf[t]++
	}
	type kv struct {
		k string
		v float64
	}
	pairs := make([]kv, 0, len(tf))
	for k, v := range tf {
		pairs = append(pairs, kv{k, v / float64(len(tokens))})
	}
	// Insertion-sort by key for a deterministic vector order.
	for i := 0; i < len(pairs)-1; i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[i].k > pairs[j].k {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	vec := make([]float64, len(pairs))
	for i, p := range pairs {
		vec[i] = p.v
	}
	return normalise(vec)
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

// ---------------------------------------------------------------------------
// Serialization — SQLite BLOB
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Cosine Similarity & Retrieval Ranking
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Context Block Formatting
// ---------------------------------------------------------------------------

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
