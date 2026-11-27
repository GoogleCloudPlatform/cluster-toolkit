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

"""Two requests setting up one network's working directory at once.

Saving the subnet form straight after creating a network ran create_subnet
while the "Create VPC" background task was still in its permission
preflight. Both saw no vpcs/vpc_<id> directory, both copied the templates
into it, and the second copy failed with FileExistsError (HTTP 500).

The races are replayed with threads and events, never sleeps. The
background task's create_vpc runs in a worker thread and either waits
inside its preflight until the request in the main thread reaches its own
preflight (race), or is paused inside its template copy, holding the
directory lock, until the request is about to take that lock
(race_during_set_up). Every wait ends: each event the main thread waits
for is also set when the worker finishes, the worker is always resumed
before it is joined, and after that it only waits for a lock the main
thread releases when its call returns. Every event wait and join is also
bounded by WAIT_SECONDS and fails the test when the bound passes, so code
that deadlocks (for example a preflight moved under the lock, where the
worker holds the lock while it waits for the main thread, which waits for
the lock) fails the test instead of hanging the suite. Offline: preflight,
Terraform and the state namespace are replaced.

Separately, a failed init, validate or plan leaves the directory not set
up, so the next caller runs them again.

start_vpc's apply and state read-back, and destroy_vpc's destroy, hold the
same lock, so a subnet saved or deleted meanwhile waits for them instead of
changing the files Terraform is reading. Replayed the same way: the worker
is paused inside the Terraform command, and the request in the main thread
first tries the lock without waiting, which records whether it was held,
and only then waits for it. The subnet pages refuse a network being created
or destroyed, so a request does not wait minutes for that lock.
"""

import fcntl
import json
import shutil
import subprocess
import tempfile
import threading
from pathlib import Path
from unittest import mock

from django.contrib.messages import get_messages
from django.db.models import prefetch_related_objects
from django.test import TestCase
from django.urls import reverse

from ghpcfe.cluster_manager import credentials
from ghpcfe.cluster_manager import utils
from ghpcfe.cluster_manager import vpc as cm_vpc
from ghpcfe.models import Credential, Role, User, VirtualNetwork, VirtualSubnet
from ghpcfe.tests.test_blueprint_identities import make_world

PREFLIGHT = "ghpcfe.cluster_manager.credentials.preflight"
RUN_TERRAFORM = "ghpcfe.cluster_manager.utils.run_terraform"
READY = ".ofe-vpc-ready"
SETTING_UP = ".ofe-vpc-setting-up"

# Bound on each wait for another thread: a deadlock must fail the test, not
# hang the suite. Every wait normally ends within milliseconds;
# the bound is generous so a slow or loaded machine never fails a correct
# run.
WAIT_SECONDS = 30

SUBNET_TEMPLATE = (
    'resource "google_compute_subnetwork" "subnet_{SUBNET_ID}" {\n'
    '  ip_cidr_range = "{CIDR_TEXT}"\n'
    "  private_ip_google_access = {PRIVATE_GOOGLE_ACCESS_ENABLED}\n"
    "}\n"
)


