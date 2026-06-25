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

import yaml
import glob
import os
import subprocess

def get_git_modified():
    out = subprocess.check_output(['git', 'diff', '--name-only']).decode('utf-8')
    return [line.strip() for line in out.splitlines() if line.strip()]

modified_files = get_git_modified()
yaml_files = glob.glob('tools/cloud-build/daily-tests/builds/*.yaml')
yaml_files += glob.glob('tools/cloud-build/daily-tests/builds/*.yml')

required_secrets = ['TRIAGE_GCS_BUCKET', 'TRIAGE_PROJECT_NUMBER', 'TRIAGE_INVOKER_SA', 'TRIAGE_CLOUD_RUN_URL']
missing_files = []
invalid_yaml = []
missing_secrets = []

for f in yaml_files:
    try:
        with open(f) as file:
            content = file.read()
            # check if yaml is valid
            try:
                data = yaml.safe_load(content)
            except Exception as e:
                invalid_yaml.append(f)
                continue
            
            if 'ansible-playbook' in content:
                # it should have been modified
                if f not in modified_files:
                    missing_files.append(f)
                
                # Check for secrets
                has_all_secrets = all(sec in content for sec in required_secrets)
                if not has_all_secrets:
                    missing_secrets.append(f)
    except Exception as e:
        print(f"Error reading {f}: {e}")

print(f"Invalid YAML: {invalid_yaml}")
print(f"Files with ansible-playbook but not modified: {missing_files}")
print(f"Files with ansible-playbook but missing secrets: {missing_secrets}")
