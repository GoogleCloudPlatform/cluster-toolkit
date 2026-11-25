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

"""A failed credential set-up is shown to administrators.

manage.py ofe_bootstrap_credentials records the errors of a failed run in
credential-setup-errors.json under the backend's baseDir and removes the
record when a run succeeds; every page shows the recorded errors to
superusers only. credentials.bootstrap itself is replaced by a fake here
(test_credential_bootstrap covers it), and baseDir points at a scratch
directory for each test.
"""

import io
import json
import shutil
import tempfile
from pathlib import Path
from unittest import mock

from django.contrib.auth.models import AnonymousUser
from django.core.management import call_command
from django.core.management.base import CommandError
from django.test import RequestFactory, TestCase
from django.urls import reverse

from ghpcfe import context_processors
from ghpcfe.cluster_manager import utils
from ghpcfe.models import Role, User

RECORD = "credential-setup-errors.json"
ERRORS = [
    "the instance metadata key ofe-db-key-secret is not set",
    "could not register the provisioning account <svc@example>",
]
HEADING = "Preparing stored credentials failed when the server last started"
FIX = (
    "Fix the cause (see the Admin Guide, Stored credentials at start), then "
    "restart the server or run <code>python manage.py ofe_bootstrap_credentials</code>."
)


class ScratchBaseDirMixin:
    """baseDir is a new scratch directory for each test"""

    def setUp(self):
        super().setUp()
        self.base = Path(tempfile.mkdtemp(prefix="ofe-credential-setup-"))
        self.addCleanup(shutil.rmtree, self.base, ignore_errors=True)
        patcher = mock.patch.dict(utils.g_config, {"baseDir": self.base})
        patcher.start()
        self.addCleanup(patcher.stop)
        self.record = self.base / RECORD


class CommandRecordTests(ScratchBaseDirMixin, TestCase):
    """manage.py ofe_bootstrap_credentials records its errors"""

    def run_command(self, errors):
        with mock.patch(
            "ghpcfe.cluster_manager.credentials.bootstrap", return_value=list(errors)
        ):
            out, err = io.StringIO(), io.StringIO()
            call_command("ofe_bootstrap_credentials", stdout=out, stderr=err)
            return out.getvalue(), err.getvalue()

    def test_a_failed_run_records_its_errors_and_still_fails(self):
        with self.assertRaisesRegex(CommandError, "ofe-db-key-secret"):
            self.run_command(ERRORS)
        record = json.loads(self.record.read_text())
        self.assertEqual(record["errors"], ERRORS)
        self.assertRegex(record["time"], r"^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\+00:00$")
        # Written beside the record and renamed over it, leaving no other file
        self.assertEqual([p.name for p in self.base.iterdir()], [RECORD])

    def test_a_later_failure_replaces_the_record(self):
        self.record.write_text(json.dumps({"time": "old", "errors": ["old error"]}))
        with self.assertRaises(CommandError):
            self.run_command(ERRORS[:1])
        self.assertEqual(json.loads(self.record.read_text())["errors"], ERRORS[:1])

    def test_a_successful_run_removes_the_record(self):
        self.record.write_text(json.dumps({"time": "old", "errors": ["old error"]}))
        out, _ = self.run_command([])
        self.assertIn("Credentials are ready", out)
        self.assertFalse(self.record.exists())

    def test_a_successful_run_with_no_record_succeeds(self):
        out, _ = self.run_command([])
        self.assertIn("Credentials are ready", out)
        self.assertEqual(list(self.base.iterdir()), [])

    def test_a_record_that_cannot_be_written_does_not_hide_the_errors(self):
        # baseDir is missing, so the record cannot be written; the command
        # still fails with the run's own errors and says why nothing was
        # recorded.
        shutil.rmtree(self.base)
        with self.assertLogs(
            "ghpcfe.management.commands.ofe_bootstrap_credentials", level="ERROR"
        ):
            with mock.patch(
                "ghpcfe.cluster_manager.credentials.bootstrap", return_value=ERRORS
            ):
                err = io.StringIO()
                with self.assertRaisesRegex(CommandError, "ofe-db-key-secret"):
                    call_command(
                        "ofe_bootstrap_credentials", stdout=io.StringIO(), stderr=err
                    )
        self.assertIn("Could not record the credential set-up result", err.getvalue())

    def test_an_unexpected_error_is_recorded_by_its_type_only(self):
        # bootstrap() itself crashing, not one of its steps failing: the
        # pages must still show that the set-up failed, and the record must
        # not quote the error's text, which could hold stored data. Any
        # kind of error counts.
        for error_type in (RuntimeError, KeyError, ValueError):
            with self.subTest(error_type.__name__):
                name = error_type.__name__
                self.record.write_text(json.dumps({"time": "old", "errors": ["old error"]}))
                with mock.patch(
                    "ghpcfe.cluster_manager.credentials.bootstrap",
                    side_effect=error_type("text that must stay in the log"),
                ), self.assertLogs(
                    "ghpcfe.management.commands.ofe_bootstrap_credentials", level="ERROR"
                ) as logs:
                    with self.assertRaisesRegex(CommandError, f"unexpected error \\({name}\\)"):
                        call_command(
                            "ofe_bootstrap_credentials",
                            stdout=io.StringIO(),
                            stderr=io.StringIO(),
                        )
                record = json.loads(self.record.read_text())
                self.assertEqual(
                    record["errors"],
                    [
                        "Preparing stored credentials stopped with an unexpected error "
                        f"({name}); see the server log (/opt/gcluster/run/django.log) "
                        "or the start-up output"
                    ],
                )
                self.assertNotIn("text that must stay in the log", self.record.read_text())
                # The full error, with its traceback, is in the log
                self.assertIn("text that must stay in the log", "\n".join(logs.output))

    def test_a_record_that_cannot_be_removed_does_not_fail_a_successful_run(self):
        self.record.mkdir()
        with self.assertLogs(
            "ghpcfe.management.commands.ofe_bootstrap_credentials", level="ERROR"
        ):
            out, err = self.run_command([])
        self.assertIn("Credentials are ready", out)
        self.assertIn("Could not record the credential set-up result", err)


