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

"""Tests for the state-bucket logic in deploy.sh.

deploy_state_bucket() decides whether to create or adopt the shared Terraform
state bucket and which service accounts may write to it. A mistake there can
orphan the state of every resource the front end provisions, so these tests
execute the real function rather than matching on the script's text.

How: the function is extracted from deploy.sh by brace-matching (never
retyped), then run under bash inside a temporary directory whose PATH holds
stub `gcloud` and `terraform` executables and a stub script/service_account.sh.
The stubs record their calls and answer from canned data, so the tests need no
network, credentials, gcloud or terraform install, and are deterministic.

Standard library only (no Django; see test_tf_state_backend.py), so it runs
under both `python3 -m pytest` and `python3 -m unittest`.
"""

import json
import re
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

OFE_DIR = Path(__file__).resolve().parents[3]
DEPLOY_SH = OFE_DIR / "deploy.sh"


def _extract_bash_function(path: Path, name: str) -> str:
    """Return a bash function's full source, found by brace-matching.

    Looks for "<name>() {" or "<name> () {" and returns everything up to
    the matching closing brace, so a test runs exactly what the script
    ships rather than a copy that could drift from it.
    """
    text = path.read_text()
    match = re.search(rf"^{re.escape(name)}\s*\(\)\s*{{", text, re.MULTILINE)
    if not match:
        raise AssertionError(f"no function named {name!r} found in {path}")
    depth = 0
    start = match.end() - 1  # position of the opening brace
    for i in range(start, len(text)):
        if text[i] == "{":
            depth += 1
        elif text[i] == "}":
            depth -= 1
            if depth == 0:
                return text[match.start() : i + 1]
    raise AssertionError(f"unbalanced braces extracting {name!r} from {path}")


class RegionDerivationTests(unittest.TestCase):
    """A --config deployment must derive region from zone, as the
    interactive path does.

    Without this, deploy_from_config() leaves `region` unset, so the
    generated tf/state_bucket/terraform.tfvars and tf/terraform.tfvars both
    get `region = ""`, and `terraform apply` fails outright.
    """

    def test_config_deployment_derives_region_from_zone(self):
        body = _extract_bash_function(DEPLOY_SH, "deploy_from_config")
        self.assertIn(
            "region=${zone%-*}",
            body,
            "deploy_from_config() must derive region from zone, the same "
            "way the interactive deploy() path does",
        )


