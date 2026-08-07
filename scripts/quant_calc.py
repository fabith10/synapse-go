import math

# Perform calculation of combinations C(10,5)
result = math.factorial(10) // (math.factorial(5) * math.factorial(5))

# Since file writing inside Docker failed, this script serves as the artifact documentation of the calculation logic.
print(f'Calculation C(10,5) performed: {result}')