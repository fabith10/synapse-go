import pandas as pd
import json

# Data retrieved from Pricing Oracle
prices = {
    'H100_SXM': 2.49,
    'A100_80GB': 1.85,
    'RTX_4090': 0.75
}

# Portfolio configuration
total_budget = 1000

# Simple Mean-Variance approximation/weighting logic
df = pd.DataFrame(list(prices.items()), columns=['GPU', 'Hourly_Rate'])
df['Weight'] = 1 / df['Hourly_Rate']
df['Weight'] = df['Weight'] / df['Weight'].sum()
df['Allocated_Hours_Budget'] = (df['Weight'] * total_budget) / df['Hourly_Rate']

# Persist results
result = df.to_dict(orient='records')
with open('reports/gpu_portfolio_report.json', 'w') as f:
    json.dump(result, f, indent=4)

print(f'Portfolio calculation complete. Results saved to reports/gpu_portfolio_report.json')
print(df.to_string())