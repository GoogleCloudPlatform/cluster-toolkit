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

"""A refused operation says why on the resource's log page.

When the permission check before provisioning refuses a cluster or
filesystem operation, the "OFE: ..." line is added to the log the
resource's Terraform log page shows, and the page returns it. Offline: the
Google client libraries are replaced at their own module boundaries, and no
tool runs.
"""

import contextlib
import json
import shutil
import tempfile
from pathlib import Path
from unittest import mock

from django.template import TemplateSyntaxError
from django.test import SimpleTestCase, TestCase
from django.urls import reverse

from ghpcfe.cluster_manager import credentials
from ghpcfe.cluster_manager import filesystem as cm_filesystem
from ghpcfe.cluster_manager.clusterinfo import ClusterInfo
from ghpcfe.models import Credential, Filesystem, GCPFilestoreFilesystem
from ghpcfe.tests import test_destroy_blockers as blockers
from ghpcfe.tests.test_blueprint_identities import (
    PROFILE_CASES,
    make_world,
    scenario_minimal,
)
from ghpcfe.tests.test_credentials import ACCOUNT, KEY_JSON
from ghpcfe.tests.test_preflight import FakeGoogle
from ghpcfe.views.clusters import BackendReconfigureCluster
from ghpcfe.views.view_utils import TerraformLogFile

PRIVATE_KEY = json.loads(KEY_JSON)["private_key"]


def ofe_lines(text):
    return [line for line in text.splitlines() if line.startswith("OFE: ")]