class StateBucketSandbox(unittest.TestCase):
    """Runs the real deploy_state_bucket() against stubbed external tools.

    Not a test case itself (it defines no tests); the classes below share it.
    """

    # deploy_state_bucket() calls prepare_state_bucket_dir(), which uses
    # STATE_BUCKET_ROOT, and validate_bucket_name(), which uses error() and
    # BUCKET_NAME_MAX_LENGTH, so all of them are taken from deploy.sh.
    FUNCTION_BODY = "\n".join(
        [
            _extract_bash_function(DEPLOY_SH, "error"),
            re.search(
                r"^BUCKET_NAME_MAX_LENGTH=\d+$", DEPLOY_SH.read_text(), re.MULTILINE
            ).group(0),
            _extract_bash_function(DEPLOY_SH, "validate_bucket_name"),
            re.search(
                r"^STATE_BUCKET_ROOT=\S+$", DEPLOY_SH.read_text(), re.MULTILINE
            ).group(0),
            _extract_bash_function(DEPLOY_SH, "prepare_state_bucket_dir"),
            _extract_bash_function(DEPLOY_SH, "deploy_state_bucket"),
        ]
    )
    SB_DIR = "tf/state_bucket/deployments/testdeploy"

    BUCKET = "testdeploy-tfstate-abcd1234"
    PROJECT_NUMBER = "111111111111"
    DEFAULT_SA = "serviceAccount:testdeploy-tkfe-sa@test-project.iam.gserviceaccount.com"
    FOREIGN = "serviceAccount:other@other-project.iam.gserviceaccount.com"

    def _sandbox_run(
        self,
        *,
        sa_exists: bool,
        preexisting_tfvars: str | None = None,
        preexisting_admin: bytes | None = None,
        bucket: str = "absent",
        in_local_state: bool = False,
        policy: str = '{"bindings": []}',
        local_output: str | None = None,
        apply_fails: bool = False,
        state_unreadable: bool = False,
        computed_name: str | None = None,
        state_show_name: str | None = None,
        owner: str | None = None,
        legacy: dict[str, str] | None = None,
        other_deployments: dict[str, dict[str, str]] | None = None,
        deployment_name: str = "testdeploy",
    ) -> dict:
        """Run deploy_state_bucket() in an isolated sandbox.

        `bucket` controls the stub gcloud's answer to `storage buckets list`
        for this project: "absent" (success, nothing listed), "exists"
        (success, the bucket listed), "error" (non-zero exit, with a message
        that deliberately includes "not found: 404" to show the message text
        is not what decides). `local_output` is what `terraform output -raw
        bucket_name` reports from local state: None -> the computed bucket,
        "" -> no outputs. `apply_fails` makes `terraform apply` exit non-zero
        with a 409 message, as when the name is already taken.
        `in_local_state` and `state_unreadable` both create a local
        terraform.tfstate; with `state_unreadable`, `terraform state list`
        fails on it. `computed_name` overrides the name `terraform console`
        derives. `owner` is what `gcloud storage buckets describe --raw`
        reports as the bucket's project number after a failed create: None
        -> describe fails (bucket not visible), "this" -> this project's
        number, anything else -> that number. `legacy` maps file names to
        contents left in tf/state_bucket/ itself by an earlier release;
        `other_deployments` maps deployment names to the files already in
        their own directories.
        """
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            sb_root = root / "tf" / "state_bucket"
            sb = root / self.SB_DIR
            sb.mkdir(parents=True)
            for tf_file in ("main.tf", "variables.tf"):
                (sb_root / tf_file).write_text("# tracked\n")
            for name, content in (legacy or {}).items():
                (sb_root / name).write_text(content)
            for deployment, files in (other_deployments or {}).items():
                (sb_root / "deployments" / deployment).mkdir(parents=True, exist_ok=True)
                for name, content in files.items():
                    path = sb_root / "deployments" / deployment / name
                    if content.startswith("-> "):
                        path.symlink_to(content[3:])
                    else:
                        path.write_text(content)
            (root / "script").mkdir()
            (root / "bin").mkdir()

            if preexisting_tfvars is not None:
                (sb / "terraform.tfvars").write_text(preexisting_tfvars)
            if preexisting_admin is not None:
                (sb / "admin.auto.tfvars").write_bytes(preexisting_admin)
            if in_local_state or state_unreadable:
                (sb / "terraform.tfstate").write_text("{}")
            (root / "policy.json").write_text(policy)

            def stub(path: Path, body: str) -> None:
                path.write_text("#!/bin/bash\n" + body)
                path.chmod(path.stat().st_mode | stat.S_IEXEC)

            stub(root / "script" / "service_account.sh", f"exit {0 if sa_exists else 1}\n")

            if local_output is None:
                output_action = f"echo {self.BUCKET}"
            elif local_output:
                output_action = f"echo {local_output}"
            else:
                output_action = 'echo "No outputs found" >&2; exit 1'
            if state_unreadable:
                state_list_action = (
                    'echo "Error: Failed to load state: unsupported state version 9" >&2; exit 1'
                )
            elif in_local_state:
                state_list_action = "echo google_storage_bucket.tfstate"
            else:
                state_list_action = ":"
            apply_action = (
                'echo "Error 409: bucket name already in use" >&2; exit 1'
                if apply_fails
                else ":"
            )
            name = computed_name if computed_name is not None else self.BUCKET
            stub(
                root / "bin" / "terraform",
                f'echo "$*" >> "{root}/tf_calls.log"\n'
                'case "$*" in\n'
                f'  *" console"*) cat >/dev/null; echo \'"{name}"\' ;;\n'
                f'  *"state list"*) {state_list_action} ;;\n'
                + (
                    f'  *"state show"*) echo \'  name = "{state_show_name}"\' ;;\n'
                    if state_show_name
                    else ""
                )
                +
                f'  *"output -raw bucket_name"*) {output_action} ;;\n'
                f'  *" apply"*) {apply_action} ;;\n'
                "esac\n"
                "exit 0\n",
            )

            listing = {
                "absent": "exit 0",
                "exists": f"echo {self.BUCKET}; exit 0",
                "error": (
                    'echo "ERROR: HTTPError 403: denied; not found: 404" >&2; exit 1'
                ),
            }[bucket]
            if owner is None:
                describe = 'echo "ERROR: not visible" >&2; exit 1'
            else:
                describe = f"echo {self.PROJECT_NUMBER if owner == 'this' else owner}; exit 0"
            stub(
                root / "bin" / "gcloud",
                f'echo "$*" >> "{root}/gcloud_calls.log"\n'
                'case "$1 $2 $3" in\n'
                f'  "storage buckets list") {listing} ;;\n'
                f'  "storage buckets get-iam-policy") cat "{root}/policy.json" ;;\n'
                f'  "storage buckets describe") {describe} ;;\n'
                f'  "projects describe "*) echo {self.PROJECT_NUMBER} ;;\n'
                "esac\n",
            )

            # Call it exactly as deploy() does: under the script's top-level
            # `set -e`, but as an `if !` condition, which suspends errexit for
            # the whole function. Calling it bare under `set -e` would test a
            # stricter context than production runs.
            script = (
                f"{self.FUNCTION_BODY}\n"
                "set -e\n"
                "if ! deploy_state_bucket; then exit 1; fi\n"
            )
            env = {
                "PATH": f"{root / 'bin'}:/usr/bin:/bin",
                "SCRIPT_DIR": str(root),
                "project_id": "test-project",
                "deployment_name": deployment_name,
                "region": "europe-west4",
                "USER": "test-user",
            }
            result = subprocess.run(
                ["bash", "-c", script],
                cwd=root,
                env=env,
                capture_output=True,
                text=True,
                timeout=30,
            )
            tfvars = sb / "terraform.tfvars"
            admin = sb / "admin.auto.tfvars"
            calls = root / "tf_calls.log"
            gcalls = root / "gcloud_calls.log"

            def snapshot(path: Path) -> dict:
                return {
                    str(f.relative_to(path)): (
                        "-> " + str(f.readlink()) if f.is_symlink() else f.read_text()
                    )
                    for f in sorted(path.rglob("*"))
                    if f.is_file() or f.is_symlink()
                }

            return {
                "rc": result.returncode,
                "files": snapshot(sb_root),
                "out": result.stdout + result.stderr,
                "tfvars": tfvars.read_text() if tfvars.exists() else None,
                "admin": admin.read_bytes() if admin.exists() else None,
                "tf_calls": calls.read_text().splitlines() if calls.exists() else [],
                "gcloud_calls": gcalls.read_text().splitlines() if gcalls.exists() else [],
            }

    def _ok(self, **kw) -> dict:
        res = self._sandbox_run(**kw)
        self.assertEqual(res["rc"], 0, f"deploy_state_bucket() failed: {res['out']!r}")
        return res

    @staticmethod
    def _list(text: str, name: str) -> list[str]:
        match = re.search(rf"^{name}\s*=\s*\[(.*?)\]", text, re.MULTILINE | re.DOTALL)
        assert match, f"no {name} in: {text!r}"
        return re.findall(r'"([^"]+)"', match.group(1))

    @staticmethod
    def _imports(res):
        return [c for c in res["tf_calls"] if c.split()[1:2] == ["import"]]

    @staticmethod
    def _applied(res):
        return any("apply" in c.split()[:2] for c in res["tf_calls"])


