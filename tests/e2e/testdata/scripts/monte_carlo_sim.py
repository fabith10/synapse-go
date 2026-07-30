import numpy as np

def run_monte_carlo(n_iterations=10000, initial_rate=0.50, vol=0.2, drift=0.01):
    # Simulate geometric brownian motion for GPU spot rates
    # dS = S * (mu * dt + sigma * epsilon * sqrt(dt))
    dt = 1/365
    returns = np.random.normal(drift * dt, vol * np.sqrt(dt), n_iterations)
    simulated_rates = initial_rate * np.exp(np.cumsum(returns))
    
    # Calculate VaR (95%)
    var_95 = np.percentile(simulated_rates, 5)
    return simulated_rates, var_95

if __name__ == '__main__':
    rates, var = run_monte_carlo()
    print(f'Simulation Results (10,000 iterations):')
    print(f'Mean Rate: {np.mean(rates):.4f}')
    print(f'95% Value-at-Risk (VaR): {var:.4f}')