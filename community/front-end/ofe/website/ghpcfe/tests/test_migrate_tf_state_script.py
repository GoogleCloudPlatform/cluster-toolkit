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

"""End-to-end tests for script/migrate_tf_state_to_gcs.sh.

The script moves each resource's local Terraform state into the state bucket
and keeps the original as terraform.tfstate.premigration. That copy is the
operator's only record of the state as it was before migration, so it must
survive later operations on the resource. For clusters, filesystems and image
builders the Terraform working directory is a Cluster Toolkit deployment
group, which `gcluster create -w` moves into .ghpc/previous_deployment_groups/
on every reconfigure, deleting the set it moved there the time before. The
copy therefore lives in the resource's run-time directory instead.

Terraform runs as each resource's credential stored in the database, through
`manage.py ofe_terraform_as_credential`, not with a key file: the server
deletes the key files older releases kept in the run-time directories each
time it starts, so none exists here.

Each test runs the real script in a synthetic TKFE tree in a temporary
directory: a copy of the script, the real cluster_manager/utils.py, a
configuration.yaml, and stubs for `terraform` and for the application's
Python interpreter, which stands in for the management commands. Nothing
here needs Django, network access or cloud credentials.
"""

import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

OFE_DIR = Path(__file__).resolve().parents[3]
CLUSTER_MANAGER = OFE_DIR / "website" / "ghpcfe" / "cluster_manager"
MIGRATE_SCRIPT = OFE_DIR / "script" / "migrate_tf_state_to_gcs.sh"

NAMESPACE = "20261015-0a1b2c3d"
BUCKET = "test-bucket"
STATE = '{"version": 4, "resources": [{"type": "google_compute_network"}]}'
BACKUP_NAME = "terraform.tfstate.premigration"

# A stand-in for `terraform`. Every call is appended to $TF_CALL_LOG, after
# the resource it runs as ($AS_CREDENTIAL, set only by the stand-in for
# ofe_terraform_as_credential) in brackets.
# `init -migrate-state` exits with $INIT_RC; on success it empties the local
# state, as the real one does after copying it to the backend.
# `plan -detailed-exitcode` exits with $PLAN_RC.
TERRAFORM_STUB = """#!/bin/bash
echo "[${AS_CREDENTIAL:-no credential}] $*" >>"${TF_CALL_LOG}"
dir=${1#-chdir=}
case "$*" in
*"init -migrate-state"*)
	[[ ${INIT_RC:-0} -eq 0 ]] || exit "${INIT_RC}"
	echo '{"version": 4, "resources": []}' >"${dir}/terraform.tfstate"
	;;
*"plan -detailed-exitcode"*) exit "${PLAN_RC:-0}" ;;
esac
"""

# A stand-in for the application's interpreter. It answers the namespace
# command; for ofe_terraform_as_credential <kind> <id> -- <args> it logs
# "<kind> <id> <working directory>" to $CMD_CALL_LOG, refuses with exit code
# 1 when "<kind> <id>" is $REFUSE_CREDENTIAL, as the command does for a
# credential it cannot use, and otherwise runs `terraform <args>` as that
# resource; anything else runs with this test's interpreter.
OFE_PYTHON_STUB = """#!/bin/bash
if [[ $1 == manage.py ]]; then
	case $2 in
	tfstate_namespace)
		echo "{namespace}"
		exit 0
		;;
	ofe_terraform_as_credential)
		echo "$3 $4 $PWD" >>"${{CMD_CALL_LOG}}"
		[[ $5 == -- ]] || exit 64
		if [[ "$3 $4" == "${{REFUSE_CREDENTIAL:-}}" ]]; then
			echo "$3 $4: its stored credential cannot be used" >&2
			exit 1
		fi
		export AS_CREDENTIAL="$3 $4"
		shift 5
		exec terraform "$@"
		;;
	esac
	exit 64
fi
exec "{python}" "$@"
"""