class NetworkDirectoryRaceTests(TestCase):
    """create_vpc and create_subnet set up a network's directory once, and
    again only when an earlier set-up did not finish"""

    def setUp(self):
        owner, _, _ = make_world()
        credential = Credential.objects.create(
            name="race-credential",
            owner=owner,
            detail=json.dumps(
                {"type": "service_account", "project_id": "race-project"}
            ),
        )
        self.vpc = VirtualNetwork.objects.create(
            name="net-race",
            cloud_credential=credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
            cloud_state="nm",
        )
        self.subnet = VirtualSubnet.objects.create(
            name="subnet-race",
            vpc=self.vpc,
            cidr="10.20.0.0/24",
            cloud_credential=credential,
            cloud_region="us-central1",
            cloud_state="nm",
        )
        # Load the related rows here: the worker thread must not query the
        # test database, whose rows only this thread's transaction sees
        self.assertEqual(self.subnet.vpc.cloud_credential, credential)
        self.assertEqual(self.vpc.cloud_credential, credential)

        self.target_dir = cm_vpc._tf_dir_for_vpc(self.vpc.id)  # pylint: disable=protected-access
        shutil.rmtree(self.target_dir, ignore_errors=True)
        self.addCleanup(shutil.rmtree, self.target_dir, ignore_errors=True)

        tmp = Path(tempfile.mkdtemp(prefix="ofe-vpc-race-"))
        self.addCleanup(shutil.rmtree, tmp, ignore_errors=True)
        source = tmp / "vpc_tf"
        source.mkdir()
        (source / "main.tf").write_text("# network stand-in\n", encoding="utf-8")
        (source / "subnet.tf.template").write_text(
            SUBNET_TEMPLATE, encoding="utf-8"
        )

        real_copytree = shutil.copytree
        self.copytree = mock.Mock(side_effect=real_copytree)
        for patcher in (
            mock.patch.object(
                cm_vpc, "_tf_source_dir_for_vpc", return_value=source
            ),
            mock.patch.object(
                utils, "get_state_namespace", return_value="race-ns"
            ),
            mock.patch.object(
                utils, "get_terraform_state_bucket", return_value="race-bucket"
            ),
            mock.patch.object(cm_vpc.shutil, "copytree", self.copytree),
        ):
            patcher.start()
            self.addCleanup(patcher.stop)
        run = mock.patch(RUN_TERRAFORM, side_effect=self.terraform)
        self.run_terraform = run.start()
        self.addCleanup(run.stop)

        self.worker = None
        self.worker_errors = []
        # set by the worker when it is in its preflight, or is done
        self.in_preflight = threading.Event()
        # set by the main thread's preflight: the worker may leave its own
        self.go = threading.Event()
        # set by the worker when it is about to take the lock, or is done
        self.progress = threading.Event()

    @staticmethod
    def terraform(target_dir, command, **unused_kwargs):
        """Stand-in for run_terraform: init creates .terraform, as the
        real one does before it can fail on a provider or the plan"""
        if command == "init":
            (Path(target_dir) / ".terraform").mkdir(exist_ok=True)

    def wait(self, event, what):
        """Wait for event, or fail (in any thread) after WAIT_SECONDS, so a
        deadlock fails the test instead of hanging the suite"""
        if not event.wait(WAIT_SECONDS):
            raise self.failureException(
                f"no {what} within {WAIT_SECONDS}s: deadlock?"
            )

    def preflight(self, *unused_args, **unused_kwargs):
        if threading.current_thread() is self.worker:
            self.in_preflight.set()
            self.wait(self.go, "preflight in the request")
        else:
            self.go.set()
            self.wait(self.progress, "progress in the background task")

    def flock(self, fd, operation):
        if (
            threading.current_thread() is self.worker
            and operation == fcntl.LOCK_EX
        ):
            self.progress.set()
        return self.real_flock(fd, operation)

    def start_worker(self, target):
        def run():
            try:
                target()
            except Exception as err:  # pylint: disable=broad-except
                # Re-raised as a test failure by finish_worker
                self.worker_errors.append(err)
            finally:
                self.in_preflight.set()
                self.progress.set()

        self.worker = threading.Thread(target=run, name="create-vpc-task")
        self.worker.start()

    def finish_worker(self):
        self.go.set()
        # bounded, like every wait here: a deadlock fails, never hangs
        self.worker.join(WAIT_SECONDS)
        self.assertFalse(
            self.worker.is_alive(),
            f"background task still running after {WAIT_SECONDS}s: deadlock?",
        )
        self.assertEqual(self.worker_errors, [])

    def race(self, background, request):
        """Run background in the worker, held in its preflight, and request
        in this thread while it is held. The request starts only once the
        worker is in its preflight, so the order is the same every run."""
        self.real_flock = fcntl.flock
        with mock.patch(PREFLIGHT, side_effect=self.preflight), mock.patch.object(
            fcntl, "flock", side_effect=self.flock
        ):
            self.start_worker(background)
            try:
                self.wait(self.in_preflight, "preflight in the background task")
                request()
            finally:
                self.finish_worker()

    def terraform_commands(self):
        return [c.args[1] for c in self.run_terraform.call_args_list]

    def assert_set_up_once(self):
        self.assertEqual(self.copytree.call_count, 1)
        self.assertEqual(
            self.terraform_commands(), ["init", "validate", "plan"]
        )
        self.assertTrue((self.target_dir / "terraform.tfvars").is_file())
        self.assertTrue((self.target_dir / "backend.tf").is_file())
        self.assertFalse((self.target_dir / "terraform.tfvars.partial").exists())
        self.assertTrue((self.target_dir / READY).is_file())
        self.assertFalse((self.target_dir / f"{READY}.partial").exists())
        self.assertFalse((self.target_dir / SETTING_UP).exists())

    def test_second_create_vpc_finds_the_directory_set_up(self):
        self.race(
            lambda: cm_vpc.create_vpc(self.vpc),
            lambda: cm_vpc.create_vpc(self.vpc),
        )
        self.assert_set_up_once()

    def test_subnet_saved_during_create_vpc_is_written(self):
        self.race(
            lambda: cm_vpc.create_vpc(self.vpc),
            lambda: cm_vpc.create_subnet(self.subnet),
        )
        self.assert_set_up_once()
        subnet_file = self.target_dir / f"subnet-{self.subnet.id}.tf"
        content = subnet_file.read_text(encoding="utf-8")
        self.assertIn(f"subnet_{self.subnet.id}", content)
        self.assertIn('"10.20.0.0/24"', content)

    def race_during_set_up(self, request):
        """Run create_vpc in the worker, paused inside its template copy
        (past its set-up check, holding the lock), and request in this
        thread. Only the lock keeps request out: when request is about to
        take it, the worker resumes."""
        in_copy = threading.Event()
        resume = threading.Event()
        real_copytree = self.copytree.side_effect
        real_flock = fcntl.flock

        def copytree(*args, **kwargs):
            if threading.current_thread() is self.worker:
                in_copy.set()
                self.wait(resume, "request taking the lock")
            return real_copytree(*args, **kwargs)

        def flock(fd, operation):
            if (
                threading.current_thread() is not self.worker
                and operation == fcntl.LOCK_EX
            ):
                resume.set()
            return real_flock(fd, operation)

        def background():
            try:
                cm_vpc.create_vpc(self.vpc)
            finally:
                in_copy.set()

        self.copytree.side_effect = copytree
        with mock.patch(PREFLIGHT), mock.patch.object(
            fcntl, "flock", side_effect=flock
        ):
            self.start_worker(background)
            try:
                self.wait(in_copy, "template copy in the background task")
                request()
            finally:
                resume.set()
                self.finish_worker()

    def test_second_create_vpc_waits_for_a_set_up_in_progress(self):
        self.race_during_set_up(lambda: cm_vpc.create_vpc(self.vpc))
        self.assert_set_up_once()

    def test_subnet_waits_for_a_set_up_in_progress(self):
        self.race_during_set_up(lambda: cm_vpc.create_subnet(self.subnet))
        self.assert_set_up_once()
        self.assertTrue(
            (self.target_dir / f"subnet-{self.subnet.id}.tf").is_file()
        )

    def test_create_vpc_again_leaves_a_set_up_directory_alone(self):
        with mock.patch(PREFLIGHT):
            cm_vpc.create_vpc(self.vpc)
            cm_vpc.create_vpc(self.vpc)
        self.assert_set_up_once()

    def test_directory_left_part_way_is_completed(self):
        # A crash after the template copy and before terraform.tfvars
        (self.target_dir).mkdir(parents=True)
        (self.target_dir / "main.tf").write_text("# partial\n", encoding="utf-8")
        with mock.patch(PREFLIGHT) as preflight:
            cm_vpc.create_subnet(self.subnet)
        preflight.assert_called_once()
        self.assert_set_up_once()
        self.assertEqual(
            (self.target_dir / "main.tf").read_text(encoding="utf-8"),
            "# network stand-in\n",
        )
        self.assertTrue(
            (self.target_dir / f"subnet-{self.subnet.id}.tf").is_file()
        )

    # A failed init, validate or plan leaves the directory to be set up
    # again; directories from earlier OFE versions are recognised

    def failing_on(self, failing):
        """run_terraform stand-in that fails on one command"""

        def run(target_dir, command, **kwargs):
            self.terraform(target_dir, command, **kwargs)
            if command == failing:
                raise subprocess.CalledProcessError(1, ["terraform", command])

        return run

    def is_set_up(self):
        return cm_vpc._vpc_dir_is_set_up(self.target_dir)  # pylint: disable=protected-access

    def test_failed_terraform_command_is_run_again(self):
        for failing in ("init", "validate", "plan"):
            with self.subTest(failing=failing):
                shutil.rmtree(self.target_dir, ignore_errors=True)
                self.copytree.reset_mock()
                self.run_terraform.reset_mock()
                self.run_terraform.side_effect = self.failing_on(failing)
                with mock.patch(PREFLIGHT):
                    with self.assertRaises(subprocess.CalledProcessError):
                        cm_vpc.create_vpc(self.vpc)
                    self.assertFalse((self.target_dir / READY).exists())
                    self.assertFalse(self.is_set_up())

                    self.copytree.reset_mock()
                    self.run_terraform.reset_mock()
                    self.run_terraform.side_effect = self.terraform
                    cm_vpc.create_vpc(self.vpc)
                self.assert_set_up_once()

    def test_subnet_after_a_failed_init_runs_it_again(self):
        self.run_terraform.side_effect = self.failing_on("init")
        with mock.patch(PREFLIGHT):
            with self.assertRaises(subprocess.CalledProcessError):
                cm_vpc.create_vpc(self.vpc)
            self.copytree.reset_mock()
            self.run_terraform.reset_mock()
            self.run_terraform.side_effect = self.terraform
            cm_vpc.create_subnet(self.subnet)
        self.assert_set_up_once()
        self.assertTrue(
            (self.target_dir / f"subnet-{self.subnet.id}.tf").is_file()
        )

    def test_not_set_up_until_plan_has_succeeded(self):
        seen = []

        def run(target_dir, command, **kwargs):
            self.terraform(target_dir, command, **kwargs)
            seen.append(
                (command, (self.target_dir / READY).exists(), self.is_set_up())
            )

        self.run_terraform.side_effect = run
        with mock.patch(PREFLIGHT):
            cm_vpc.create_vpc(self.vpc)
        self.assertEqual(
            seen,
            [
                ("init", False, False),
                ("validate", False, False),
                ("plan", False, False),
            ],
        )
        self.assert_set_up_once()
        self.assertTrue(self.is_set_up())

    def write_earlier_version_directory(self, initialised):
        self.target_dir.mkdir(parents=True)
        (self.target_dir / "main.tf").write_text("# earlier\n", encoding="utf-8")
        (self.target_dir / "subnet.tf.template").write_text(
            SUBNET_TEMPLATE, encoding="utf-8"
        )
        (self.target_dir / "terraform.tfvars").write_text(
            'project = "race-project"\n', encoding="utf-8"
        )
        if initialised:
            (self.target_dir / ".terraform").mkdir()

    def test_earlier_version_directory_is_left_alone(self):
        self.write_earlier_version_directory(initialised=True)
        with mock.patch(PREFLIGHT) as preflight:
            cm_vpc.create_vpc(self.vpc)
            cm_vpc.create_subnet(self.subnet)
        preflight.assert_called_once()  # create_vpc's own, before the lock
        self.copytree.assert_not_called()
        self.run_terraform.assert_not_called()
        self.assertEqual(
            (self.target_dir / "main.tf").read_text(encoding="utf-8"),
            "# earlier\n",
        )
        self.assertTrue(
            (self.target_dir / f"subnet-{self.subnet.id}.tf").is_file()
        )

    def test_earlier_version_directory_never_initialised_is_set_up(self):
        self.write_earlier_version_directory(initialised=False)
        with mock.patch(PREFLIGHT):
            cm_vpc.create_subnet(self.subnet)
        self.assert_set_up_once()
        self.assertEqual(
            (self.target_dir / "main.tf").read_text(encoding="utf-8"),
            "# network stand-in\n",
        )

    # An imported network (one OFE does not manage) is read with a data
    # block instead of the template's main.tf

    def create_imported_vpc(self):
        """create_vpc for this network as an imported one. The data block
        is written by a stand-in: generate_vpc_tf_datablock reads a field
        networks do not have, which fails the same way before this change."""
        self.vpc.cloud_state = "i"
        self.vpc.save()

        def datablock(unused_vpc, target_dir):
            (target_dir / "vpc.tf").write_text("# data block\n", encoding="utf-8")

        # The files each Terraform command sees: main.tf must already be
        # gone and the data block written before init
        self.files_at = {}

        def terraform(target_dir, command, **kwargs):
            self.files_at[command] = {
                p.name for p in Path(target_dir).iterdir() if p.is_file()
            }
            self.terraform(target_dir, command, **kwargs)

        self.run_terraform.side_effect = terraform
        with mock.patch(PREFLIGHT), mock.patch.object(
            cm_vpc, "generate_vpc_tf_datablock", side_effect=datablock
        ) as generate:
            cm_vpc.create_vpc(self.vpc)
        generate.assert_called_once_with(self.vpc, self.target_dir)

    def assert_imported_set_up(self):
        self.assert_set_up_once()
        for command in ("init", "validate", "plan"):
            self.assertNotIn("main.tf", self.files_at[command], command)
            self.assertIn("vpc.tf", self.files_at[command], command)
            self.assertIn("terraform.tfvars", self.files_at[command], command)
        self.assertFalse((self.target_dir / "main.tf").exists())
        self.assertEqual(
            (self.target_dir / "vpc.tf").read_text(encoding="utf-8"),
            "# data block\n",
        )

    def test_imported_network_uses_a_data_block(self):
        self.create_imported_vpc()
        self.assert_imported_set_up()

    def test_imported_network_left_part_way_is_completed(self):
        # A crash after main.tf was removed and before terraform.tfvars:
        # the template copy puts main.tf back, and it is removed again
        self.target_dir.mkdir(parents=True)
        (self.target_dir / SETTING_UP).touch()
        (self.target_dir / "vpc.tf").write_text("# partial\n", encoding="utf-8")
        self.create_imported_vpc()
        self.assert_imported_set_up()

    def test_managed_network_in_any_state_keeps_main_tf(self):
        # Managed means OFE created it, in whatever state it is now
        for state in ("nm", "cm", "m", "dm", "xm", "um"):
            with self.subTest(state=state):
                shutil.rmtree(self.target_dir, ignore_errors=True)
                self.copytree.reset_mock()
                self.run_terraform.reset_mock()
                self.vpc.cloud_state = state
                self.vpc.save()
                with mock.patch(PREFLIGHT), mock.patch.object(
                    cm_vpc, "generate_vpc_tf_datablock"
                ) as generate:
                    cm_vpc.create_vpc(self.vpc)
                generate.assert_not_called()
                self.assert_set_up_once()
                self.assertEqual(
                    (self.target_dir / "main.tf").read_text(encoding="utf-8"),
                    "# network stand-in\n",
                )

    # start_vpc's apply and destroy_vpc's destroy hold the directory lock

    def subnet_file(self):
        return self.target_dir / f"subnet-{self.subnet.id}.tf"

    def race_during_terraform(
        self, command, background, request, pause_in_state_read=False
    ):
        """Run background in the worker, paused inside its Terraform
        command (or, with pause_in_state_read, inside its state read-back),
        and request in this thread. When request is about to take the
        directory lock it first tries it without waiting: if the lock is
        held, the worker resumes and request then waits for it; otherwise
        request goes on, and the worker resumes only when request has
        returned. Returns what was seen: whether request found the lock
        held, and whether the subnet file existed when the command ended
        and when the state read-back ended."""
        seen = {}
        in_command = threading.Event()
        resume = threading.Event()
        real_flock = fcntl.flock

        def terraform(target_dir, cmd, **kwargs):
            self.terraform(target_dir, cmd, **kwargs)
            if cmd == command and threading.current_thread() is self.worker:
                if not pause_in_state_read:
                    in_command.set()
                    self.wait(resume, "request taking the lock")
                seen[f"{command} ended"] = self.subnet_file().exists()

        def state(*unused_args, **unused_kwargs):
            if pause_in_state_read:
                in_command.set()
                self.wait(resume, "request taking the lock")
            seen["state read"] = self.subnet_file().exists()
            return {
                "outputs": {
                    "vpc_id": {"value": "net-race-id"},
                    f"subnet-{self.subnet.id}": {"value": "subnet-race-id"},
                }
            }

        def flock(fd, operation):
            if (
                threading.current_thread() is self.worker
                or operation != fcntl.LOCK_EX
            ):
                return real_flock(fd, operation)
            try:
                real_flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                seen["request waited"] = True
                resume.set()
                return real_flock(fd, operation)
            seen["request waited"] = False
            return None

        def run():
            try:
                background()
            finally:
                in_command.set()

        # The worker must not query the test database: its subnets are
        # loaded here and its saves are not written
        prefetch_related_objects([self.vpc], "subnets")
        self.run_terraform.side_effect = terraform
        with mock.patch(PREFLIGHT), mock.patch.object(
            utils, "get_terraform_state", side_effect=state
        ), mock.patch.object(VirtualNetwork, "save"), mock.patch.object(
            VirtualSubnet, "save"
        ), mock.patch.object(
            fcntl, "flock", side_effect=flock
        ):
            self.start_worker(run)
            try:
                self.wait(in_command, f"{command} in the background task")
                request()
            finally:
                resume.set()
                self.finish_worker()
        return seen

    def set_up_directory(self, with_subnet=False):
        with mock.patch(PREFLIGHT):
            cm_vpc.create_vpc(self.vpc)
            if with_subnet:
                cm_vpc.create_subnet(self.subnet)

    def test_subnet_saved_during_apply_waits_for_it(self):
        self.set_up_directory()
        seen = self.race_during_terraform(
            "apply",
            lambda: cm_vpc.start_vpc(self.vpc),
            lambda: cm_vpc.create_subnet(self.subnet),
        )
        # The subnet file appeared only after the apply and its state
        # read-back
        self.assertEqual(
            seen,
            {"request waited": True, "apply ended": False, "state read": False},
        )
        self.assertTrue(self.subnet_file().is_file())
        self.assertEqual(self.vpc.cloud_state, "m")
        self.assertEqual(self.vpc.cloud_id, "net-race-id")

    def test_subnet_saved_during_the_state_read_back_waits_for_it(self):
        self.set_up_directory()
        seen = self.race_during_terraform(
            "apply",
            lambda: cm_vpc.start_vpc(self.vpc),
            lambda: cm_vpc.create_subnet(self.subnet),
            pause_in_state_read=True,
        )
        # The lock is still held while the state is read back, and the
        # subnet file appeared only after that
        self.assertEqual(
            seen,
            {"request waited": True, "apply ended": False, "state read": False},
        )
        self.assertTrue(self.subnet_file().is_file())
        self.assertEqual(self.vpc.cloud_id, "net-race-id")

    def test_preflight_does_not_hold_the_lock(self):
        """A subnet deleted while start_vpc's or destroy_vpc's preflight is
        held is removed straight away: the preflight runs before the lock"""
        for name, call in (
            ("start", cm_vpc.start_vpc),
            ("destroy", cm_vpc.destroy_vpc),
        ):
            with self.subTest(name):
                self.set_up_directory(with_subnet=True)
                self.assertTrue(self.subnet_file().is_file())
                seen = {}
                in_preflight = threading.Event()
                release = threading.Event()
                real_flock = fcntl.flock

                def preflight(*unused_args, **unused_kwargs):
                    if threading.current_thread() is self.worker:
                        in_preflight.set()
                        self.wait(release, "request deleting the subnet")

                def flock(fd, operation):
                    if (
                        threading.current_thread() is self.worker
                        or operation != fcntl.LOCK_EX
                    ):
                        return real_flock(fd, operation)
                    try:
                        real_flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    except BlockingIOError:
                        # Held by the preflight: let it go on, so the test
                        # fails instead of waiting for ever
                        seen["request waited"] = True
                        release.set()
                        return real_flock(fd, operation)
                    seen["request waited"] = False
                    return None

                def run():
                    try:
                        call(self.vpc)
                    finally:
                        in_preflight.set()

                self.worker = None
                self.worker_errors = []
                self.go.clear()
                with mock.patch(PREFLIGHT, side_effect=preflight), mock.patch.object(
                    VirtualNetwork, "save"
                ), mock.patch.object(VirtualSubnet, "save"), mock.patch.object(
                    utils,
                    "get_terraform_state",
                    return_value={
                        "outputs": {
                            "vpc_id": {"value": "net-race-id"},
                            f"subnet-{self.subnet.id}": {"value": "subnet-race-id"},
                        }
                    },
                ), mock.patch.object(
                    fcntl, "flock", side_effect=flock
                ):
                    prefetch_related_objects([self.vpc], "subnets")
                    self.start_worker(run)
                    try:
                        self.wait(in_preflight, "preflight in the background task")
                        cm_vpc.delete_subnet(self.subnet)
                        # Removed while the preflight is still held
                        seen["removed during preflight"] = (
                            not release.is_set() and not self.subnet_file().exists()
                        )
                    finally:
                        release.set()
                        self.finish_worker()
                self.assertEqual(
                    seen,
                    {"request waited": False, "removed during preflight": True},
                )

    def test_subnet_deleted_during_destroy_waits_for_it(self):
        self.set_up_directory(with_subnet=True)
        seen = self.race_during_terraform(
            "destroy",
            lambda: cm_vpc.destroy_vpc(self.vpc),
            lambda: cm_vpc.delete_subnet(self.subnet),
        )
        # The subnet file was removed only after the destroy
        self.assertEqual(seen, {"request waited": True, "destroy ended": True})
        self.assertFalse(self.subnet_file().exists())
        self.assertEqual(self.vpc.cloud_state, "xm")

    def assert_lock_free(self):
        lock = self.target_dir.parent / f".vpc_{self.vpc.id}.lock"
        with open(lock, "a", encoding="utf-8") as fp:
            # Raises BlockingIOError if a failure left the lock held
            fcntl.flock(fp, fcntl.LOCK_EX | fcntl.LOCK_NB)
            fcntl.flock(fp, fcntl.LOCK_UN)

    def test_failures_reset_the_state_and_release_the_lock(self):
        self.set_up_directory()
        refused = credentials.CredentialError("no permission")
        for name, call, state, failure, expected in (
            ("start preflight", cm_vpc.start_vpc, "cm", refused, "nm"),
            ("destroy preflight", cm_vpc.destroy_vpc, "dm", refused, "m"),
            (
                "apply",
                cm_vpc.start_vpc,
                "cm",
                subprocess.CalledProcessError(1, ["terraform", "apply"]),
                "um",
            ),
        ):
            with self.subTest(name):
                self.run_terraform.reset_mock()
                self.vpc.cloud_state = state
                self.vpc.save()
                preflight = refused if failure is refused else None
                if failure is not refused:
                    self.run_terraform.side_effect = self.failing_on("apply")
                with mock.patch(PREFLIGHT, side_effect=preflight):
                    with self.assertRaises(type(failure)):
                        call(self.vpc)
                self.run_terraform.side_effect = self.terraform
                self.assertEqual(
                    VirtualNetwork.objects.get(pk=self.vpc.pk).cloud_state,
                    expected,
                )
                if failure is refused:
                    self.run_terraform.assert_not_called()
                self.assert_lock_free()
                # A subnet saved next is not kept waiting
                with mock.patch(PREFLIGHT):
                    cm_vpc.create_subnet(self.subnet)
                    cm_vpc.delete_subnet(self.subnet)


