import numpy as np

# Hourly spot pricing profile for H100 and RTX 4090
h_prices_h100 = np.array([2.2, 2.0, 1.8, 1.6, 1.5, 1.5, 1.7, 2.1, 3.0, 3.8, 4.2, 4.5, 4.5, 4.2, 3.9, 3.5, 3.7, 4.0, 4.2, 4.0, 3.5, 3.0, 2.7, 2.4])
h_prices_rtx4090 = np.array([0.45, 0.40, 0.35, 0.30, 0.28, 0.28, 0.32, 0.40, 0.60, 0.75, 0.85, 0.90, 0.90, 0.85, 0.80, 0.70, 0.75, 0.80, 0.85, 0.80, 0.70, 0.60, 0.55, 0.50])

def find_optimal_window(prices, k):
    n = len(prices)
    min_cost = float('inf')
    start_idx = 0
    for t in range(n):
        window = [prices[(t + h) % n] for h in range(k)]
        current_cost = sum(window)
        if current_cost < min_cost:
            min_cost = current_cost
            start_idx = t
    return start_idx, min_cost

k = 6
idx_h100, cost_h100 = find_optimal_window(h_prices_h100, k)
avg_h100 = np.mean(h_prices_h100) * k
savings_h100 = (1 - cost_h100 / avg_h100) * 100

idx_rtx, cost_rtx = find_optimal_window(h_prices_rtx4090, k)
avg_rtx = np.mean(h_prices_rtx4090) * k
savings_rtx = (1 - cost_rtx / avg_rtx) * 100

print(f'H100 Optimal Window: {idx_h100:02d}:00 - {(idx_h100+k)%24:02d}:00 | Cost: ${cost_h100:.2f} | Savings: {savings_h100:.2f}%')
print(f'RTX 4090 Optimal Window: {idx_rtx:02d}:00 - {(idx_rtx+k)%24:02d}:00 | Cost: ${cost_rtx:.2f} | Savings: {savings_rtx:.2f}%')