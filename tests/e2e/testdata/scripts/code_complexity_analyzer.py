import os
import ast

def calculate_cyclomatic_complexity(code):
    tree = ast.parse(code)
    complexity = 1
    for node in ast.walk(tree):
        if isinstance(node, (ast.If, ast.For, ast.While, ast.ExceptHandler, ast.With, ast.BoolOp)):
            complexity += 1
    return complexity

def run_analysis(directory='.'):
    print(f"Analyzing codebase in {directory}...")
    for root, _, files in os.walk(directory):
        for file in files:
            if file.endswith('.py'):
                path = os.path.join(root, file)
                with open(path, 'r') as f:
                    try:
                        complexity = calculate_cyclomatic_complexity(f.read())
                        if complexity > 10:
                            print(f"Hotspot detected: {path} (Complexity: {complexity})")
                    except Exception as e:
                        print(f"Could not parse {path}: {e}")

if __name__ == '__main__':
    run_analysis()