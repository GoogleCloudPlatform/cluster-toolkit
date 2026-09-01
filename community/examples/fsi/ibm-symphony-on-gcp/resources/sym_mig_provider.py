# Copyright 2026 Google LLC
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

import json
import re
import sys
from typing import Any, Dict, List

# The structure of the template to be reflected in JSON
def make_template(
    id: str,
    max_num: int,
    mtype: str,
    ncpus: str,
    nram: str,
    ncores: str,
    zone: str,
    mig: str,
) -> Dict[str, Any]:
    """
    Creates a dictionary representing a single machine template.
    """
    temp: Dict[str, Any] = {
        'templateId': id,
        'maxNumber': max_num,
        'attributes': {
            'type': ['String', mtype],
            'ncpus': ['Numeric', ncpus],
            'ncores': ['Numeric', ncores], 
            'nram': ['Numeric', nram],
        },
        'gcp_zone': zone,
        'gcp_instance_group': mig,
    }
    return temp

# --- Main script execution ---

# Check if command-line arguments are provided
if len(sys.argv) < 2:
    print("Error: No template arguments provided.", file=sys.stderr)
    print("Usage: python script.py 'id1,max,type,cpus,cores,ram,zone,mig__id2,max,...'", file=sys.stderr)
    sys.exit(1)

# Required info is passed, separated by a double underscore for each template
template_args: List[str] = re.split(r"__", sys.argv[1])

# Start the JSON structure
template_output: Dict[str, List[Dict[str, Any]]] = {
   'templates': []
}

# Loop over templates
for arg in template_args:
    # Parse the settings for each template
    # The original code had `ncores` but it was unused. We assign it to `_`.
    try:
        (id_str, max_str, mtype, ncpus_str, ncores_str, nram_str, zone, mig) = arg.split(',')
    except ValueError:
        print(f"Error: Invalid format for argument string: '{arg}'", file=sys.stderr)
        print("Expected 8 comma-separated values.", file=sys.stderr)
        continue # Skip this malformed argument

    # Append the template to the list, converting numeric strings to integers
    template_output['templates'].append(
        make_template(
            id=id_str,
            max_num=int(max_str),
            mtype=mtype,
            ncpus=ncpus_str,
            ncores=ncores_str,
            nram=nram_str,
            zone=zone,
            mig=mig,
        )
    )

print(json.dumps(template_output, indent=2))