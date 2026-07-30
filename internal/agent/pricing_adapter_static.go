package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// StaticJSONAdapter implements PricingProvider by reading a local JSON pricing matrix.
// It is useful for air-gapped environments or as a deterministic test fixture.
// If the configured file does not exist a built-in baseline matrix is returned.
type StaticJSONAdapter struct {
	name     string
	filePath string
}

// NewStaticJSONAdapter creates a new static adapter reading from filePath.
// If filePath is empty it defaults to "pricing_matrix.json" in the working directory.
func NewStaticJSONAdapter(name, filePath string) *StaticJSONAdapter {
	if name == "" {
		name = "static_json"
	}
	if filePath == "" {
		filePath = "pricing_matrix.json"
	}
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(".", filePath)
	}
	return &StaticJSONAdapter{name: name, filePath: filePath}
}

func (a *StaticJSONAdapter) Name() string { return a.name }

func (a *StaticJSONAdapter) Query(_ context.Context, q PricingQuery) (*PricingResult, error) {
	data, err := os.ReadFile(a.filePath)
	if err != nil {
		// Return the built-in baseline matrix; missing file is not a hard error.
		raw := a.baselineMatrix(q.Asset, q.MarketType)
		return buildResult(a.name, q.Asset, q.MarketType, raw), nil
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("static_json adapter %q: parse error: %w", a.name, err)
	}
	b, _ := json.Marshal(root)
	return buildResult(a.name, q.Asset, q.MarketType, string(b)), nil
}

// baselineMatrix returns a hardcoded minimal pricing matrix when no file is present.
func (a *StaticJSONAdapter) baselineMatrix(asset, marketType string) string {
	res := map[string]interface{}{
		"status":   "success",
		"provider": a.name,
		"asset":    asset,
		"market":   marketType,
		"matrix": map[string]interface{}{
			"H100_SXM":  2.49,
			"A100_80GB": 1.29,
			"RTX_4090":  0.45,
		},
	}
	b, _ := json.Marshal(res)
	return string(b)
}
