import numpy as np

def calculate_optimal_window():
    # Together NVIDIA-RTX-4090 spot price: $0.45/hr
    base_rate = 0.45
    tariff_variance = np.array([1.2, 1.1, 1.05, 1.0, 0.9, 0.95, 1.0, 1.1, 1.2, 1.3, 1.4, 1.4, 1.3, 1.25, 1.2, 1.15, 1.1, 1.05, 1.1, 1.2, 1.3, 1.35, 1.3, 1.25])
    prices = base_rate * tariff_variance
    K = 4
    min_cost = float('inf')
    start_hour = 0
    for t in range(24):
        current_window_cost = sum(prices[(t + h) % 24] for h in range(K))
        if current_window_cost < min_cost:
            min_cost = current_window_cost
            start_hour = t
    peak_cost = sum(sorted(prices, reverse=True)[:K])
    savings = ((peak_cost - min_cost) / peak_cost) * 100
    return start_hour, min_cost, savings

if __name__ == '__main__':
    start, cost, savings = calculate_optimal_window()
    print(f'Optimal Start Hour: {start:02d}:00')
    print(f'Cost for 4-hour Block: ${cost:.2f}')
    print(f'Potential Savings vs. Peak: {savings:.2f}%')