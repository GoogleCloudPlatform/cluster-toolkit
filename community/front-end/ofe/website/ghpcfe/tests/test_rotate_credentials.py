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

"""Rotating the database key: stored keys sealed with an older version of
the database-key secret are sealed again with the newest, by the start-up
and by the ofe_rotate_credentials command.

The metadata server and Secret Manager are the fakes of the start-up tests.
"""

import contextlib
import io
import shutil
import tempfile
from unittest import mock

from cryptography.fernet import Fernet, InvalidToken
from django.core.management import call_command
from django.core.management.base import CommandError
from django.db import connection
from django.test import TestCase, override_settings
from django.test.utils import CaptureQueriesContext
from google.api_core.exceptions import ServiceUnavailable

from ghpcfe.cluster_manager import credentials
from ghpcfe.models import Credential, User
from ghpcfe.tests.test_credential_bootstrap import FakeMetadata, FakeSecretManager
from ghpcfe.tests.test_credentials import KEY_JSON, PROJECT, make_key_json

OTHER_PROJECT = "edited-project"
OTHER_KEY_JSON = make_key_json(
    OTHER_PROJECT, f"edited@{OTHER_PROJECT}.iam.gserviceaccount.com"
)


@override_settings(OFE_CREDENTIAL_ENCRYPTION_KEY=None)
class RotationTestCase(TestCase):
    """Rows sealed with one key, then a server that reads every enabled
    version of the database-key secret"""

    def setUp(self):
        credentials.forget_db_key()
        self.addCleanup(credentials.forget_db_key)
        self.owner = User.objects.create_user(username="key-owner", password="pass")
        self.old, self.new = Fernet.generate_key(), Fernet.generate_key()

    def sealed_with(self, key, name, text=KEY_JSON):
        """A key credential encrypted with key alone"""
        credentials.use_db_key(key)
        credential = Credential.objects.create(name=name, owner=self.owner, detail=text)
        credentials.forget_db_key()
        return credential

    def start(self, secrets):
        """What the start-up does first: use every enabled version"""
        credentials.forget_db_key()
        return credentials.ensure_db_key(
            True, http_get=FakeMetadata(), client=secrets
        )

    def stored(self, credential):
        return Credential.objects.values_list("detail", "project_id").get(
            pk=credential.pk
        )

    def token(self, credential):
        detail = self.stored(credential)[0]
        self.assertTrue(credentials.is_sealed(detail))
        return detail[len(credentials.ENCRYPTED_PREFIX):].encode("ascii")

    def opens_with(self, key, credential):
        """The stored key decrypted with key alone, or None if it cannot"""
        try:
            return Fernet(key).decrypt(self.token(credential)).decode("utf-8")
        except InvalidToken:
            return None

    def updates(self, run):
        """run(), and the UPDATE statements it sent"""
        with CaptureQueriesContext(connection) as queries:
            result = run()
        return result, [
            query["sql"] for query in queries.captured_queries
            if query["sql"].lstrip().upper().startswith("UPDATE")
        ]

    def sealed_text(self, key, text=KEY_JSON):
        """text (KEY_JSON by default) as a row sealed with key alone stores
        it"""
        return credentials.ENCRYPTED_PREFIX + Fernet(key).encrypt(
            text.encode("utf-8")
        ).decode("ascii")

    @contextlib.contextmanager
    def changed_before_first(self, function, change):
        """Within it, change() runs once, on the first call of the
        credentials module's function, before that call does anything: as
        another process's edit or delete made after the row was read and
        before it is written"""
        real = getattr(credentials, function)
        calls = []

        def change_then_call(*args):
            if not calls:
                calls.append(args)
                change()
            return real(*args)

        with mock.patch.object(credentials, function, side_effect=change_then_call):
            yield
        self.assertEqual(len(calls), 1)

    def edited_while_resealing(self, row, edited_detail):
        """Within it, row's detail becomes edited_detail once a re-seal has
        read the row and before it writes, as another process's save"""
        return self.changed_before_first(
            "_resealed",
            lambda: Credential.objects.filter(pk=row.pk).update(detail=edited_detail),
        )


