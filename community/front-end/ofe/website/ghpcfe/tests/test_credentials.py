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

"""How OFE stores credentials and hands them to tools and clients.

Every test is offline: keys are generated here, and Google client libraries
are replaced only at their own module boundaries.
"""

import contextlib
import io
import json
import logging
import os
import re
import shutil
import stat
import tempfile
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from django.core.management import call_command
from django.core.management.base import CommandError
from django.test import SimpleTestCase, TestCase, override_settings
from django.urls import reverse
from google.api_core.exceptions import NotFound, PermissionDenied
from grafana_api.grafana_api import GrafanaClientError

from ghpcfe import grafana
from ghpcfe.cluster_manager import credentials
from ghpcfe.cluster_manager.image import ImageBackend
from ghpcfe.forms import CredentialForm, ImageForm
from ghpcfe.models import Credential, Image, StartupScript, User

PROJECT = "example-project"
ACCOUNT = f"provisioner@{PROJECT}.iam.gserviceaccount.com"
GHPCFE_DIR = Path(__file__).resolve().parent.parent


def make_key_json(project=PROJECT, email=ACCOUNT):
    """Service account key JSON with a freshly generated private key"""
    private_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    pem = private_key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.PKCS8,
        serialization.NoEncryption(),
    ).decode("ascii")
    return json.dumps({
        "type": "service_account",
        "project_id": project,
        "private_key_id": "0" * 40,
        "private_key": pem,
        "client_email": email,
        "client_id": "123456789",
        "token_uri": "https://oauth2.googleapis.com/token",
    })


KEY_JSON = make_key_json()


def owner():
    user = User.objects.filter(username="credential-owner").first()
    return user or User.objects.create_user(
        username="credential-owner", password="credential-pass"
    )


DATABASE_KEY_UNREADABLE = (
    "TKFE cannot read its database key; re-run deploy.sh (see Upgrading in "
    "the admin guide) and try again."
)


@contextlib.contextmanager
def database_key_unreadable():
    """The server as deployed (no test key setting) on a VM whose metadata
    server answers 404 for every path, so the database key cannot be read"""
    no_metadata = mock.Mock(status_code=404, text="")
    with override_settings(OFE_CREDENTIAL_ENCRYPTION_KEY=None), mock.patch(
        "requests.get", return_value=no_metadata
    ):
        credentials.forget_db_key()
        try:
            yield
        finally:
            credentials.forget_db_key()


DB_KEY_SECRET = "ofe-db-key"
SERVER_PROJECT = "ofe-server-project"
# Text in the secret that is not a Fernet key
MALFORMED_DB_KEY = b"not-a-fernet-key-but-secret-text"


def _deployed_metadata(url, headers, timeout):
    """requests.get for the metadata server of a deployed server: it names
    the database-key secret and the server's project"""
    del headers, timeout
    if url.endswith(credentials.DB_KEY_SECRET_ATTRIBUTE):
        return mock.Mock(status_code=200, text=DB_KEY_SECRET)
    if url.endswith("project/project-id"):
        return mock.Mock(status_code=200, text=SERVER_PROJECT)
    return mock.Mock(status_code=404, text="")


def malformed_db_key_version(request):
    """Secret Manager's answer for a secret whose one version holds text
    that is not a Fernet key"""
    del request
    return SimpleNamespace(
        name=f"projects/123456789012/secrets/{DB_KEY_SECRET}/versions/1",
        payload=SimpleNamespace(data=MALFORMED_DB_KEY),
    )


@contextlib.contextmanager
def database_key_secret(access_secret_version):
    """The server as deployed (no test key setting): instance metadata names
    the database-key secret, and the Secret Manager client, replaced at
    google.cloud.secretmanager, answers each read with access_secret_version
    (a function, or an error to raise)"""
    with override_settings(OFE_CREDENTIAL_ENCRYPTION_KEY=None), mock.patch(
        "requests.get", side_effect=_deployed_metadata
    ), mock.patch(
        "google.cloud.secretmanager.SecretManagerServiceClient"
    ) as client_class:
        client_class.return_value.access_secret_version.side_effect = (
            access_secret_version
        )
        credentials.forget_db_key()
        try:
            yield
        finally:
            credentials.forget_db_key()


# A database key that cannot be read or used, as the credential pages meet it
DATABASE_KEY_FAILURES = {
    "Secret Manager refuses the read": lambda: database_key_secret(
        PermissionDenied(
            "Permission 'secretmanager.versions.access' denied for resource "
            f"'projects/{SERVER_PROJECT}/secrets/{DB_KEY_SECRET}/versions/latest'"
        )
    ),
    "the secret has no enabled version": lambda: database_key_secret(
        NotFound("Secret Version not found or has no versions")
    ),
    "the secret holds a malformed key": lambda: database_key_secret(
        malformed_db_key_version
    ),
    "the key setting is malformed": lambda: override_settings(
        OFE_CREDENTIAL_ENCRYPTION_KEY=MALFORMED_DB_KEY.decode("ascii")
    ),
}


def key_credential(name="key-cred", text=KEY_JSON):
    return Credential.objects.create(name=name, owner=owner(), detail=text)


def impersonate_credential(name="keyless-cred", email=ACCOUNT, project=PROJECT):
    return Credential.objects.create(
        name=name,
        owner=owner(),
        kind=credentials.KIND_IMPERSONATE,
        detail=email,
        project_id=project,
    )


