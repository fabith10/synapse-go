# H100 SXM Cost Optimizer
# Analyzes optimal execution windows based on 24-hour tariff cycles.

base_rate = 2.49
# Hourly rates for a 24-hour cycle
hourly_prices = [
    2.00, 1.85, 1.75, 1.70, 1.75, 1.90,  
    2.20, 2.60, 3.00, 3.20, 3.10, 3.00,  
    2.90, 2.85, 2.90, 3.05, 3.15, 3.25,  
    3.00, 2.70, 2.40, 2.20, 2.10, 2.05   
]

def calculate_optimal_window(K):
    min_cost = float('inf')
    best_start = 0
    for t in range(24):
        window_cost = sum(hourly_prices[(t + h) % 24] for h in range(K))
        if window_cost < min_cost:
            min_cost = window_cost
            best_start = t
    return best_start, min_cost

if __name__ == '__main__':
    K = 4
    best_start, min_cost = calculate_optimal_window(K)
    peak_rate = max(hourly_prices)
    print(f"Optimal Window: {best_start:02d}:00 - {(best_start + K) % 24:02d}:00")
    print(f"Total Cost: ${min_cost:.2f}")