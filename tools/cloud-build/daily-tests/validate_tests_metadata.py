# Copyright 2024 "Google LLC"
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

import unittest
import glob
import os
import yaml # pip install pyyaml
import re
from typing import Optional
import itertools

CATEGORICAL_TAGS = frozenset([
    "batch", 
    "crd", 
    "gke",
    "htcondor", 
    "monitoring", 
    "ofe", 
    "packer", 
    "pbspro",
    "slurm5", 
    "slurm6", 
    "spack",
    "tpu", 
    "vm",
    "dockerfile", 
])

def module_tag(src: str) -> Optional[str]:
    """
    Returns tag for a specific module source.
    Remote sources are not supported (None).
    Ex: "modules/network/vpc" -> "m.vpc"
    """
    if not src.startswith(("modules/", "community/modules/")):
        return None
    return f"m.{os.path.basename(src)}"


MODULE_TAGS = frozenset(
    filter(None, 
           map(module_tag, 
               glob.glob("modules/*/*") + glob.glob("community/modules/*/*"))),
)


ALL_TAGS = CATEGORICAL_TAGS | MODULE_TAGS

BUILDS_DIR="tools/cloud-build/daily-tests/builds"

def read_yaml(path: str) -> dict:
    with open(path) as yf:
        return yaml.safe_load(yf)

def get_blueprint(build_path: str) -> Optional[str]:
    """
    Extracts the blueprint path by inspecting build (and test) files
    """
    SPECIAL_CASES = {
        f"{BUILDS_DIR}/e2e.yaml": "tools/cloud-build/daily-tests/blueprints/e2e.yaml",
        f"{BUILDS_DIR}/ofe-deployment.yaml": None,
        f"{BUILDS_DIR}/chrome-remote-desktop.yaml": "tools/cloud-build/daily-tests/blueprints/crd-default.yaml",
        f"{BUILDS_DIR}/chrome-remote-desktop-ubuntu.yaml": "tools/cloud-build/daily-tests/blueprints/crd-ubuntu.yaml",
        f"{BUILDS_DIR}/gcluster-dockerfile.yaml": "tools/cloud-build/daily-tests/blueprints/e2e.yaml",
        f"{BUILDS_DIR}/slurm-gcp-v6-reconfig-size.yaml": "tools/python-integration-tests/blueprints/slurm-simple-reconfig.yaml",
        f"{BUILDS_DIR}/slurm-gcp-v6-simple-job-completion.yaml": "tools/python-integration-tests/blueprints/slurm-simple.yaml",
        f"{BUILDS_DIR}/slurm-gcp-v6-topology.yaml": "tools/python-integration-tests/blueprints/topology-test.yaml",
    }
    if build_path in SPECIAL_CASES:
        return SPECIAL_CASES[build_path]

    with open(build_path) as yf:
        data = yf.read()
    m = re.search(r'cloud-build/daily-tests/tests/.*\.yml', data)
    if not m:
        raise ValueError(f"Couldn't find test file reference in {build_path}\nConsider adding it to validate_tests_metadata.py:SPECIAL_CASES")
    
    tst_path = "tools/" + m.group()
    tst_yaml = read_yaml(tst_path)
    if "blueprint_yaml" not in tst_yaml:
        raise ValueError(f"{tst_path} doesn't specify a `blueprint_yaml`")
    bp_path = tst_yaml["blueprint_yaml"]
    if bp_path.startswith("{{ workspace }}/"):
        bp_path = bp_path[len("{{ workspace }}/"):]
    return bp_path

def get_modules_tags(build_path) -> set[str]:
    bp_path = get_blueprint(build_path)
    if bp_path is None:
        return set()
    bp = read_yaml(bp_path)
    tags = set()
    for group in bp["deployment_groups"]:
        for module in group["modules"]:
            tag = module_tag(module["source"])
            if tag:
                tags.add(tag)
    return tags

class TestIntegrationTestsMeta(unittest.TestCase):
    def check_tags(self, build_path: str) -> None:
        y = read_yaml(build_path)
        if not "tags" in y: 
            self.fail("All integration tests must have `tags`")
        tags = set(y["tags"]) # specified tags

        # Common checks
        self.assertEqual(tags - ALL_TAGS, set(), msg="Invalid tags")
        self.assertLessEqual(len(tags), 64, msg="Too many tags")

        # Module tags check
        declared_mod_tags = set(filter(lambda t: t.startswith("m."), tags))
        required_mod_tags = get_modules_tags(build_path)
        # do "missing entries" comparison explicitly to get copy-pastable error message for addition
        missing_mod_tags = required_mod_tags - declared_mod_tags
        if missing_mod_tags:
            hint = "\n- ".join([""] + sorted(missing_mod_tags))
            self.fail(msg=f"Some used modules aren't declared\nHINT: add following tags to {build_path}: {hint}")
        self.assertEquals(declared_mod_tags, required_mod_tags)

        self.assertNotEqual(tags & CATEGORICAL_TAGS, set(), msg=f"No categorical tags, pick/add one: {CATEGORICAL_TAGS}")

    def check_metadata(self, path: str) -> None:
        y = read_yaml(path)

        # NOTE: don't use assertIn to avoid printing the whole yaml
        if not "timeout" in y:
            self.fail("All integration tests must have a `timeout`")

        self.check_tags(path)

    def test_integration_tests_meta(self) -> None:
        its = glob.glob(f"{BUILDS_DIR}/*.yaml")
        self.assertNotEqual(len(its), 0, msg="No integration tests found")
        for it in its:
            with self.subTest(os.path.basename(it)):
                self.check_metadata(it)

    def test_sanity_intersections(self) -> None:
        for (a, b) in itertools.combinations([CATEGORICAL_TAGS, MODULE_TAGS], 2):
            self.assertEqual(a & b, set(), msg="tag types intersect")

    def test_sanity_tag_limits(self) -> None:
        # see https://cloud.google.com/build/docs/view-build-results#filter_build_results_by_using_tags
        for tag in ALL_TAGS:
            with self.subTest(tag):
                self.assertLessEqual(len(tag), 128)
                self.assertRegex(tag, r'^[a-zA-Z0-9_\-\.]+$')
                
if __name__ == "__main__":
    unittest.main()
    