class BannerTests(ScratchBaseDirMixin, TestCase):
    """The pages built on base_generic.html show the recorded errors to
    superusers only"""

    def setUp(self):
        super().setUp()
        for role_id, _ in Role.ROLE_CHOICES:
            Role.objects.get_or_create(id=role_id)
        self.superuser = User.objects.create_superuser(
            username="root-admin", password="pass", email="root@example.com"
        )
        self.user = User.objects.create_user(username="plain-user", password="pass")

    def write_record(self, errors=ERRORS, time="2026-11-24T10:00:00+00:00"):
        self.record.write_text(json.dumps({"time": time, "errors": errors}))

    def home_page(self, user):
        self.client.force_login(user)
        response = self.client.get(reverse("index"))
        self.assertEqual(response.status_code, 200)
        return response

    def test_superuser_sees_the_errors(self):
        self.write_record()
        response = self.home_page(self.superuser)
        self.assertContains(response, HEADING)
        self.assertContains(response, "(2026-11-24T10:00:00+00:00)")
        self.assertContains(response, ERRORS[0])
        # Escaped like any other text
        self.assertContains(response, "provisioning account &lt;svc@example&gt;")
        self.assertNotContains(response, "<svc@example>")
        self.assertContains(response, FIX, html=False)

    def test_other_users_do_not_see_them(self):
        self.write_record()
        response = self.home_page(self.user)
        self.assertNotContains(response, HEADING)
        self.assertNotContains(response, ERRORS[0])

    def test_staff_who_are_not_superusers_do_not_see_them(self):
        self.write_record()
        staff = User.objects.create_user(
            username="staff-user", password="pass", is_staff=True, is_superuser=False
        )
        response = self.home_page(staff)
        self.assertNotContains(response, HEADING)
        self.assertNotContains(response, ERRORS[0])

    def test_superusers_who_are_not_staff_see_them(self):
        self.write_record()
        superuser = User.objects.create_user(
            username="nonstaff-root", password="pass", is_staff=False, is_superuser=True
        )
        response = self.home_page(superuser)
        self.assertContains(response, HEADING)
        self.assertContains(response, ERRORS[0])

    def test_anonymous_visitors_get_nothing(self):
        self.write_record()
        request = RequestFactory().get("/")
        request.user = AnonymousUser()
        self.assertEqual(context_processors.credential_setup_errors(request), {})
        request = RequestFactory().get("/")
        self.assertEqual(context_processors.credential_setup_errors(request), {})

    def test_nothing_is_shown_without_a_record(self):
        response = self.home_page(self.superuser)
        self.assertNotContains(response, HEADING)
        self.assertNotIn("credential_setup_errors", response.context)

    def test_a_garbled_record_gives_a_generic_error(self):
        for content in (
            "{not json",
            json.dumps(["a list"]),
            json.dumps({"time": "t", "errors": []}),
            json.dumps({"time": "t", "errors": "not a list"}),
            json.dumps({"time": "t", "errors": [3]}),
            json.dumps({"time": 3, "errors": ["e"]}),
            b"\xff\xfe".decode("latin-1"),
            # Nested too deeply for the JSON parser
            "[" * 100000 + "]" * 100000,
        ):
            with self.subTest(content=content):
                self.record.write_text(content, encoding="latin-1")
                with self.assertLogs("ghpcfe.context_processors", level="WARNING"):
                    response = self.home_page(self.superuser)
                self.assertContains(response, HEADING)
                self.assertContains(response, f"the record of the errors ({RECORD}) could not be read")

    def test_an_unreadable_record_gives_a_generic_error(self):
        self.record.mkdir()
        with self.assertLogs("ghpcfe.context_processors", level="WARNING"):
            response = self.home_page(self.superuser)
        self.assertContains(response, HEADING)
        self.assertContains(response, f"the record of the errors ({RECORD}) could not be read")

    def test_without_the_backend_configuration_pages_still_load(self):
        # The Django admin does not need the backend configuration; a
        # missing configuration.yaml must not make it fail for superusers
        self.write_record()
        request = RequestFactory().get("/")
        request.user = self.superuser
        with mock.patch.object(
            utils, "load_config", side_effect=FileNotFoundError("configuration.yaml")
        ), self.assertLogs("ghpcfe.context_processors", level="WARNING"):
            self.assertEqual(context_processors.credential_setup_errors(request), {})
