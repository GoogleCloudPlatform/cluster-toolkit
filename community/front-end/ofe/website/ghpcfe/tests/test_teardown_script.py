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

"""Tests for the state-bucket logic in teardown.sh.

Deleting the state bucket loses the state of every resource still recorded in
it, so teardown.sh deletes it only on request, and only when it is provably
empty.

Like test_deploy_script.py, these run the real teardown.sh, or functions
extracted from it by brace-matching, under bash in a temporary directory with
stub external tools. Standard library only (no Django).
"""

import json
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

from .test_deploy_script import OFE_DIR, _extract_bash_function

TEARDOWN_SH = OFE_DIR / "teardown.sh"


class StateBucketDirTests(unittest.TestCase):
    """teardown.sh reports the bucket of the deployment being torn down,
    from that deployment's own directory, or from tf/state_bucket/ if an
    earlier release left that same deployment's files there.
    """

    FUNCTION_BODY = _extract_bash_function(TEARDOWN_SH, "state_bucket_dir")

    def _run(self, files: dict[str, str], name: str = "dep1") -> str:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for path, content in files.items():
                (root / path).parent.mkdir(parents=True, exist_ok=True)
                (root / path).write_text(content)
            result = subprocess.run(
                ["bash", "-c", f'{self.FUNCTION_BODY}\nstate_bucket_dir "{name}"'],
                cwd=root,
                capture_output=True,
                text=True,
                check=True,
                timeout=30,
            )
            return result.stdout.strip()

    def test_own_directory(self):
        files = {
            "tf/state_bucket/deployments/dep1/terraform.tfstate": "{}",
            "tf/state_bucket/deployments/dep2/terraform.tfstate": "{}",
        }
        self.assertEqual(self._run(files), "tf/state_bucket/deployments/dep1")

    def test_earlier_release_files_of_this_deployment(self):
        files = {
            "tf/state_bucket/terraform.tfstate": "{}",
            "tf/state_bucket/terraform.tfvars": 'deployment_name = "dep1"\n',
        }
        self.assertEqual(self._run(files), "tf/state_bucket")

    def test_earlier_release_files_of_another_deployment_are_not_used(self):
        files = {
            "tf/state_bucket/terraform.tfstate": "{}",
            "tf/state_bucket/terraform.tfvars": 'deployment_name = "dep10"\n',
        }
        self.assertEqual(self._run(files), "")

    def test_dot_names_are_never_resolved(self):
        files = {"tf/state_bucket/terraform.tfstate": "{}"}
        for name in (".", ".."):
            with self.subTest(name=name):
                self.assertEqual(self._run(files, name=name), "")

    def test_no_state(self):
        self.assertEqual(self._run({}), "")


