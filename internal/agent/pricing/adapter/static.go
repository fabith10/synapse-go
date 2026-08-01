package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fabith10/synapse-go/internal/agent/pricing/types"
)

// StaticJSONAdapter implements PricingProvider by reading a local JSON pricing matrix.
type StaticJSONAdapter struct {
	name     string
	filePath string
}

// NewStaticJSONAdapter creates a new static adapter reading from filePath.
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

func (a *StaticJSONAdapter) Query(_ context.Context, q types.PricingQuery) (*types.PricingResult, error) {
	data, err := os.ReadFile(a.filePath)
	if err != nil {
		raw := a.baselineMatrix(q.Asset, q.MarketType)
		return BuildResult(a.name, q.Asset, q.MarketType, raw), nil
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("static_json adapter %q: parse error: %w", a.name, err)
	}
	b, _ := json.Marshal(root)
	return BuildResult(a.name, q.Asset, q.MarketType, string(b)), nil
}

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