class DeployStateBucketWritersTests(StateBucketSandbox):
    """The state bucket's writer list is split by owner and never rebuilt
    from a hand-edited file.

    terraform.tfvars belongs to deploy.sh and is regenerated on every deploy
    (default_writers: the provisioning SA, when it exists). admin.auto.tfvars
    belongs to the administrator (state_bucket_writers): deploy.sh creates it
    once and never reads or rewrites it. The binding is authoritative, so the
    regression guarded against is a hand-added writer being silently revoked
    by the next deploy.
    """

    def test_generated_tfvars_never_carries_state_bucket_writers(self):
        res = self._ok(sa_exists=True)
        self.assertNotIn("state_bucket_writers", res["tfvars"])
        self.assertEqual(self._list(res["tfvars"], "default_writers"), [self.DEFAULT_SA])

    def test_default_writers_empty_when_default_sa_absent(self):
        res = self._ok(sa_exists=False)
        self.assertEqual(self._list(res["tfvars"], "default_writers"), [])

    def test_first_deploy_creates_empty_admin_file(self):
        res = self._ok(sa_exists=True)
        self.assertEqual(self._list(res["admin"].decode(), "state_bucket_writers"), [])

    def test_hand_edited_admin_file_is_left_byte_for_byte(self):
        # Multi-line, indented, commented, CRLF: layouts a line-based parser
        # would misread, silently dropping writers.
        hand_edited = (
            b"# my writers\r\n"
            b"state_bucket_writers = [\r\n"
            b'    "' + self.FOREIGN.encode() + b'",  # cross-project\r\n'
            b'    "serviceAccount:third@p.iam.gserviceaccount.com"\r\n'
            b"]\r\n"
            b"noncurrent_version_retention_days = 365\r\n"
        )
        res = self._ok(sa_exists=True, preexisting_admin=hand_edited)
        self.assertEqual(res["admin"], hand_edited)
        self.assertTrue(self._applied(res))

    def test_admin_file_untouched_on_redeploy_without_default_sa(self):
        admin = b'state_bucket_writers = ["' + self.FOREIGN.encode() + b'"]\n'
        res = self._ok(sa_exists=False, preexisting_admin=admin)
        self.assertEqual(res["admin"], admin)

    def test_old_format_tfvars_without_admin_file_stops_before_apply(self):
        res = self._sandbox_run(
            sa_exists=True,
            preexisting_tfvars=f'state_bucket_writers = ["{self.FOREIGN}"]\n',
        )
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("Move that line into", res["out"])
        self.assertFalse(self._applied(res))
        self.assertIsNone(res["admin"])

    def test_old_format_tfvars_with_admin_file_present_proceeds(self):
        res = self._ok(
            sa_exists=True,
            preexisting_tfvars=f'state_bucket_writers = ["{self.FOREIGN}"]\n',
            preexisting_admin=b"state_bucket_writers = []\n",
        )
        self.assertNotIn("state_bucket_writers", res["tfvars"])