# A Toolkit-generated deployment group already carries its backend block.
GENERATED_MAIN_TF = 'terraform {\n  backend "gcs" {\n    bucket = "test-bucket"\n  }\n}\n'


class MigrateScriptBackupTests(unittest.TestCase):
    """Where the pre-migration copy of each resource's state is kept."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)

        self.ofe = root / "ofe"
        (self.ofe / "script").mkdir(parents=True)
        shutil.copy2(MIGRATE_SCRIPT, self.ofe / "script" / MIGRATE_SCRIPT.name)
        cluster_manager = self.ofe / "website" / "ghpcfe" / "cluster_manager"
        cluster_manager.mkdir(parents=True)
        (self.ofe / "website" / "ghpcfe" / "__init__.py").write_text("")
        (cluster_manager / "__init__.py").write_text("")
        shutil.copy2(CLUSTER_MANAGER / "utils.py", cluster_manager / "utils.py")
        (self.ofe / "configuration.yaml").write_text(
            f'config:\n  server:\n    terraform_state_bucket: "{BUCKET}"\n'
        )

        bin_dir = root / "bin"
        bin_dir.mkdir()
        self._write_executable(bin_dir / "terraform", TERRAFORM_STUB)
        ofe_python = bin_dir / "ofe-python"
        self._write_executable(
            ofe_python,
            OFE_PYTHON_STUB.format(namespace=NAMESPACE, python=sys.executable),
        )
        # The script also calls plain python3 for JSON checks; make sure it
        # resolves to an interpreter that exists in this environment.
        self._write_executable(
            bin_dir / "python3", f'#!/bin/bash\nexec "{sys.executable}" "$@"\n'
        )

        self.tf_call_log = root / "terraform-calls.log"
        self.tf_call_log.write_text("")
        self.cmd_call_log = root / "command-calls.log"
        self.cmd_call_log.write_text("")
        self.env = {
            "PATH": f"{bin_dir}:/usr/bin:/bin",
            "OFE_PYTHON": str(ofe_python),
            "TF_CALL_LOG": str(self.tf_call_log),
            "CMD_CALL_LOG": str(self.cmd_call_log),
            # Keep the tree byte-for-byte unchanged by a dry run.
            "PYTHONDONTWRITEBYTECODE": "1",
        }

    @staticmethod
    def _write_executable(path, content):
        path.write_text(content)
        path.chmod(0o755)

    def _resource(self, rel_resource_dir, rel_working_dir="", generated=False):
        """Create a resource run-time directory and its Terraform working
        directory holding local state. Returns (resource_dir, working_dir).
        As on a server that has started this release, there is no key file.
        """
        resource_dir = self.ofe / rel_resource_dir
        working_dir = resource_dir / rel_working_dir
        working_dir.mkdir(parents=True)
        (working_dir / "terraform.tfstate").write_text(STATE)
        if generated:
            (working_dir / "main.tf").write_text(GENERATED_MAIN_TF)
        return resource_dir, working_dir

    def _run(self, *args, init_rc=0, plan_rc=0, refuse=""):
        return subprocess.run(
            ["bash", str(self.ofe / "script" / MIGRATE_SCRIPT.name), *args],
            env={
                **self.env,
                "INIT_RC": str(init_rc),
                "PLAN_RC": str(plan_rc),
                "REFUSE_CREDENTIAL": refuse,
            },
            cwd=self.ofe,
            capture_output=True,
            text=True,
            timeout=60,
            check=False,
        )

    def _snapshot(self):
        """Every file under the OFE tree, with its content."""
        return {
            str(path.relative_to(self.ofe)): path.read_bytes()
            for path in sorted(self.ofe.rglob("*"))
            if path.is_file()
        }

    def _simulate_gcluster_create(self, deployment_dir):
        """Do to a deployment directory what `gcluster create -w` does
        before it writes the groups again (pkg/modulewriter, prepDepDir):
        delete the previous backup set, then move every group directory into
        .ghpc/previous_deployment_groups/.
        """
        previous = deployment_dir / ".ghpc" / "previous_deployment_groups"
        shutil.rmtree(previous, ignore_errors=True)
        previous.mkdir(parents=True)
        for entry in sorted(deployment_dir.iterdir()):
            if entry.is_dir() and entry.name != ".ghpc":
                entry.rename(previous / entry.name)

    def test_terraform_runs_as_stored_credential_without_key_file(self):
        cluster, working_dir = self._resource(
            "clusters/cluster_5", "c5/primary", generated=True
        )
        vpc, _ = self._resource("vpcs/vpc_1")
        self.assertEqual(list(self.ofe.rglob("cloud_credentials")), [])

        res = self._run("--yes")
        self.assertEqual(res.returncode, 0, res.stdout + res.stderr)
        self.assertIn("Migrated: 2", res.stdout)
        self.assertEqual((cluster / BACKUP_NAME).read_text(), STATE)
        self.assertEqual((vpc / BACKUP_NAME).read_text(), STATE)
        self.assertFalse((working_dir / "terraform.tfstate").exists())

        website = self.ofe / "website"
        self.assertEqual(
            self.cmd_call_log.read_text().splitlines(),
            [f"cluster 5 {website}"] * 3 + [f"vpc 1 {website}"] * 3,
        )
        # Every Terraform run went through the command, as the resource's
        # credential, with the working directory as an absolute path.
        self.assertEqual(
            self.tf_call_log.read_text().splitlines(),
            [
                "[cluster 5] version",
                f"[cluster 5] -chdir={working_dir} init -migrate-state -force-copy -no-color",
                f"[cluster 5] -chdir={working_dir} plan -detailed-exitcode -no-color",
                "[vpc 1] version",
                f"[vpc 1] -chdir={vpc} init -migrate-state -force-copy -no-color",
                f"[vpc 1] -chdir={vpc} plan -detailed-exitcode -no-color",
            ],
        )

    def test_unusable_credential_is_reported_and_changes_nothing(self):
        cluster, working_dir = self._resource(
            "clusters/cluster_5", "c5/primary", generated=True
        )
        vpc, _ = self._resource("vpcs/vpc_1")
        before = self._snapshot()

        res = self._run("--yes", refuse="vpc 1")
        self.assertEqual(res.returncode, 1, res.stdout + res.stderr)
        self.assertIn("vpc 1: its stored credential cannot be used", res.stderr)
        self.assertIn(
            f"FAIL: {vpc}: Terraform cannot run as the stored credential of",
            res.stderr,
        )
        self.assertIn("vpc 1 (see the message above); nothing was changed", res.stderr)
        self.assertNotIn("cloud_credentials", res.stdout + res.stderr)
        self.assertIn("Migrated: 1   Skipped: 0   Failed: 1", res.stdout)

        # The network is exactly as it was; the cluster still migrated.
        after = self._snapshot()
        for path, content in before.items():
            if path.startswith("vpcs/"):
                self.assertEqual(after.get(path), content, path)
        self.assertEqual(
            sorted(p for p in after if p.startswith("vpcs/")),
            sorted(p for p in before if p.startswith("vpcs/")),
        )
        self.assertEqual((cluster / BACKUP_NAME).read_text(), STATE)
        self.assertFalse((working_dir / "terraform.tfstate").exists())
        self.assertNotIn("[vpc 1]", self.tf_call_log.read_text())

    def test_blueprint_resource_backup_is_kept_in_resource_dir(self):
        cases = {
            "cluster": ("clusters/cluster_5", "c5/primary"),
            "filesystem": ("fs/fs_3", "shared/primary"),
            "image": ("images/image_4", "image_4/builder-env"),
        }
        for kind, (rel_resource, rel_working) in cases.items():
            with self.subTest(kind=kind):
                resource_dir, working_dir = self._resource(
                    rel_resource, rel_working, generated=True
                )
                res = self._run("--yes", "--dir", str(working_dir))
                self.assertEqual(res.returncode, 0, res.stdout + res.stderr)

                backup = resource_dir / BACKUP_NAME
                self.assertEqual(backup.read_text(), STATE)
                self.assertFalse((working_dir / BACKUP_NAME).exists())
                self.assertFalse((working_dir / "terraform.tfstate").exists())
                self.assertIn(f"pre-migration state kept as {backup}", res.stdout)
                # The generated group already has its backend block.
                self.assertFalse((working_dir / "backend.tf").exists())

    def test_template_network_backup_location_is_unchanged(self):
        vpc, _ = self._resource("vpcs/vpc_1")
        res = self._run("--yes")
        self.assertEqual(res.returncode, 0, res.stdout + res.stderr)
        self.assertEqual((vpc / BACKUP_NAME).read_text(), STATE)
        self.assertIn(f'prefix = "{NAMESPACE}/vpcs/vpc_1"', (vpc / "backend.tf").read_text())

    def test_template_workbench_backup_is_kept_in_resource_dir(self):
        workbench, working_dir = self._resource("workbenches/workbench_10", "terraform/google")
        res = self._run("--yes")
        self.assertEqual(res.returncode, 0, res.stdout + res.stderr)
        self.assertEqual((workbench / BACKUP_NAME).read_text(), STATE)
        self.assertFalse((working_dir / BACKUP_NAME).exists())
        self.assertTrue((working_dir / "backend.tf").exists())

    def test_backup_survives_toolkit_replacing_deployment_groups(self):
        cluster, working_dir = self._resource(
            "clusters/cluster_5", "c5/primary", generated=True
        )
        res = self._run("--yes")
        self.assertEqual(res.returncode, 0, res.stdout + res.stderr)

        # Two reconfigures: the first moves the migrated group aside, the
        # second deletes it.
        deployment_dir = working_dir.parent
        for _ in range(2):
            self._simulate_gcluster_create(deployment_dir)
            working_dir.mkdir()
            (working_dir / "main.tf").write_text(GENERATED_MAIN_TF)

        self.assertEqual((cluster / BACKUP_NAME).read_text(), STATE)
        previous = deployment_dir / ".ghpc" / "previous_deployment_groups" / "primary"
        self.assertEqual(sorted(p.name for p in previous.iterdir()), ["main.tf"])

    def test_existing_different_backup_is_not_overwritten(self):
        cluster, working_dir = self._resource(
            "clusters/cluster_5", "c5/primary", generated=True
        )
        earlier = '{"version": 4, "resources": [{"type": "earlier"}]}'
        (cluster / BACKUP_NAME).write_text(earlier)

        for mode in ("--dry-run", "--yes"):
            with self.subTest(mode=mode):
                res = self._run(mode)
                self.assertEqual(res.returncode, 1, res.stdout + res.stderr)
                self.assertIn("refusing to overwrite", res.stderr)
                self.assertIn(str(cluster / BACKUP_NAME), res.stderr)
                self.assertIn("Failed: 1", res.stdout)
                self.assertEqual((cluster / BACKUP_NAME).read_text(), earlier)
                self.assertEqual((working_dir / "terraform.tfstate").read_text(), STATE)
        self.assertEqual(self.tf_call_log.read_text(), "")

    def test_rerun_after_failed_init_reuses_identical_backup(self):
        cluster, working_dir = self._resource(
            "clusters/cluster_5", "c5/primary", generated=True
        )
        res = self._run("--yes", init_rc=1)
        self.assertEqual(res.returncode, 1, res.stdout + res.stderr)
        self.assertIn("terraform init -migrate-state failed", res.stderr)
        self.assertEqual((cluster / BACKUP_NAME).read_text(), STATE)
        self.assertEqual((working_dir / "terraform.tfstate").read_text(), STATE)

        rerun = self._run("--yes")
        self.assertEqual(rerun.returncode, 0, rerun.stdout + rerun.stderr)
        self.assertEqual((cluster / BACKUP_NAME).read_text(), STATE)
        self.assertFalse((working_dir / "terraform.tfstate").exists())

    def test_rerun_after_success_skips_and_keeps_backup(self):
        cluster, _ = self._resource(
            "clusters/cluster_5", "c5/primary", generated=True
        )
        self.assertEqual(self._run("--yes").returncode, 0)
        rerun = self._run("--yes")
        self.assertEqual(rerun.returncode, 0, rerun.stdout + rerun.stderr)
        self.assertIn("No local Terraform state found", rerun.stdout)
        self.assertEqual((cluster / BACKUP_NAME).read_text(), STATE)

    def test_dry_run_writes_no_backup(self):
        cluster, working_dir = self._resource(
            "clusters/cluster_5", "c5/primary", generated=True
        )
        vpc, _ = self._resource("vpcs/vpc_1")
        before = self._snapshot()

        res = self._run("--dry-run")
        self.assertEqual(res.returncode, 0, res.stdout + res.stderr)
        self.assertIn(f"would migrate: {working_dir}", res.stdout)
        self.assertIn(f"original to be kept as {cluster / BACKUP_NAME}", res.stdout)
        self.assertIn(f"original to be kept as {vpc / BACKUP_NAME}", res.stdout)
        self.assertEqual(self._snapshot(), before)
        # Only the credential check, which changes nothing, ran Terraform.
        self.assertEqual(
            self.tf_call_log.read_text().splitlines(),
            ["[cluster 5] version", "[vpc 1] version"],
        )

    def test_dry_run_reports_an_unusable_credential_as_failed(self):
        cluster, working_dir = self._resource(
            "clusters/cluster_5", "c5/primary", generated=True
        )
        vpc, _ = self._resource("vpcs/vpc_1")
        before = self._snapshot()

        res = self._run("--dry-run", refuse="vpc 1")
        self.assertEqual(res.returncode, 1, res.stdout + res.stderr)
        self.assertIn("vpc 1: its stored credential cannot be used", res.stderr)
        self.assertIn(
            f"FAIL: {vpc}: Terraform cannot run as the stored credential of",
            res.stderr,
        )
        self.assertIn("vpc 1 (see the message above); nothing was changed", res.stderr)
        # The preview does not claim the network would migrate.
        self.assertIn(f"would migrate: {working_dir}", res.stdout)
        self.assertNotIn(f"would migrate: {vpc}\n", res.stdout)
        self.assertIn("Migrated: 0   Skipped: 0   Failed: 1", res.stdout)
        self.assertEqual(self._snapshot(), before)
        self.assertEqual(
            self.tf_call_log.read_text().splitlines(), ["[cluster 5] version"]
        )

    def test_failed_plan_points_to_backup_in_resource_dir(self):
        cluster, working_dir = self._resource(
            "clusters/cluster_5", "c5/primary", generated=True
        )
        backup = cluster / BACKUP_NAME
        res = self._run("--yes", plan_rc=2)
        self.assertEqual(res.returncode, 1, res.stdout + res.stderr)
        self.assertIn(f"The original is {backup}.", res.stderr)
        self.assertEqual(backup.read_text(), STATE)
        self.assertTrue((working_dir / "terraform.tfstate.migration-incomplete").exists())

        rerun = self._run("--yes")
        self.assertEqual(rerun.returncode, 1, rerun.stdout + rerun.stderr)
        self.assertIn("did not finish", rerun.stderr)
        self.assertIn(f"the original is {backup}", rerun.stderr)
        self.assertEqual(backup.read_text(), STATE)


if __name__ == "__main__":
    unittest.main()
