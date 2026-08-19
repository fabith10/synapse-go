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

## 3. Institutional Derivatives Engine: Black-76 Model & Greeks Surface

Options contracts give the agent or user the right, but not the obligation, to lock in a specific GPU capacity strike price. This protects agents orchestrating long-running training loops or sandboxes against unexpected spot surges and capacity shortages.

### Black-76 European Commodity Option Model
SynapseGo uses the institutional **Black-76 model** for pricing European calls and puts on compute forward contracts $F$ with strike $K$, expiration $T$ (years), risk-free rate $r$, and annualized volatility $\sigma$:

$$d_1 = \frac{\ln(F/K) + \frac{1}{2}\sigma^2 T}{\sigma \sqrt{T}}, \quad d_2 = d_1 - \sigma \sqrt{T}$$

$$C = e^{-r T} \left[ F \Phi(d_1) - K \Phi(d_2) \right]$$

$$P = e^{-r T} \left[ K \Phi(-d_2) - F \Phi(-d_1) \right]$$

### Complete Analytical Greeks Surface
The engine calculates exact first- and second-order Greek sensitivities:

* **Delta ($\Delta$):** Optimal hedge ratio:
  $$\Delta_{\text{call}} = e^{-r T} \Phi(d_1), \quad \Delta_{\text{put}} = -e^{-r T} \Phi(-d_1)$$
* **Gamma ($\Gamma$):** Curvature / acceleration of delta per $1/hr change in forward rate:
  $$\Gamma = \frac{e^{-r T} \phi(d_1)}{F \sigma \sqrt{T}}$$
* **Vega ($\mathcal{V}$):** Sensitivity to shifts in implied volatility:
  $$\mathcal{V} = F e^{-r T} \sqrt{T} \phi(d_1)$$
* **Theta ($\Theta$):** Daily time decay of option premium:
  $$\Theta_{\text{call}} = \frac{1}{365} \left( -\frac{F e^{-r T} \phi(d_1) \sigma}{2 \sqrt{T}} - r C \right)$$
* **Rho ($\rho$):** Sensitivity to capital cost / interest rate $r$:
  $$\rho_{\text{call}} = -T C, \quad \rho_{\text{put}} = -T P$$

---

## 4. Stochastic Spot Dynamics: Ornstein-Uhlenbeck Jump-Diffusion (OU-MRJD)

Compute spot prices exhibit diurnal demand seasonality, strong mean reversion, and sudden Poisson jumps caused by cluster demand spikes and instance evictions:

$$dS_t = \kappa \left( \theta(t) - S_t \right) dt + \sigma S_t dW_t + J_t dN_t$$

Where:
* $\kappa$: Mean reversion rate ($\approx 0.75/\text{hr}$).
* $\theta(t) = a_0 + \sum_{k=1}^2 \left( a_k \cos\left(\frac{2\pi k t}{24}\right) + b_k \sin\left(\frac{2\pi k t}{24}\right) \right)$: Diurnal Fourier harmonic mean.
* $\sigma$: Continuous diffusion volatility.
* $N_t$: Poisson jump process with intensity $\lambda$ (arrival rate of capacity eviction/spike events).
* $J_t \sim \text{LogNormal}(\mu_J, \sigma_J^2)$: Random jump amplitude.

---

## 5. Execution Window Tail Risk: Monte Carlo VaR & CVaR (Expected Shortfall)

For every candidate execution window $T \in [0, 23]$, the engine runs $N = 10,000$ Monte Carlo paths simulating the OU-MRJD stochastic process to compute tail risk metrics:

* **$\text{VaR}_{95}$ (95% Value-at-Risk):** The 95th percentile worst execution cost.
* **$\text{CVaR}_{95}$ (95% Conditional VaR / Expected Shortfall):** The expected average cost during the 5% worst spike/eviction scenarios:
  $$\text{CVaR}_{95} = \mathbb{E}\left[ \text{Cost} \mid \text{Cost} \ge \text{VaR}_{95} \right]$$
* **Eviction Risk Probability:** The empirical probability $P(\text{Eviction})$ that a jump spike occurs during the execution window.

---

## 6. Real Options Analysis (ROA): Valuation of Flexibility