class StorageTests(TestCase):
    """What Credential.detail holds"""

    def test_key_is_encrypted_when_saved(self):
        credential = key_credential()
        stored = Credential.objects.values_list("detail", flat=True).get(
            pk=credential.pk
        )
        self.assertTrue(stored.startswith(credentials.ENCRYPTED_PREFIX))
        self.assertNotIn(json.loads(KEY_JSON)["private_key"], stored)
        self.assertNotIn("private_key", stored)
        self.assertNotIn(ACCOUNT, stored)

    def test_key_reads_back_and_fills_project_id(self):
        credential = Credential.objects.get(pk=key_credential().pk)
        self.assertEqual(credential.project_id, PROJECT)
        self.assertEqual(credentials.key_json(credential), KEY_JSON)
        self.assertEqual(credentials.identity_email(credential), ACCOUNT)

    def test_saving_again_does_not_encrypt_twice(self):
        credential = key_credential()
        first = credential.detail
        credential.name = "renamed"
        credential.save()
        self.assertEqual(credential.detail, first)
        self.assertEqual(credentials.key_json(credential), KEY_JSON)

    def test_cloud_resource_project_reads_the_field(self):
        credential = impersonate_credential(project="other-project")
        image = Image(cloud_credential=credential)
        self.assertEqual(image.project_id, "other-project")

    def test_impersonate_stores_only_the_email(self):
        credential = impersonate_credential(email=f"  {ACCOUNT} ")
        stored = Credential.objects.values_list("detail", flat=True).get(
            pk=credential.pk
        )
        self.assertEqual(stored, ACCOUNT)
        self.assertEqual(credentials.identity_email(credential), ACCOUNT)
        with self.assertRaises(credentials.CredentialError):
            credentials.key_json(credential)

    def test_impersonate_never_stores_key_json(self):
        # An impersonate row is not encrypted, so only a plain email address
        # may be stored in it
        refused = {
            "key JSON": KEY_JSON,
            "key JSON as a downloaded key file": json.dumps(
                json.loads(KEY_JSON), indent=2
            ),
            "a quoted local part": '"{private_key}"@example.com',
            "not an address": "not an email",
            "nothing": "",
        }
        for label, text in refused.items():
            with self.subTest(label):
                with self.assertRaises(credentials.CredentialError) as raised:
                    impersonate_credential(name="keyless", email=text)
                self.assertFalse(Credential.objects.filter(name="keyless").exists())
                self.assertNotIn("private_key", str(raised.exception))
                self.assertIn("Nothing was saved", str(raised.exception))
        # An existing row edited to hold a key keeps what it held
        existing = impersonate_credential()
        existing.detail = KEY_JSON
        with self.assertRaises(credentials.CredentialError):
            existing.save()
        self.assertEqual(
            Credential.objects.values_list("detail", flat=True).get(pk=existing.pk),
            ACCOUNT,
        )
        # Any address the credential form accepts is still stored, a default
        # Compute Engine account among them
        compute = "123456789012-compute@developer.gserviceaccount.com"
        stored = impersonate_credential(name="compute", email=compute)
        self.assertEqual(
            Credential.objects.values_list("detail", flat=True).get(pk=stored.pk),
            compute,
        )

    def test_wrong_database_key_fails_clearly(self):
        credential = key_credential()
        with self.settings(
            OFE_CREDENTIAL_ENCRYPTION_KEY=credentials.Fernet.generate_key()
        ):
            with self.assertRaisesRegex(
                credentials.CredentialError, "cannot be decrypted"
            ):
                credentials.key_json(credential)