class ResealTests(RotationTestCase):
    """credentials.fix_existing_credentials re-seals with the newest key"""

    def test_a_row_sealed_with_an_older_key_is_resealed_with_the_newest(self):
        row = self.sealed_with(self.old, "before-rotation")
        self.start(FakeSecretManager([self.old, self.new]))
        self.assertIsNone(self.opens_with(self.new, row))
        counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(counts["resealed"], 1)
        self.assertEqual(counts["unchanged"], 0)
        self.assertEqual(counts["unreadable"], 0)
        self.assertEqual(self.opens_with(self.new, row), KEY_JSON)
        self.assertIsNone(self.opens_with(self.old, row))
        self.assertEqual(self.stored(row)[1], PROJECT)

    def test_a_resealed_row_stays_readable_once_the_older_version_is_disabled(self):
        row = self.sealed_with(self.old, "before-rotation")
        self.start(FakeSecretManager([self.old, self.new]))
        credentials.fix_existing_credentials(Credential)
        self.start(FakeSecretManager([self.old, self.new], disabled={1}))
        self.assertEqual(
            credentials.key_json(Credential.objects.get(pk=row.pk)), KEY_JSON
        )
        counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(counts["unreadable"], 0)
        self.assertEqual(counts["unchanged"], 1)

    def test_a_row_with_no_project_id_is_resealed_and_given_one(self):
        row = self.sealed_with(self.old, "no-project")
        Credential.objects.filter(pk=row.pk).update(project_id="")
        self.start(FakeSecretManager([self.old, self.new]))
        counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(counts["resealed"], 1)
        self.assertEqual(counts["project_filled"], 1)
        self.assertEqual(self.opens_with(self.new, row), KEY_JSON)
        self.assertEqual(self.stored(row)[1], PROJECT)

    def test_a_second_run_writes_nothing(self):
        rows = [self.sealed_with(self.old, f"row-{n}") for n in range(2)]
        current = self.sealed_with(self.new, "already-newest")
        self.start(FakeSecretManager([self.old, self.new]))
        counts, first = self.updates(
            lambda: credentials.fix_existing_credentials(Credential)
        )
        self.assertEqual(counts["resealed"], 2)
        self.assertEqual(counts["unchanged"], 1)
        self.assertEqual(len(first), 2)
        after_first = [self.stored(row) for row in (*rows, current)]
        counts, second = self.updates(
            lambda: credentials.fix_existing_credentials(Credential)
        )
        self.assertEqual(second, [])
        self.assertEqual(counts["resealed"], 0)
        self.assertEqual(counts["unchanged"], 3)
        self.assertEqual([self.stored(row) for row in (*rows, current)], after_first)

    def test_an_edit_sealed_with_the_newest_key_is_kept(self):
        row = self.sealed_with(self.old, "edited")
        self.start(FakeSecretManager([self.old, self.new]))
        # Saved by a process that uses the newest key
        edited = self.sealed_text(self.new)
        with self.edited_while_resealing(row, edited), self.assertNoLogs(
            credentials.logger, "WARNING"
        ):
            counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(self.stored(row)[0], edited)
        self.assertEqual(counts["resealed"], 0)
        self.assertEqual(counts["changed"], 0)
        self.assertEqual(counts["unchanged"], 1)

    def test_an_edit_sealed_with_an_older_key_is_kept_and_reported(self):
        row = self.sealed_with(self.old, "edited")
        self.start(FakeSecretManager([self.old, self.new]))
        # Saved by a process that started before the newest version existed
        edited = self.sealed_text(self.old)
        with self.edited_while_resealing(row, edited), self.assertLogs(
            credentials.logger, "WARNING"
        ) as logs:
            counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(self.stored(row)[0], edited)
        self.assertEqual(counts["resealed"], 0)
        self.assertEqual(counts["changed"], 1)
        self.assertEqual(counts["changed_ids"], [row.pk])
        self.assertEqual(counts["unchanged"], 0)
        output = "\n".join(logs.output)
        self.assertIn(f"Credential {row.pk} changed", output)
        self.assertNotIn(edited, output)
        # The next run re-seals it
        counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(counts["resealed"], 1)
        self.assertEqual(self.opens_with(self.new, row), KEY_JSON)

    def test_the_start_up_reseals_and_says_so(self):
        row = self.sealed_with(self.old, "before-rotation")
        base_dir = tempfile.mkdtemp(prefix="ofe-rotate-")
        self.addCleanup(shutil.rmtree, base_dir, ignore_errors=True)
        lines = []
        errors = credentials.bootstrap(
            Credential,
            User,
            http_get=FakeMetadata(provisioner=""),
            client=FakeSecretManager([self.old, self.new]),
            report=lines.append,
            base_dir=base_dir,
        )
        self.assertEqual(errors, [])
        self.assertIn(
            "Stored keys sealed with an older database key: 1 re-sealed with "
            "the newest, 0 changed meanwhile and left for the next start",
            lines,
        )
        self.assertEqual(self.opens_with(self.new, row), KEY_JSON)

    def test_a_row_deleted_while_being_resealed_is_skipped(self):
        row = self.sealed_with(self.old, "deleted")
        kept = self.sealed_with(self.old, "kept")
        self.start(FakeSecretManager([self.old, self.new]))
        with self.changed_before_first(
            "_resealed", Credential.objects.filter(pk=row.pk).delete
        ), self.assertNoLogs(credentials.logger, "WARNING"):
            counts = credentials.fix_existing_credentials(Credential)
        self.assertFalse(Credential.objects.filter(pk=row.pk).exists())
        # Not counted as already sealed, or as anything else
        self.assertEqual(counts["unchanged"], 0)
        self.assertEqual(counts["changed"], 0)
        self.assertEqual(counts["unreadable"], 0)
        self.assertEqual(counts["resealed"], 1)
        self.assertEqual(self.opens_with(self.new, kept), KEY_JSON)

    def test_a_row_made_unreadable_while_being_resealed_is_kept_and_reported(self):
        lost = Fernet.generate_key()
        for case, edited in {
            "sealed with a key the server does not have": self.sealed_text(lost),
            "not a token TKFE wrote": (
                credentials.ENCRYPTED_PREFIX + "gAAAAé-not-a-token"
            ),
        }.items():
            with self.subTest(case):
                Credential.objects.all().delete()
                row = self.sealed_with(self.old, "made-unreadable")
                self.start(FakeSecretManager([self.old, self.new]))
                with self.edited_while_resealing(row, edited), self.assertLogs(
                    credentials.logger, "WARNING"
                ) as logs:
                    counts = credentials.fix_existing_credentials(Credential)
                # Not overwritten; reported as changed, so the command fails
                # and the next run reports it as unreadable
                self.assertEqual(self.stored(row)[0], edited)
                self.assertEqual(counts["changed"], 1)
                self.assertEqual(counts["changed_ids"], [row.pk])
                self.assertEqual(counts["resealed"], 0)
                self.assertEqual(counts["unchanged"], 0)
                self.assertNotIn(edited, "\n".join(logs.output))
                with self.assertLogs(credentials.logger, "WARNING"):
                    counts = credentials.fix_existing_credentials(Credential)
                self.assertEqual(counts["unreadable_ids"], [row.pk])


