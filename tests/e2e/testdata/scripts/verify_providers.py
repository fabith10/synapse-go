import time
import subprocess

providers = ['runpod', 'lambda', 'aws', 'gcp', 'azure']

def check_latency(provider):
    # Simulate latency check
    return f"Provider {provider} latency: 45ms"

for p in providers:
    print(check_latency(p))
