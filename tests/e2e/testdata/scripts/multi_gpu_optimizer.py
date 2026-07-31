import pandas as pd
import json
import os

# Data from Pricing Oracle
prices = {'H100 SXM': 2.49, 'A100 SXM': 1.85, 'RTX 4090': 0.75}

# Calculate simple inverse volatility/cost weights for a mock portfolio of compute
# Assuming risk is proportional to price for this demonstration
df = pd.DataFrame.from_dict(prices, orient='index', columns=['price'])
df['weight'] = (1 / df['price']) / (1 / df['price']).sum()

# Save report
report_data = df.to_dict(orient='index')
if not os.path.exists('reports'):
    os.makedirs('reports')
with open('reports/gpu_portfolio_report.json', 'w') as f:
    json.dump(report_data, f, indent=4)

print(f"Optimization complete. Weights:\n{df['weight']}")