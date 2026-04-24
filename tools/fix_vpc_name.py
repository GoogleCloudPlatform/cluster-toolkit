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
import sys

def modify_vpcs(blueprint_path, prefix):
    with open(blueprint_path, 'r') as f:
        data = yaml.safe_load(f)

    # Find network used by filestore
    primary_network_id = None
    if data and 'deployment_groups' in data:
        for group in data['deployment_groups']:
            for module in group.get('modules', []):
                if module.get('source') == 'modules/file-system/filestore':
                    uses = module.get('use', [])
                    for u in uses:
                        for g2 in data['deployment_groups']:
                            for m2 in g2.get('modules', []):
                                if m2.get('id') == u and m2.get('source') == 'modules/network/vpc':
                                    primary_network_id = u
                                    break
                            if primary_network_id:
                                break
                        if primary_network_id:
                            break
                if primary_network_id:
                    break
            if primary_network_id:
                break

    print(f"Identified primary network (used by filestore): {primary_network_id}")

    vpc_count = 1 # Start other VPCs from 1
    # Iterate through all deployment groups and modules
    if data and 'deployment_groups' in data:
        for group in data['deployment_groups']:
            for module in group.get('modules', []):
                # Identify VPC modules by their source path
                if 'modules/network/vpc' in module.get('source', ''):
                    if 'settings' not in module:
                        module['settings'] = {}
                    
                    mod_id = module.get('id')
                    if mod_id == primary_network_id:
                        name = f"{prefix}$(vars.test_name)-0"
                    else:
                        name = f"{prefix}$(vars.test_name)-{vpc_count}"
                        vpc_count += 1
                        
                    module['settings']['network_name'] = name
                    print(f"Updated module '{mod_id}' to: {name}")

    with open(blueprint_path, 'w') as f:
        yaml.dump(data, f, sort_keys=False)

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <blueprint_path> <prefix>", file=sys.stderr)
        sys.exit(1)
    modify_vpcs(sys.argv[1], sys.argv[2])
