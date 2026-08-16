# Compute Derivatives Pricing Specification

This document details the mathematical models and implementation specifications used by the Go Agent-Framework to calculate Spot prices, Forwards, and Options premiums for cloud GPU compute allocations.

---

## 1. Core Concepts & Indexing

To establish financial pricing models for compute capacity (such as GPU spot instances, fixed-term leases, or future allocations), the framework queries a live compute market index:
- **Spot Index:** Direct USD spot rate per GPU-hour ($/hour) for target hardware architectures (e.g., NVIDIA H100 SXM baseline at `$2.49/hr`, A100 80GB at `$1.29/hr`).
- **Tool Endpoint:** Managed natively via the `query_pricing_oracle` Tier 1 tool, routed dynamically through `PricingOracleManager`.
- **Dynamic Provider Selection:** Fully configurable at runtime via environment variables (`PRICING_PROVIDER`), `pricing_providers.json`, or the `/api/config/pricing-provider` HTTP endpoint without recompilation.

---

## 2. Forward Contract Rate Calculation

Forward contracts allow users or agents to lock in compute prices for future runs, hedging against sudden spot price spikes or hardware shortages.

### Pricing Formula
The 30-day forward rate is priced using the spot index plus a cost-of-carry risk premium factor of **2.5%**:

$$\text{Forward Rate} = \text{Spot Price} \times (1 + r_{\text{carry}})$$

Where:
- $r_{\text{carry}} = 0.025$ (representing cost of capital reservation, container host allocation scheduling, and risk premium).

### Concrete Example
If the live spot rate for an H100 SXM node is **$2.49/hr**:
- **30-Day Forward Rate:** $\$2.49 \times 1.025 = \$2.5523$ per GPU-hour.

---

## 3. Options Contract Premium Calculation

Options contracts give the agent or user the right, but not the obligation, to lock in a specific GPU capacity strike price. This is critical for agents orchestrating long-running training loops or math sandboxes that might require resource escalation.

### Pricing Model (Heuristic Call Option)
Options are calculated as European Call Options with a Strike Price set at **105%** of the current Spot rate:

$$\text{Strike Price (K)} = \text{Spot Price} \times 1.05$$

The option premium (the cost to purchase the call contract) is priced dynamically at a volatility factor of **1.8%** of the current Spot price:

$$\text{Option Premium (C)} = \text{Spot Price} \times \sigma_{\text{compute}}$$

Where:
- $\sigma_{\text{compute}} = 0.018$ (representing compute market volatility, network demand deviations, and time value of options expiration).

### Concrete Example
If the current H100 SXM spot rate is **$2.49/hr**:
- **105% Strike Price:** $\$2.49 \times 1.05 = \$2.6145$ per GPU-hour.
- **Call Option Premium:** $\$2.49 \times 0.018 = \$0.0448$ per GPU-hour.

---

## 4. Implementation Code Reference

```go
// Inside internal/agent/pricing_oracle.go: PricingOracleManager

baseSpotRate := 2.49 // USD/hour for NVIDIA H100 SXM

// Forward contract (30-day): Spot + 2.5% premium
forwardRate := baseSpotRate * 1.025

// Options (Call Strike 105%): Option Premium = 1.8% of spot
optionPremium := baseSpotRate * 0.018

result := map[string]interface{}{
    "status":                     "synchronized",
    "provider":                   "mock",
    "asset":                      asset,
    "spot_gpu_rate_per_hour_usd": fmt.Sprintf("$%.4f", baseSpotRate),
    "30d_forward_contract_rate":  fmt.Sprintf("$%.4f", forwardRate),
    "option_premium_call_105":    fmt.Sprintf("$%.4f", optionPremium),
}
```

---

## 5. Optimal Execution Window Scheduling & Hedging

To find the most cost-effective start time for compute jobs, the oracle supports optimal execution window queries. This allows agents to compute scheduling paths that minimize expenditures over a 24-hour cycle.

### Optimization Objectives (`cost_mode`)

The sliding-window cost-minimization algorithm supports two objectives:

#### A. Raw Spot Optimization (`cost_mode = "spot"`)
Calculates the total cost of running a job of length $K$ starting at hour $T$ using the raw forecasted spot rate per hour:

$$\text{Cost}(T) = \sum_{h=0}^{K-1} \text{SpotRate}_{(T+h) \bmod 24}$$

$$\text{Optimal Start } T^* = \arg\min_{T \in [0, 23]} \text{Cost}(T)$$

#### B. Hedged Spot Optimization (`cost_mode = "hedged_spot"`)
Applies a synthetic cap to protect the scheduling window from spot spikes, assuming the user owns a forward contract that locks in a ceiling price $S_{\text{forward}}$ (e.g. forward contract rate):

$$\text{EffectiveRate}_h = \min\bigl(\text{SpotRate}_h,\ S_{\text{forward}}\bigr)$$

$$\text{Cost}(T) = \sum_{h=0}^{K-1} \text{EffectiveRate}_{(T+h) \bmod 24}$$

This determines if holding a forward contract alters the optimal start window or shifts the execution timing from off-peak to peak shoulder hours.

#### C. Volatility Risk-Adjusted Optimization (`cost_mode = "risk_adjusted"`)
Applies a risk premium to spot rates based on options implied volatility ($\text{ImpliedVol}$) to protect scheduling against expected price spikes or capacity evictions during spot off-peak hours:

$$\text{EffectiveRate}_h = \text{SpotRate}_h \times \left(1.0 + \text{ImpliedVol}_h \times k_{\text{risk}}\right)$$

$$\text{Cost}(T) = \sum_{h=0}^{K-1} \text{EffectiveRate}_{(T+h) \bmod 24}$$