class EditedWhileFixedTests(RotationTestCase):
    """A plaintext key being encrypted, or a sealed key given its project
    id, is written only if the row is still as it was read"""

    def setUp(self):
        super().setUp()
        self.start(FakeSecretManager([self.old, self.new]))

    def no_project(self, detail=None):
        """A row with no project id, holding detail (KEY_JSON sealed with
        the newest key by default)"""
        row = Credential.objects.create(name="no-project", owner=self.owner, detail=KEY_JSON)
        Credential.objects.filter(pk=row.pk).update(
            detail=detail or self.sealed_text(self.new), project_id=""
        )
        return row

    def edited_while_sealing(self, row, detail, project_id):
        """Within it, another process saves row with detail and project_id
        after the fix has read the row and before it writes"""
        return self.changed_before_first(
            "seal",
            lambda: Credential.objects.filter(pk=row.pk).update(
                detail=detail, project_id=project_id
            ),
        )

    def test_a_row_with_no_project_id_deleted_meanwhile_is_skipped(self):
        row = self.no_project()
        with self.changed_before_first(
            "seal", Credential.objects.filter(pk=row.pk).delete
        ), self.assertNoLogs(credentials.logger, "WARNING"):
            counts = credentials.fix_existing_credentials(Credential)
        self.assertFalse(Credential.objects.filter(pk=row.pk).exists())
        for count in ("project_filled", "unchanged", "changed", "unreadable"):
            self.assertEqual(counts[count], 0, count)

    def test_a_plaintext_row_deleted_meanwhile_is_skipped(self):
        row = self.no_project(detail=KEY_JSON)
        with self.changed_before_first(
            "seal", Credential.objects.filter(pk=row.pk).delete
        ):
            counts = credentials.fix_existing_credentials(Credential)
        self.assertFalse(Credential.objects.filter(pk=row.pk).exists())
        self.assertEqual(counts["encrypted"], 0)

    def test_the_project_id_fill_keeps_an_edit_made_meanwhile(self):
        row = self.no_project()
        edited = self.sealed_text(self.new, OTHER_KEY_JSON)
        with self.edited_while_sealing(
            row, edited, OTHER_PROJECT
        ), self.assertNoLogs(credentials.logger, "WARNING"):
            counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(self.stored(row), (edited, OTHER_PROJECT))
        self.assertEqual(counts["project_filled"], 0)
        self.assertEqual(counts["unchanged"], 1)

    def test_encrypting_a_plaintext_row_keeps_an_edit_made_meanwhile(self):
        row = self.no_project(detail=KEY_JSON)
        edited = self.sealed_text(self.new, OTHER_KEY_JSON)
        with self.edited_while_sealing(
            row, edited, OTHER_PROJECT
        ), self.assertNoLogs(credentials.logger, "WARNING"):
            counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(self.stored(row), (edited, OTHER_PROJECT))
        self.assertEqual(counts["encrypted"], 0)
        self.assertEqual(counts["unchanged"], 1)

    def test_encrypting_a_plaintext_row_keeps_an_edit_of_its_key_alone(self):
        # The row has its project id, and the edit replaces its key with
        # another of the same project: only the stored text differs from
        # what the fix read
        row = Credential.objects.create(name="plaintext", owner=self.owner, detail=KEY_JSON)
        Credential.objects.filter(pk=row.pk).update(detail=KEY_JSON)
        self.assertEqual(self.stored(row), (KEY_JSON, PROJECT))
        edited = self.sealed_text(
            self.new,
            make_key_json(PROJECT, f"edited@{PROJECT}.iam.gserviceaccount.com"),
        )
        with self.edited_while_sealing(
            row, edited, PROJECT
        ), self.assertNoLogs(credentials.logger, "WARNING"):
            counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(self.stored(row), (edited, PROJECT))
        self.assertEqual(counts["encrypted"], 0)
        self.assertEqual(counts["unchanged"], 1)

    def test_the_project_id_fill_keeps_an_edit_of_the_project_id_alone(self):
        # The edit sets only the project id; the stored text is still what
        # the fix read
        row = self.no_project()
        detail = self.stored(row)[0]
        with self.edited_while_sealing(
            row, detail, OTHER_PROJECT
        ), self.assertNoLogs(credentials.logger, "WARNING"):
            counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(self.stored(row), (detail, OTHER_PROJECT))
        self.assertEqual(counts["project_filled"], 0)
        self.assertEqual(counts["unchanged"], 1)

    def test_an_edit_sealed_with_an_older_key_is_kept_and_reported(self):
        row = self.no_project(detail=KEY_JSON)
        edited = self.sealed_text(self.old, OTHER_KEY_JSON)
        with self.edited_while_sealing(
            row, edited, OTHER_PROJECT
        ), self.assertLogs(credentials.logger, "WARNING") as logs:
            counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(self.stored(row), (edited, OTHER_PROJECT))
        self.assertEqual(counts["changed_ids"], [row.pk])
        self.assertEqual(counts["encrypted"], 0)
        self.assertIn(f"Credential {row.pk} changed", "\n".join(logs.output))
        # The next run re-seals it
        counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(counts["resealed"], 1)
        self.assertEqual(self.opens_with(self.new, row), OTHER_KEY_JSON)


