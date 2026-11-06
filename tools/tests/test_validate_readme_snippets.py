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

import glob
import os
import re
import sys
import tempfile
import unittest
import unittest.mock
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

    def test_is_checked_repo_file_and_path_filter_ignored_targets(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            tracked_dir = os.path.join(tmpdir, "tracked_dir")
            ignored_dir = os.path.join(tmpdir, "ignored_dir")
            os.makedirs(tracked_dir)
            os.makedirs(ignored_dir)
            tracked = os.path.join(tracked_dir, "tracked.md")
            ignored = os.path.join(ignored_dir, "ignored.md")
            for p in (tracked, ignored):
                with open(p, "w", encoding="utf-8") as f:
                    f.write("# test\n")
            non_ignored = {os.path.normpath(tracked)}
            non_ignored_dirs = {os.path.normpath(tmpdir), os.path.normpath(tracked_dir)}
            self.assertTrue(vrs._is_checked_repo_file(tracked, non_ignored))
            self.assertFalse(vrs._is_checked_repo_file(ignored, non_ignored))
            self.assertTrue(vrs._is_checked_repo_file(ignored, None))
            self.assertTrue(vrs._is_checked_repo_path(tracked_dir, non_ignored, non_ignored_dirs))
            self.assertFalse(vrs._is_checked_repo_path(ignored_dir, non_ignored, non_ignored_dirs))

    def test_get_tf_vars_and_outputs_skips_ignored_tf_files(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            tracked_tf = os.path.join(tmpdir, "variables.tf")
            ignored_tf = os.path.join(tmpdir, "ignored.tf")
            with open(tracked_tf, "w", encoding="utf-8") as f:
                f.write('variable "project_id" {\n  type = string\n}\n')
            with open(ignored_tf, "w", encoding="utf-8") as f:
                f.write('variable "local_scratch_var" {\n  type = string\n}\n')
            inputs, _, _ = vrs.get_tf_vars_and_outputs(tmpdir, {os.path.normpath(tracked_tf)})
            self.assertEqual(inputs, {"project_id"})

    def test_find_markdown_files_under_filters_non_ignored_set(self):
        readme = os.path.join(vrs.REPO_ROOT, "cmd", "README.md")
        outside = os.path.join(vrs.REPO_ROOT, "README.md")
        found = vrs._find_markdown_files_under(("cmd",), {readme, outside})
        self.assertEqual(found, [readme])

    def test_validate_markdown_links_checks_llms_files(self):
        llms_txt = os.path.join(vrs.REPO_ROOT, "llms.txt")
        llms_full = os.path.join(vrs.REPO_ROOT, "llms-full.txt")
        errs = vrs.validate_markdown_links_and_cli(
            vrs._get_non_ignored_repo_files() | {llms_txt, llms_full}
        )
        self.assertEqual(errs, [])

    def test_validate_yaml_block_nested_object_keys(self):
        snippet = """
- id: gke_cluster
  source: modules/scheduler/gke-cluster
  settings:
    cluster_autoscaling:
      enabled: true
      limits:
        - autoprovisioning_machine_type: g2-standard-48
          invalid_limit_field: 10
- id: workload_mgr
  source: modules/management/kubectl-apply
  settings:
    kueue:
      install: true
      wait: true
      wait_for_pods_ready:
        bad_timeout_key: 30m
    jobset:
      install: true
      wait: true
"""
        tf_cache = {}
        errs = vrs.validate_yaml_block(snippet, "test.md", tf_cache)
        self.assertEqual(len(errs), 5)
        self.assertIn("cluster_autoscaling.enabled", errs[0])
        self.assertIn("cluster_autoscaling.limits.invalid_limit_field", errs[1])
        self.assertIn("kueue.wait", errs[2])
        self.assertIn("kueue.wait_for_pods_ready.bad_timeout_key", errs[3])
        self.assertIn("jobset.wait", errs[4])

    def test_strip_yaml_elision_lines(self):
        raw = (
            "    vars:\n"
            "      project_id: test\n"
            "      # ...\n"
            "    ---\n"
            "    deployment_groups: []\n"
        )
        cleaned = vrs._strip_yaml_elision_lines(raw)
        docs = list(yaml.safe_load_all(cleaned))
        self.assertEqual(len(docs), 2)
        self.assertEqual(docs[0]["vars"]["project_id"], "test")

    def test_uncommented_yaml_ellipsis_is_rejected(self):
        raw = (
            "  ...\n"
            "  - id: script\n"
            "    source: modules/scripts/startup-script\n"
        )
        errs = vrs.validate_yaml_block(raw, "test.md", {})
        self.assertEqual(len(errs), 1)
        self.assertIn("invalid YAML syntax in snippet", errs[0])

    def test_backticked_and_leading_slash_links_rejected(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            md_path = os.path.join(tmpdir, "README.md")
            with open(md_path, "w", encoding="utf-8") as f:
                f.write(
                    "See [`missing`](missing.md) and [abs](/modules/foo.md).\n"
                )
            with unittest.mock.patch.object(vrs, "REPO_ROOT", tmpdir), unittest.mock.patch.object(
                vrs, "_find_markdown_files_under", return_value=[]
            ):
                errs = vrs.validate_markdown_links_and_cli({os.path.normpath(md_path)})
            self.assertEqual(len(errs), 2)
            self.assertTrue(any("missing.md" in e for e in errs))
            self.assertTrue(any("/modules/foo.md" in e for e in errs))

    def test_validate_yaml_block_required_kms_key_settings(self):
        snippet = """
- id: kms_key
  source: community/modules/security/kms-key
  settings:
    project_id: $(vars.project_id)
    location: $(vars.region)
    key_ring_name: my-keyring
    key_name: my-key
"""
        errs = vrs.validate_yaml_block(snippet, "kms-key-iam/README.md", {})
        self.assertEqual(len(errs), 1)
        self.assertIn("missing required setting 'deletion_policy'", errs[0])

    def test_nested_object_keys_match_variables_tf(self):
        """NESTED_OBJECT_KEYS must mirror the object({...}) attributes declared in variables.tf."""

        def object_attrs(type_expr: str) -> dict[str, str]:
            start = type_expr.index("object({") + len("object({")
            body = vrs._hcl_block_body(type_expr, start)[:-1]
            attrs: dict[str, str] = {}
            depth, cur = 0, None
            for line in body.splitlines():
                m = re.match(r"\s*([A-Za-z_]\w*)\s*=\s*(.*)$", line)
                if depth == 0 and m:
                    cur = m.group(1)
                    attrs[cur] = m.group(2)
                elif cur:
                    attrs[cur] += "\n" + line
                depth += sum(line.count(c) for c in "({[") - sum(line.count(c) for c in ")}]")
            return attrs

        for (module, dotted), allowed in vrs.NESTED_OBJECT_KEYS.items():
            with self.subTest(module=module, key=dotted):
                root, *rest = dotted.split(".")
                text = ""
                for tf in sorted(glob.glob(os.path.join(vrs.REPO_ROOT, module, "*.tf"))):
                    with open(tf, encoding="utf-8") as f:
                        text += vrs._strip_hcl_comments(f.read())
                m = re.search(rf'variable\s+"{root}"\s*\{{', text)
                self.assertIsNotNone(m, f"variable {root!r} not found in {module}")
                body = vrs._hcl_block_body(text, m.end())
                type_attr = re.search(r"^\s*type\s*=", body, re.MULTILINE)
                self.assertIsNotNone(type_attr, f"no type for {root!r} in {module}")
                attrs = object_attrs(body[type_attr.start():])
                for part in rest:
                    attrs = object_attrs(attrs[part])
                self.assertEqual(allowed, set(attrs))

    def test_required_module_settings_have_no_default(self):
        """REQUIRED_MODULE_SETTINGS entries must be variables without a default in variables.tf."""
        for module, required in vrs.REQUIRED_MODULE_SETTINGS.items():
            text = ""
            for tf in sorted(glob.glob(os.path.join(vrs.REPO_ROOT, module, "*.tf"))):
                with open(tf, encoding="utf-8") as f:
                    text += vrs._strip_hcl_comments(f.read())
            for name in sorted(required):
                with self.subTest(module=module, var=name):
                    m = re.search(rf'variable\s+"{name}"\s*\{{', text)
                    self.assertIsNotNone(m, f"variable {name!r} not found in {module}")
                    body = vrs._hcl_block_body(text, m.end())
                    self.assertIsNone(
                        re.search(r"^\s*default\s*=", body, re.MULTILINE),
                        f"{module}: {name!r} now has a default; drop it from REQUIRED_MODULE_SETTINGS",
                    )


if __name__ == "__main__":
    unittest.main()