class SubnetPageWhileBusyTests(TestCase):
    """The subnet page refuses a network being created or destroyed, whose
    directory lock an apply or destroy holds"""

    REGIONS = {"us-central1": ["us-central1-a"]}

    def setUp(self):
        _, _, self.subnet = make_world()
        self.vpc = self.subnet.vpc
        for role_id, _ in Role.ROLE_CHOICES:
            Role.objects.get_or_create(id=role_id)
        self.admin = User.objects.create_superuser(
            username="subnet-admin", password="admin-pass", email="admin@example.com"
        )
        self.url = reverse("vpc-subnets", kwargs={"vpc_id": self.vpc.pk})
        for name in ("create_subnet", "delete_subnet"):
            patcher = mock.patch(f"ghpcfe.views.vpc.{name}")
            setattr(self, name, patcher.start())
            self.addCleanup(patcher.stop)
        patcher = mock.patch(
            "ghpcfe.views.vpc.cloud_info.get_region_zone_info",
            return_value=self.REGIONS,
        )
        self.region_info = patcher.start()
        self.addCleanup(patcher.stop)

    def post_data(self):
        """The subnet formset: the existing subnet unchanged and a new one"""
        prefix = "subnets"
        return {
            f"{prefix}-TOTAL_FORMS": "2",
            f"{prefix}-INITIAL_FORMS": "1",
            f"{prefix}-MIN_NUM_FORMS": "0",
            f"{prefix}-MAX_NUM_FORMS": "1000",
            # A subnet's primary key is its CloudResource row
            f"{prefix}-0-cloudresource_ptr": str(self.subnet.pk),
            f"{prefix}-0-vpc": str(self.vpc.pk),
            f"{prefix}-0-name": self.subnet.name,
            f"{prefix}-0-cidr": self.subnet.cidr,
            f"{prefix}-0-cloud_region": "us-central1",
            f"{prefix}-1-vpc": str(self.vpc.pk),
            f"{prefix}-1-name": "subnet-new",
            f"{prefix}-1-cidr": "10.30.0.0/24",
            f"{prefix}-1-cloud_region": "us-central1",
        }

    def set_state(self, state):
        VirtualNetwork.objects.filter(pk=self.vpc.pk).update(cloud_state=state)
        # A new session for each request, so it shows only its own messages
        self.client = self.client_class()
        self.client.force_login(self.admin)

    def test_refused_while_creating_or_destroying(self):
        detail = reverse("vpc-detail", kwargs={"pk": self.vpc.pk})
        for state, action in (("cm", "created"), ("dm", "destroyed")):
            for method in ("get", "post"):
                with self.subTest(state=state, method=method):
                    self.set_state(state)
                    before = list(VirtualSubnet.objects.values_list("pk", "name"))
                    if method == "get":
                        response = self.client.get(self.url)
                    else:
                        response = self.client.post(self.url, self.post_data())
                    self.assertRedirects(
                        response, detail, fetch_redirect_response=False
                    )
                    self.assertEqual(
                        [str(m) for m in get_messages(response.wsgi_request)],
                        [
                            f"VPC {self.vpc.name} is being {action}. Edit its "
                            "subnets when that has finished."
                        ],
                    )
                    self.assertEqual(
                        list(VirtualSubnet.objects.values_list("pk", "name")),
                        before,
                    )
        self.create_subnet.assert_not_called()
        self.delete_subnet.assert_not_called()
        self.region_info.assert_not_called()

    def test_works_in_other_states(self):
        for state in ("nm", "i", "m", "um", "xm"):
            with self.subTest(state=state):
                self.set_state(state)
                response = self.client.get(self.url)
                self.assertEqual(response.status_code, 200)
                self.assertContains(response, "Update VPC Subnets")
        self.set_state("m")
        response = self.client.post(self.url, self.post_data())
        self.assertRedirects(
            response,
            reverse("vpc-detail", kwargs={"pk": self.vpc.pk}),
            fetch_redirect_response=False,
        )
        new = VirtualSubnet.objects.get(name="subnet-new")
        self.create_subnet.assert_any_call(new)