The engine quantifies managerial and algorithmic flexibilities available to the agent:
* **Option to Defer:** The net present economic value of waiting for an off-peak execution slot versus executing immediately under spot uncertainty:
  $$\text{Value}_{\text{Defer}} = \max(0, \text{Cost}_{\text{immediate}} - \text{Cost}_{\text{deferred}}) + \text{VolPremium}_{\text{flexibility}}$$
* **Option to Switch:** The economic value of maintaining dynamic multi-tier fallback (Local Apple Silicon Metal $\leftrightarrow$ Spot Cloud $\leftrightarrow$ On-Demand).

---

## 7. Optimal Execution Window Scheduling & Hedging

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

---

## 8. Specialized Structural Models for Compute Flow Commodities

Unlike financial equities, GPU compute is a **non-storable flow commodity** subject to **technological obsolescence** and **cluster capacity queuing**. SynapseGo provides three specialized structural models:

### 1. Lucia-Schwartz Two-Factor Model
Decomposes spot price dynamics into short-term diurnal mean reversion and long-term Moore's Law deflation:

$$\ln S_t = f(t) + X_t + Y_t$$

* **Factor 1 (Short-Term OU):** $dX_t = -\kappa_X X_t dt + \sigma_X dW_t^X$ (hourly demand fluctuations).
* **Factor 2 (Long-Term Deflation):** $dY_t = \mu_Y dt + \sigma_Y dW_t^Y$ ($\mu_Y < 0$, annual hardware price decay as new GPU architectures launch).
* **Closed-Form Forward Curve:**
  $$F(t, T) = \exp\left( f(T) + X_t e^{-\kappa_X(T-t)} + Y_t + \mu_Y(T-t) + \frac{1}{2}\text{Var}(T-t) \right)$$

### 2. Markov Regime-Switching Model (MRS / Hamilton)
Models discrete shifts between two physical market regimes:
* **Regime 0 (Normal Liquid):** Low volatility ($\sigma_0 \approx 12\%$), low eviction risk ($P(\text{evict}) = 0.5\%$).
* **Regime 1 (Cluster Congestion):** High volatility ($\sigma_1 \approx 85\%$), high eviction probability ($P(\text{evict}) = 28\%$).

Uses Bayesian posterior filtering to classify the current market state from observed spot prices:
$$P(R_t = 1 \mid S_{\text{obs}}) = \frac{P(S_{\text{obs}} \mid R_t = 1) \pi_1}{P(S_{\text{obs}} \mid R_t = 1) \pi_1 + P(S_{\text{obs}} \mid R_t = 0) \pi_0}$$

### 3. M/M/c Queuing Capacity Congestion Model (Erlang-C)
Derives spot pricing directly from cluster capacity load $\rho = \frac{\text{Allocated Nodes}}{\text{Total Available Nodes}}$:

$$S(\rho) = S_{\text{base}} + \frac{\alpha \cdot \rho^\gamma}{1 - \rho}$$

* **Implied Cluster Load Inversion:** Automatically inverts observed spot rates $S_{\text{obs}}$ to infer current data center utilization.
* **Queuing Eviction Risk Curve:** Models physical eviction probability as utilization approaches saturation ($100\%$).

---

## 9. Game-Theoretic Anti-Herding & Multi-Cluster Portfolio Suite

When multiple autonomous agents or autoscalers optimize for the same off-peak window, deterministic scheduling causes **herding spikes / the El Farol Bar paradox**. SynapseGo provides game-theoretic mechanisms to achieve stable Nash Equilibria and eliminate correlated failure:

### 1. Mixed-Strategy Nash Equilibrium (Boltzmann-Gibbs Scheduling)
Instead of deterministically picking a single minimum-cost hour, the engine computes a Gibbs probability distribution over all 24 candidate start hours:

$$P(\text{Start} = t) = \frac{\exp\left(-\beta \cdot \text{Cost}(t)\right)}{\sum_{\tau=0}^{23} \exp\left(-\beta \cdot \text{Cost}(\tau)\right)}$$