class FixExistingKeysTests(TestCase):
    """The start-up fix of stored keys skips a key it cannot read, still
    does every other row, and the start-up command reports the skipped
    count"""

    def rows(self):
        """A key sealed with a database key the server does not have, first,
        then a plaintext key and a sealed key with no project id"""
        unreadable = key_credential("unreadable")
        elsewhere = credentials.Fernet(credentials.Fernet.generate_key())
        sealed_elsewhere = credentials.ENCRYPTED_PREFIX + elsewhere.encrypt(
            KEY_JSON.encode("utf-8")
        ).decode("ascii")
        Credential.objects.filter(pk=unreadable.pk).update(
            detail=sealed_elsewhere, project_id=""
        )
        plaintext = key_credential("plaintext")
        Credential.objects.filter(pk=plaintext.pk).update(detail=KEY_JSON, project_id="")
        no_project = key_credential("no-project")
        Credential.objects.filter(pk=no_project.pk).update(project_id="")
        return unreadable, plaintext, no_project

    def stored(self, credential):
        return Credential.objects.values_list("detail", "project_id").get(
            pk=credential.pk
        )

    def test_an_unreadable_key_is_skipped_and_the_others_are_done(self):
        unreadable, plaintext, no_project = self.rows()
        before = self.stored(unreadable)
        with self.assertLogs(credentials.logger, "WARNING") as logs:
            counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(counts["unreadable"], 1)
        self.assertEqual(counts["unreadable_ids"], [unreadable.pk])
        self.assertEqual(counts["encrypted"], 1)
        self.assertEqual(counts["project_filled"], 2)
        for row in (plaintext, no_project):
            detail, project_id = self.stored(row)
            self.assertTrue(detail.startswith(credentials.ENCRYPTED_PREFIX))
            self.assertEqual(project_id, PROJECT)
        # The skipped row is left as it was.
        self.assertEqual(self.stored(unreadable), before)
        # Logged by credential id, never with its key.
        output = "\n".join(logs.output)
        self.assertIn(f"Credential {unreadable.pk} was skipped", output)
        self.assertIn("cannot be decrypted", output)
        self.assertNotIn(before[0], output)
        self.assertNotIn("private_key", output)

    def test_the_start_up_command_reports_the_skipped_count(self):
        unreadable, _, _ = self.rows()
        base_dir = tempfile.mkdtemp(prefix="ofe-fix-keys-")
        self.addCleanup(shutil.rmtree, base_dir, ignore_errors=True)
        out = io.StringIO()
        with mock.patch.object(
            credentials, "ensure_db_key", return_value="present"
        ), mock.patch.object(
            credentials, "_backend_base_dir", return_value=base_dir
        ), mock.patch.object(
            credentials, "metadata_value", return_value=""
        ), self.assertLogs(credentials.logger, "WARNING"):
            with self.assertRaises(CommandError) as raised:
                call_command("ofe_bootstrap_credentials", stdout=out)
        message = str(raised.exception)
        self.assertIn(
            f"Stored keys: 1 could not be read and were skipped "
            f"(credential ids: {unreadable.pk})",
            message,
        )
        self.assertIn("delete each of these credentials in TKFE", message)
        # The other rows were still done, and said so.
        self.assertIn(
            "Stored keys: 1 encrypted, 2 given a project id, 0 already done",
            out.getvalue(),
        )

    def fixed(self, credential):
        """Run the fix; return its counts and its log text"""
        with self.assertLogs(credentials.logger, "WARNING") as logs:
            counts = credentials.fix_existing_credentials(Credential)
        output = "\n".join(logs.output)
        self.assertIn(f"Credential {credential.pk} was skipped", output)
        self.assertNotIn("private_key", output)
        return counts, output

    def assert_only_unreadable(self, counts, credential):
        self.assertEqual(counts["unreadable"], 1)
        self.assertEqual(counts["unreadable_ids"], [credential.pk])
        self.assertEqual(counts["project_filled"], 0)
        self.assertEqual(counts["unchanged"], 0)

    def test_sealed_key_naming_no_project_is_unreadable(self):
        info = json.loads(KEY_JSON)
        del info["project_id"]
        row = key_credential("no-project-anywhere", text=json.dumps(info))
        Credential.objects.filter(pk=row.pk).update(project_id="")
        self.assertTrue(credentials.is_sealed(self.stored(row)[0]))
        counts, _ = self.fixed(row)
        self.assert_only_unreadable(counts, row)
        self.assertEqual(counts["encrypted"], 0)

    def test_plaintext_that_is_not_json_is_sealed_and_unreadable(self):
        row = key_credential("not-json")
        Credential.objects.filter(pk=row.pk).update(
            detail="not a key at all", project_id=""
        )
        counts, output = self.fixed(row)
        self.assert_only_unreadable(counts, row)
        # It may hold part of a key, so it is still encrypted
        self.assertEqual(counts["encrypted"], 1)
        detail, project_id = self.stored(row)
        self.assertTrue(credentials.is_sealed(detail))
        self.assertEqual(project_id, "")
        self.assertNotIn("not a key at all", output)

    def test_plaintext_json_list_is_sealed_and_unreadable(self):
        row = key_credential("json-list")
        listed = json.dumps([json.loads(KEY_JSON)])
        Credential.objects.filter(pk=row.pk).update(detail=listed, project_id="")
        counts, output = self.fixed(row)
        self.assert_only_unreadable(counts, row)
        self.assertEqual(counts["encrypted"], 1)
        self.assertTrue(credentials.is_sealed(self.stored(row)[0]))
        self.assertNotIn(listed, output)

    def test_sealed_key_with_project_but_another_database_key_is_unreadable(self):
        row = key_credential("sealed-elsewhere")
        elsewhere = credentials.Fernet(credentials.Fernet.generate_key())
        sealed_elsewhere = credentials.ENCRYPTED_PREFIX + elsewhere.encrypt(
            KEY_JSON.encode("utf-8")
        ).decode("ascii")
        Credential.objects.filter(pk=row.pk).update(detail=sealed_elsewhere)
        before = self.stored(row)
        self.assertEqual(before[1], PROJECT)
        counts, output = self.fixed(row)
        self.assert_only_unreadable(counts, row)
        self.assertEqual(counts["encrypted"], 0)
        self.assertIn("cannot be decrypted", output)
        self.assertNotIn(sealed_elsewhere, output)
        # Left as it was
        self.assertEqual(self.stored(row), before)

    def test_sealed_non_ascii_text_with_a_project_is_unreadable(self):
        # Not a token TKFE wrote: decrypting it raises UnicodeEncodeError, a
        # ValueError, which must not stop the fix of the other rows
        row = key_credential("non-ascii")
        other = key_credential("plaintext-after")
        Credential.objects.filter(pk=other.pk).update(detail=KEY_JSON, project_id="")
        garbled = credentials.ENCRYPTED_PREFIX + "gAAAA\u00e9\u00e8-secret-text"
        Credential.objects.filter(pk=row.pk).update(detail=garbled)
        before = self.stored(row)
        self.assertEqual(before[1], PROJECT)
        counts, output = self.fixed(row)
        self.assertEqual(counts["unreadable"], 1)
        self.assertEqual(counts["unreadable_ids"], [row.pk])
        self.assertEqual(counts["unchanged"], 0)
        # The other row is still done
        self.assertEqual(counts["encrypted"], 1)
        self.assertEqual(counts["project_filled"], 1)
        self.assertTrue(credentials.is_sealed(self.stored(other)[0]))
        # Logged by the error's type, never its text or the row's
        self.assertIn("(UnicodeEncodeError)", output)
        self.assertNotIn("secret-text", output)
        self.assertNotIn("\u00e9", output)
        self.assertEqual(self.stored(row), before)

    def test_a_decrypt_raising_type_error_is_unreadable(self):
        # cryptography's Fernet.decrypt raises TypeError for a token that is
        # neither bytes nor str. Neither path that decrypts a stored key
        # (a sealed row with a project id, and the save of a sealed row
        # without one) may let it stop the fix of the other rows.
        with_project = key_credential("type-error-with-project")
        no_project = key_credential("type-error-no-project")
        Credential.objects.filter(pk=no_project.pk).update(project_id="")
        plaintext = key_credential("plaintext-after")
        Credential.objects.filter(pk=plaintext.pk).update(
            detail=KEY_JSON, project_id=""
        )
        before = {row.pk: self.stored(row) for row in (with_project, no_project)}
        with mock.patch(
            "cryptography.fernet.Fernet.decrypt",
            side_effect=TypeError("token must be bytes or str"),
        ) as decrypt, self.assertLogs(credentials.logger, "WARNING") as logs:
            counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(decrypt.call_count, 2)
        self.assertEqual(counts["unreadable"], 2)
        self.assertEqual(
            sorted(counts["unreadable_ids"]), sorted([with_project.pk, no_project.pk])
        )
        self.assertEqual(counts["unchanged"], 0)
        # The other row is still done
        self.assertEqual(counts["encrypted"], 1)
        self.assertEqual(counts["project_filled"], 1)
        detail, project_id = self.stored(plaintext)
        self.assertTrue(credentials.is_sealed(detail))
        self.assertEqual(project_id, PROJECT)
        # Logged by credential id and the error's type, never its text
        output = "\n".join(logs.output)
        for row in (with_project, no_project):
            self.assertIn(f"Credential {row.pk} was skipped", output)
            # Left as it was
            self.assertEqual(self.stored(row), before[row.pk])
        self.assertEqual(output.count("(TypeError)"), 2)
        self.assertNotIn("token must be", output)
        self.assertNotIn("private_key", output)

    def test_a_project_id_that_is_not_a_string_is_unreadable(self):
        row = key_credential("listed-project")
        info = json.loads(KEY_JSON)
        info["project_id"] = ["a"]
        text = json.dumps(info)
        Credential.objects.filter(pk=row.pk).update(detail=text, project_id="")
        counts, output = self.fixed(row)
        self.assert_only_unreadable(counts, row)
        self.assertEqual(counts["encrypted"], 1)
        detail, project_id = self.stored(row)
        self.assertTrue(credentials.is_sealed(detail))
        # Nothing is made of the list: the stored project id stays empty
        self.assertEqual(project_id, "")
        self.assertIn("names a project", output)
        self.assertNotIn(text, output)

    def test_a_readable_sealed_key_with_a_project_is_unchanged(self):
        row = key_credential("already-done")
        before = self.stored(row)
        counts = credentials.fix_existing_credentials(Credential)
        self.assertEqual(counts["unchanged"], 1)
        self.assertEqual(counts["unreadable"], 0)
        self.assertEqual(self.stored(row), before)


