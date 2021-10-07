# Copyright 2021 Google LLC
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

import json
import subprocess
from subprocess import PIPE
from jinja2 import Template
import os
import pathlib
import argparse

## Parse Arguments
parser = argparse.ArgumentParser()
parser.add_argument("--template", type=str, required=True)
parser.add_argument("--outfile", type=str, required=True)
parser.add_argument("--deployment_name", type=str, required=True)
args = parser.parse_args()

## Get cluster information
gcloud_cmd = ["gcloud", "compute", "instances", "list",
              "--filter", f"labels.ghpc_deployment={args.deployment_name}",
              "--format",
              "json(name,labels.ghpc_role,networkInterfaces[0].networkIP)"]
gcloud_process = subprocess.run(gcloud_cmd, stdout=PIPE, stderr=PIPE)
if gcloud_process.returncode != 0:
  print(f"Failed to return omnia VM instance list: {gcloud_process.stderr}")
  exit(1)
json_omnia_vms = gcloud_process.stdout
omnia_vms = json.loads(json_omnia_vms)

## Extract cluster information
compute_vms = []
for vm in omnia_vms:
  if vm["labels"]["ghpc_role"] == "omnia-manager":
    omnia_manager = vm["name"]
  else:
    compute_vms.append(vm["name"])

## Write as inventory file
with open(args.template) as inventory_tmpl:
  template = Template(inventory_tmpl.read())
with open(args.outfile, "w") as inventory_file:
  inventory_file.write(
    template.render(omnia_manager=omnia_manager, compute_vms=compute_vms)
  )