class DeployStateBucketAdoptionTests(StateBucketSandbox):
    """A deploy from a machine without tf/state_bucket's local state must
    adopt the existing bucket, never create a second one, and must stop
    rather than act on local state for a different bucket.

    Whether the bucket exists is decided from gcloud's exit status and output,
    never from its error wording: exists -> import, listed nothing -> create,
    failure -> stop before apply.
    """

    def test_absent_bucket_is_created_not_imported(self):
        res = self._ok(sa_exists=True, bucket="absent")
        self.assertEqual(self._imports(res), [])
        self.assertTrue(self._applied(res))

    def test_existing_bucket_is_imported_and_seeds_admin_file(self):
        policy = (
            '{"bindings": [{"role": "roles/storage.objectAdmin", "members": ['
            f'"{self.FOREIGN}", "{self.DEFAULT_SA}", '
            '"deleted:serviceAccount:gone@p.iam.gserviceaccount.com?uid=1"]},'
            ' {"role": "roles/storage.legacyBucketOwner", "members": ["projectOwner:p"]}]}'
        )
        res = self._ok(sa_exists=True, bucket="exists", policy=policy)
        imports = self._imports(res)
        self.assertEqual(len(imports), 1, res["tf_calls"])
        self.assertTrue(
            imports[0].endswith(f"google_storage_bucket.tfstate test-project/{self.BUCKET}"),
            imports[0],
        )
        # Live holders seed the admin file, minus deleted principals, other
        # roles, and the default SA (which deploy.sh supplies itself).
        self.assertEqual(self._list(res["admin"].decode(), "state_bucket_writers"), [self.FOREIGN])
        self.assertEqual(self._list(res["tfvars"], "default_writers"), [self.DEFAULT_SA])
        self.assertTrue(self._applied(res))

    def test_adoption_does_not_overwrite_an_existing_admin_file(self):
        policy = (
            '{"bindings": [{"role": "roles/storage.objectAdmin",'
            f' "members": ["{self.FOREIGN}"]}}]}}'
        )
        admin = b"state_bucket_writers = []  # deliberately empty\n"
        res = self._ok(sa_exists=False, bucket="exists", policy=policy, preexisting_admin=admin)
        self.assertEqual(res["admin"], admin)

    def test_bucket_already_in_local_state_is_not_reimported(self):
        res = self._ok(sa_exists=True, bucket="exists", in_local_state=True)
        self.assertEqual(self._imports(res), [])

    def test_local_state_for_a_different_bucket_stops_before_apply(self):
        # A bucket named under an earlier, non-deterministic naming scheme.
        res = self._sandbox_run(
            sa_exists=True, in_local_state=True, local_output="testdeploy-tfstate-c1d6"
        )
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("gs://testdeploy-tfstate-c1d6", res["out"])
        self.assertIn(f"gs://{self.BUCKET}", res["out"])
        self.assertIn(f"mv {self.SB_DIR}/admin.auto.tfvars", res["out"])
        self.assertFalse(self._applied(res))
        self.assertEqual(self._imports(res), [])

    def test_local_state_without_bucket_output_stops_before_apply(self):
        res = self._sandbox_run(sa_exists=True, in_local_state=True, local_output="")
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("<unknown bucket>", res["out"])
        self.assertFalse(self._applied(res))

    def test_local_state_without_output_uses_the_name_in_state(self):
        # An adoption whose apply failed: the bucket is in state, no outputs.
        res = self._sandbox_run(
            sa_exists=True, in_local_state=True, local_output="", state_show_name=self.BUCKET
        )
        # The stub writes no outputs on apply, so only the guard is checked:
        # no wrong-bucket stop, and the apply runs.
        self.assertNotIn("belongs to", res["out"])
        self.assertNotIn("<unknown bucket>", res["out"])
        self.assertTrue(self._applied(res))

    def test_bucket_is_looked_up_by_exact_name_within_the_project(self):
        res = self._ok(sa_exists=True, bucket="absent")
        lists = [c for c in res["gcloud_calls"] if c.startswith("storage buckets list")]
        self.assertEqual(len(lists), 1, res["gcloud_calls"])
        self.assertIn("--project=test-project", lists[0])
        self.assertIn(f"--filter=name={self.BUCKET}", lists[0])

    def test_lookup_error_stops_before_apply(self):
        # The stub's message contains "not found: 404"; it must still stop.
        res = self._sandbox_run(sa_exists=True, bucket="error")
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("could not determine whether", res["out"])
        self.assertIn("403", res["out"])
        self.assertFalse(self._applied(res))
        self.assertEqual(self._imports(res), [])

    def test_unparseable_policy_stops_before_apply(self):
        res = self._sandbox_run(sa_exists=True, bucket="exists", policy="not json")
        self.assertNotEqual(res["rc"], 0)
        self.assertFalse(self._applied(res))

    def test_name_owned_by_another_project_fails_at_create_with_explanation(self):
        # Bucket names are global. The project's listing cannot show a bucket
        # owned elsewhere, so the create is attempted, fails with a 409, and
        # the owner is then named from its project number.
        res = self._sandbox_run(sa_exists=True, bucket="absent", apply_fails=True, owner="999")
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("already exists in another project (number", res["out"])
        self.assertIn("999", res["out"])
        self.assertEqual(self._imports(res), [])

    def test_failed_create_of_own_bucket_says_to_rerun_and_adopt(self):
        # A 409 can also mean the listing missed this project's own bucket.
        res = self._sandbox_run(sa_exists=True, bucket="absent", apply_fails=True, owner="this")
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("already exists in test-project", res["out"])
        self.assertIn("Rerun deploy.sh to adopt it", res["out"])
        self.assertNotIn("another project", res["out"])

    def test_failed_update_of_bucket_in_local_state_is_not_called_a_name_clash(self):
        # An ordinary redeploy whose apply fails for another reason (IAM,
        # quota) must not be told to rerun and adopt a bucket it manages.
        res = self._sandbox_run(
            sa_exists=True, bucket="exists", in_local_state=True, apply_fails=True, owner="this"
        )
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("already managed by this deployment", res["out"])
        self.assertNotIn("Rerun deploy.sh to adopt it", res["out"])
        self.assertFalse(
            any(c.startswith("storage buckets describe") for c in res["gcloud_calls"]),
            res["gcloud_calls"],
        )

    def test_failed_apply_after_adopting_the_bucket_is_not_called_a_name_clash(self):
        res = self._sandbox_run(sa_exists=True, bucket="exists", apply_fails=True, owner="this")
        self.assertNotEqual(res["rc"], 0)
        self.assertEqual(len(self._imports(res)), 1, res["tf_calls"])
        self.assertIn("already managed by this deployment", res["out"])
        self.assertNotIn("Rerun deploy.sh to adopt it", res["out"])

    def test_failed_create_of_invisible_bucket_says_so(self):
        res = self._sandbox_run(sa_exists=True, bucket="absent", apply_fails=True, owner=None)
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("a project", res["out"])
        self.assertIn("this account cannot see", res["out"])


