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

CATEGORICAL_TAGS = frozenset([
    "slurm5", "slurm6", "tpu", 
])
OS_TAGS = frozenset([
    "rocky8"
])

VALID_TAGS = CATEGORICAL_TAGS | OS_TAGS

class TestIntegrationTestsMeta(unittest.TestCase):
    def check_tags(self, tags: list[str]) -> None:
        tags = set(tags)
        self.assertEqual(tags - VALID_TAGS, set(), msg="Invalid tags")

        # TODO: check that all tests have at least one categorical tag
        # TODO: inspect referenced blueprint to extract used modules

    def check_metadata(self, path: str) -> None:
        with open(path) as yf:
            y = yaml.safe_load(yf)

        # NOTE: don't use assertIn to avoid printing the whole yaml
        if not "timeout" in y:
            self.fail("All integration tests must have a `timeout`")

        if not "tags" in y: 
            self.fail("All integration tests must have `tags`")

        self.check_tags(y["tags"])

    def test_integration_tests_meta(self) -> None:
        its = glob.glob("tools/cloud-build/daily-tests/builds/*.yaml")
        self.assertNotEqual(len(its), 0, msg="No integration tests found")
        for it in its:
            with self.subTest(os.path.basename(it)):
                self.check_metadata(it)

    def test_sanity(self) -> None:
        self.assertEqual(CATEGORICAL_TAGS & OS_TAGS, set(), msg="tag types intersect")

if __name__ == "__main__":
    unittest.main()