Where:
- $\text{ImpliedVol}_h$ is the implied volatility at hour $h$ obtained from the options market / volatility surface.
- $k_{\text{risk}} = 1.0$ is the risk scaling factor.

If options pricing models expect a demand spike during specific off-peak hours (e.g. night-time batch runs), the implied volatility will spike, raising the risk-adjusted rate. The optimizer will automatically shift the optimal start $T^*$ to lower-risk windows (e.g., shoulder hours) despite higher raw spot rates.

---

## 6. Provider Architecture: Native Adapters vs. Declarative Zero-Code

SynapseGo supports a two-tier pricing provider architecture that balances out-of-the-box convenience with complete runtime extensibility:

```
                               ┌──────────────────────────────────────────────┐
                               │             PricingOracleManager             │
                               └──────────────────────┬───────────────────────┘
                                                      │
                       +------------------------------+------------------------------+
                       │                                                             │
                       v                                                             v
        ┌──────────────────────────────┐                              ┌──────────────────────────────┐
        │   1. Built-in Native Go      │                              │  2. Declarative Zero-Code    │
        │      (Out-of-the-Box)        │                              │    (Runtime JSON Config)     │
        ├──────────────────────────────┤                              ├──────────────────────────────┤
        │ • free_live (Composite)      │                              │ • declarative_http           │
        │ • openrouter (Token rates)   │                              │ • custom_http                │
        │ • vastai (GPU spot bundles)  │                              │ • Arbitrary 3rd-party REST   │
        │ • deribit (Forwards & IV)    │                              │ • JSONPath / Dot-Notation    │
        │ • static_json / mock / null  │                              │ • Header ${ENV_VAR} expand   │
        └──────────────────────────────┘                              └──────────────────────────────┘
```

### Tier 1: Built-in Native Adapters (Out-of-the-Box)

Built-in Go adapters provide turnkey zero-configuration integration for major free public feeds:

| Provider | Type | Capabilities & Description |
| :--- | :--- | :--- |
| **`free_live`** | `free_live` | Composite turnkey provider aggregating OpenRouter (LLM token costs), Vast.ai (GPU spot rates), and Deribit (forward curves & IV). |
| **`openrouter`** | `openrouter` | Direct feed querying `https://openrouter.ai/api/v1/models` (free, unauthenticated) for 200+ models. |
| **`vastai`** | `vastai` | Direct feed querying `https://vast.ai/api/v0/bundles/` (free, unauthenticated) for real-time GPU spot asking prices. |
| **`deribit`** | `deribit` | Direct feed querying Deribit public futures & volatility endpoints for real-time term structures. |
| **`static_json`** | `static_json` | Reads local rate sheets (e.g. `open_weight_rates.json` or `pricing_matrix.json`) for air-gapped environments. |
| **`mock`** | `mock` | Internal mock simulation server for unit tests and offline benchmarking. |

---

### Tier 2: Declarative Zero-Code Adapters (`declarative_http` / `custom_http`)

Declarative adapters allow operators and agents to integrate **any arbitrary third-party REST API** without writing Go code or recompiling the binary. All URL endpoints, headers, and JSON extraction rules are declared in `pricing_providers.json`:

```json
{
  "active_provider": "my_cloud_provider",
  "providers": {
    "my_cloud_provider": {
      "type": "declarative_http",
      "description": "Arbitrary third-party GPU spot or token pricing API",
      "base_url": "https://api.mycloud.com/v1",
      "headers": {
        "Authorization": "Bearer ${MYCLOUD_API_KEY}"
      },
      "endpoints": {
        "spot": {
          "path": "/quotes",
          "query_params": { "currency": "USD" }
        }
      },
      "mappings": {
        "spot_rate_path": "quotes.{{asset}}.hourly_rate",
        "input_token_rate_path": "models.{{asset}}.prompt_rate",
        "output_token_rate_path": "models.{{asset}}.completion_rate",
        "forward_rate_path": "futures.30d_mark_price",
        "implied_vol_path": "volatility.atm_vol",
        "token_rate_multiplier": 1000000.0,
        "array_match_key": "id",
        "static_matrix": {
          "H100_SXM": 2.49,
          "RTX_4090": 0.45
        }
      }
    }
  }
}
```

#### Declarative Extraction Features
* **Dot & Bracket Traversal:** Extract deeply nested properties (e.g., `rates.gpu.price` or `offers[0].dph_total`).
* **Dynamic `{{asset}}` Substitution:** The queried asset name (e.g., `H100_SXM`, `deepseek-r1`) is dynamically injected into paths and query params.
* **Array Search Matching:** Setting `array_match_key: "id"` automatically filters JSON arrays for elements matching the requested asset.
* **Environment Variable Expansion:** Headers automatically expand `${ENV_VAR_NAME}` from the host environment.
* **Resilient Fallbacks:** If a live endpoint is temporarily unreachable, the adapter gracefully falls back to `static_matrix` without interrupting agent workflows.

---

### Comparison: When to Use Which

| Feature | Built-in Native Adapters | Declarative Zero-Code Adapters |
| :--- | :--- | :--- |
| **Setup Effort** | 0 setup (select name) | 1 JSON block in `pricing_providers.json` |
| **Go Code Required** | Yes (pre-compiled) | **Zero Go code** |
| **Recompilation Needed** | Yes (to add new ones) | **No (hot-reloads instantly)** |
| **Custom Auth / Headers** | Fixed | Dynamic (`${ENV_VAR}` expansion) |
| **Arbitrary REST Schema Support** | Hardcoded per struct | Flexible JSONPath / Dot-notation mapping |
| **Best For** | Standard out-of-the-box free feeds | Proprietary in-house APIs, new cloud vendors |



