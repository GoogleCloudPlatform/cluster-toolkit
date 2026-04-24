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

    vpc_count = 0
    # Iterate through all deployment groups and modules
    if data and 'deployment_groups' in data:
        for group in data['deployment_groups']:
            for module in group.get('modules', []):
                # Identify VPC modules by their source path
                if 'modules/network/vpc' in module.get('source', ''):
                    if 'settings' not in module:
                        module['settings'] = {}
                    # Set the consecutive name using the $(vars.test_name) variable
                    module['settings']['network_name'] = f"{prefix}$(vars.test_name)-{vpc_count}"
                    print(f"Updated module '{module.get('id')}' to: {prefix}$(vars.test_name)-{vpc_count}")
                    vpc_count += 1

    with open(blueprint_path, 'w') as f:
        yaml.dump(data, f, sort_keys=False)

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <blueprint_path> <prefix>", file=sys.stderr)
        sys.exit(1)
    modify_vpcs(sys.argv[1], sys.argv[2])