* **Shannon Dispersion Entropy:** $H(P) = -\sum_{t=0}^{23} P(t) \ln P(t)$ quantifies anti-herding diversity across the agent swarm.
* Disperses agent batch dispatches across near-optimal off-peak and shoulder hours, preventing synchronized cluster saturation and demand spikes.

### 2. Minority Game & Crowding Density Feedback $D(t)$
Penalizes over-subscribed off-peak windows where other external bots herd:

$$\text{Cost}_{\text{crowd}}(t) = \text{Cost}(t) \cdot \left(1 + \eta \cdot D(t)^\gamma\right)$$

Dynamically shifts the optimal execution recommendation from crowded troughs to calm, high-capacity shoulder hours.

### 3. Colonel Blotto Multi-Cluster Portfolio Allocation
For heavy compute batches, divides the workload across non-correlated spot providers (e.g. Vast.ai Spot + RunPod Spot + Local Metal Buffer):

$$P(\text{Joint Preemption}) = \prod_{i=1}^K P(\text{Eviction}_i)^{w_i \cdot N} \ll P(\text{Single Cluster})$$

Reduces joint pipeline preemption risk by **$>95\%$** compared to single-cluster execution.

---

## 10. Quantitative Backtesting & Model Risk Validation Engine

To validate pricing, hedging, and deferral models against real and synthetic market dynamics, SynapseGo includes an institutional backtesting engine in `internal/agent/pricing/backtest`:

### Core Backtesting Strategies
1. **Unhedged Spot (Baseline):** Pure spot market execution subject to diurnal volatility and jump surges.
2. **Forward Lock:** 100% capacity locked at forward rate $F = S_0 (1 + r_{\text{carry}})$.
3. **Delta-Hedged Call Protection (Black-76):** Capped spot rate at strike $K$ with amortized option premium.
4. **Execution Window Deferral:** Dynamic task scheduling across diurnal spot tariffs guaranteeing SLA deadline adherence ($0\%$ breach rate).
5. **Game-Theoretic Nash Bidding:** Dynamic bid adjustment based on implied cluster load $\rho$.

### Model Risk & VaR Calibration: Kupiec POF Likelihood Ratio Test
The engine validates 95% and 99% Value-at-Risk (VaR) forecasts using the **Kupiec Proportion-of-Failures (POF) test**:

$$\text{LR}_{\text{POF}} = -2 \ln \left[ (1 - p)^{N - x} p^x \right] + 2 \ln \left[ \left(1 - \frac{x}{N}\right)^{N - x} \left(\frac{x}{N}\right)^x \right] \sim \chi^2(1)$$

* Where $p$ is the theoretical failure rate ($\alpha = 0.05$ or $0.01$), $x$ is the count of empirical VaR exceedances, and $N$ is the sample size.
* If $\text{LR}_{\text{POF}} \le 3.841$ (critical value at $95\%$ confidence with 1 degree of freedom), the pricing oracle model is validated as **statistically well-calibrated**.

### Empirical Historical Datasets Catalog
The framework includes 6 curated 30-day (720-hour) empirical spot price time series across AWS EC2 Spot and Vast.ai GPU markets:
* `aws_g5_xlarge`: NVIDIA A10G 24GB ($0.30–$0.63/hr spot vs $1.006 on-demand, us-east-1).
* `aws_g4dn_xlarge`: NVIDIA T4 16GB ($0.15–$0.28/hr spot vs $0.526 on-demand, us-east-1).
* `aws_p3_2xlarge`: NVIDIA V100 16GB ($0.91–$1.79/hr spot vs $3.060 on-demand, us-west-2).
* `vastai_h100_sxm`: NVIDIA H100 SXM5 80GB ($2.15–$3.85/hr spot clearing rates).
* `vastai_a100_80gb`: NVIDIA A100 SXM4 80GB ($1.15–$2.06/hr spot clearing rates).
* `vastai_rtx4090`: NVIDIA RTX 4090 24GB ($0.36–$0.72/hr spot clearing rates).

### Agent Tool Interface
Backtests can be triggered programmatically by developers or autonomously by the Quant Agent using `run_pricing_backtest` (e.g. `{"dataset": "aws_g5_xlarge"}`) or `query_pricing_oracle` with `market_type='backtest'`.





