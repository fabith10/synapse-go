import numpy as np

def run_monte_carlo():
    # Parameters for GPU spot rate volatility simulation
    # S0: current price, mu: drift, sigma: volatility, T: time horizon
    S0 = 0.50  # Base rate per hour
    mu = 0.02
    sigma = 0.15
    T = 1.0
    dt = 1/24  # Daily steps
    iterations = 10000

    # Simulate price paths using Geometric Brownian Motion
    # S_t = S_0 * exp((mu - 0.5 * sigma^2) * T + sigma * sqrt(T) * Z)
    Z = np.random.normal(0, 1, iterations)
    S_T = S0 * np.exp((mu - 0.5 * sigma**2) * T + sigma * np.sqrt(T) * Z)
    
    # Calculate returns
    returns = S_T - S0
    
    # Calculate 95% Value-at-Risk (VaR)
    var_95 = np.percentile(returns, 5)
    
    print(f"Simulation Results (10,000 iterations):")
    print(f"Initial Rate: ${S0:.4f}/hr")
    print("Expected Final Mean Rate: ${:.4f}/hr".format(np.mean(S_T)))
    print("95% Value-at-Risk (VaR): ${:.4f}/hr".format(abs(var_95)))

if __name__ == '__main__':
    run_monte_carlo()