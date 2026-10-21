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

"""Tests for the custom-image builder's startup script wrapper.

The wrapper is run with bash against fake `curl` and `gcloud` commands, so the
tests check what it would report to Packer without a VM. The module is loaded
by path, without Django, for the same reason as in test_tf_state_backend.py.
"""

import importlib.util
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

CLUSTER_MANAGER = Path(__file__).resolve().parents[1] / "cluster_manager"


def _load_image_startup():
    spec = importlib.util.spec_from_file_location(
        "ofe_cluster_manager_image_startup", CLUSTER_MANAGER / "image_startup.py"
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


image_startup = _load_image_startup()

# Fake metadata server. Its instance name differs from $HOSTNAME (an FQDN, as
# on some builder VM boots), so reporting under $HOSTNAME fails these tests.
FAKE_CURL = """#!/bin/bash
case "${@: -1}" in
	*/instance/name) echo packer-abc123 ;;
	*/instance/zone) echo projects/123456/zones/europe-west4-a ;;
	*) exit 22 ;;
esac
"""

FAKE_GCLOUD = """#!/bin/bash
echo "$@" >>"$GCLOUD_LOG"
"""


class WrapStartupScriptTests(unittest.TestCase):
    """The wrapped script runs unchanged and reports done or error to Packer
    under the instance name from the metadata server.
    """

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        tmp = Path(self.tmp.name)
        bin_dir = tmp / "bin"
        bin_dir.mkdir()
        for name, body in (("curl", FAKE_CURL), ("gcloud", FAKE_GCLOUD)):
            path = bin_dir / name
            path.write_text(body)
            path.chmod(0o755)
        self.gcloud_log = tmp / "gcloud.log"
        self.env = dict(
            os.environ,
            PATH=f"{bin_dir}:{os.environ['PATH']}",
            TMPDIR=str(tmp),
            GCLOUD_LOG=str(self.gcloud_log),
            HOSTNAME="packer-abc123.c.example-project.internal",
        )

    def _run(self, script):
        wrapper = Path(self.tmp.name) / "wrapper.sh"
        wrapper.write_text(image_startup.wrap_startup_script(script))
        result = subprocess.run(
            ["bash", str(wrapper)], env=self.env, capture_output=True,
            text=True, check=False,
        )
        calls = (self.gcloud_log.read_text().splitlines()
                 if self.gcloud_log.exists() else [])
        return result, calls

    def test_success_reports_done_under_instance_name(self):
        result, calls = self._run("#!/bin/bash\necho ran\n")
        self.assertEqual(result.returncode, 0)
        self.assertIn("ran", result.stdout)
        self.assertEqual(calls, [
            "compute instances add-metadata packer-abc123 --zone "
            "europe-west4-a --metadata startup-script-status=done"
        ])

    def test_failure_reports_error_and_keeps_exit_code(self):
        result, calls = self._run("#!/bin/bash\nexit 3\n")
        self.assertEqual(result.returncode, 3)
        self.assertEqual(calls, [
            "compute instances add-metadata packer-abc123 --zone "
            "europe-west4-a --metadata startup-script-status=error"
        ])

    def test_script_text_is_passed_through_unchanged(self):
        script = ('#!/bin/bash\nx=\'$HOME `id` $(id)\'\n'
                  'cat <<"EOF"\n$x \\n\nEOF\necho "$x"\n')
        result, _ = self._run(script)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout,
                         "$x \\n\n$HOME `id` $(id)\n")

    def test_temporary_copy_is_removed(self):
        self._run("#!/bin/bash\ntrue\n")
        self.assertEqual(
            sorted(p.name for p in Path(self.tmp.name).iterdir()),
            ["bin", "gcloud.log", "wrapper.sh"],
        )

    def test_rejects_script_containing_the_delimiter(self):
        with self.assertRaises(ValueError):
            image_startup.wrap_startup_script(
                "#!/bin/bash\nOFE_IMAGE_STARTUP_SCRIPT_EOF\n")


if __name__ == "__main__":
    unittest.main()
