import time
import random

def check_latency(provider):
    start = time.time()
    time.sleep(random.uniform(0.05, 0.3))
    return time.time() - start

providers = ['RunPod', 'Lambda', 'AWS', 'GCP', 'Azure']

print(f"{'Provider':<15} | {'Latency (s)':<12}")
print("-" * 30)
for p in providers:
    latency = check_latency(p)
    print(f"{p:<15} | {latency:.4f}")