class ToolEnvironmentTests(TestCase):
    """The environment each tool run gets"""

    AMBIENT = {
        "PATH": "/usr/bin",
        "GOOGLE_APPLICATION_CREDENTIALS": "/somewhere/else.json",
        "GOOGLE_CREDENTIALS": "{}",
        "CLOUDSDK_CONFIG": "/home/someone/.config/gcloud",
        "CLOUDSDK_CORE_ACCOUNT": "someone@example.com",
        "GOOGLE_IMPERSONATE_SERVICE_ACCOUNT": "other@example.com",
    }

    def test_impersonate_sets_the_impersonation_variables(self):
        credential = impersonate_credential()
        with credentials.tool_environment(
            credential, key_file=True, base_env=self.AMBIENT
        ) as env:
            self.assertEqual(env["GOOGLE_IMPERSONATE_SERVICE_ACCOUNT"], ACCOUNT)
            self.assertEqual(
                env["CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT"], ACCOUNT
            )
            self.assertEqual(env["CLOUDSDK_CORE_PROJECT"], PROJECT)
            self.assertNotIn("GOOGLE_CREDENTIALS", env)
            self.assertNotIn("GOOGLE_APPLICATION_CREDENTIALS", env)
            self.assertNotIn("CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE", env)
            self.assertNotIn("CLOUDSDK_CORE_ACCOUNT", env)
            self.assertEqual(env["PATH"], "/usr/bin")
            config = Path(env["CLOUDSDK_CONFIG"])
            self.assertTrue(config.is_dir())
            self.assertEqual(os.listdir(config), [])
        self.assertFalse(config.exists())

    def test_backend_credentials_variable_is_dropped(self):
        # Terraform's GCS backend reads GOOGLE_BACKEND_CREDENTIALS before the
        # credential's own variables, so an ambient one would make state
        # reads and writes act as another identity
        ambient = dict(self.AMBIENT, GOOGLE_BACKEND_CREDENTIALS="/other/key.json")
        for credential in (impersonate_credential(), key_credential()):
            with credentials.tool_environment(
                credential, key_file=True, base_env=ambient
            ) as env:
                self.assertNotIn("GOOGLE_BACKEND_CREDENTIALS", env)

    def test_backend_impersonation_variable_is_dropped(self):
        # Terraform's GCS backend impersonates
        # GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT for state reads and
        # writes, ahead of the credential's own identity
        ambient = dict(
            self.AMBIENT,
            GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT="other@example.com",
        )
        for credential in (impersonate_credential(), key_credential()):
            with self.subTest(kind=credential.kind), credentials.tool_environment(
                credential, key_file=True, base_env=ambient
            ) as env:
                self.assertNotIn("GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT", env)

    def test_key_without_file_is_in_memory_only(self):
        credential = key_credential()
        with credentials.tool_environment(
            credential, base_env=self.AMBIENT
        ) as env:
            self.assertEqual(env["GOOGLE_CREDENTIALS"], KEY_JSON)
            self.assertNotIn("GOOGLE_APPLICATION_CREDENTIALS", env)
            self.assertNotIn("GOOGLE_IMPERSONATE_SERVICE_ACCOUNT", env)
            workdir = Path(env["CLOUDSDK_CONFIG"]).parent
            self.assertEqual(sorted(os.listdir(workdir)), ["gcloud"])

    def test_key_file_is_private_and_removed_after_the_run(self):
        credential = key_credential()
        with credentials.tool_environment(
            credential, key_file=True, base_env=self.AMBIENT
        ) as env:
            path = Path(env["GOOGLE_APPLICATION_CREDENTIALS"])
            self.assertEqual(env["CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE"], str(path))
            self.assertEqual(env["GOOGLE_CREDENTIALS"], KEY_JSON)
            self.assertEqual(path.read_text(encoding="utf-8"), KEY_JSON)
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            self.assertEqual(stat.S_IMODE(path.parent.stat().st_mode), 0o700)
            if credentials.KEY_FILE_PARENT.is_dir():
                self.assertEqual(path.parent.parent, credentials.KEY_FILE_PARENT)
        self.assertFalse(path.exists())
        self.assertFalse(path.parent.exists())

    def test_key_file_is_removed_when_the_run_raises(self):
        credential = key_credential()
        with self.assertRaises(RuntimeError):
            with credentials.tool_environment(credential, key_file=True) as env:
                path = Path(env["GOOGLE_APPLICATION_CREDENTIALS"])
                self.assertTrue(path.exists())
                raise RuntimeError("tool failed")
        self.assertFalse(path.exists())
        self.assertFalse(path.parent.exists())

    def test_private_dir_falls_back_without_tmpfs(self):
        missing = Path("/nonexistent-ofe-tmpfs")
        with self.assertLogs(credentials.logger, level="WARNING"):
            path = credentials._private_dir(missing)  # pylint: disable=protected-access
        try:
            self.assertFalse(str(path).startswith(str(missing)))
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o700)
        finally:
            path.rmdir()

    def test_process_environment_is_never_changed(self):
        before = dict(os.environ)
        for credential in (key_credential(), impersonate_credential()):
            with credentials.tool_environment(credential, key_file=True) as env:
                self.assertEqual(dict(os.environ), before)
                env["EXTRA"] = "only in this run"
            self.assertEqual(dict(os.environ), before)

    def test_unreadable_key_fails_before_any_file_is_written(self):
        credential = key_credential(text="not a key")
        with self.assertRaisesRegex(credentials.CredentialError, "does not hold"):
            with credentials.tool_environment(credential, key_file=True):
                self.fail("the block must not run")


