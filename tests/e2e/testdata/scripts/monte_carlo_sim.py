import numpy as np

def run_monte_carlo(n_iterations=10000, initial_price=2.0, volatility=0.2, drift=0.05, days=30):
    # Simulate price paths using Geometric Brownian Motion
    dt = 1/24  # hourly steps
    steps = days * 24
    
    # Generate random shocks
    shocks = np.random.normal(0, np.sqrt(dt), (n_iterations, steps))
    
    # Calculate price evolution
    price_paths = initial_price * np.exp(np.cumsum((drift - 0.5 * volatility**2) * dt + volatility * shocks, axis=1))
    
    # Get final prices
    final_prices = price_paths[:, -1]
    
    # Calculate 95% VaR (Value at Risk)
    # Defined as the loss at the 5th percentile relative to initial price
    returns = final_prices - initial_price
    var_95 = np.percentile(returns, 5)
    
    print(f'Simulation Parameters: Initial Price=${initial_price}, Vol={volatility}, Drift={drift}')
    print(f'10,000 Iteration Results:')
    print(f'Expected Mean Final Price: ${np.mean(final_prices):.4f}')
    print(f'95% Value-at-Risk (VaR): ${abs(var_95):.4f}')

if __name__ == '__main__':
    run_monte_carlo()