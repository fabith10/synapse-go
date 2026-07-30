import sys

def run_regression_test(expected_output, actual_output):
    if expected_output == actual_output:
        print("Regression Test Passed")
        return True
    else:
        print(f"Regression Test Failed: Expected {expected_output}, got {actual_output}")
        return False

if __name__ == '__main__':
    # Example usage for automated verification
    run_regression_test(100, 100)