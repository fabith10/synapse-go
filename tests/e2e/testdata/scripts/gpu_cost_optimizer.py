import numpy as np

# Mock data for 24-hour tariff cycle (US-East RunPod)
# Base price is $2.49, simulating daily fluctuations for demonstration
# as per Quantitative Finance Standard 1
rates = [
    2.49, 2.45, 2.40, 2.35, 2.30, 2.25, 2.28, 2.35,
    2.45, 2.49, 2.50, 2.50, 2.50, 2.50, 2.49, 2.48,
    2.47, 2.49, 2.50, 2.50, 2.50, 2.50, 2.49, 2.48
]

def calculate_optimal_window(hourly_rates, window_len=4):
    n = len(hourly_rates)
    min_cost = float('inf')
    best_start = 0
    
    for t in range(n):
        current_window_cost = sum(hourly_rates[(t + h) % n] for h in range(window_len))
        if current_window_cost < min_cost:
            min_cost = current_window_cost
            best_start = t
            
    return best_start, min_cost

window_len = 4
start_hour, min_cost = calculate_optimal_window(rates, window_len)

print(f'Optimal {window_len}-hour execution window starts at: {start_hour:02d}:00')
print(f'Minimum cost for window: ${min_cost:.2f}')
print(f'Average rate during window: ${min_cost/window_len:.2f}/hr')