import numpy as np

# Simulating 24-hour tariff cycle for RunPod H100 spot pricing with base $2.49/hr
np.random.seed(42)
hours = 24
base_price = 2.49
# Create a realistic 24-hour tariff curve (higher during peak business hours 09:00 - 18:00)
tariffs = []
for h in range(hours):
    if 9 <= h <= 18:
        mult = 1.0 + 0.4 * np.sin((h - 9) * np.pi / 9)
    else:
        mult = 0.75 + 0.15 * np.cos(h * np.pi / 12)
    tariffs.append(round(base_price * mult, 4))

# Find optimal 4-hour sliding window
k = 4
min_cost = float('inf')
optimal_start = 0

for t in range(hours):
    window_cost = sum(tariffs[(t + h) % hours] for h in range(k))
    if window_cost < min_cost:
        min_cost = window_cost
        optimal_start = t

optimal_end = (optimal_start + k) % hours
standard_cost = sum(tariffs[h] for h in range(9, 9+k)) if 9+k <= hours else sum(tariffs[h] for h in range(hours))
# Let's compute standard peak cost for comparison (e.g. starting at hour 10)
peak_start = 10
peak_window_cost = sum(tariffs[(peak_start + h) % hours] for h in range(k))
savings_pct = ((peak_window_cost - min_cost) / peak_window_cost) * 100

print(f"Host Hardware Profile: Apple Silicon Metal (arm64), 12 CPU Cores, Unified Memory")
print(f"Base Spot Rate: ${base_price}/hr")
print(f"24-Hour Hourly Tariffs: {tariffs}")
print(f"Optimal 4-Hour Execution Window: {optimal_start:02d}:00 - {optimal_end:02d}:00")
print(f"Optimal Window Total Cost: ${min_cost:.4f}")
print(f"Standard Peak Window Cost ({peak_start:02d}:00 - {(peak_start+k)%24:02d}:00): ${peak_window_cost:.4f}")
print(f"Calculated Dollar Savings: ${peak_window_cost - min_cost:.4f}")
print(f"Percentage Savings: {savings_pct:.2f}%")