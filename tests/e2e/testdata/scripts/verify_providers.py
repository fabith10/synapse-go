import time
import requests

PROVIDERS = {
    "RunPod": "https://api.runpod.io",
    "Lambda": "https://cloud.lambdalabs.com",
    "AWS": "https://aws.amazon.com",
    "GCP": "https://cloud.google.com"
}

def verify_latency():
    results = {}
    print(f"{'Provider':<15} | {'Latency (ms)':<15}")
    print("-"*30)
    for name, url in PROVIDERS.items():
        try:
            start = time.perf_counter()
            requests.head(url, timeout=5)
            end = time.perf_counter()
            latency = (end - start) * 1000
            results[name] = latency
            print(f"{name:<15} | {latency:.2f} ms")
        except Exception as e:
            print(f"{name:<15} | Error: {str(e)}")
    return results

if __name__ == '__main__':
    verify_latency()
