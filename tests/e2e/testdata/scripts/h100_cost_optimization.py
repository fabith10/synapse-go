import numpy as np

# Data from Pricing Oracle
base_rate = 2.49
forward_rate = 2.5522

# Assuming a standard 24-hour cycle where rates might fluctuate +/- 20% in a mock market
hours = np.arange(24)
# Mocking hourly price variance (sinusoidal pattern reflecting peak/off-peak)
rates = base_rate * (1 + 0.2 * np.sin(np.pi * (hours - 6) / 12))

# Sliding window calculation for 6-hour continuous compute block
K = 6
min_cost = float('inf')
best_start = 0

for t in range(24):
    window = [rates[(t + h) % 24] for h in range(K)]
    current_cost = sum(window)
    if current_cost < min_cost:
        min_cost = current_cost
        best_start = t

savings = (base_rate * K - min_cost) / (base_rate * K) * 100

print(f'Optimal 6-hour window starts at: {best_start:02d}:00')
print(f'Total cost for window: ${min_cost:.2f}')
print(f'Percentage savings vs flat rate: {savings:.2f}%')