class DeployStateBucketDirectoryTests(StateBucketSandbox):
    """Each deployment name keeps its state bucket files in its own
    directory, tf/state_bucket/deployments/<name>/, so a second deployment
    from the same checkout neither collides with an earlier one nor needs
    its files moved by hand. Files an earlier release left in
    tf/state_bucket/ move to their owner's directory only when the owner is
    certain; otherwise the deploy stops before anything runs.
    """

    LEGACY_TFVARS = 'project_id = "test-project"\ndeployment_name = "olddeploy"\n'

    def test_files_are_written_in_the_deployments_own_directory(self):
        res = self._ok(sa_exists=True)
        self.assertIn("deployments/testdeploy/terraform.tfvars", res["files"])
        self.assertIn("deployments/testdeploy/admin.auto.tfvars", res["files"])
        self.assertEqual(res["files"]["deployments/testdeploy/main.tf"], "-> ../../main.tf")
        self.assertEqual(
            res["files"]["deployments/testdeploy/variables.tf"], "-> ../../variables.tf"
        )
        for call in res["tf_calls"]:
            self.assertTrue(call.startswith(f"-chdir={self.SB_DIR} "), call)

    def test_links_are_refreshed_on_every_deploy(self):
        # A link to a .tf file since removed from the tracked root must not
        # linger, and existing links are replaced, not duplicated.
        links = {"main.tf": "-> ../../main.tf", "removed.tf": "-> ../../removed.tf"}
        res = self._ok(sa_exists=True, other_deployments={"testdeploy": links})
        self.assertEqual(
            sorted(
                k
                for k in res["files"]
                if k.startswith("deployments/testdeploy/") and k.endswith(".tf")
            ),
            ["deployments/testdeploy/main.tf", "deployments/testdeploy/variables.tf"],
        )

    def test_another_deployments_files_are_left_untouched(self):
        other = {
            "terraform.tfstate": '{"other": true}',
            "terraform.tfvars": 'deployment_name = "other"\n',
            "admin.auto.tfvars": "state_bucket_writers = []\n",
        }
        res = self._ok(sa_exists=True, other_deployments={"other": other})
        for name, content in other.items():
            self.assertEqual(res["files"][f"deployments/other/{name}"], content)
        self.assertNotIn("mv ", res["out"])

    def test_earlier_release_files_move_to_their_owner(self):
        legacy = {
            "terraform.tfstate": '{"legacy": true}',
            "terraform.tfstate.backup": "{}",
            "terraform.tfvars": self.LEGACY_TFVARS,
            "admin.auto.tfvars": "state_bucket_writers = []\n",
        }
        res = self._ok(sa_exists=True, legacy=legacy)
        for name, content in legacy.items():
            self.assertNotIn(name, res["files"])
            self.assertEqual(res["files"][f"deployments/olddeploy/{name}"], content)
        self.assertIn("Moving deployment olddeploy's state bucket files", res["out"])

    def test_earlier_release_state_for_another_deployments_bucket_stops(self):
        # An earlier release rewrote terraform.tfvars before applying, so a
        # failed deploy under another name left it naming the wrong owner.
        state = json.dumps(
            {
                "resources": [
                    {
                        "type": "google_storage_bucket",
                        "name": "tfstate",
                        "instances": [{"attributes": {"labels": {"ghpcfe_id": "first"}}}],
                    }
                ]
            }
        )
        legacy = {"terraform.tfstate": state, "terraform.tfvars": self.LEGACY_TFVARS}
        res = self._sandbox_run(sa_exists=True, legacy=legacy)
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("cannot tell which deployment", res["out"])
        self.assertEqual(res["files"]["terraform.tfstate"], state)
        self.assertEqual(res["tf_calls"], [])

    def test_earlier_release_state_whose_label_matches_moves(self):
        state = json.dumps(
            {
                "resources": [
                    {
                        "type": "google_storage_bucket",
                        "name": "tfstate",
                        "instances": [{"attributes": {"labels": {"ghpcfe_id": "olddeploy"}}}],
                    }
                ]
            }
        )
        legacy = {"terraform.tfstate": state, "terraform.tfvars": self.LEGACY_TFVARS}
        res = self._ok(sa_exists=True, legacy=legacy)
        self.assertEqual(res["files"]["deployments/olddeploy/terraform.tfstate"], state)

    def test_dot_deployment_names_are_rejected(self):
        for name in (".", ".."):
            with self.subTest(name=name):
                res = self._sandbox_run(sa_exists=True, deployment_name=name)
                self.assertNotEqual(res["rc"], 0)
                self.assertIn("invalid deployment name", res["out"])
                self.assertEqual(res["tf_calls"], [])

    def test_earlier_release_files_without_an_owner_stop_before_anything(self):
        legacy = {"admin.auto.tfvars": "state_bucket_writers = []\n"}
        res = self._sandbox_run(sa_exists=True, legacy=legacy)
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("cannot tell which deployment", res["out"])
        self.assertEqual(res["files"]["admin.auto.tfvars"], legacy["admin.auto.tfvars"])
        self.assertEqual(res["tf_calls"], [])
        self.assertEqual(res["gcloud_calls"], [])

    def test_earlier_release_files_stop_if_the_owner_already_has_files(self):
        legacy = {"terraform.tfstate": "{}", "terraform.tfvars": self.LEGACY_TFVARS}
        res = self._sandbox_run(
            sa_exists=True,
            legacy=legacy,
            other_deployments={"olddeploy": {"terraform.tfstate": '{"newer": true}'}},
        )
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("cannot tell which deployment", res["out"])
        self.assertEqual(res["files"]["terraform.tfstate"], "{}")
        self.assertEqual(
            res["files"]["deployments/olddeploy/terraform.tfstate"], '{"newer": true}'
        )
        self.assertEqual(res["tf_calls"], [])


