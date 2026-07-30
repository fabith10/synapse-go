import os

def analyze_codebase(root_dir='.'):
    stats = {'total_lines': 0, 'files': 0}
    for root, dirs, files in os.walk(root_dir):
        for file in files:
            if file.endswith(('.py', '.js', '.ts', '.java', '.cpp')):
                stats['files'] += 1
                with open(os.path.join(root, file), 'r', encoding='utf-8', errors='ignore') as f:
                    stats['total_lines'] += len(f.readlines())
    return stats

if __name__ == '__main__':
    results = analyze_codebase()
    print(f"Codebase Analysis: {results['files']} files found, {results['total_lines']} total lines.")