class ShownLogFileTests(SimpleTestCase):
    """credentials.shown_log_file and record_refusal on plain directories"""

    def setUp(self):
        self.work = Path(tempfile.mkdtemp(prefix="ofe-refusal-logs-"))
        self.addCleanup(shutil.rmtree, self.work, ignore_errors=True)
        # gcluster's layout: <working dir>/<deployment>/<group>
        self.terraform_dir = self.work / "deployment" / "primary"

    def write(self, name, text, directory=None):
        path = (directory or self.terraform_dir) / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
        return path

    def test_newest_phase_first_and_stderr_only_with_text(self):
        self.write("terraform_init_log.stdout", "init\n")
        self.write("terraform_apply_log.stdout", "applied\n")
        self.write("terraform_apply_log.stderr", "")
        self.assertEqual(
            credentials.shown_log_file(self.terraform_dir).name,
            "terraform_apply_log.stdout",
        )
        self.write("terraform_apply_log.stderr", "Error: boom\n")
        self.assertEqual(
            credentials.shown_log_file(self.terraform_dir).name,
            "terraform_apply_log.stderr",
        )
        self.write("terraform_destroy_log.stdout", "destroyed\n")
        self.assertEqual(
            credentials.shown_log_file(self.terraform_dir).name,
            "terraform_destroy_log.stdout",
        )

    def test_gcluster_log_before_terraform_has_run(self):
        self.assertIsNone(credentials.shown_log_file(self.terraform_dir))
        self.write("ghpc_create_log.stderr", "Error: blueprint\n", self.work)
        self.assertEqual(
            credentials.shown_log_file(self.terraform_dir),
            self.work / "ghpc_create_log.stderr",
        )
        # A Terraform log wins as soon as there is one
        self.write("terraform_init_log.stdout", "init\n")
        self.assertEqual(
            credentials.shown_log_file(self.terraform_dir).name,
            "terraform_init_log.stdout",
        )

    def test_the_log_page_shows_the_same_file(self):
        log_page = TerraformLogFile(self.terraform_dir)
        self.assertFalse(log_page.exists())
        self.write("ghpc_create_log.stdout", "created\n", self.work)
        self.assertEqual(log_page.get_file(), self.work / "ghpc_create_log.stdout")
        self.write("terraform_plan_log.stdout", "planned\n")
        self.assertEqual(
            log_page.get_file(), credentials.shown_log_file(self.terraform_dir)
        )

    def test_appends_to_the_shown_log(self):
        shown = self.write("terraform_apply_log.stdout", "Apply complete!\n")
        written = credentials.record_refusal(
            self.terraform_dir, credentials.CredentialError("refused"), "apply"
        )
        self.assertEqual(written, shown)
        self.assertEqual(shown.read_text(), "Apply complete!\n\nOFE: refused\n")
        credentials.record_refusal(
            self.terraform_dir, credentials.CredentialError("again"), "apply"
        )
        self.assertEqual(ofe_lines(shown.read_text()), ["OFE: refused", "OFE: again"])

    def test_starts_the_phase_log_in_a_terraform_dir_without_logs(self):
        self.terraform_dir.mkdir(parents=True)
        written = credentials.record_refusal(
            self.terraform_dir, credentials.CredentialError("refused"), "destroy"
        )
        self.assertEqual(written, self.terraform_dir / "terraform_destroy_log.stderr")
        self.assertEqual(credentials.shown_log_file(self.terraform_dir), written)

    def test_never_creates_the_deployment_dir(self):
        written = credentials.record_refusal(
            self.terraform_dir, credentials.CredentialError("refused"), "apply"
        )
        self.assertEqual(written, self.work / "ghpc_create_log.stderr")
        self.assertEqual(ofe_lines(written.read_text()), ["OFE: refused"])
        # gcluster refuses a deployment directory it did not create
        self.assertFalse(self.terraform_dir.parent.exists())

    def test_creates_the_working_dir(self):
        shutil.rmtree(self.work)
        written = credentials.record_refusal(
            self.terraform_dir, credentials.CredentialError("refused"), "apply"
        )
        self.assertEqual(written, self.work / "ghpc_create_log.stderr")
        self.assertFalse(self.terraform_dir.parent.exists())

    def test_private_key_text_is_removed(self):
        err = credentials.CredentialError(
            "Could not check the permissions: bad key "
            + json.dumps({"private_key": PRIVATE_KEY, "client_email": ACCOUNT})
            + " and "
            + PRIVATE_KEY
            # Built from parts so the file holds no literal key marker
            + " and a cut-off -----BEGIN "
            + "PRIVATE KEY-----\nMIIEv"
        )
        written = credentials.record_refusal(self.terraform_dir, err, "apply")
        text = written.read_text()
        self.assertNotIn("MII", text)
        self.assertNotIn("PRIVATE KEY", text)
        self.assertIn('"private_key": "[removed]"', text)
        self.assertIn(ACCOUNT, text)

    def test_a_log_that_cannot_be_written_is_only_a_warning(self):
        # The working directory is a file, so no log can go below it
        self.work.rmdir()
        self.work.write_text("not a directory")
        with self.assertLogs(
            "ghpcfe.cluster_manager.credentials", "WARNING"
        ) as logs:
            written = credentials.record_refusal(
                self.terraform_dir, credentials.CredentialError("refused"), "apply"
            )
        self.assertIsNone(written)
        self.assertIn("Could not add the refusal", "\n".join(logs.output))
        self.work.unlink()


class LogPageMixin:
    """Fetch a log page as the world's owner"""

    def page_status(self, url):
        self.client.force_login(self.owner)
        return self.client.get(url).status_code

    def page_text(self, url):
        self.client.force_login(self.owner)
        response = self.client.get(url)
        self.assertEqual(response.status_code, 200)
        try:
            return b"".join(response.streaming_content).decode("utf-8")
        finally:
            response.close()


