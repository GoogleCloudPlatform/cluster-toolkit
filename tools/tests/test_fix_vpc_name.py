#!/usr/bin/env python3
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

"""Unit tests for tools/fix_vpc_name.py."""

import os
import shutil
import subprocess
import sys
import tempfile
import unittest
import yaml


class TestFixVpcName(unittest.TestCase):
    """Tests for VPC network name freezing in tools/fix_vpc_name.py."""

    def setUp(self):
        self.test_dir = tempfile.mkdtemp()
        self.repo_root = os.path.abspath(
            os.path.join(os.path.dirname(__file__), "../..")
        )
        self.script = os.path.join(self.repo_root, "tools", "fix_vpc_name.py")

    def tearDown(self):
        shutil.rmtree(self.test_dir)

    def test_netapp_volumes_blueprint_uses_primary_zero_suffix(self):
        """NetApp storage pool blueprint VPC should be named {prefix}$(vars.test_name)-0."""
        src = os.path.join(self.repo_root, "examples", "netapp-volumes.yaml")
        dst = os.path.join(self.test_dir, "netapp-volumes.yaml")
        shutil.copy2(src, dst)

        res = subprocess.run(
            [sys.executable, self.script, dst, "daily-"],
            capture_output=True,
            text=True,
            check=True,
        )
        self.assertIn("Updated module 'network' to: daily-$(vars.test_name)-0", res.stdout)

        with open(dst, "r", encoding="utf-8") as f:
            updated = yaml.safe_load(f)

        vpc_modules = [
            m
            for g in updated["deployment_groups"]
            for m in g.get("modules", [])
            if m.get("source") == "modules/network/vpc"
        ]
        self.assertEqual(len(vpc_modules), 1)
        self.assertEqual(
            vpc_modules[0]["settings"]["network_name"],
            "daily-$(vars.test_name)-0",
        )

    def test_netapp_volumes_blueprint_with_empty_prefix(self):
        """Empty prefix should produce $(vars.test_name)-0."""
        src = os.path.join(self.repo_root, "examples", "netapp-volumes.yaml")
        dst = os.path.join(self.test_dir, "netapp-volumes.yaml")
        shutil.copy2(src, dst)

        subprocess.run(
            [sys.executable, self.script, dst, ""],
            capture_output=True,
            text=True,
            check=True,
        )

        with open(dst, "r", encoding="utf-8") as f:
            updated = yaml.safe_load(f)

        vpc_module = next(
            m
            for g in updated["deployment_groups"]
            for m in g.get("modules", [])
            if m.get("id") == "network"
        )
        self.assertEqual(
            vpc_module["settings"]["network_name"],
            "$(vars.test_name)-0",
        )


if __name__ == "__main__":
    unittest.main()
