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

"""Unit tests for tools/validate_readme_snippets.py."""

import os
import sys
import tempfile
import unittest
import yaml

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
import validate_readme_snippets as vrs


class TestValidateReadmeSnippets(unittest.TestCase):

    def test_hcl_block_body_ignores_braces_in_comments_and_strings(self):
        hcl = (
            'variable "deprecated_var" {\n'
            '  description = "Legacy {brace} and } in string \\"}\\"\n"\n'
            "  # Comment with unmatched }\n"
            "  // Another comment with }\n"
            "  /* Block comment with } and { */\n"
            "  type = any\n"
            "  default = null\n"
            "  validation {\n"
            "    condition = var.deprecated_var == null # inline comment\n"
            '    error_message = "Deprecated"\n'
            "  }\n"
            "}\n"
        )
        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "variables.tf"), "w", encoding="utf-8") as f:
                f.write(hcl)
            inputs, deprecated, _ = vrs.get_tf_vars_and_outputs(tmpdir)
            self.assertEqual(inputs, set())
            self.assertEqual(deprecated, {"deprecated_var"})

    def test_extract_modules_from_yaml_group_list(self):
        snippet = """
- group: cloud-build
  modules:
  - id: cloud-build
    source: community/modules/container/cloud-build
    settings:
      project_id: $(vars.project_id)
"""
        data = yaml.safe_load(snippet)
        mods = vrs.extract_modules_from_yaml(data)
        self.assertEqual(len(mods), 1)
        self.assertEqual(mods[0]["id"], "cloud-build")

    def test_is_remote_source_prefixes(self):
        self.assertTrue(vrs.is_remote_source("git@github.com:org/repo.git"))
        self.assertTrue(vrs.is_remote_source("gcs::https://www.googleapis.com/storage/v1/b/m"))
        self.assertTrue(vrs.is_remote_source("github.com/GoogleCloudPlatform/pb"))
        self.assertFalse(vrs.is_remote_source("modules/network/vpc"))


if __name__ == "__main__":
    unittest.main()
