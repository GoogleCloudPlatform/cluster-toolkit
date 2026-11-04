# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Ansible integration checks, run by make test-agentgateway outside pytest hooks."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[1]
PLAYBOOK = ROOT / "tools/cloud-build/daily-tests/ansible_playbooks/agentgateway-integration-test.yml"


class OptionalProviderPlaybookTests(unittest.TestCase):
    def test_provider_arguments(self):
        tasks = yaml.safe_load(PLAYBOOK.read_text())[0]["tasks"]
        block = next(t["block"] for t in tasks if "block" in t)
        task = next(t for t in block if t["name"] == "Enable configured external-provider routes")
        for providers in ([], ["openai"], ["anthropic"], ["openai", "anthropic"]):
            with self.subTest(providers=providers), tempfile.TemporaryDirectory() as directory:
                workspace = Path(directory)
                arguments = workspace / "arguments.json"
                stub = workspace / "gcluster"
                stub.write_text("#!" + sys.executable + "\nimport json, sys\n"
                                + "from pathlib import Path\n"
                                + "Path('arguments.json').write_text(json.dumps(sys.argv[1:]))\n")
                stub.chmod(0o755)
                play = [{"hosts": "localhost", "connection": "local", "gather_facts": False,
                         "vars": {"workspace": directory, "blueprint_yaml": "blueprint.yaml",
                                  "project": "test-project", "deployment_name": "agw-test",
                                  "state_bucket": "", "deployment_vars_str": "",
                                  "ansible_python_interpreter": sys.executable,
                                  "external_provider_tests": [
                                      {"provider": name, "secret_name": name + "-secret"}
                                      for name in providers]}, "tasks": [task]}]
                test_playbook = workspace / "test.yml"
                test_playbook.write_text(yaml.safe_dump(play))
                result = subprocess.run(
                    [sys.executable, "-m", "ansible.cli.playbook", "-i", "localhost,", str(test_playbook)],
                    env=dict(os.environ, ANSIBLE_NOCOLOR="1"),
                    capture_output=True, text=True, timeout=60)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                if not providers:
                    self.assertFalse(arguments.exists(), "No providers must skip regeneration")
                    continue
                argv = json.loads(arguments.read_text())
                variables = [argv[i + 1] for i, arg in enumerate(argv) if arg == "--vars"]
                self.assertEqual(variables, ["project_id=test-project", "deployment_name=agw-test"]
                                 + [name + "_secret_name=" + name + "-secret" for name in providers])