class DeployStateBucketLocalStateTests(StateBucketSandbox):
    """Local tf/state_bucket state is read only when it exists, and a file
    that exists but cannot be read stops the deploy with Terraform's own
    error instead of being mistaken for "no state".
    """

    def test_init_upgrades_a_lock_file_from_an_older_provider_pin(self):
        # A lock file written under an earlier provider constraint makes a
        # plain `terraform init` fail; -upgrade re-resolves it in range.
        res = self._ok(sa_exists=True, bucket="absent")
        inits = [c for c in res["tf_calls"] if c.split()[1:2] == ["init"]]
        self.assertEqual(len(inits), 1, res["tf_calls"])
        self.assertIn("-upgrade", inits[0].split())

    def test_no_local_state_file_skips_state_list(self):
        res = self._ok(sa_exists=True, bucket="absent")
        self.assertFalse(any("state list" in c for c in res["tf_calls"]), res["tf_calls"])

    def test_unreadable_local_state_stops_with_terraform_error(self):
        res = self._sandbox_run(sa_exists=True, bucket="exists", state_unreadable=True)
        self.assertNotEqual(res["rc"], 0)
        self.assertIn(f"could not read {self.SB_DIR}/terraform.tfstate", res["out"])
        self.assertIn("unsupported state version 9", res["out"])
        self.assertIn("terraform.tfstate.unreadable", res["out"])
        self.assertEqual(self._imports(res), [])
        self.assertFalse(self._applied(res))
        self.assertFalse(
            any(c.startswith("storage buckets list") for c in res["gcloud_calls"]),
            res["gcloud_calls"],
        )


