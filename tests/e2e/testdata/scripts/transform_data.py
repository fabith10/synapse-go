import csv
import json
import os
import zipfile
import subprocess

def run_status():
    result = subprocess.run(['git', 'status'], capture_output=True, text=True)
    print('Git Status:', result.stdout)

def transform_csv():
    csv_data = 'name,role\nAlice,Admin\nBob,Dev\n'
    output_path = 'reports/users.json'
    os.makedirs(os.path.dirname(output_path), exist_ok=True)
    reader = csv.DictReader(csv_data.splitlines())
    with open(output_path, 'w') as f:
        json.dump(list(reader), f, indent=4)
    return output_path

def archive_data(source_path):
    zip_path = 'reports/user_data.zip'
    with zipfile.ZipFile(zip_path, 'w', zipfile.ZIP_DEFLATED) as zipf:
        zipf.write(source_path, arcname=os.path.basename(source_path))
    print(f'Successfully archived to {zip_path}')

if __name__ == '__main__':
    run_status()
    json_path = transform_csv()
    archive_data(json_path)