class PythonCredentialTests(TestCase):
    """google.auth credentials for client libraries"""

    def test_key_gives_service_account_credentials(self):
        creds = credentials.python_credentials(key_credential())
        self.assertEqual(creds.service_account_email, ACCOUNT)
        self.assertIn(credentials.CLOUD_PLATFORM_SCOPE, creds.scopes)

    def test_impersonate_gives_impersonated_credentials(self):
        source = mock.Mock(name="server-identity")
        creds = credentials.python_credentials(
            impersonate_credential(), source_credentials=source
        )
        # pylint: disable=protected-access
        self.assertEqual(creds._target_principal, ACCOUNT)
        self.assertEqual(creds._target_scopes, [credentials.CLOUD_PLATFORM_SCOPE])


class ShellIdentityTests(TestCase):
    """The identity a command typed by hand (a stale state lock's
    force-unlock) is told to act as"""

    def test_impersonate_names_the_account(self):
        self.assertEqual(
            credentials.shell_identity(impersonate_credential()),
            f"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT={ACCOUNT}",
        )

    def test_key_asks_for_a_key_file_and_shows_no_key(self):
        identity = credentials.shell_identity(key_credential())
        self.assertEqual(
            identity, f"GOOGLE_APPLICATION_CREDENTIALS='<a key file of {ACCOUNT}>'"
        )
        self.assertNotIn("PRIVATE", identity)

    def test_no_credential_or_an_unreadable_one(self):
        placeholder = "GOOGLE_APPLICATION_CREDENTIALS='<a key file of the resource'\"'\"'s credential>'"
        self.assertEqual(credentials.shell_identity(None), placeholder)
        self.assertEqual(
            credentials.shell_identity(key_credential(text="not a key")), placeholder
        )


class OnlyTheHelperReadsKeysTests(SimpleTestCase):
    """No module other than credentials.py reads keys or sets the process
    environment"""

    KEY_READERS = re.compile(
        r"\.detail\b"
        r"|__dict__\[\s*['\"]detail"
        r"|cloud_credentials"
        r"|from_service_account_(?:info|file)"
        r"|GOOGLE_APPLICATION_CREDENTIALS"
        r"|GOOGLE_CREDENTIALS"
    )
    ENV_WRITES = re.compile(
        r"os\.environ\s*\[[^\]]+\]\s*=(?!=)"
        r"|os\.environ\.(?:update|pop|popitem|setdefault|clear)\s*\("
        r"|os\.(?:putenv|unsetenv)\s*\("
        r"|del\s+os\.environ"
    )

    @staticmethod
    def source_files():
        for path in sorted(GHPCFE_DIR.rglob("*")):
            if path.suffix not in (".py", ".html", ".j2", ".sh"):
                continue
            relative = path.relative_to(GHPCFE_DIR)
            if relative.parts[0] in ("tests", "migrations", "static"):
                continue
            yield relative, path.read_text(encoding="utf-8")

    def test_no_other_module_reads_a_key(self):
        found = [
            f"{relative}:{number}: {line.strip()}"
            for relative, text in self.source_files()
            if relative != Path("cluster_manager/credentials.py")
            for number, line in enumerate(text.splitlines(), 1)
            # A commented-out Python line reads nothing (views/containers.py
            # keeps upstream's commented-out image listing unchanged)
            if not (relative.suffix == ".py" and line.lstrip().startswith("#"))
            and self.KEY_READERS.search(line)
        ]
        self.assertEqual(found, [])

    def test_nothing_changes_the_process_environment(self):
        found = [
            f"{relative}:{number}: {line.strip()}"
            for relative, text in self.source_files()
            for number, line in enumerate(text.splitlines(), 1)
            if self.ENV_WRITES.search(line)
        ]
        self.assertEqual(found, [])

    def test_workbench_wait_script_uses_the_run_environment(self):
        """The script Terraform runs while a workbench starts gets gcloud's
        identity and configuration from the environment it is given"""
        script = (
            GHPCFE_DIR.parent.parent / "infrastructure_files" / "workbench_tf"
            / "google" / "wait-for-startup" / "scripts"
            / "wait-for-startup-status.sh"
        ).read_text(encoding="utf-8")
        for forbidden in (
            r"cloud_credentials",
            r"activate-service-account",
            r"^\s*(?:export\s+)?CLOUDSDK_CONFIG=",
            r"^\s*(?:export\s+)?GOOGLE_APPLICATION_CREDENTIALS=",
            r"\brm\b.*CLOUDSDK_CONFIG",
        ):
            self.assertNotRegex(script, re.compile(forbidden, re.MULTILINE))
        # It polls the instance's state, with the gcloud the run sets up
        self.assertIn("gcloud workbench instances describe", script)

    def test_the_checks_find_what_they_look_for(self):
        for line in (
            "creds = cluster.cloud_credential.detail",
            "fp.write(fs.cloud_credential.detail)",
            'open(target_dir / "cloud_credentials", "w")',
            "service_account.Credentials.from_service_account_info(info)",
            'env["GOOGLE_APPLICATION_CREDENTIALS"] = path',
        ):
            self.assertRegex(line, self.KEY_READERS)
        for line in (
            'os.environ["GOOGLE_APPLICATION_CREDENTIALS"] = path',
            'os.environ.pop("X", None)',
            "os.environ.update(env)",
            'os.putenv("X", "1")',
        ):
            self.assertRegex(line, self.ENV_WRITES)
        self.assertNotRegex("env = os.environ.copy()", self.ENV_WRITES)
        self.assertNotRegex('x = os.environ["X"] == "1"', self.ENV_WRITES)