class TeardownSandbox(unittest.TestCase):
    """Runs the real teardown.sh, or functions from it, in a temporary
    directory with stub gcloud, terraform and script/service_account.sh.

    The stub gcloud serves the bucket's objects from `objects` (name ->
    content); the stub terraform records every call with its -chdir, and on
    `apply` copies the main.tf it was given so a test can check what was
    applied. tf/state_bucket/ holds the real tracked .tf files.
    """

    BUCKET = "dep1-tfstate-abcd1234"
    EMPTY = json.dumps({"version": 4, "resources": []})
    BUSY = json.dumps(
        {"version": 4, "resources": [{"mode": "managed"}, {"mode": "managed"}]}
    )

    def _run(
        self,
        args: str = "",
        *,
        objects: dict[str, str] | None = None,
        list_fails: bool = False,
        fe_deployed: bool = True,
        lock: bool = True,
        show_fails: bool = False,
        tf_tfvars: str | None = 'project_id = "proj"\ndeployment_name = "dep1"\n',
        stdin: str = "",
        script: str | None = None,
        noncurrent: list[str] | None = None,
        busy_after_apply: bool = False,
        plan_actions: tuple[str, ...] = ("update",),
        interrupt_on_apply: bool = False,
    ) -> dict:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "bin").mkdir()
            (root / "script").mkdir()
            (root / "objects").mkdir()
            sb_root = root / "tf" / "state_bucket"
            sb_dir = sb_root / "deployments" / "dep1"
            sb_dir.mkdir(parents=True)
            for tf_file in (OFE_DIR / "tf" / "state_bucket").glob("*.tf"):
                (sb_root / tf_file.name).write_text(tf_file.read_text())
                (sb_dir / tf_file.name).symlink_to(f"../../{tf_file.name}")
            (sb_dir / "terraform.tfstate").write_text('{"bucket": "in state"}')
            (sb_dir / "terraform.tfvars").write_text('deployment_name = "dep1"\n')
            (sb_dir / "admin.auto.tfvars").write_text("state_bucket_writers = []\n")
            if tf_tfvars is not None:
                (root / "tf" / "terraform.tfvars").write_text(tf_tfvars)
            if lock:
                (root / "tf" / ".tkfe.lock").write_text("")
            for name, content in (objects or {}).items():
                path = root / "objects" / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(content)

            # Noncurrent versions are listed by name with a noncurrent time.
            (root / "noncurrent").write_text(
                "".join(f"{name}\t2026-10-20T10:00:00+0000\n" for name in noncurrent or [])
            )

            def stub(path: Path, body: str) -> None:
                path.write_text("#!/bin/bash\n" + body)
                path.chmod(path.stat().st_mode | stat.S_IEXEC)

            stub(root / "script" / "service_account.sh", "exit 1\n")
            # Kept out of the f-string below: Python 3.11 rejects a backslash
            # inside an f-string expression.
            show_action = "echo '  ghpcfe_id = \"dep1\"'" if fe_deployed else ":"
            if show_fails:
                # As with tf/ not initialised: no provider schemas.
                show_action = 'echo "Error: Failed to load plugin schemas" >&2; exit 1'
            listing = (
                'echo "ERROR: denied" >&2; exit 1'
                if list_fails
                else f'(cd "{root}/objects" && find . -type f | sed "s|^./||"; cat "{root}/noncurrent")'
            )
            stub(
                root / "bin" / "gcloud",
                f'echo "$*" >> "{root}/gcloud_calls.log"\n'
                'case "$1 $2 $3" in\n'
                '  "storage --help "*) exit 0 ;;\n'
                f'  "storage objects list") {listing} ;;\n'
                f'  "storage cat "*) cat "{root}/objects/${{3#gs://{self.BUCKET}/}}" ;;\n'
                "esac\n",
            )
            stub(
                root / "bin" / "terraform",
                'dir=.; case "$1" in -chdir=*) dir=${1#-chdir=}; shift ;; esac\n'
                f'echo "$dir $*" >> "{root}/tf_calls.log"\n'
                'case "$1" in\n'
                f'  output) echo {self.BUCKET} ;;\n'
                '  plan) : ;;\n'
                "esac\n"
                # Only `show -json` reads the plan; plain `show` is the front
                # end's state, answered below.
                + '[ "$1 $2" = "show -json" ] && echo \''
                + json.dumps(
                    {
                        "resource_changes": [
                            {
                                "address": "google_storage_bucket.tfstate",
                                "change": {"actions": list(plan_actions)},
                            }
                        ]
                    }
                )
                + "' && exit 0\n"
                'case "$1" in\n'
                f"  show) {show_action} ;;\n"
                f'  apply) cp "$dir/main.tf" "{root}/applied_main.tf"'
                + (
                    # As on Ctrl-C: the run recorded something, then the
                    # script is interrupted.
                    """; echo '{"interrupted": true}' > "$dir/terraform.tfstate"; """
                    "kill -INT $PPID"
                    if interrupt_on_apply
                    else ""
                )
                + (
                    f'; mkdir -p "{root}/objects/ns/vpcs/vpc_9"; '
                    f"echo '{self.BUSY}' > \"{root}/objects/ns/vpcs/vpc_9/default.tfstate\""
                    if busy_after_apply
                    else ""
                )
                + " ;;\n"
                '  destroy) [ "$dir" = . ] || '
                'echo \'{"destroyed": true}\' > "$dir/terraform.tfstate" ;;\n'
                "esac\n",
            )
            (root / "tmpdir").mkdir()
            env = {
                "PATH": f"{root / 'bin'}:/usr/bin:/bin",
                "HOME": tmp,
                "TMPDIR": str(root / "tmpdir"),
            }
            if script is None:
                (root / "teardown.sh").write_text(TEARDOWN_SH.read_text())
                command = ["bash", str(root / "teardown.sh"), *args.split()]
            else:
                command = ["bash", "-c", script]
            result = subprocess.run(
                command,
                cwd=root,
                env=env,
                input=stdin,
                capture_output=True,
                text=True,
                timeout=60,
            )
            calls = root / "tf_calls.log"
            gcalls = root / "gcloud_calls.log"
            applied = root / "applied_main.tf"
            return {
                "rc": result.returncode,
                "out": result.stdout + result.stderr,
                "tf_calls": calls.read_text().splitlines() if calls.exists() else [],
                "gcloud_calls": gcalls.read_text().splitlines() if gcalls.exists() else [],
                "applied_main_tf": applied.read_text() if applied.exists() else None,
                "main_tf": (sb_root / "main.tf").read_text(),
                "state": (sb_dir / "terraform.tfstate").read_text(),
                "tmp": tmp,
                "tmp_left": sorted(p.name for p in (root / "tmpdir").iterdir()),
            }

    @staticmethod
    def _fe_destroyed(res) -> bool:
        return ". destroy -auto-approve" in res["tf_calls"]

    @staticmethod
    def _bucket_destroyed(res) -> bool:
        return any(
            c.split()[1:2] == ["destroy"] and not c.startswith(". ")
            for c in res["tf_calls"]
        )


