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

import os
import glob
import sys
import re

from typing import List

class ModulePath:
    """Convenience class to get various path related information about a module"""

    def __init__(self, module_path: str):
        self.module_path = module_path

    def has_main(self) -> bool:
        return os.path.isfile(self.main())

    def has_vars(self) -> bool:
        return os.path.isfile(self.vars())

    def has_versions(self) -> bool:
        return os.path.isfile(self.versions())

    def has_outputs(self) -> bool:
        return os.path.isfile(self.outputs())

    def main(self) -> str:
        return self._filepath("main.tf")

    def vars(self) -> str:
        return self._filepath("variables.tf")

    def versions(self) -> str:
        return self._filepath("versions.tf")

    def outputs(self) -> str:
        return self._filepath("outputs.tf")

    def primary_file(self) -> str:
        """The file that should contain the labels definition"""
        return self.main() if self.has_main() else self.outputs()

    def name(self) -> str:
        return os.path.basename(self.module_path)

    def name_label(self) -> str:
        return self.name().lower()

    def _filepath(self, name: str) -> str:
        return os.path.join(self.module_path, name)
    
    def role(self) -> str:
        role = os.path.basename(os.path.dirname(self.module_path))
        return role if role not in ["..", ".", "/"] else "other"


def get_module_paths(root_dir:str="./") -> List[ModulePath]:
    community_modules_glob = os.path.join(
        root_dir, "community/modules", "*", "*")
    community_modules = glob.glob(community_modules_glob)
    core_modules_glob = os.path.join(root_dir, "modules", "*", "*")
    core_modules = glob.glob(core_modules_glob)
    return [ModulePath(path) for path in community_modules + core_modules]


def has_labels_variable(module_path: ModulePath) -> bool:
    if not module_path.has_vars():
        return False
    with open(module_path.vars(), encoding="utf-8") as var_file:
        return 'variable "labels"' in var_file.read()




def check_for_labels_local_block(module: ModulePath) -> bool:
    regex = re.compile(r'''locals {
  \# .*
  labels = merge\(var.labels, { ghpc_module = "([\w-]+)", ghpc_role = "([\w-]+)" }\)
}''', re.MULTILINE)
    
    file_to_check = module.primary_file()
    with open(file_to_check, encoding="utf-8") as file:
        match = regex.search(file.read())
    if not match:
        print(f"""{module.primary_file()} does not define a local.labels with required fields. Add:
locals {{
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, {{ ghpc_module = \"{module.name_label()}\", ghpc_role = \"{module.role()}\" }})
}}
""")
        return False
    ghpc_module, ghpc_role, = match.groups()
    if module.name_label() != ghpc_module:
        print(f"{module.primary_file()} defines label {ghpc_module=} but module name is `{module.name_label()}`")
        return False

    if module.role() != ghpc_role:
        print(f"{module.primary_file()} defines label {ghpc_role=} but module role is `{module.role()}`")
        return False

    return True

def check_label_usage(module_path: ModulePath) -> bool:
    passed = True
    with open(module_path.primary_file(), encoding="utf-8") as file:
        if file.read().count('var.labels') > 1:  # there will be one reference in local block
            print("{} contains references to var.labels instead of local.labels".format(
                module_path.primary_file()))
            passed = False

    if module_path.primary_file() != module_path.outputs() and module_path.has_outputs():
        with open(module_path.outputs(), encoding="utf-8") as outputs:
            if outputs.read().count('var.labels') > 0:
                print("{} contains references to var.labels instead of local.labels".format(
                    module_path.outputs()))
                passed = False
    return passed


def check_provider_meta(module_path: ModulePath) -> bool:
    """This is enforcing that the provider meta name matches the module name"""
    if not module_path.has_versions():
        return True
    version_file_path = module_path.versions()
    with open(version_file_path, encoding="utf-8") as version_file:
        content = version_file.read()
        if content.count('provider_meta "google') == content.count(
                'blueprints/terraform/hpc-toolkit:{}'.format(module_path.name())):
            return True
        print('{} provider meta does not match module name'.format(
            version_file_path))
        return False


def check_module(module_path: ModulePath) -> bool:
    passed = check_provider_meta(module_path)
    if not has_labels_variable(module_path):
        return passed
    if not check_for_labels_local_block(module_path):
        return False
    return check_label_usage(module_path) and passed

def main() -> bool:
    """Performs some basic checks on all modules.

    This function will check that every module with a var.labels is merging in a
    `ghpc_module` label. If missing, the locals block will be added. It will
    check that all other references to labels points to the merged local.labels.
    
    This function also checks that the provider meta name matches the module
    name
    
    Returns: True if checks passed
    """

    passed = [check_module(m) for m in get_module_paths()]
    return all(passed)


if __name__ == "__main__":
    if not main():
        sys.exit(1)
