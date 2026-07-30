import pandas as pd
import json
import os

# Pricing data
data = {
    'gpu': ['H100', 'A100'],
    'price': [2.49, 1.50]  # Using representative spot prices
}

df = pd.DataFrame(data)

# Calculate simple inverse weight allocation (allocate more to cheaper GPUs)
df['inverse_price'] = 1 / df['price']
df['weight'] = df['inverse_price'] / df['inverse_price'].sum()

# Prepare output
report = df.to_dict(orient='records')

# Create directory if it doesn't exist
os.makedirs('reports', exist_ok=True)

with open('reports/gpu_portfolio_report.json', 'w') as f:
    json.dump(report, f, indent=4)

print(f'Report saved: {report}')