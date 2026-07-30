import numpy as np

# Configuration for GPU Spot Rate Simulation
iterations = 10000
initial_price = 0.50  # Base hourly price for A100
mu = 0.01             # Daily drift
volatility = 0.05     # Daily volatility
days = 30

# Generate random paths
np.random.seed(42)
returns = np.random.normal(mu/252, volatility/np.sqrt(252), (days, iterations))
price_paths = initial_price * np.exp(np.cumsum(returns, axis=0))

# Final prices at the end of 30 days
final_prices = price_paths[-1, :]

# Calculate Value at Risk (VaR) at 95% confidence level
var_95 = np.percentile(final_prices, 5)
print(f'Monte Carlo Simulation Results (10,000 trials):')
print(f'Initial Price: ${initial_price:.4f}')
print(f'Expected Price after 30 days: ${np.mean(final_prices):.4f}')
print(f'95% Value-at-Risk (Lower bound): ${var_95:.4f}')