class GrafanaTests(TestCase):
    """Dashboards of keyless credentials read metrics as the web server"""

    def test_keyless_credential_uses_the_gce_datasource(self):
        credential = impersonate_credential()
        self.assertEqual(grafana.datasource_name(credential), grafana.GCE_DATASOURCE)
        # Returns before contacting Grafana: no key, no datasource
        self.assertIsNone(grafana.add_gcp_datasource(credential))

    def test_key_credential_has_its_own_jwt_datasource(self):
        credential = key_credential(name="with-key")
        self.assertEqual(grafana.datasource_name(credential), "with-key")
        json_data, secure = credentials.grafana_key_settings(credential)
        self.assertEqual(json_data["authenticationType"], "jwt")
        self.assertEqual(json_data["clientEmail"], ACCOUNT)
        self.assertEqual(json_data["defaultProject"], PROJECT)
        self.assertEqual(secure["privateKey"], json.loads(KEY_JSON)["private_key"])


class ImageBuildTests(TestCase):
    """Image builds need a key until Packer can impersonate"""

    def make_image(self, credential):
        script = StartupScript.objects.create(
            name="hello.sh",
            type="shell",
            content="startup-scripts/hello.sh",
            owner=owner(),
        )
        image = Image.objects.create(
            name="keyless-image",
            family="keyless-family",
            owner=owner(),
            cloud_credential=credential,
            cloud_region="us-central1",
            cloud_zone="us-central1-a",
        )
        image.startup_script.add(script)
        return image

    def test_build_with_impersonate_credential_stops_with_a_clear_message(self):
        image = self.make_image(impersonate_credential())
        backend = ImageBackend(image)
        with self.assertLogs("ghpcfe.cluster_manager.image", level="ERROR"):
            with self.assertRaises(credentials.ImageBuildNeedsKeyError) as caught:
                backend.prepare()
        message = str(caught.exception)
        self.assertIn("need an uploaded service account key", message)
        self.assertIn("Packer", message)
        self.assertIn(ACCOUNT, message)
        self.assertEqual(Image.objects.get(pk=image.pk).status, "e")
        self.assertFalse(backend.image_dir.exists())

    def test_image_form_refuses_an_impersonate_credential(self):
        form = ImageForm(
            data={"cloud_credential": impersonate_credential().pk},
            user=owner(),
        )
        form.is_valid()
        self.assertIn("cloud_credential", form.errors)
        self.assertIn(
            "need an uploaded service account key",
            " ".join(form.errors["cloud_credential"]),
        )

    def test_image_form_accepts_a_key_credential(self):
        form = ImageForm(
            data={"cloud_credential": key_credential().pk}, user=owner()
        )
        form.is_valid()
        self.assertNotIn("cloud_credential", form.errors)