class CheckStateBucketEmptyTests(TeardownSandbox):
    """The bucket counts as empty only if every state in it, under every
    namespace and prefix, provably records no resources and none is
    locked.
    """

    FUNCTION_BODY = _extract_bash_function(TEARDOWN_SH, "check_state_bucket_empty")

    def _check(self, **kw) -> dict:
        script = f'{self.FUNCTION_BODY}\ncheck_state_bucket_empty "{self.BUCKET}"'
        return self._run(script=script, **kw)

    def test_all_states_empty(self):
        res = self._check(
            objects={
                "20261020-0a1b2c3d/vpcs/vpc_1/default.tfstate": self.EMPTY,
                "20261019-99999999/clusters/cluster_2/default.tfstate": self.EMPTY,
            }
        )
        self.assertEqual(res["rc"], 0, res["out"])

    def test_empty_bucket(self):
        self.assertEqual(self._check(objects={})["rc"], 0)

    def test_state_with_resources_in_any_namespace_is_listed(self):
        res = self._check(
            objects={
                "20261020-0a1b2c3d/vpcs/vpc_1/default.tfstate": self.EMPTY,
                "20261019-99999999/clusters/cluster_2/default.tfstate": self.BUSY,
            }
        )
        self.assertNotEqual(res["rc"], 0)
        self.assertIn(
            "20261019-99999999/clusters/cluster_2/default.tfstate (2 resources)",
            res["out"],
        )
        self.assertNotIn("vpc_1", res["out"])

    def test_unreadable_state_is_not_treated_as_empty(self):
        res = self._check(objects={"ns/fs/fs_1/default.tfstate": "not json"})
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("ns/fs/fs_1/default.tfstate (could not be read)", res["out"])

    def test_lock_is_not_treated_as_empty(self):
        res = self._check(
            objects={
                "ns/vpcs/vpc_1/default.tfstate": self.EMPTY,
                "ns/vpcs/vpc_1/default.tflock": "{}",
            }
        )
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("default.tflock (locked", res["out"])

    def test_noncurrent_versions_are_ignored(self):
        # Every released lock and superseded state leaves a noncurrent
        # version in the versioned bucket; only live objects count.
        res = self._check(
            objects={"ns/vpcs/vpc_1/default.tfstate": self.EMPTY},
            noncurrent=["ns/vpcs/vpc_1/default.tflock", "ns/vpcs/vpc_1/default.tfstate"],
        )
        self.assertEqual(res["rc"], 0, res["out"])
        cats = [c for c in res["gcloud_calls"] if c.startswith("storage cat")]
        self.assertEqual(len(cats), 1, res["gcloud_calls"])

    def test_listing_failure_is_not_treated_as_empty(self):
        res = self._check(list_fails=True)
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("could not list", res["out"])


