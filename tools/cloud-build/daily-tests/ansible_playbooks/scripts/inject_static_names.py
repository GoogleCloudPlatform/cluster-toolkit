#!/usr/bin/env python3
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
import yaml
import sys

def main():
    if len(sys.argv) < 3:
        print("Usage: inject_static_names.py <blueprint_yaml_path> <static_test_name>")
        sys.exit(1)

    blueprint_path = sys.argv[1]
    static_test_name = sys.argv[2]

    with open(blueprint_path, "r") as f:
        data = yaml.safe_load(f)

    if not data:
        return

    net_count = 0
    storage_count = 0

    for group in data.get("deployment_groups") or []:
        for mod in group.get("modules") or []:
            if not isinstance(mod, dict):
                continue
            
            src = mod.get("source", "")

            if src in ["modules/network/vpc", "modules/network/pre-existing-vpc"]:
                if "settings" not in mod:
                    mod["settings"] = {}
                
                # First network matches Ansible firewall exact name, others get suffixes
                if net_count == 0:
                    net_name = f"{static_test_name}-net"[:63]
                    sub_name = f"{static_test_name}-subnet"[:63]
                else:
                    net_name = f"{static_test_name}-n{net_count}"[:63]
                    sub_name = f"{static_test_name}-n{net_count}-sub"[:63]

                mod["settings"]["network_name"] = net_name
                mod["settings"]["subnetwork_name"] = sub_name
                net_count += 1
                
            elif src == "modules/network/multivpc":
                if "settings" not in mod:
                    mod["settings"] = {}
                
                # multivpc creates multiple networks, pass prefix
                # To prevent collisions, we suffix it with current count
                if net_count == 0:
                    net_name_prefix = f"{static_test_name}-net"[:63]
                else:
                    net_name_prefix = f"{static_test_name}-n{net_count}"[:63]
                    
                mod["settings"]["network_name_prefix"] = net_name_prefix
                net_count += mod.get("settings", {}).get("network_count", 4)
                
            elif src == "modules/network/gpu-rdma-vpc":
                if "settings" not in mod:
                    mod["settings"] = {}
                
                if net_count == 0:
                    net_name = f"{static_test_name}-net"[:63]
                else:
                    net_name = f"{static_test_name}-n{net_count}"[:63]
                    
                mod["settings"]["network_name"] = net_name
                net_count += 1
                
            elif src in ["modules/file-system/filestore", "modules/file-system/managed-lustre"]:
                if "settings" not in mod:
                    mod["settings"] = {}

                mod_id = mod.get("id")
                if mod_id:
                    sanitized_id = mod_id.replace("_", "-")
                    name = f"{static_test_name}-{sanitized_id}"[:63]
                else:
                    name = f"{static_test_name}-storage-{storage_count}"[:63]
                    storage_count += 1
                mod["settings"]["name"] = name

    with open(blueprint_path, "w") as f:
        yaml.dump(data, f, sort_keys=False)

if __name__ == "__main__":
    main()