class DeployStateBucketNameTests(StateBucketSandbox):
    """An invalid computed bucket name stops the deploy before any lookup,
    import or create. tf/state_bucket/main.tf derives a valid name, so this
    is a safety net.
    """

    def _assert_stopped_before_anything(self, name):
        res = self._sandbox_run(sa_exists=True, bucket="absent", computed_name=name)
        self.assertNotEqual(res["rc"], 0, res["out"])
        self.assertIn(f"State bucket name '{name}' is invalid", res["out"])
        self.assertFalse(self._applied(res))
        self.assertEqual(self._imports(res), [])
        self.assertFalse(
            any(c.startswith("storage buckets list") for c in res["gcloud_calls"]),
            res["gcloud_calls"],
        )

    def test_name_longer_than_63_characters_stops(self):
        self._assert_stopped_before_anything("a" * 47 + "-tfstate-abcd1234")

    def test_name_with_a_dot_stops(self):
        self._assert_stopped_before_anything("my.deploy-tfstate-abcd1234")

    def test_name_ending_in_a_dash_stops(self):
        self._assert_stopped_before_anything("testdeploy-tfstate-")

    def test_63_character_name_is_accepted(self):
        name = "a" * 46 + "-tfstate-abcd1234"
        self.assertEqual(len(name), 63)
        res = self._sandbox_run(sa_exists=True, bucket="absent", computed_name=name)
        self.assertEqual(res["rc"], 0, res["out"])
