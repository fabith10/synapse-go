import numpy as np

# Simulation parameters
iterations = 10000
base_rate = 0.50  # Base GPU spot rate in $/hr
volatility = 0.25  # Annualized volatility factor
time_horizon = 1 / 8760  # 1 hour expressed in years

# Generate random price paths using Geometric Brownian Motion
# dS = S * (mu * dt + sigma * epsilon * sqrt(dt))
# Assuming drift (mu) is 0 for volatility analysis
returns = np.random.normal(0, volatility * np.sqrt(time_horizon), iterations)
spot_prices = base_rate * np.exp(returns)

# Calculate Value at Risk (VaR) at 95% confidence level
# VaR is the difference between current rate and the 5th percentile
var_95 = base_rate - np.percentile(spot_prices, 5)

print(f"Simulation Results (10,000 iterations):")
print(f"Base Rate: ${base_rate:.4f}")
print(f"95% VaR: ${var_95:.4f}")
print(f"5th Percentile Spot Rate: ${np.percentile(spot_prices, 5):.4f}")
print(f"95th Percentile Spot Rate: ${np.percentile(spot_prices, 95):.4f}")