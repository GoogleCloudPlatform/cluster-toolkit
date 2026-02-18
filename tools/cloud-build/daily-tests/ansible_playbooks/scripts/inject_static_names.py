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

    net_count = 0

    if "deployment_groups" in data:
        for group in data["deployment_groups"]:
            if "modules" in group:
                for idx, mod in enumerate(group["modules"]):
                    src = mod.get("source", "")
                    mod_id = mod.get("id", f"unknown-mod-{idx}")

                    if src.startswith("modules/network/"):
                        if "settings" not in mod:
                            mod["settings"] = {}
                        
                        # First network matches Ansible firewall exact name, others get suffixes
                        if net_count == 0:
                            net_name = f"{static_test_name}-net"
                            sub_name = f"{static_test_name}-subnet"
                        else:
                            net_name = f"{static_test_name}-n{net_count}"
                            sub_name = f"{static_test_name}-n{net_count}-sub"

                        mod["settings"]["network_name"] = net_name
                        mod["settings"]["subnetwork_name"] = sub_name
                        net_count += 1
                        
                    elif src in ["modules/file-system/filestore", "modules/file-system/managed-lustre"]:
                        if "settings" not in mod:
                            mod["settings"] = {}
                        mod["settings"]["name"] = f"{static_test_name}-{mod_id}"

    with open(blueprint_path, "w") as f:
        yaml.dump(data, f, sort_keys=False)

if __name__ == "__main__":
    main()
