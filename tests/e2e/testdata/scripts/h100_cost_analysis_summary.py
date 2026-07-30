def print_summary():
    print("--- H100 Cost Analysis Summary ---")
    print("Host Environment: Apple Silicon (Unified Memory)")
    print("Local Compute Capability: Development only (no NVIDIA CUDA)")
    print("Recommendation: Use cloud-based H100 SXM instances via RunPod for heavy workloads.")
    print("--- Economics ---")
    print("Cloud Provider: RunPod (us-east)")
    print("Current Spot Price: $2.49/hr")
    print("Recommended Max Spot Bid: $2.99/hr")
    print("Projected Savings: 36.0% vs On-Demand ($3.89/hr)")
    print("--- Deployment Strategy ---")
    print("Local development is CPU-bound on this host; offload all training/inference to RunPod H100 spot instances.")

if __name__ == '__main__':
    print_summary()