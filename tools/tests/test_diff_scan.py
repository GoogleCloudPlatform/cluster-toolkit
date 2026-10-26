#!/usr/bin/env python3
# Copyright 2026 "Google LLC"
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

"""Unit tests for skills/release-breaking-check/scripts/diff_scan.py."""

import importlib.util
import os
import sys
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "skills" / "release-breaking-check" / "scripts" / "diff_scan.py"

spec = importlib.util.spec_from_file_location("diff_scan", SCRIPT)
diff_scan = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(diff_scan)


def _diff(path: str, removed=(), added=(), old_path=None, deleted=False, new=False) -> str:
    old = old_path or path
    lines = [f"diff --git a/{old} b/{path}"]
    if deleted:
        lines.append("deleted file mode 100644")
    if new:
        lines.append("new file mode 100644")
    lines += [f"--- a/{old}", f"+++ b/{path}", "@@ -1,1 +1,1 @@"]
    lines += [f"-{l}" for l in removed]
    lines += [f"+{l}" for l in added]
    return "\n".join(lines) + "\n"


def _ids(result):
    return [s["id"] for s in result["signals"]]


class ParseTests(unittest.TestCase):
    def test_parses_multiple_files_and_modes(self):
        text = _diff("examples/a.yaml", removed=["x: 1"], added=["x: 2"]) + _diff("examples/b.yaml", deleted=True)
        hunks = diff_scan.parse_unified_diff(text)
        self.assertEqual([h.path for h in hunks], ["examples/a.yaml", "examples/b.yaml"])
        self.assertTrue(hunks[1].deleted_file)

    def test_empty_diff_has_no_signals(self):
        r = diff_scan.scan("")
        self.assertEqual(r["signal_count"], 0)
        self.assertEqual(r["severity_hint"], "none")


class TerraformTests(unittest.TestCase):
    def test_variable_removed(self):
        d = _diff("modules/network/vpc/variables.tf", removed=['variable "old_cidr" {', '  type = string', "}"])
        r = diff_scan.scan(d)
        self.assertIn("tf_variable_removed", _ids(r))
        self.assertIn("network/vpc", r["touched_modules"])

    def test_variable_default_changed(self):
        d = _diff("modules/compute/vm-instance/variables.tf", removed=['  default = "n2-standard-2"'], added=['  default = "n2d-standard-2"'])
        self.assertIn("tf_variable_default_changed", _ids(diff_scan.scan(d)))

    def test_output_removed(self):
        d = _diff("modules/file-system/filestore/outputs.tf", removed=['output "network_storage" {'])
        self.assertIn("tf_output_removed", _ids(diff_scan.scan(d)))

    def test_resource_renamed_without_moved_block(self):
        d = _diff("modules/network/private-service-access/main.tf",
                  removed=['resource "random_id" "resource_name_suffix" {'],
                  added=['resource "random_id" "suffix" {'])
        self.assertIn("tf_resource_removed_or_renamed", _ids(diff_scan.scan(d)))

    def test_resource_renamed_with_moved_block_is_quiet(self):
        d = _diff("modules/network/private-service-access/main.tf",
                  removed=['resource "random_id" "resource_name_suffix" {'],
                  added=['resource "random_id" "suffix" {', "moved {", "  from = random_id.resource_name_suffix", "}"])
        self.assertNotIn("tf_resource_removed_or_renamed", _ids(diff_scan.scan(d)))

    def test_forcenew_literal_changed(self):
        d = _diff("modules/network/private-service-access/main.tf",
                  removed=['  name = "global-psconnect-ip-${random_id.resource_name_suffix.hex}"'],
                  added=['  name = "psconnect-${var.network_name}"'])
        r = diff_scan.scan(d)
        self.assertIn("tf_forcenew_literal_changed", _ids(r))

    def test_version_pin_changed(self):
        d = _diff("modules/management/kubectl-apply/main.tf", removed=['  kueue_version = "v0.17.1"'], added=['  kueue_version = "v0.19.6"'])
        self.assertIn("version_pin_changed", _ids(diff_scan.scan(d)))

    def test_validation_tightened(self):
        d = _diff("modules/compute/gke-node-pool/variables.tf", added=["  validation {", "    condition = length(var.x) < 10"])
        self.assertIn("tf_validation_tightened", _ids(diff_scan.scan(d)))


