import pandas as pd
import json
import os

# Data from Pricing Oracle
prices = {'H100_SXM': 2.49, 'Forward_30d': 2.5522, 'Option_Call': 0.0448}

# Portfolio Calculation: Allocation based on inverse cost
# (Simulating allocation strategy across multiple tiers)
assets = ['H100_SXM', 'Forward_30d']
costs = [prices['H100_SXM'], prices['Forward_30d']]
weights = [1/c for c in costs]
normalized_weights = [w / sum(weights) for w in weights]

df = pd.DataFrame({'Asset': assets, 'Cost': costs, 'Weight': normalized_weights})

# Ensure reports directory exists
os.makedirs('reports', exist_ok=True)

# Save report
report_data = df.to_dict(orient='records')
with open('reports/gpu_portfolio_report.json', 'w') as f:
    json.dump(report_data, f, indent=4)

print(f'Portfolio report generated: {report_data}')