class CredentialFormTests(TestCase):
    """The credential page registers either kind"""

    def granted(self, permissions):
        """A discovery client whose testIamPermissions grants permissions"""
        service = mock.Mock(name="cloudresourcemanager")
        service.projects.return_value.testIamPermissions.return_value.execute.return_value = {
            "permissions": list(permissions)
        }
        return service

    def test_impersonate_is_validated_through_impersonation(self):
        service = self.granted(credentials.VALIDATION_PERMISSIONS)
        with mock.patch(
            "google.auth.default", return_value=(mock.Mock(), None)
        ), mock.patch("googleapiclient.discovery.build", return_value=service) as build:
            form = CredentialForm(data={
                "name": "keyless",
                "kind": credentials.KIND_IMPERSONATE,
                "service_account_email": ACCOUNT,
                "project_id": PROJECT,
            })
            self.assertTrue(form.is_valid(), form.errors)
        creds = build.call_args.kwargs["credentials"]
        self.assertEqual(creds._target_principal, ACCOUNT)  # pylint: disable=protected-access
        service.projects.return_value.testIamPermissions.assert_called_once_with(
            resource=PROJECT,
            body={"permissions": list(credentials.VALIDATION_PERMISSIONS)},
        )
        form.instance.owner = owner()
        credential = form.save()
        self.assertEqual(credential.kind, credentials.KIND_IMPERSONATE)
        self.assertEqual(credential.project_id, PROJECT)
        self.assertEqual(credentials.impersonation_target(credential), ACCOUNT)

    def test_impersonate_without_access_is_refused(self):
        with mock.patch(
            "google.auth.default", return_value=(mock.Mock(), None)
        ), mock.patch(
            "googleapiclient.discovery.build", return_value=self.granted([])
        ):
            form = CredentialForm(data={
                "name": "keyless",
                "kind": credentials.KIND_IMPERSONATE,
                "service_account_email": ACCOUNT,
                "project_id": PROJECT,
            })
            self.assertFalse(form.is_valid())
        errors = " ".join(form.non_field_errors())
        self.assertIn(f"Identity {ACCOUNT} lacks permission", errors)

    def test_impersonate_needs_email_and_project(self):
        form = CredentialForm(
            data={"name": "keyless", "kind": credentials.KIND_IMPERSONATE}
        )
        self.assertFalse(form.is_valid())
        self.assertIn("service_account_email", form.errors)
        self.assertIn("project_id", form.errors)

    def test_uploaded_key_is_encrypted_and_gives_the_project(self):
        form = CredentialForm(data={
            "name": "with-key",
            "kind": credentials.KIND_KEY,
            "project_id": "ignored-project",
            "detail": KEY_JSON,
        })
        self.assertTrue(form.is_valid(), form.errors)
        form.instance.owner = owner()
        credential = form.save()
        self.assertEqual(credential.project_id, PROJECT)
        self.assertTrue(credentials.is_sealed(credential.detail))

    def test_invalid_key_is_refused(self):
        form = CredentialForm(data={
            "name": "with-key", "kind": credentials.KIND_KEY, "detail": "{}"
        })
        self.assertFalse(form.is_valid())
        self.assertIn("Credential cannot be validated.", form.non_field_errors())

    def test_update_form_shows_the_impersonated_account(self):
        form = CredentialForm(instance=impersonate_credential())
        self.assertEqual(form.fields["service_account_email"].initial, ACCOUNT)

    def test_blank_key_is_refused_unless_a_key_is_stored(self):
        # Only an edit of a key credential keeps its stored key; a new
        # credential, or an impersonate one changed to a key, needs one
        keyless = impersonate_credential()
        stored = Credential.objects.values_list(
            "kind", "detail", "project_id"
        ).get(pk=keyless.pk)
        for case, instance in (("new", None), ("impersonate to key", keyless)):
            with self.subTest(case):
                form = CredentialForm(
                    instance=instance,
                    data={
                        "name": "blank-key",
                        "kind": credentials.KIND_KEY,
                        "detail": "",
                    },
                )
                self.assertFalse(form.is_valid())
                self.assertIn(
                    "Credential cannot be validated.", form.non_field_errors()
                )
        self.assertFalse(Credential.objects.filter(name="blank-key").exists())
        self.assertEqual(
            Credential.objects.values_list("kind", "detail", "project_id").get(
                pk=keyless.pk
            ),
            stored,
        )


class CredentialCreateViewTests(TestCase):
    """The credential page saves the row and its Grafana datasource
    together, or neither"""

    def setUp(self):
        admin = User.objects.create_superuser(
            username="credential-admin", password="admin-pass", email="admin@example.com"
        )
        self.client.force_login(admin)
        # The Grafana client, so grafana.add_gcp_datasource itself runs and
        # builds the datasource it sends
        patcher = mock.patch.object(grafana, "GrafanaFace")
        self.addCleanup(patcher.stop)
        self.add_datasource = patcher.start().return_value.datasource.create_datasource
        # The state-bucket check would ask Google for a token with the test
        # key; it is faked so these tests make no network call
        patcher = mock.patch(
            "ghpcfe.cluster_manager.validate_credential.check_state_bucket_access",
            return_value=None,
        )
        self.addCleanup(patcher.stop)
        self.check_bucket = patcher.start()

    def post(self, name="with-key"):
        return self.client.post(
            reverse("credential-create"),
            {"name": name, "kind": credentials.KIND_KEY, "detail": KEY_JSON},
        )

    def capture_logs(self):
        """Every record logged from here on, at any level"""
        records = []
        handler = logging.Handler(logging.DEBUG)
        handler.emit = records.append
        root = logging.getLogger()
        old_level = root.level
        root.addHandler(handler)
        root.setLevel(logging.DEBUG)
        self.addCleanup(root.setLevel, old_level)
        self.addCleanup(root.removeHandler, handler)
        return records

    def test_datasource_is_made_for_the_saved_row(self):
        saved_when_called = []
        self.add_datasource.side_effect = lambda payload: saved_when_called.append(
            Credential.objects.filter(name=payload["name"]).exists()
        )
        records = self.capture_logs()
        response = self.post()
        self.assertRedirects(response, reverse("credentials"), fetch_redirect_response=False)
        credential = Credential.objects.get(name="with-key")
        self.assertTrue(credentials.is_sealed(credential.detail))
        self.assertEqual(saved_when_called, [True])
        # The datasource signs in with the credential's key, for its project
        key = json.loads(KEY_JSON)
        self.add_datasource.assert_called_once_with({
            "name": "with-key",
            "type": "stackdriver",
            "access": "proxy",
            "jsonData": {
                "authenticationType": "jwt",
                "tokenUri": key["token_uri"],
                "clientEmail": ACCOUNT,
                "defaultProject": PROJECT,
            },
            "secureJsonData": {"privateKey": key["private_key"]},
        })
        # The private key goes to Grafana only, never to a log
        body = key["private_key"].split("-----")[2].strip().splitlines()[0]
        for record in records:
            self.assertNotIn(body, record.getMessage())

    def test_failed_save_makes_no_datasource(self):
        # As when the database key cannot be read
        with mock.patch.object(
            credentials,
            "seal",
            side_effect=credentials.CredentialError("Cannot read the database key"),
        ):
            response = self.post()
        self.assertEqual(response.status_code, 200)
        self.assertContains(response, DATABASE_KEY_UNREADABLE)
        self.add_datasource.assert_not_called()
        self.assertFalse(Credential.objects.exists())

    def test_unreadable_database_key_is_a_form_error(self):
        # A server whose instance metadata names no database-key secret
        # cannot seal the key: the page says so, with no 500, and nothing
        # is saved
        with database_key_unreadable():
            response = self.post()
        self.assertEqual(response.status_code, 200)
        self.assertContains(response, DATABASE_KEY_UNREADABLE)
        self.add_datasource.assert_not_called()
        self.check_bucket.assert_not_called()
        self.assertFalse(Credential.objects.exists())

    def test_database_key_that_cannot_be_read_or_used_is_a_form_error(self):
        # Secret Manager refusing or failing the read, or a key that is
        # not a Fernet key: the page says so, with no 500, nothing is saved,
        # and no key text is shown or logged
        for case, failure in DATABASE_KEY_FAILURES.items():
            with self.subTest(case):
                records = self.capture_logs()
                with failure():
                    response = self.post()
                self.assertEqual(response.status_code, 200)
                self.assertContains(response, DATABASE_KEY_UNREADABLE)
                self.assertNotContains(response, MALFORMED_DB_KEY.decode("ascii"))
                self.add_datasource.assert_not_called()
                self.check_bucket.assert_not_called()
                self.assertFalse(Credential.objects.exists())
                for record in records:
                    self.assertNotIn(
                        MALFORMED_DB_KEY.decode("ascii"), record.getMessage()
                    )

    def test_existing_datasource_name_leaves_no_row(self):
        self.add_datasource.side_effect = GrafanaClientError(
            409,
            {},
            "Client Error 409: data source with the same name already exists",
        )
        response = self.post()
        self.assertEqual(response.status_code, 200)
        self.assertContains(response, "Credential with the same name already exists")
        self.assertFalse(Credential.objects.exists())

    def test_other_grafana_error_leaves_no_row(self):
        self.add_datasource.side_effect = GrafanaClientError(400, {}, "Client Error 400: bad")
        with self.assertRaises(GrafanaClientError):
            self.post()
        self.assertFalse(Credential.objects.exists())

    def test_state_bucket_warning_is_shown_and_the_row_kept(self):
        self.check_bucket.return_value = "This credential cannot use the state bucket"
        response = self.post()
        self.assertRedirects(response, reverse("credentials"), fetch_redirect_response=False)
        credential = Credential.objects.get(name="with-key")
        self.check_bucket.assert_called_once_with(credential)
        shown = [str(m) for m in response.wsgi_request._messages]
        self.assertIn("This credential cannot use the state bucket", shown)


