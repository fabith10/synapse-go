rates = [
    2.10, 2.00, 1.90, 1.85, 1.95, 2.10, 2.40, 2.80, 
    3.20, 3.50, 3.50, 3.40, 3.30, 3.20, 3.10, 3.00, 
    2.80, 2.60, 2.40, 2.30, 2.20, 2.15, 2.10, 2.05
]

window_len = 4
min_cost = float('inf')
optimal_start = 0

for t in range(24):
    window_indices = [(t + h) % 24 for h in range(window_len)]
    current_window_cost = sum(rates[idx] for idx in window_indices)
    
    if current_window_cost < min_cost:
        min_cost = current_window_cost
        optimal_start = t

avg_rate = sum(rates) / len(rates)
standard_4h_cost = avg_rate * window_len
savings = standard_4h_cost - min_cost
percentage_savings = (savings / standard_4h_cost) * 100

print(f"Optimal 4-hour window starts at: {optimal_start:02d}:00")
print(f"Minimum Cost for 4-hour block: ${min_cost:.2f}")
print(f"Standard 4-hour Cost: ${standard_4h_cost:.2f}")
print(f"Total Savings: ${savings:.2f} ({percentage_savings:.2f}%)")