import json

def calculate_wfh_costs():
    subtotal = 1300.0
    tax = 110.50000000000001
    shipping = 75.0
    total_pp = 1485.5
    team_total = 7427.5
    buffer_pp = 114.5
    print(f"Subtotal per person: ${subtotal:.2f}")
    print(f"Sales Tax (8.5%): ${tax:.2f}")
    print(f"Shipping: ${shipping:.2f}")
    print(f"Total Per Person: ${total_pp:.2f}")
    print(f"Team Total (5 people): ${team_total:.2f}")
    print(f"Buffer Per Person: ${buffer_pp:.2f}")

if __name__ == '__main__':
    calculate_wfh_costs()