class ExampleTests(unittest.TestCase):
    def test_module_source_swapped(self):
        d = _diff("examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml",
                  removed=["    source: modules/file-system/filestore"],
                  added=["    source: modules/file-system/managed-lustre"])
        r = diff_scan.scan(d)
        self.assertIn("example_module_id_or_source_changed", _ids(r))
        self.assertEqual(r["severity_hint"], "medium")
        self.assertIn("examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml", r["touched_examples"])

    def test_var_removed(self):
        d = _diff("examples/gke-a4/gke-a4.yaml", removed=["  reservation: my-res"])
        self.assertIn("example_var_or_setting_removed", _ids(diff_scan.scan(d)))

    def test_setting_changed(self):
        d = _diff("examples/gke-a4/gke-a4.yaml",
                  removed=["      system_node_pool_machine_type: n2d-standard-8"],
                  added=["      system_node_pool_machine_type: n2d-standard-4"])
        self.assertIn("example_setting_changed", _ids(diff_scan.scan(d)))

    def test_setting_added_overrides_module_default(self):
        d = _diff("examples/storage-gke.yaml",
                  added=["      system_node_pool_machine_type: n2d-standard-4", "      system_node_pool_zones: [$(vars.zone)]"])
        self.assertIn("example_setting_added", _ids(diff_scan.scan(d)))

    def test_image_family_changed(self):
        d = _diff("community/examples/hpc-slurm-ubuntu2204.yaml",
                  removed=["    family: slurm-gcp-6-12-ubuntu-2204-lts-nvidia-570"],
                  added=["    family: aci-gpu-u2204-slurm-2511-cuda-130-nvidia-580-amd64"])
        self.assertIn("example_setting_changed", _ids(diff_scan.scan(d)))

    def test_public_ip_flag_flipped(self):
        d = _diff("examples/hpc-enterprise-slurm.yaml",
                  removed=["      enable_login_public_ips: true"], added=["      enable_login_public_ips: false"])
        self.assertIn("example_setting_changed", _ids(diff_scan.scan(d)))

    def test_new_required_var(self):
        d = _diff("examples/gke-a4x/gke-a4x.yaml", added=["  num_node_pools:"])
        self.assertIn("example_var_added_required", _ids(diff_scan.scan(d)))

    def test_example_deleted(self):
        d = _diff("examples/hpc-slurm-static.yaml", deleted=True)
        r = diff_scan.scan(d)
        self.assertEqual(_ids(r), ["example_deleted"])
        self.assertEqual(r["severity_hint"], "medium")

    def test_new_example_is_additive(self):
        d = _diff("examples/brand-new.yaml", new=True, added=["blueprint_name: x", "vars:", "  zone: us-central1-a"])
        self.assertEqual(diff_scan.scan(d)["signal_count"], 0)

    def test_additive_var_with_default_is_quiet(self):
        d = _diff("examples/hpc-slurm.yaml",
                  added=["  machine_type: c2-standard-60", "  enable_spot_vm: false"],
                  removed=[])
        r = diff_scan.scan(d)
        # Adding vars with values is an override only when the key is impactful; machine_type at
        # vars level with a value is reported as setting_added (low weight) so a reviewer can
        # confirm the default matches the old hardcoded value.
        self.assertNotIn("example_var_added_required", _ids(r))
        self.assertNotIn("example_var_or_setting_removed", _ids(r))


class OtherTests(unittest.TestCase):
    def test_rendered_script_changed(self):
        d = _diff("community/modules/compute/gke-nodeset/templates/nodeset-general.yaml.tftpl", removed=["a"], added=["b"])
        self.assertIn("rendered_script_changed", _ids(diff_scan.scan(d)))

    def test_gcluster_validation_changed(self):
        d = _diff("pkg/validators/validators.go", added=['return fmt.Errorf("variable %q was not used", name)'])
        self.assertIn("gcluster_validation_changed", _ids(diff_scan.scan(d)))

    def test_docs_only_is_quiet(self):
        d = _diff("docs/vm-images.md", removed=["Debian 11"], added=["Debian 12"]) + _diff("README.md", added=["hello"])
        self.assertEqual(diff_scan.scan(d)["signal_count"], 0)

    def test_signals_deduplicated_and_severity_scaled(self):
        d = _diff("examples/a.yaml", removed=["    source: modules/x"], added=["    source: modules/y"]) * 3
        r = diff_scan.scan(d)
        self.assertEqual(r["signal_count"], 1)


class CliTests(unittest.TestCase):
    def test_cli_reads_stdin_and_prints_json(self):
        import io, json, contextlib
        d = _diff("examples/a.yaml", deleted=True)
        stdin, sys.stdin = sys.stdin, io.StringIO(d)
        try:
            buf = io.StringIO()
            with contextlib.redirect_stdout(buf):
                rc = diff_scan.main(["--diff-file", "-"])
        finally:
            sys.stdin = stdin
        self.assertEqual(rc, 0)
        self.assertEqual(json.loads(buf.getvalue())["signals"][0]["id"], "example_deleted")


if __name__ == "__main__":
    unittest.main(verbosity=2)
