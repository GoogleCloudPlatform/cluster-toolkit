# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import glob
import os
import re
import sys
from ruamel.yaml import YAML
from ruamel.yaml.scalarstring import DoubleQuotedScalarString as dq

BUILDS_DIR = "tools/cloud-build/daily-tests/builds/"

def auto_fix_resources(filepath):
    yaml = YAML()
    yaml.preserve_quotes = True
    yaml.width = 4096

    with open(filepath, 'r') as f:
        content = f.read()
        
    if "submit_and_monitor_kueue_job.sh" not in content:
        return False
        
    # Check if we have test-locks at all
    if not re.search(r'test-locks/', content):
        print(f"ERROR: {filepath} uses Kueue but does not request any test-locks in its job specification!")
        print("Please manually add at least 'test-locks/<test-name>: 1' to the resources block.")
        return True # Failed validation
        
    try:
        with open(filepath, 'r') as f:
            data = yaml.load(f)
    except Exception as e:
        print(f"Failed to parse outer yaml for {filepath}: {e}")
        return True
        
    modified = False
    
    if not data or not isinstance(data, dict):
        return False
        
    for step in data.get('steps', []):
        if 'args' in step:
            for i, arg in enumerate(step['args']):
                if isinstance(arg, str) and 'cat <<' in arg and 'job.yaml' in arg:
                    lines = arg.split('\n')
                    start_idx = -1
                    end_idx = -1
                    eof_marker = None
                    for j, line in enumerate(lines):
                        match = re.search(r'cat <<\s*[\'"]?([A-Z_]+)[\'"]?\s*>.*job\.yaml', line)
                        if match:
                            start_idx = j + 1
                            eof_marker = match.group(1)
                            break
                    if eof_marker:
                        for j in range(start_idx, len(lines)):
                            if lines[j].strip() == eof_marker:
                                end_idx = j
                                break
                    
                    if start_idx != -1 and end_idx != -1:
                        job_yaml_str = '\n'.join(lines[start_idx:end_idx])
                        
                        job_yaml = YAML()
                        job_yaml.preserve_quotes = True
                        try:
                            job_data = job_yaml.load(job_yaml_str)
                            
                            containers = job_data.get('spec', {}).get('template', {}).get('spec', {}).get('containers', [])
                            for container in containers:
                                if not isinstance(container, dict):
                                    continue
                                resources = container.get('resources') or {}
                                if not isinstance(resources, dict):
                                    resources = {}
                                requests = resources.get('requests') or {}
                                if not isinstance(requests, dict):
                                    requests = {}
                                limits = resources.get('limits') or {}
                                if not isinstance(limits, dict):
                                    limits = {}

                                lock_name = None
                                for k in list(requests.keys()) + list(limits.keys()):
                                    if k.startswith('test-locks/'):
                                        lock_name = k
                                        break
                                        
                                if lock_name:
                                    needs_fix = False
                                    if requests.get('cpu') != '200m' or str(requests.get('memory')) != '2Gi':
                                        needs_fix = True
                                    
                                    if str(limits.get('cpu')) != '1' or str(limits.get('memory')) != '2Gi' or limits.get(lock_name) != 1:
                                        needs_fix = True
                                        
                                    if needs_fix:
                                        print(f"Auto-fixing and standardizing resources for {filepath}...")
                                        if 'requests' not in resources or not isinstance(resources['requests'], dict):
                                            resources['requests'] = {}
                                        resources['requests']['cpu'] = '200m'
                                        resources['requests']['memory'] = dq('2Gi')
                                        
                                        if 'limits' not in resources or not isinstance(resources['limits'], dict):
                                            resources['limits'] = {}
                                        resources['limits']['cpu'] = 1
                                        resources['limits']['memory'] = dq('2Gi')
                                        resources['limits'][lock_name] = 1
                                        
                                        container['resources'] = resources
                                        
                                        from io import StringIO
                                        out = StringIO()
                                        job_yaml.dump(job_data, out)
                                        new_job_yaml_str = out.getvalue()
                                        
                                        # Preserve the original indentation of the heredoc
                                        # Find the first non-empty line to calculate correct indentation
                                        indent = 0
                                        for line in lines[start_idx:end_idx]:
                                            if line.strip():
                                                indent = len(line) - len(line.lstrip())
                                                break
                                                
                                        indent_str = ' ' * indent
                                        indented_job_yaml = ''.join(
                                            indent_str + line if line.strip() else line
                                            for line in new_job_yaml_str.splitlines(keepends=True)
                                        )
                                        
                                        new_arg = '\n'.join(lines[:start_idx]) + '\n' + indented_job_yaml + '\n'.join(lines[end_idx:])
                                        step['args'][i] = new_arg
                                        modified = True
                        except Exception as e:
                            # It might fail if the bash script has $VARS inside the yaml that breaks the YAML parser
                            print(f"Warning: Failed to parse embedded job.yaml in {filepath}: {e}")
                            
    if modified:
        with open(filepath, 'w') as f:
            yaml.dump(data, f)
        return True # Failed validation (was modified)
    
    return False # Passed

if __name__ == "__main__":
    failed = False
    for filepath in glob.glob(os.path.join(BUILDS_DIR, "*.yaml")):
        if auto_fix_resources(filepath):
            failed = True
            
    if failed:
        print("\nValidation failed: One or more Kueue tests were missing test-locks, or their resource blocks were auto-fixed.")
        sys.exit(1)
    sys.exit(0)
