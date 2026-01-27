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

import argparse
import sys
import yaml
from typing import Dict, Any

def get_mock_vars(blueprint_file: str, predefined_vars_str: str = "", deployment_file: str = "") -> str:
    """
    Parses the blueprint YAML and optional deployment YAML, extracts required variables,
    and generates a comma-separated string of mock variables.
    """
    
    def load_yaml(path: str) -> Dict[str, Any]:
        if not path:
            return {}
        try:
            with open(path, 'r') as f:
                data = yaml.safe_load(f) or {}
                return data.get('vars', {})
        except Exception as e:
            print(f"Error reading {path}: {e}", file=sys.stderr)
            return {}

    # 1. Load variables from files
    blueprint_vars = load_yaml(blueprint_file)
    deployment_vars = load_yaml(deployment_file)
    
    # 2. Parse predefined variables
    predefined_vars = {}
    if predefined_vars_str:
        for item in predefined_vars_str.split(','):
            if '=' in item:
                k, v = item.split('=', 1)
                predefined_vars[k] = v.strip()

    # 3. Merge variables (Deployment overrides Blueprint)
    # logic: we want to verify everything in blueprint + deployment, 
    # but actual values come from predefined > deployment > blueprint (defaults)
    all_vars = {**blueprint_vars, **deployment_vars}
    
    final_mock_vars = {}

    for var_name, var_value in all_vars.items():
        # Priority 1: Predefined
        if var_name in predefined_vars:
            final_mock_vars[var_name] = predefined_vars[var_name]
            continue

        # Priority 2: Heuristic defaults for missing/empty values
        # (Your original logic for determining if a mock value is needed)
        needs_mock = (
            var_value is None or 
            var_value == "" or 
            (isinstance(var_value, str) and var_value.isupper() and 
             any(x in var_value for x in ["SIZE", "COUNT", "NAME", "ID", "REGION", "ZONE", "PROJECT", "BUCKET"]))
        )

        if needs_mock:
            if any(x in var_name for x in ["size", "count", "num_"]):
                final_mock_vars[var_name] = "1"
            elif "enable" in var_name:
                final_mock_vars[var_name] = "false"
            else:
                final_mock_vars[var_name] = "test-value"

    # Priority 3: Add any remaining predefined vars not already covered
    for k, v in predefined_vars.items():
        if k not in final_mock_vars:
             final_mock_vars[k] = v

    return ",".join(f"{k}={v}" for k, v in final_mock_vars.items())

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Generate mock variables for validation.")
    parser.add_argument("blueprint_file", help="Path to the blueprint YAML file")
    parser.add_argument("predefined_vars", nargs="?", default="", help="Comma-separated string of predefined variables")
    parser.add_argument("deployment_file", nargs="?", default="", help="Path to the deployment YAML file")
    
    args = parser.parse_args()
    
    print(get_mock_vars(args.blueprint_file, args.predefined_vars, args.deployment_file))
