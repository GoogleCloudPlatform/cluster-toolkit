#!/usr/bin/env python3
# Copyright 2024 Google LLC
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

import yaml # pip install pyyaml

def delete_module(bp, mod_id: str):
  for g in bp["deployment_groups"]:
    for im, m in enumerate(g["modules"]):
      if m["id"] == mod_id:
        del g["modules"][im]
        return
  raise RuntimeError(f"Module {mod_id} not found")

def update_bp(bp, mod_id: str):
  delete_module(bp, mod_id)
   # remove module from "use"
  for g in bp["deployment_groups"]:
    for m in g["modules"]:
      if mod_id in m.get("use", []):
        m["use"].remove(mod_id)
  # TODO:
  # - move all settings of removed module into some "/dev/null" to prevent
  #   "variable not used" validation errors
  # - handle references to removed module (e.g. raise error if referenced)

def main():
  import argparse
  parser = argparse.ArgumentParser()
  parser.add_argument("--blueprint", required=True, type=str)
  parser.add_argument("--module", required=True, type=str, help="Module ID to remove")
  args = parser.parse_args()

  with open(args.blueprint) as yf:
    bp = yaml.safe_load(yf)
  update_bp(bp, mod_id=args.module)
  with open(args.blueprint, "w") as yf:
    yaml.dump(bp, yf)

if __name__ == "__main__":
  main()
