# Removed dependency on numpy

def calculate_execution_window(hourly_rates, duration_hours):
    """
    Calculates the cheapest window to run a job of K hours in a 24-hour cycle.
    """
    n = len(hourly_rates)
    min_cost = float('inf')
    start_idx = 0
    
    for t in range(n):
        current_window_cost = sum(hourly_rates[(t + h) % n] for h in range(duration_hours))
        if current_window_cost < min_cost:
            min_cost = current_window_cost
            start_idx = t
            
    return start_idx, min_cost

# Mock tariff data for RunPod US-East (Spot cycle proxy)
rates = [2.49, 2.49, 2.49, 2.48, 2.47, 2.45, 2.45, 2.46, 2.48, 2.49, 2.50, 2.51, 2.52, 2.52, 2.51, 2.50, 2.49, 2.49, 2.49, 2.49, 2.49, 2.49, 2.49, 2.49]
K = 4

start_h, cost = calculate_execution_window(rates, K)
# Added validation constraint: Only proceed if cost is within budget ($2.50/hr average)
if (cost / K) <= 2.50:
    print(f"Optimal Start Time: {start_h:02d}:00")
    print(f"Total Cost for {K} hours: ${cost:.2f} (Avg: ${cost/K:.2f}/hr)")
else:
    print(f"No valid window found under $2.50/hr limit. Minimum cost: ${cost:.2f}")