class CredentialUpdateViewTests(TestCase):
    """The credential edit page replaces a stored key, or leaves the row as
    it was"""

    NEW_PROJECT = "replacement-project"

    def setUp(self):
        admin = User.objects.create_superuser(
            username="credential-admin", password="admin-pass", email="admin@example.com"
        )
        self.client.force_login(admin)
        # Faked so these tests make no network call
        patcher = mock.patch(
            "ghpcfe.cluster_manager.validate_credential.check_state_bucket_access",
            return_value=None,
        )
        self.addCleanup(patcher.stop)
        self.check_bucket = patcher.start()
        self.credential = Credential.objects.create(
            name="key-cred", owner=admin, detail=KEY_JSON
        )
        self.stored = Credential.objects.values_list(
            "detail", "project_id"
        ).get(pk=self.credential.pk)

    def post(self):
        return self.client.post(
            reverse("credential-update", args=[self.credential.pk]),
            {
                "name": self.credential.name,
                "kind": credentials.KIND_KEY,
                "detail": make_key_json(
                    project=self.NEW_PROJECT,
                    email=f"provisioner@{self.NEW_PROJECT}.iam.gserviceaccount.com",
                ),
            },
        )

    def test_a_new_key_is_saved(self):
        response = self.post()
        self.assertRedirects(response, reverse("credentials"), fetch_redirect_response=False)
        self.credential.refresh_from_db()
        self.assertEqual(self.credential.project_id, self.NEW_PROJECT)
        self.assertTrue(credentials.is_sealed(self.credential.detail))
        self.assertNotEqual(self.credential.detail, self.stored[0])

    def test_rename_with_a_blank_key_keeps_the_stored_key(self):
        with mock.patch(
            "ghpcfe.cluster_manager.validate_credential.validate_credential"
        ) as validate, mock.patch.object(
            credentials, "validate_key_json"
        ) as validate_key:
            response = self.client.post(
                reverse("credential-update", args=[self.credential.pk]),
                {"name": "renamed", "kind": credentials.KIND_KEY, "detail": ""},
            )
        self.assertRedirects(
            response, reverse("credentials"), fetch_redirect_response=False
        )
        validate.assert_not_called()
        validate_key.assert_not_called()
        self.assertEqual(
            Credential.objects.values_list(
                "name", "kind", "detail", "project_id"
            ).get(pk=self.credential.pk),
            ("renamed", credentials.KIND_KEY, *self.stored),
        )

    def test_edit_page_says_a_blank_key_keeps_the_stored_one(self):
        response = self.client.get(
            reverse("credential-update", args=[self.credential.pk])
        )
        self.assertContains(response, "Leave blank to keep the stored key.")
        # Nothing is stored for a new credential
        response = self.client.get(reverse("credential-create"))
        self.assertNotContains(response, "Leave blank to keep the stored key.")

    def test_unreadable_database_key_is_a_form_error(self):
        with database_key_unreadable():
            response = self.post()
        self.assertEqual(response.status_code, 200)
        self.assertContains(response, DATABASE_KEY_UNREADABLE)
        self.check_bucket.assert_not_called()
        self.assertEqual(
            Credential.objects.values_list("detail", "project_id").get(
                pk=self.credential.pk
            ),
            self.stored,
        )

    def test_database_key_that_cannot_be_read_or_used_is_a_form_error(self):
        for case, failure in DATABASE_KEY_FAILURES.items():
            with self.subTest(case):
                with failure():
                    response = self.post()
                self.assertEqual(response.status_code, 200)
                self.assertContains(response, DATABASE_KEY_UNREADABLE)
                self.assertNotContains(response, MALFORMED_DB_KEY.decode("ascii"))
                self.check_bucket.assert_not_called()
                # The row is as it was
                self.assertEqual(
                    Credential.objects.values_list("detail", "project_id").get(
                        pk=self.credential.pk
                    ),
                    self.stored,
                )
