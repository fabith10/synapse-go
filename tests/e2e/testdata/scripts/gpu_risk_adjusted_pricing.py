def calculate_risk_adjusted_cost(base_spot, eviction_prob, restart_overhead):
    """
    Calculates Risk_Adjusted_Cost = Base_Spot_Price * (1.0 + Eviction_Probability * Restart_Overhead_Factor)
    """
    return base_spot * (1.0 + (eviction_prob * restart_overhead))

def main():
    base_spot = 2.49
    eviction_prob = 0.05
    restart_overhead = 2.0
    forward_rate = 2.5522

    risk_adjusted = calculate_risk_adjusted_cost(base_spot, eviction_prob, restart_overhead)

    print(f'--- Infrastructure Compute Cost Analysis ---')
    print(f'Host: Apple Silicon Metal (Unified Memory)')
    print(f'Base H100 Spot Rate: ${base_spot}/hr')
    print(f'Assumed Eviction Risk: {eviction_prob*100}%')
    print(f'Restart Overhead Factor: {restart_overhead}x')
    print(f'Risk-Adjusted Spot Cost: ${risk_adjusted:.4f}/hr')
    print(f'30d Forward Contract Fixed Rate: ${forward_rate}/hr')

    if risk_adjusted > forward_rate:
        print('Recommendation: Forward contract is cost-effective under current eviction risk assumptions.')
    else:
        print('Recommendation: Spot market remains cheaper on a risk-adjusted basis.')

if __name__ == '__main__':
    main()