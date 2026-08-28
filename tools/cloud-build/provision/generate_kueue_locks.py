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

BUILDS_DIR = "tools/cloud-build/daily-tests/builds/"
CONFIGS_DIR = "tools/cloud-build/daily-tests/blueprints/test-infra-kueue/configs/"

def extract_locks_from_builds():
    """Extract all requested test-locks from all yaml build files."""
    locks = set()
    for filepath in glob.glob(os.path.join(BUILDS_DIR, "*.yaml")):
        test_name = os.path.basename(filepath).replace('.yaml', '')
        with open(filepath, 'r') as f:
            content = f.read()
            
        # We consider it a Kueue test if it uses Kueue (either the script or explicitly requests a lock)
        # Some old tests might have the while-loop manually but still request test-locks
        found_locks = re.findall(r'test-locks/([a-zA-Z0-9_-]+)', content)
        if "submit_and_monitor_kueue_job.sh" in content or found_locks:
            if found_locks:
                for lock in found_locks:
                    locks.add(lock)
            else:
                locks.add(test_name)
    return sorted(list(locks))

def update_dummy_device_plugin(new_locks):
    yaml = YAML()
    yaml.preserve_quotes = True
    yaml.width = 4096
    filepath = os.path.join(CONFIGS_DIR, "dummy-device-plugin.yaml")
    
    with open(filepath, 'r') as f:
        data = yaml.load(f)
        
    args = data['spec']['template']['spec']['containers'][0]['args']
    
    existing_locks = set()
    for arg in args:
        match = re.search(r'--device=\{"name": "([^"]+)"', arg)
        if match:
            existing_locks.add(match.group(1))
            
    insert_idx = len(args)
    for i, arg in enumerate(args):
        if str(arg).startswith('--listen='):
            insert_idx = i
            break
            
    added = 0
    for lock in new_locks:
        if lock not in existing_locks:
            args.insert(insert_idx, f'--device={{"name": "{lock}", "groups": [{{"count": 1000, "paths": [{{"path": "/dev/null"}}]}}]}}')
            insert_idx += 1
            added += 1
            
    if added > 0:
        with open(filepath, 'w') as f:
            yaml.dump(data, f)
    return added

def update_kueue_setup(new_locks):
    yaml = YAML()
    yaml.preserve_quotes = True
    yaml.width = 4096
    filepath = os.path.join(CONFIGS_DIR, "kueue-setup.yaml")
    
    with open(filepath, 'r') as f:
        docs = list(yaml.load_all(f))
        
    # Find the ClusterQueue doc
    cq_doc = None
    for doc in docs:
        if doc and doc.get('kind') == 'ClusterQueue':
            cq_doc = doc
            break
            
    if not cq_doc:
        print("Could not find ClusterQueue in kueue-setup.yaml!")
        sys.exit(1)
        
    existing_locks = set()
    test_lock_groups = []
    
    for group in cq_doc['spec']['resourceGroups']:
        is_test_lock_group = False
        for res in group['coveredResources']:
            if res.startswith('test-locks/'):
                existing_locks.add(res.replace('test-locks/', ''))
                is_test_lock_group = True
        if is_test_lock_group:
            test_lock_groups.append(group)
            
    added = 0
    missing_locks = [lock for lock in new_locks if lock not in existing_locks]
    
    if not missing_locks:
        return 0
        
    if test_lock_groups:
        last_group = test_lock_groups[-1]
    else:
        last_group = {
            "coveredResources": [],
            "flavors": [{
                "name": "test-lock-flavor",
                "resources": []
            }]
        }
        cq_doc['spec']['resourceGroups'].append(last_group)
        test_lock_groups.append(last_group)
    

    
    for lock in missing_locks:
        if len(last_group['coveredResources']) >= 50:
            # Create a new flavor document and a new group
            flavor_idx = len(test_lock_groups) + 1
            new_flavor_name = f"test-lock-flavor-{flavor_idx}"
            
            # Insert the new ResourceFlavor document right before the ClusterQueue
            cq_index = docs.index(cq_doc)
            docs.insert(cq_index, {
                "apiVersion": "kueue.x-k8s.io/v1beta1",
                "kind": "ResourceFlavor",
                "metadata": {"name": new_flavor_name}
            })
            
            last_group = {
                "coveredResources": [],
                "flavors": [{
                    "name": new_flavor_name,
                    "resources": []
                }]
            }
            cq_doc['spec']['resourceGroups'].append(last_group)
            test_lock_groups.append(last_group)
            
        last_group['coveredResources'].append(f"test-locks/{lock}")
        last_group['flavors'][0]['resources'].append({
            "name": f"test-locks/{lock}",
            "nominalQuota": 1
        })
        added += 1

    if added > 0:
        with open(filepath, 'w') as f:
            yaml.dump_all(docs, f)
            
    return added

if __name__ == "__main__":
    locks = extract_locks_from_builds()
    d_added = update_dummy_device_plugin(locks)
    k_added = update_kueue_setup(locks)
    print(f"Added {d_added} new locks to dummy-device-plugin.yaml")
    print(f"Added {k_added} new locks to kueue-setup.yaml")
    if d_added > 0 or k_added > 0:
        sys.exit(1) # Fail so pre-commit forces user to commit the changes
    sys.exit(0)
