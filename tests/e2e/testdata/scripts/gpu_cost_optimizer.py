import numpy as np

# Compute Infrastructure Audit & Financial Cost Optimization Model
h100_spot = 2.49
h100_od = 3.89
rtx_spot = 2.49
rtx_od = 3.89

forwards = {
    '1M': 2.54,
    '3M': 2.62,
    '6M': 2.75
}

print("=== COMPUTE INFRASTRUCTURE AUDIT & FINANCIAL MODEL ===")
print(f"H100 Spot Rate: ${h100_spot}/hr | On-Demand: ${h100_od}/hr")
print(f"RTX 4090 Spot Rate: ${rtx_spot}/hr | On-Demand: ${rtx_od}/hr")
print("Forward Curve Tenors:")
for tenor, price in forwards.items():
    print(f"  Tenor {tenor}: ${price}/hr")
