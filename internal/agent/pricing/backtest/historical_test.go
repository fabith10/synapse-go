package backtest_test

import (
	"os"
	"strings"
	"testing"

	"github.com/fabith10/synapse-go/internal/agent/pricing/backtest"
)

func TestHistoricalDatasets_CatalogAndLoading(t *testing.T) {
	datasets := backtest.ListHistoricalDatasets()
	if len(datasets) < 6 {
		t.Fatalf("expected at least 6 historical datasets in catalog, got %d", len(datasets))
	}

	for _, ds := range datasets {
		prices, info, err := backtest.LoadHistoricalDataset(ds.ID)
		if err != nil {
			t.Errorf("failed to load dataset %s: %v", ds.ID, err)
			continue
		}
		if len(prices) != 720 {
			t.Errorf("expected 720 hours for dataset %s, got %d", ds.ID, len(prices))
		}
		if info.AverageSpotRate <= 0 || info.MaxSpotRate <= 0 || info.MinSpotRate <= 0 {
			t.Errorf("expected positive stats for dataset %s, got avg=%f, max=%f, min=%f", ds.ID, info.AverageSpotRate, info.MaxSpotRate, info.MinSpotRate)
		}
		if info.MaxSpotRate < info.MinSpotRate {
			t.Errorf("invalid range for dataset %s: max %f < min %f", ds.ID, info.MaxSpotRate, info.MinSpotRate)
		}
	}
}

func TestBacktestEngine_EmpiricalAWS_G5(t *testing.T) {
	engine := backtest.NewBacktestEngine()
	cfg := backtest.BacktestConfig{
		DatasetName:      "aws_g5_xlarge",
		NumWorkloadTasks: 100,
		RandomSeed:       42,
	}

	report, err := engine.RunBacktest(cfg)
	if err != nil {
		t.Fatalf("failed to run empirical backtest on aws_g5_xlarge: %v", err)
	}

	if report.DatasetInfo == nil {
		t.Fatalf("expected report.DatasetInfo to be populated")
	}
	if report.DatasetInfo.ID != "aws_g5_xlarge" {
		t.Errorf("expected dataset ID aws_g5_xlarge, got %s", report.DatasetInfo.ID)
	}
	if report.TotalHours != 720 {
		t.Errorf("expected 720 total hours, got %d", report.TotalHours)
	}

	// Unhedged Spot vs Delta-Hedged Call vs Forward
	spot, hasSpot := report.Strategies["Unhedged Spot"]
	if !hasSpot || spot.TotalCostUSD <= 0 {
		t.Fatalf("expected valid spot metrics: %+v", spot)
	}

	call, hasCall := report.Strategies["Delta-Hedged Call Protection"]
	if !hasCall || call.TotalCostUSD <= 0 {
		t.Fatalf("expected valid call hedge metrics: %+v", call)
	}

	// Deferral on empirical diurnal curve
	if report.WorkloadSummary.CostSavingsPct <= 0 {
		t.Errorf("expected positive cost savings from workload deferral, got %.2f%%", report.WorkloadSummary.CostSavingsPct)
	}
	if report.WorkloadSummary.SLAViolationsCount != 0 {
		t.Errorf("expected 0 SLA violations, got %d", report.WorkloadSummary.SLAViolationsCount)
	}

	// Check markdown formatting contains empirical dataset details
	md := backtest.FormatMarkdownReport(report)
	if !strings.Contains(md, "AWS EC2 Spot: g5.xlarge") {
		t.Errorf("markdown report missing empirical dataset name")
	}
}

func TestBacktestEngine_EmpiricalVastAI_H100(t *testing.T) {
	engine := backtest.NewBacktestEngine()
	cfg := backtest.BacktestConfig{
		DatasetName:      "vastai_h100_sxm",
		NumWorkloadTasks: 100,
		RandomSeed:       42,
	}

	report, err := engine.RunBacktest(cfg)
	if err != nil {
		t.Fatalf("failed to run empirical backtest on vastai_h100_sxm: %v", err)
	}

	if report.DatasetInfo == nil || report.DatasetInfo.ID != "vastai_h100_sxm" {
		t.Errorf("expected dataset vastai_h100_sxm, got %+v", report.DatasetInfo)
	}

	// Verify Kupiec test ran on empirical series
	if report.Kupiec95.SampleSize != 720 {
		t.Errorf("expected sample size 720, got %d", report.Kupiec95.SampleSize)
	}
}

func TestBacktestEngine_CustomCSVLoading(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "custom_spot_*.csv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	csvData := `timestamp,spot_price,on_demand_price
2026-08-01T00:00:00Z,1.25,2.00
2026-08-01T01:00:00Z,1.20,2.00
2026-08-01T02:00:00Z,1.18,2.00
2026-08-01T03:00:00Z,2.80,2.00
2026-08-01T04:00:00Z,1.30,2.00
`
	if _, err := tmpFile.WriteString(csvData); err != nil {
		t.Fatalf("failed to write custom CSV: %v", err)
	}
	tmpFile.Close()

	engine := backtest.NewBacktestEngine()
	report, err := engine.RunBacktest(backtest.BacktestConfig{
		DatasetPath:      tmpFile.Name(),
		NumWorkloadTasks: 5,
		RandomSeed:       42,
	})
	if err != nil {
		t.Fatalf("failed to run custom CSV backtest: %v", err)
	}

	if report.TotalHours != 5 {
		t.Errorf("expected 5 hours from custom CSV, got %d", report.TotalHours)
	}
	spot := report.Strategies["Unhedged Spot"]
	if spot.MaxHourlyCost != 2.80 {
		t.Errorf("expected max hourly cost 2.80, got %f", spot.MaxHourlyCost)
	}
}
