import json

def calculate_cost_comparison(spot_rate, forward_rate, option_premium, hours=720):
    # Calculation: Spot vs Forward vs Hedged (Forward + Call Option)
    spot_total = spot_rate * hours
    forward_total = forward_rate * hours
    hedged_total = (forward_rate * hours) + option_premium
    
    print(f"--- GPU Cost Analysis (H100 SXM, {hours} hours) ---")
    print(f"Host Environment: Apple Silicon Metal (Local Dev)")
    print(f"Spot Total Cost: ${spot_total:.2f}")
    print(f"Forward Contract Total Cost: ${forward_total:.2f}")
    print(f"Hedged Strategy Cost: ${hedged_total:.2f}")
    print(f"Savings (Forward vs Spot): ${spot_total - forward_total:.2f}")

if __name__ == '__main__':
    calculate_cost_comparison(2.4900, 2.5522, 0.0448)