class ClusterRefusalLogTests(LogPageMixin, TestCase):
    """A refused cluster create, update or destroy is on the cluster's
    Terraform log page"""

    def setUp(self):
        world = make_world()
        self.owner = world[0]
        self.cluster = scenario_minimal(world, "legacy")
        # A key for ACCOUNT, whose identity has no permission at all
        self.cluster.cloud_credential = Credential.objects.create(
            name="no-permissions", owner=self.owner, detail=KEY_JSON
        )
        self.cluster.save()
        self.info = ClusterInfo(self.cluster)
        # Cluster ids restart in each test's database, so a directory can be
        # left by an earlier test or run
        shutil.rmtree(self.info.cluster_dir, ignore_errors=True)
        self.addCleanup(shutil.rmtree, self.info.cluster_dir, ignore_errors=True)
        self.terraform_dir = self.info.get_terraform_dir()
        self.google = FakeGoogle(set())
        patcher = mock.patch("subprocess.run")
        self.subprocess_run = patcher.start()
        self.addCleanup(patcher.stop)

    def refused(self, func, error=credentials.PermissionGapError):
        with self.assertRaises(error) as caught:
            self.google.run(func)
        self.subprocess_run.assert_not_called()
        return caught.exception

    def start_refused(self, error=credentials.PermissionGapError):
        # Only the refusal: no SSH key and no blueprint or bucket upload
        with mock.patch.object(ClusterInfo, "_create_ssh_key"), mock.patch.object(
            ClusterInfo, "update"
        ):
            return self.refused(self.info.start_cluster, error=error)

    def log_page(self):
        return self.page_text(
            reverse("cluster-log-file", args=[self.cluster.id, 0])
        )

    def write_log(self, name, text):
        self.terraform_dir.mkdir(parents=True, exist_ok=True)
        (self.terraform_dir / name).write_text(text, encoding="utf-8")

    def assert_on_page(self, error, before=""):
        """The page shows what it showed before, then the refusal"""
        text = self.log_page()
        self.assertTrue(text.startswith(before))
        self.assertEqual(ofe_lines(text), [f"OFE: {error}"])
        self.assertTrue(text.rstrip("\n").endswith(f"OFE: {error}"))
        self.assertIn(f"Identity {ACCOUNT} lacks permission", text)
        self.assertNotIn(PRIVATE_KEY, text)

    def test_first_create(self):
        error = self.start_refused()
        self.assertIn(f"needed to create cluster {self.cluster.name}", str(error))
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "e")
        # The page's "View Logs" shows for every status but new
        self.assertEqual(
            self.page_status(reverse("cluster-log", args=[self.cluster.id])), 200
        )
        self.assert_on_page(error)
        self.assertTrue((self.info.cluster_dir / "ghpc_create_log.stderr").is_file())
        # gcluster can still create the deployment on a retry
        self.assertFalse(self.terraform_dir.parent.exists())

    def test_retried_create_after_a_failed_apply(self):
        before = "Error: googleapi: Error 409: already exists\n"
        self.write_log("terraform_apply_log.stdout", "Plan: 3 to add\n")
        self.write_log("terraform_apply_log.stderr", before)
        self.cluster.status = "e"
        self.cluster.save()
        error = self.start_refused()
        self.assert_on_page(error, before)

    def test_create_after_a_destroy(self):
        before = "Destroy complete! Resources: 12 destroyed.\n"
        self.write_log("terraform_apply_log.stdout", "Apply complete!\n")
        self.write_log("terraform_destroy_log.stdout", before)
        self.write_log("terraform_destroy_log.stderr", "")
        self.cluster.status = "d"
        self.cluster.save()
        error = self.start_refused()
        self.assert_on_page(error, before)

    def test_update(self):
        before = "Apply complete! Resources: 12 added.\n"
        self.write_log("terraform_apply_log.stdout", before)
        self.write_log("terraform_apply_log.stderr", "")
        self.cluster.status = "r"
        self.cluster.save()
        # Only the refusal: no blueprint or bucket upload
        with mock.patch.object(ClusterInfo, "update"):
            error = self.refused(self.info.reconfigure_cluster)
        self.assertIn(f"needed to update cluster {self.cluster.name}", str(error))
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "e")
        self.assert_on_page(error, before)

    def test_destroy(self):
        before = "Apply complete! Resources: 12 added.\n"
        self.write_log("terraform_apply_log.stdout", before)
        self.cluster.status = "r"
        self.cluster.save()
        error = self.refused(self.info.stop_cluster)
        self.assertIn(f"needed to destroy cluster {self.cluster.name}", str(error))
        self.cluster.refresh_from_db()
        # Nothing was destroyed
        self.assertEqual(self.cluster.status, "r")
        self.assert_on_page(error, before)

    def test_a_check_that_cannot_run(self):
        def cannot_run(creds, check):
            raise RuntimeError("the IAM API is not enabled")

        with mock.patch.dict(credentials.DEFAULT_TESTERS, {"project": cannot_run}):
            error = self.start_refused(error=credentials.CredentialError)
        self.assertIn("Could not check the permissions", str(error))
        self.assertIn(f"OFE: {error}", self.log_page())


