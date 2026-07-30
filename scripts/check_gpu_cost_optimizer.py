import os

if os.path.exists('scripts/check_gpu_cost_optimizer.py') and os.path.exists('scripts/gpu_cost_optimizer.py'):
    print('Found existing scripts')
else:
    print('No existing script found')