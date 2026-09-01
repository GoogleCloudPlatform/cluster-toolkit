#!/bin/bash
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

python3   -c "
import json
import sys


# The structure of the template to be reflected in JSON
def make_template(id,max,mtype,ncpus,nram,zone,mig):

    temp =  {
            'templateId': id,
            'maxNumber': max,
            'attributes': {
                'type': ['String', mtype],
                'ncpus': ['Numeric', ncpus],
                'nram': ['Numeric', nram]
            },
            'gcp_zone': zone,
            'gcp_instance_group': mig
        }
    return(temp)

# Required info is passed semicolon delimited for each template
template_args = sys.argv[1].split(';')

# Start the JSON structure
template_output = {
   'templates': []
}

# Loop over templates
for arg in template_args:
  
  #parse the settings for each template
  (id,max,mtype,ncpus,nram,zone,mig) = arg.split(',')

  # Append the template to the stub
  template_output['templates'].append(make_template(id,max,mtype,ncpus,nram,zone,mig))

print(json.dumps(template_output,indent=2))
" \
"$@" \
> conf/providers/gcpgceinst/gcpgceinstprov_templates.json