class ClusterPrepareFailureTests(LogPageMixin, TestCase):
    """A blueprint that cannot be prepared fails the start or reconfigure,
    and the cluster's log page says why"""

    RENDER_ERROR = "Invalid block tag on line 3: 'endfor'"

    def setUp(self):
        world = make_world()
        self.owner = world[0]
        self.cluster = PROFILE_CASES["minimal"](world)
        self.info = ClusterInfo(self.cluster)
        # Cluster ids restart in each test's database
        shutil.rmtree(self.info.cluster_dir, ignore_errors=True)
        self.addCleanup(shutil.rmtree, self.info.cluster_dir, ignore_errors=True)
        self.terraform_dir = self.info.get_terraform_dir()
        # Nothing may run or reach Google
        self.subprocess_run = self.patch("subprocess.run")
        self.gcs_upload = self.patch(
            "ghpcfe.cluster_manager.cloud_info.gcs_upload_file"
        )
        # The real permission check runs if it is reached: only the Google
        # client libraries are replaced, granting nothing, and self.google
        # records every permission the check asks Google about
        self.google = FakeGoogle(set())
        for patcher in self.google.patches():
            patcher.start()
            self.addCleanup(patcher.stop)

    def patch(self, target):
        patcher = mock.patch(target)
        self.addCleanup(patcher.stop)
        return patcher.start()

    def assert_permission_check_reached(self):
        """What assert_failed rules out: the check asked Google and refused,
        and gcluster and Terraform still did not run"""
        project = self.cluster.cloud_credential.project_id
        self.assertEqual(self.google.asked[0][0], f"projects/{project}")
        self.subprocess_run.assert_not_called()
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "e")

    def render_error(self):
        return mock.patch.object(
            ClusterInfo,
            "_prepare_ghpc_partitions",
            side_effect=TemplateSyntaxError(self.RENDER_ERROR),
        )

    def identity_error(self):
        # A profiles cluster without its Slurm cluster name cannot get its
        # accounts: _identity_context raises ValueError
        self.cluster.slurm_cluster_name = ""
        self.cluster.save()
        return contextlib.nullcontext()

    def start(self, failure, error):
        with failure, mock.patch.object(
            ClusterInfo, "_create_ssh_key"
        ), self.assertLogs("ghpcfe.cluster_manager.clusterinfo", level="ERROR"):
            with self.assertRaises(error):
                self.info.start_cluster()

    def reconfigure(self, failure, error):
        """What the update page's reconfigure task runs"""
        before = "Apply complete! Resources: 12 added.\n"
        self.info._create_cluster_dir()  # pylint: disable=protected-access
        self.terraform_dir.mkdir(parents=True)
        (self.terraform_dir / "terraform_apply_log.stdout").write_text(before)
        self.cluster.status = "r"
        self.cluster.cloud_state = "m"
        self.cluster.save()
        with failure, self.assertLogs(
            "ghpcfe.cluster_manager.clusterinfo", level="ERROR"
        ):
            with self.assertRaises(error):
                BackendReconfigureCluster().cmd(None, None, self.cluster)
        return before

    def assert_failed(self, verb, reason, before="", cloud_state="nm"):
        """A cluster being created stays "nm"; one being reconfigured was
        running and still exists, so it stays managed ("m")"""
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "e")
        self.assertEqual(self.cluster.cloud_state, cloud_state)
        # Stopped before the permission check, gcluster and Terraform
        self.assertEqual(self.google.asked, [])
        self.subprocess_run.assert_not_called()
        self.gcs_upload.assert_not_called()
        text = self.page_text(reverse("cluster-log-file", args=[self.cluster.id, 0]))
        self.assertTrue(text.startswith(before))
        (line,) = ofe_lines(text)
        self.assertTrue(
            line.startswith(
                f"OFE: Cannot {verb} cluster {self.cluster.name} "
                f"({self.cluster.cloud_id}): preparing its blueprint or startup "
                "scripts failed: "
            ),
            line,
        )
        self.assertIn(reason, line)

    def test_render_error_on_start(self):
        self.start(self.render_error(), TemplateSyntaxError)
        self.assert_failed("create", self.RENDER_ERROR)

    def test_identity_error_on_start(self):
        self.start(self.identity_error(), ValueError)
        self.assert_failed("create", "has no slurm_cluster_name")

    def test_render_error_on_reconfigure(self):
        before = self.reconfigure(self.render_error(), TemplateSyntaxError)
        self.assert_failed("update", self.RENDER_ERROR, before, cloud_state="m")

    def test_identity_error_on_reconfigure(self):
        before = self.reconfigure(self.identity_error(), ValueError)
        self.assert_failed("update", "has no slurm_cluster_name", before, cloud_state="m")

    def startup_script_inputs(self):
        """What update() needs to render the startup scripts: the real
        cluster startup templates where it reads them, and the server's C2
        Pub/Sub paths, which the server's C2 connection gives (not started
        in tests)"""
        ofe_dir = Path(__file__).resolve().parents[3]
        relative = Path("infrastructure_files") / "cluster_startup" / "templates"
        base_dir = Path(self.info.config["baseDir"])
        self.addCleanup(
            shutil.rmtree, base_dir / "infrastructure_files", ignore_errors=True
        )
        shutil.copytree(ofe_dir / relative, base_dir / relative, dirs_exist_ok=True)
        topic = self.patch("ghpcfe.cluster_manager.c2.get_topic_path")
        topic.return_value = "projects/example-project/topics/ofe-c2"
        subscription = self.patch(
            "ghpcfe.cluster_manager.c2.get_cluster_subscription_path"
        )
        subscription.return_value = "projects/example-project/subscriptions/ofe-c2-1"

    def test_start_that_prepares_reaches_the_permission_check(self):
        self.startup_script_inputs()
        with mock.patch.object(ClusterInfo, "_create_ssh_key"):
            with self.assertRaises(credentials.PermissionGapError):
                self.info.start_cluster()
        self.assert_permission_check_reached()

    def test_reconfigure_that_prepares_reaches_the_permission_check(self):
        self.startup_script_inputs()
        self.info._create_cluster_dir()  # pylint: disable=protected-access
        self.cluster.status = "r"
        self.cluster.cloud_state = "m"
        self.cluster.save()
        with self.assertRaises(credentials.PermissionGapError):
            BackendReconfigureCluster().cmd(None, None, self.cluster)
        self.assert_permission_check_reached()