class RotateCommandTests(RotationTestCase):
    """manage.py ofe_rotate_credentials, on a server that loads its keys
    from the secret on first use"""

    def rotate(self, secrets):
        """Run the command; return its output"""
        credentials.forget_db_key()
        out = io.StringIO()
        with mock.patch("requests.get", side_effect=FakeMetadata()), mock.patch.object(
            credentials, "_secret_client", return_value=secrets
        ):
            call_command("ofe_rotate_credentials", stdout=out)
        return out.getvalue()

    def test_the_command_reseals_and_a_second_run_writes_nothing(self):
        row = self.sealed_with(self.old, "before-rotation")
        secrets = FakeSecretManager([self.old, self.new])
        output = self.rotate(secrets)
        self.assertIn(
            "Stored keys: 1 re-sealed with the newest database key, 0 already "
            "sealed with it, 0 encrypted from plaintext, 0 changed while this "
            "ran and left for the next run, 0 unreadable",
            output,
        )
        self.assertIn("Every stored key is sealed with the newest database key", output)
        self.assertEqual(self.opens_with(self.new, row), KEY_JSON)
        before = self.stored(row)
        output, updates = self.updates(lambda: self.rotate(secrets))
        self.assertEqual(updates, [])
        self.assertIn("Stored keys: 0 re-sealed with the newest database key, 1 already", output)
        self.assertEqual(self.stored(row), before)
        self.assertNotIn("private_key", output)

    def test_the_command_fails_when_a_key_cannot_be_read(self):
        lost = self.sealed_with(Fernet.generate_key(), "sealed-with-a-lost-key")
        moved = self.sealed_with(self.old, "before-rotation")
        with self.assertLogs(credentials.logger, "WARNING"):
            with self.assertRaises(CommandError) as raised:
                self.rotate(FakeSecretManager([self.old, self.new]))
        message = str(raised.exception)
        self.assertIn(f"1 stored keys could not be read (credential ids: {lost.pk})", message)
        self.assertIn(
            "Do not disable or destroy any version of the database-key secret",
            message,
        )
        self.assertNotIn("private_key", message)
        # The readable row was still re-sealed
        self.assertEqual(self.opens_with(self.new, moved), KEY_JSON)

    def test_the_command_fails_when_the_older_version_is_already_disabled(self):
        row = self.sealed_with(self.old, "before-rotation")
        before = self.stored(row)
        with self.assertLogs(credentials.logger, "WARNING"):
            with self.assertRaisesRegex(CommandError, f"credential ids: {row.pk}"):
                self.rotate(FakeSecretManager([self.old, self.new], disabled={1}))
        self.assertEqual(self.stored(row), before)

    def test_the_command_fails_when_a_row_changed_while_it_ran(self):
        row = self.sealed_with(self.old, "edited")
        edited = self.sealed_text(self.old)
        with self.edited_while_resealing(row, edited), self.assertLogs(
            credentials.logger, "WARNING"
        ):
            with self.assertRaises(CommandError) as raised:
                self.rotate(FakeSecretManager([self.old, self.new]))
        self.assertIn(
            f"1 stored keys changed while this ran and were left as they are "
            f"(credential ids: {row.pk}). Run this command again",
            str(raised.exception),
        )
        self.assertEqual(self.stored(row)[0], edited)

    def test_the_command_fails_and_touches_nothing_when_the_keys_cannot_be_read(self):
        outages = {
            "latest version disabled": FakeSecretManager(
                [self.old, self.new], disabled={2}
            ),
            "Secret Manager unavailable": UnavailableSecretManager(),
            "no version": FakeSecretManager([]),
        }
        for with_rows in (True, False):
            for case, secrets in outages.items():
                with self.subTest(case, with_rows=with_rows):
                    Credential.objects.all().delete()
                    if with_rows:
                        self.sealed_with(self.old, "sealed")
                        plaintext = self.sealed_with(self.old, "plaintext")
                        Credential.objects.filter(pk=plaintext.pk).update(
                            detail=KEY_JSON, project_id=""
                        )
                    before = list(
                        Credential.objects.order_by("pk").values_list(
                            "detail", "project_id"
                        )
                    )
                    with CaptureQueriesContext(connection) as queries:
                        with self.assertRaises(CommandError) as raised:
                            self.rotate(secrets)
                    message = str(raised.exception)
                    self.assertIn(
                        "The database keys cannot be read, so no stored key "
                        "was read or changed",
                        message,
                    )
                    self.assertIn("Check the database-key secret", message)
                    # No row is reported, or advised to be deleted
                    self.assertNotIn("could not be read", message)
                    self.assertNotIn("delete each", message)
                    self.assertEqual(
                        [
                            query["sql"] for query in queries.captured_queries
                            if not query["sql"].lstrip().upper().startswith("SELECT")
                        ],
                        [],
                    )
                    self.assertEqual(
                        list(
                            Credential.objects.order_by("pk").values_list(
                                "detail", "project_id"
                            )
                        ),
                        before,
                    )


class UnavailableSecretManager:
    """A Secret Manager that cannot be reached"""

    def access_secret_version(self, request):
        raise ServiceUnavailable("The service is currently unavailable")