class DeleteStateBucketTests(TeardownSandbox):
    """teardown.sh --delete-state-bucket deletes the bucket only when it is
    provably empty and confirmed, from a temporary copy of the root, and
    plain teardown.sh still keeps the bucket.
    """

    def test_plain_teardown_keeps_the_bucket(self):
        res = self._run("-y", objects={"ns/vpcs/vpc_1/default.tfstate": self.EMPTY})
        self.assertEqual(res["rc"], 0, res["out"])
        self.assertTrue(self._fe_destroyed(res))
        self.assertFalse(self._bucket_destroyed(res))
        self.assertFalse(
            any("objects list" in c for c in res["gcloud_calls"]),
            res["gcloud_calls"],
        )
        self.assertIn("has been left in place", res["out"])
        self.assertIn("./teardown.sh --delete-state-bucket dep1", res["out"])

    def test_refuses_before_destroying_anything_when_a_state_has_resources(self):
        res = self._run(
            "-y --delete-state-bucket",
            objects={"ns/clusters/cluster_3/default.tfstate": self.BUSY},
        )
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("ns/clusters/cluster_3/default.tfstate (2 resources)", res["out"])
        self.assertFalse(self._fe_destroyed(res))
        self.assertFalse(self._bucket_destroyed(res))
        self.assertIsNone(res["applied_main_tf"])

    def test_deletes_an_empty_bucket_after_the_front_end(self):
        res = self._run(
            "-y --delete-state-bucket",
            objects={"ns/vpcs/vpc_1/default.tfstate": self.EMPTY},
        )
        self.assertEqual(res["rc"], 0, res["out"])
        bucket_calls = [c for c in res["tf_calls"] if not c.startswith(". ")]
        bucket_verbs = [c.split()[1] for c in bucket_calls]
        verbs = [v for v in bucket_verbs if v in ("init", "apply", "destroy")]
        self.assertEqual(verbs, ["init", "apply", "destroy"])
        apply_call = bucket_calls[bucket_verbs.index("apply")]
        plan_call = bucket_calls[bucket_verbs.index("plan")]
        self.assertIn("-target=google_storage_bucket.tfstate", plan_call)
        self.assertIn("-out=target.tfplan", plan_call)
        # Exactly the planned change is applied.
        self.assertTrue(apply_call.endswith(" target.tfplan"), apply_call)
        tf_index = res["tf_calls"].index(". destroy -auto-approve")
        self.assertLess(tf_index, res["tf_calls"].index(bucket_calls[-1]))
        self.assertIn(f"gs://{self.BUCKET} has been deleted", res["out"])

    def test_protection_is_lifted_only_in_a_temporary_copy(self):
        res = self._run("-y --delete-state-bucket", objects={})
        self.assertEqual(res["rc"], 0, res["out"])
        tracked = (OFE_DIR / "tf" / "state_bucket" / "main.tf").read_text()
        self.assertEqual(res["main_tf"], tracked)
        applied = res["applied_main_tf"]
        self.assertNotIn("prevent_destroy", applied)
        self.assertIn("  force_destroy = true\n", applied)
        self.assertIn("lifecycle_rule {", applied)
        for call in res["tf_calls"]:
            if call.split()[1] in ("init", "apply", "destroy"):
                self.assertFalse(call.startswith("tf/"), call)
        # The copy's state, recording what is left, replaces the local one.
        self.assertEqual(res["state"], '{"destroyed": true}\n')

    def test_rechecked_immediately_before_the_destroy(self):
        # A state written after the first check, here during the targeted
        # apply, must stop the destroy, which would delete every object.
        res = self._run("-y --delete-state-bucket", objects={}, busy_after_apply=True)
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("ns/vpcs/vpc_9/default.tfstate (2 resources)", res["out"])
        self.assertIsNotNone(res["applied_main_tf"])
        self.assertFalse(self._bucket_destroyed(res))
        # The bucket stays, so its protection is applied back from the
        # tracked main.tf.
        applies = [c for c in res["tf_calls"] if c.split()[1:2] == ["apply"]]
        self.assertEqual(len(applies), 2, res["tf_calls"])
        self.assertIn("-target=google_storage_bucket.tfstate", applies[1])
        self.assertIn("  force_destroy = false\n", res["applied_main_tf"])
        self.assertIn("prevent_destroy = true", res["applied_main_tf"])
        # The copy's state is still copied back.
        self.assertEqual(res["state"], '{"bucket": "in state"}')

    def test_refuses_anything_but_an_in_place_update(self):
        # e.g. the region in tfvars no longer matches the bucket's location,
        # so lifting the protection would replace the bucket.
        for actions in (("delete", "create"), ("create",)):
            with self.subTest(actions=actions):
                res = self._run("-y --delete-state-bucket", objects={}, plan_actions=actions)
                self.assertNotEqual(res["rc"], 0)
                self.assertIn(f"would change gs://{self.BUCKET} ({','.join(actions)})", res["out"])
                self.assertIsNone(res["applied_main_tf"])
                self.assertFalse(self._bucket_destroyed(res))
                self.assertEqual(res["tmp_left"], [])

    def test_interrupt_keeps_the_recorded_state_and_removes_the_copy(self):
        res = self._run("-y --delete-state-bucket", objects={}, interrupt_on_apply=True)
        self.assertEqual(res["rc"], 130, res["out"])
        self.assertIn("Interrupted", res["out"])
        self.assertEqual(res["state"], '{"interrupted": true}\n')
        self.assertEqual(res["tmp_left"], [])
        self.assertFalse(self._bucket_destroyed(res))

    def test_typed_confirmation_is_required_without_y(self):
        for reply, deleted in (("y\n", False), (f"{self.BUCKET}\n", True)):
            with self.subTest(reply=reply):
                # `reply` answers the bucket-name prompt and "y" the final
                # Proceed prompt. The lock-file and service-account prompts
                # are not reached (lock file present, no service account).
                res = self._run(
                    "--delete-state-bucket", objects={}, stdin=reply + "y\n"
                )
                self.assertEqual(res["rc"], 0, res["out"])
                self.assertEqual(self._bucket_destroyed(res), deleted, res["out"])
                self.assertEqual(self._fe_destroyed(res), deleted, res["out"])

    def test_bucket_only_when_the_front_end_is_already_gone(self):
        res = self._run("-y --delete-state-bucket", objects={}, fe_deployed=False, lock=False)
        self.assertEqual(res["rc"], 0, res["out"])
        self.assertFalse(self._fe_destroyed(res))
        self.assertTrue(self._bucket_destroyed(res))

    def test_named_earlier_deployment_touches_only_its_bucket(self):
        res = self._run("--delete-state-bucket dep0", objects={})
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("no state bucket for deployment 'dep0'", res["out"])
        self.assertFalse(self._fe_destroyed(res))
        self.assertEqual([c for c in res["tf_calls"] if " show" in c], [])

    def test_named_deployment_still_deployed_here_is_refused(self):
        # The named form deletes only a bucket; it must never fall through to
        # tearing down the FrontEnd, even with -y.
        res = self._run("-y --delete-state-bucket dep1", objects={})
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("'dep1' may still be deployed from here", res["out"])
        self.assertFalse(self._fe_destroyed(res))
        self.assertFalse(self._bucket_destroyed(res))
        self.assertFalse(any("objects list" in c for c in res["gcloud_calls"]), res["gcloud_calls"])

    def test_named_deployment_torn_down_here_deletes_only_its_bucket(self):
        res = self._run(
            "-y --delete-state-bucket dep1", objects={}, fe_deployed=False, lock=False
        )
        self.assertEqual(res["rc"], 0, res["out"])
        self.assertFalse(self._fe_destroyed(res))
        self.assertTrue(self._bucket_destroyed(res))
        self.assertIn(f"gs://{self.BUCKET} has been deleted", res["out"])

    def _assert_refused_untouched(self, res, text):
        self.assertNotEqual(res["rc"], 0, res["out"])
        self.assertIn(text, res["out"])
        self.assertFalse(self._fe_destroyed(res))
        self.assertFalse(self._bucket_destroyed(res))
        self.assertIsNone(res["applied_main_tf"])

    def test_named_deployment_with_unreadable_front_end_state_is_refused(self):
        # A failing `terraform show` must not read as "no FrontEnd".
        res = self._run(
            "-y --delete-state-bucket dep1", objects={}, show_fails=True, lock=False
        )
        self._assert_refused_untouched(res, "'dep1' may still be deployed from here")

    def test_named_deployment_with_lock_file_is_refused(self):
        res = self._run(
            "-y --delete-state-bucket dep1", objects={}, fe_deployed=False, lock=True
        )
        self._assert_refused_untouched(res, "'dep1' may still be deployed from here")

    def test_named_deployment_shown_without_lock_is_refused(self):
        res = self._run(
            "-y --delete-state-bucket dep1", objects={}, fe_deployed=True, lock=False
        )
        self._assert_refused_untouched(res, "'dep1' may still be deployed from here")

    def test_named_deployment_with_reformatted_tfvars_is_refused(self):
        # A hand-edited name line must not make the guard skip this FrontEnd.
        res = self._run(
            "-y --delete-state-bucket dep1",
            objects={},
            fe_deployed=True,
            lock=False,
            tf_tfvars='project_id = "proj"\ndeployment_name   =   "dep1"   # edited\n',
        )
        self._assert_refused_untouched(res, "'dep1' may still be deployed from here")

    def test_named_deployment_with_unreadable_tfvars_name_is_refused(self):
        res = self._run(
            "-y --delete-state-bucket dep1",
            objects={},
            fe_deployed=False,
            lock=True,
            tf_tfvars='project_id = "proj"\n',
        )
        self._assert_refused_untouched(res, "'dep1' may still be deployed from here")

    def _assert_refused_before_prompts(self, res):
        self._assert_refused_untouched(res, "cannot confirm that no FrontEnd is deployed")
        self.assertIn("nothing\n         has been changed", res["out"])
        self.assertNotIn("Type the bucket name", res["out"])
        self.assertNotIn("This will permanently delete", res["out"])
        self.assertFalse(
            any("objects list" in c for c in res["gcloud_calls"]), res["gcloud_calls"]
        )

    def test_bare_form_refuses_before_any_prompt_or_change(self):
        # stdin answers only the "No lock file found ... Proceed anyway?" prompt.
        res = self._run(
            "--delete-state-bucket", objects={}, show_fails=True, lock=False, stdin="y\n"
        )
        self._assert_refused_before_prompts(res)

    def test_bare_form_with_lock_refuses_before_any_prompt_or_change(self):
        res = self._run("-y --delete-state-bucket", objects={}, fe_deployed=False, lock=True)
        self._assert_refused_before_prompts(res)

    def test_refusals_say_how_to_clear_a_leftover_lock(self):
        for args in ("-y --delete-state-bucket dep1", "-y --delete-state-bucket"):
            with self.subTest(args=args):
                res = self._run(args, objects={}, fe_deployed=False, lock=True)
                self.assertNotEqual(res["rc"], 0)
                self.assertIn("leftover lock file tf/.tkfe.lock and rerun", res["out"])

    def test_named_form_parses_a_reformatted_name_for_another_deployment(self):
        # tf/ names "other", so dep1's bucket is not this FrontEnd's: deleted
        # even while that FrontEnd runs. Without the parse this would refuse.
        res = self._run(
            "-y --delete-state-bucket dep1",
            objects={},
            fe_deployed=True,
            lock=True,
            tf_tfvars='project_id = "proj"\ndeployment_name   =   "other"   # edited\n',
        )
        self.assertEqual(res["rc"], 0, res["out"])
        self.assertTrue(self._bucket_destroyed(res))
        self.assertFalse(self._fe_destroyed(res))

    def test_named_form_without_tfvars_is_refused_while_front_end_may_run(self):
        res = self._run(
            "-y --delete-state-bucket dep1",
            objects={},
            fe_deployed=False,
            lock=True,
            tf_tfvars=None,
        )
        self._assert_refused_untouched(res, "'dep1' may still be deployed from here")

    def test_named_form_without_tfvars_deletes_when_nothing_was_deployed(self):
        res = self._run(
            "-y --delete-state-bucket dep1",
            objects={},
            fe_deployed=False,
            lock=False,
            tf_tfvars=None,
        )
        self.assertEqual(res["rc"], 0, res["out"])
        self.assertTrue(self._bucket_destroyed(res))

    def test_bucket_only_with_unreadable_front_end_state_is_refused(self):
        res = self._run("-y --delete-state-bucket", objects={}, show_fails=True, lock=False)
        self._assert_refused_untouched(res, "cannot confirm that no FrontEnd is deployed")

    def test_bucket_only_with_lock_file_is_refused(self):
        res = self._run("-y --delete-state-bucket", objects={}, fe_deployed=False, lock=True)
        self._assert_refused_untouched(res, "cannot confirm that no FrontEnd is deployed")

    def test_unknown_option_is_rejected(self):
        res = self._run("--delete-state-buckets")
        self.assertNotEqual(res["rc"], 0)
        self.assertIn("Unknown option", res["out"])
        self.assertEqual(res["tf_calls"], [])