class FilesystemRefusalLogTests(LogPageMixin, TestCase):
    """A refused filesystem create or destroy is on the filesystem's page,
    in its Terraform log"""

    def setUp(self):
        self.owner, _, subnet = make_world()
        self.owner.is_superuser = True
        self.owner.save(update_fields=["is_superuser"])
        credential = Credential.objects.create(
            name="no-permissions", owner=self.owner, detail=KEY_JSON
        )
        created = GCPFilestoreFilesystem.objects.create(
            name="fs-a",
            cloud_credential=credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
            vpc=subnet.vpc,
            capacity=1024,
            cloud_state="nm",
        )
        self.fs = Filesystem.objects.get(pk=created.pk)
        self.terraform_dir = cm_filesystem.get_terraform_dir(self.fs)
        self.work_dir = self.terraform_dir.parent.parent
        # Filesystem ids restart in each test's database
        shutil.rmtree(self.work_dir, ignore_errors=True)
        self.addCleanup(shutil.rmtree, self.work_dir, ignore_errors=True)
        self.google = FakeGoogle(set())
        patcher = mock.patch("subprocess.run")
        self.subprocess_run = patcher.start()
        self.addCleanup(patcher.stop)

    def refused(self, func):
        with self.assertRaises(credentials.PermissionGapError) as caught:
            self.google.run(func)
        self.subprocess_run.assert_not_called()
        return caught.exception

    def log_page(self):
        return self.page_text(reverse("fs-log", args=[self.fs.id]))

    def set_state(self, cloud_state):
        self.fs.cloud_state = cloud_state
        self.fs.save()

    def test_first_create(self):
        self.work_dir.mkdir(parents=True)
        error = self.refused(lambda: cm_filesystem.start_filesystem(self.fs))
        self.assertEqual(Filesystem.objects.get(pk=self.fs.pk).cloud_state, "nm")
        text = self.log_page()
        self.assertEqual(ofe_lines(text), [f"OFE: {error}"])
        self.assertIn("needed to create filesystem fs-a", text)
        self.assertNotIn(PRIVATE_KEY, text)
        # gcluster -w refuses an existing deployment dir it did not create
        self.assertFalse(self.terraform_dir.parent.exists())

    def test_gcluster_create_overwrites_an_earlier_attempt(self):
        # A retry after a failed create reuses the deployment folder
        self.work_dir.mkdir(parents=True)
        with mock.patch.object(
            credentials,
            "tool_environment",
            return_value=contextlib.nullcontext({}),
        ):
            cm_filesystem._run_ghpc(  # pylint: disable=protected-access
                self.work_dir, self.fs.cloud_credential
            )
        command = self.subprocess_run.call_args.args[0]
        self.assertEqual(command[1:], ["create", "filesystem.yaml", "-w"])

    def test_retried_create_after_a_failed_apply(self):
        before = "Error: googleapi: Error 400: bad tier\n"
        self.terraform_dir.mkdir(parents=True)
        (self.terraform_dir / "terraform_apply_log.stderr").write_text(before)
        error = self.refused(lambda: cm_filesystem.start_filesystem(self.fs))
        text = self.log_page()
        self.assertTrue(text.startswith(before))
        self.assertEqual(ofe_lines(text), [f"OFE: {error}"])

    def test_destroy(self):
        before = "Apply complete! Resources: 1 added.\n"
        self.terraform_dir.mkdir(parents=True)
        (self.terraform_dir / "terraform_apply_log.stdout").write_text(before)
        self.set_state("dm")
        error = self.refused(lambda: cm_filesystem.destroy_filesystem(self.fs))
        self.assertIn("needed to destroy filesystem fs-a", str(error))
        self.assertEqual(Filesystem.objects.get(pk=self.fs.pk).cloud_state, "m")
        text = self.log_page()
        self.assertTrue(text.startswith(before))
        self.assertEqual(ofe_lines(text), [f"OFE: {error}"])

    def test_imported_destroy_writes_nothing(self):
        # An imported filesystem's page shows no log
        self.set_state("dm")
        self.refused(lambda: cm_filesystem.destroy_filesystem(self.fs))
        self.assertEqual(Filesystem.objects.get(pk=self.fs.pk).cloud_state, "i")
        self.assertFalse(self.work_dir.exists())


class LeftoverRolesLogPageTests(LogPageMixin, TestCase):
    """The custom roles a finished teardown left are named on the cluster's
    Terraform log page"""

    # The teardown set-up and stand-in Terraform of the destroy blocker
    # tests, without running those tests again
    setUp = blockers.DestroyBlockerTests.setUp
    destroy = blockers.DestroyBlockerTests.destroy

    def test_role_line_is_on_the_log_page(self):
        with self.assertLogs("ghpcfe.cluster_manager.clusterinfo", "WARNING"):
            self.destroy(blockers.ROLES_UNAVAILABLE, blockers.ROLES_UNAVAILABLE)
        self.cluster.refresh_from_db()
        self.assertEqual(self.cluster.status, "d")
        text = self.page_text(
            reverse("cluster-log-file", args=[self.cluster.id, 0])
        )
        self.assertTrue(text.startswith(
            (blockers.FIXTURES / "ctl_roles_unavailable.stderr").read_text()
        ))
        lines = ofe_lines(text)
        self.assertEqual(len(lines), 1)
        self.assertIn(
            f"gcloud iam roles delete {blockers.CTL_ROLE} "
            f"--project={blockers.PROJECT}",
            lines[0],
        )
