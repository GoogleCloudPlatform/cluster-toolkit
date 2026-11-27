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

"""What the web server does with credentials when it starts.

The metadata server and Secret Manager are fakes: the start-up code takes
them as arguments, and the command test replaces the client libraries at
their own module boundaries.
"""

import io
import shutil
import tempfile
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import requests
from cryptography.fernet import Fernet
from django.core.management import call_command
from django.core.management.base import CommandError
from django.test import SimpleTestCase, TestCase, override_settings
from google.api_core.exceptions import FailedPrecondition, NotFound, PermissionDenied
from google.auth.exceptions import DefaultCredentialsError

from ghpcfe.cluster_manager import credentials
from ghpcfe.models import Credential, User
from ghpcfe.tests.test_credentials import KEY_JSON, PROJECT

SERVER_PROJECT = "ofe-server-project"
SECRET = "ofe-db-key"
PROVISIONER = f"ofe-prov-sa@{SERVER_PROJECT}.iam.gserviceaccount.com"
SECRET_NAME = f"projects/{SERVER_PROJECT}/secrets/{SECRET}"


class FakeMetadata:
    """requests.get for the metadata server"""

    def __init__(self, provisioner=PROVISIONER, secret=SECRET):
        self.values = {
            "project/project-id": SERVER_PROJECT,
            f"instance/attributes/{credentials.DB_KEY_SECRET_ATTRIBUTE}": secret,
            f"instance/attributes/{credentials.PROVISIONING_ACCOUNT_ATTRIBUTE}": provisioner,
        }

    def __call__(self, url, headers, timeout):
        assert headers == {"Metadata-Flavor": "Google"}
        assert timeout
        path = url[len(credentials.METADATA_URL) + 1:]
        value = self.values.get(path)
        response = mock.Mock()
        if value is None:
            response.status_code = 404
            response.text = ""
        else:
            response.status_code = 200
            response.text = value
        return response


class FakeSecretManager:
    """A Secret Manager client holding one secret.

    versions[i] is the data of version i + 1. It has only the two calls the
    server's account may make on the secret: access a version and add one.
    """

    # Secret Manager names a version by project number in its answers
    PROJECT_NUMBER = "123456789012"

    def __init__(self, versions=(), disabled=()):
        self.versions = list(versions)
        self.disabled = set(disabled)
        self.added = 0
        # Answers NotFound to this many versions/latest requests, as a
        # process that read before another process added the first version
        self.stale_reads = 0

    def access_secret_version(self, request):
        (name,) = request.values()
        prefix = f"{SECRET_NAME}/versions/"
        assert name.startswith(prefix), name
        version = name[len(prefix):]
        if version == "latest":
            if self.stale_reads:
                self.stale_reads -= 1
                raise NotFound("Secret Version not found or has no versions")
            number = len(self.versions)
        else:
            number = int(version)
        if not 1 <= number <= len(self.versions):
            raise NotFound("Secret Version not found or has no versions")
        if number in self.disabled:
            raise FailedPrecondition(f"Secret Version {number} is in DISABLED state")
        return SimpleNamespace(
            name=f"projects/{self.PROJECT_NUMBER}/secrets/{SECRET}/versions/{number}",
            payload=SimpleNamespace(data=self.versions[number - 1]),
        )

    def add_secret_version(self, request):
        assert request["parent"] == SECRET_NAME
        self.versions.append(request["payload"]["data"])
        self.added += 1


@override_settings(OFE_CREDENTIAL_ENCRYPTION_KEY=None)
class BootstrapTests(TestCase):
    """credentials.bootstrap, as the start-up command runs it"""

    def setUp(self):
        credentials.forget_db_key()
        self.addCleanup(credentials.forget_db_key)
        self.admin = User.objects.create_superuser(
            username="admin", password="admin-pass", email="admin@example.com"
        )

    def legacy_rows(self):
        """Two rows saved before encryption: a key, and text that is not one.

        update() writes the columns without Credential.save(), as the rows
        of a server from before encryption were written.
        """
        credentials.use_db_key(Fernet.generate_key())
        rows = [
            Credential.objects.create(name=name, owner=self.admin, detail="{}")
            for name in ("legacy-key", "legacy-text")
        ]
        Credential.objects.filter(pk=rows[0].pk).update(detail=KEY_JSON, project_id="")
        Credential.objects.filter(pk=rows[1].pk).update(detail="not json", project_id="")
        credentials.forget_db_key()
        return rows

    def bootstrap(self, secrets, metadata=None, base_dir=None):
        lines = []
        if base_dir is None:
            # An empty working-directory tree of the test's own
            base_dir = tempfile.mkdtemp(prefix="ofe-bootstrap-")
            self.addCleanup(shutil.rmtree, base_dir, ignore_errors=True)
        errors = credentials.bootstrap(
            Credential,
            User,
            http_get=metadata or FakeMetadata(),
            client=secrets,
            report=lines.append,
            base_dir=base_dir,
        )
        return errors, lines

    def stored(self, credential):
        return Credential.objects.values_list("detail", "project_id").get(
            pk=credential.pk
        )

    def test_first_boot_creates_the_key_and_registers_the_provisioner(self):
        secrets = FakeSecretManager()
        errors, lines = self.bootstrap(secrets)
        self.assertEqual(errors, [])
        self.assertEqual(secrets.added, 1)
        Fernet(secrets.versions[0])  # a valid Fernet key
        self.assertIn("Database key: created", lines)
        registered = Credential.objects.get(kind=credentials.KIND_IMPERSONATE)
        self.assertEqual(credentials.impersonation_target(registered), PROVISIONER)
        self.assertEqual(registered.project_id, SERVER_PROJECT)
        self.assertEqual(registered.owner, self.admin)
        self.assertEqual(registered.name, credentials.PROVISIONING_CREDENTIAL_NAME)
        for line in lines:
            self.assertNotIn(secrets.versions[0].decode(), line)

    def test_second_boot_changes_nothing(self):
        secrets = FakeSecretManager()
        self.bootstrap(secrets)
        errors, lines = self.bootstrap(secrets)
        self.assertEqual(errors, [])
        self.assertEqual(secrets.added, 1)
        self.assertEqual(Credential.objects.count(), 1)
        self.assertIn("Database key: present", lines)

    def test_no_provisioner_in_metadata_registers_nothing(self):
        errors, lines = self.bootstrap(
            FakeSecretManager(), FakeMetadata(provisioner=None)
        )
        self.assertEqual(errors, [])
        self.assertFalse(Credential.objects.exists())
        self.assertIn("No provisioning account in instance metadata", lines)

    def test_empty_provisioner_registers_nothing(self):
        errors, _ = self.bootstrap(FakeSecretManager(), FakeMetadata(provisioner=""))
        self.assertEqual(errors, [])
        self.assertFalse(Credential.objects.exists())

    def test_existing_impersonate_credential_is_kept(self):
        credentials.use_db_key(Fernet.generate_key())
        existing = Credential.objects.create(
            name="mine",
            owner=self.admin,
            kind=credentials.KIND_IMPERSONATE,
            detail=PROVISIONER,
            project_id=SERVER_PROJECT,
        )
        errors, _ = self.bootstrap(FakeSecretManager())
        self.assertEqual(errors, [])
        self.assertEqual(list(Credential.objects.all()), [existing])

    def test_legacy_plaintext_rows_are_encrypted_once(self):
        secrets = FakeSecretManager()
        legacy_key, legacy_text = self.legacy_rows()
        errors, lines = self.bootstrap(secrets, FakeMetadata(provisioner=""))
        self.assertEqual(len(errors), 1)
        self.assertIn(
            "Stored keys: 1 could not be read and were skipped "
            f"(credential ids: {legacy_text.pk})",
            errors[0],
        )
        self.assertIn(
            "Stored keys: 2 encrypted, 1 given a project id, 0 already done",
            lines,
        )
        detail, project_id = self.stored(legacy_key)
        self.assertTrue(detail.startswith(credentials.ENCRYPTED_PREFIX))
        self.assertNotIn("private_key", detail)
        self.assertEqual(project_id, PROJECT)
        self.assertEqual(
            credentials.key_json(Credential.objects.get(pk=legacy_key.pk)),
            KEY_JSON,
        )
        detail, project_id = self.stored(legacy_text)
        self.assertTrue(detail.startswith(credentials.ENCRYPTED_PREFIX))
        self.assertEqual(project_id, "")

        before = list(Credential.objects.values_list("detail", flat=True))
        credentials.forget_db_key()
        errors, lines = self.bootstrap(secrets, FakeMetadata(provisioner=""))
        self.assertEqual(len(errors), 1)
        self.assertIn(
            "Stored keys: 1 could not be read and were skipped "
            f"(credential ids: {legacy_text.pk})",
            errors[0],
        )
        self.assertEqual(
            list(Credential.objects.values_list("detail", flat=True)), before
        )
        self.assertIn(
            "Stored keys: 0 encrypted, 0 given a project id, 1 already done",
            lines,
        )

    def test_lost_key_is_not_replaced(self):
        credentials.use_db_key(Fernet.generate_key())
        Credential.objects.create(name="sealed", owner=self.admin, detail=KEY_JSON)
        credentials.forget_db_key()
        secrets = FakeSecretManager()
        errors, _ = self.bootstrap(secrets)
        self.assertEqual(secrets.added, 0)
        self.assertEqual(len(errors), 1)
        self.assertIn("has no version, but the database holds encrypted keys", errors[0])
        # Registration does not need the key
        self.assertTrue(
            Credential.objects.filter(kind=credentials.KIND_IMPERSONATE).exists()
        )

    def test_missing_secret_metadata_is_reported(self):
        errors, _ = self.bootstrap(FakeSecretManager(), FakeMetadata(secret=None))
        self.assertEqual(len(errors), 1)
        self.assertIn(credentials.DB_KEY_SECRET_ATTRIBUTE, errors[0])

    def test_key_is_read_from_the_secret_when_used(self):
        key = Fernet.generate_key()
        secrets = FakeSecretManager([key])
        loaded = credentials.load_db_keys(http_get=FakeMetadata(), client=secrets)
        self.assertEqual(loaded, [key])

    def test_bootstrap_removes_old_key_copies(self):
        with tempfile.TemporaryDirectory() as base:
            old_copy = Path(base) / "clusters" / "cluster_1" / "cloud_credentials"
            old_copy.parent.mkdir(parents=True)
            old_copy.write_text(KEY_JSON)
            errors, lines = self.bootstrap(FakeSecretManager(), base_dir=base)
            self.assertEqual(errors, [])
            self.assertFalse(old_copy.exists())
            self.assertIn(
                "Old key copies on disk: 1 files and 0 gcloud configuration "
                "directories removed, 0 symbolic links skipped",
                lines,
            )

    # Each bootstrap step, the function it calls that may raise, and how
    # the test sees that the step ran
    STEPS = {
        "Old key copies on disk": "remove_legacy_key_copies",
        "Database key or stored keys": "ensure_db_key",
        "Provisioning credential": "register_provisioning_credential",
    }
    # Key-like text, as a parse error about a stored key could quote it
    KEY_MARKER = '"private_key": "-----BEGIN'

    def assert_other_steps_ran(self, failed_step, lines):
        ran = {
            "Old key copies on disk": any(
                line.startswith("Old key copies on disk: 0 files") for line in lines
            ),
            "Database key or stored keys": "Database key: created" in lines,
            "Provisioning credential": Credential.objects.filter(
                kind=credentials.KIND_IMPERSONATE
            ).exists(),
        }
        del ran[failed_step]
        for step, did_run in ran.items():
            self.assertTrue(did_run, f"{step} did not run after {failed_step} failed")

    def test_unexpected_error_text_is_logged_not_returned(self):
        # An exception that is not a CredentialError may quote stored data:
        # only the step and the exception's type are returned (the pages
        # show them to superusers); the full text goes to the server log
        # Errors of several kinds, including OSError, whose text can quote
        # a file name or its contents
        kinds = (ValueError, OSError, KeyError)
        for (step, function), kind in (
            (item, kind) for item in self.STEPS.items() for kind in kinds
        ):
            with self.subTest(step=step, kind=kind.__name__):
                credentials.forget_db_key()
                Credential.objects.all().delete()
                error = kind(f"cannot parse {{{self.KEY_MARKER}...")
                with mock.patch.object(
                    credentials, function, side_effect=error
                ), self.assertLogs(credentials.logger, level="ERROR") as logs:
                    errors, lines = self.bootstrap(FakeSecretManager())
                self.assertEqual(
                    errors,
                    [
                        f"{step}: unexpected error ({kind.__name__}); "
                        "see the server log"
                    ],
                )
                for text in errors + lines:
                    self.assertNotIn(self.KEY_MARKER, text)
                self.assertIn(self.KEY_MARKER, "\n".join(logs.output))
                self.assert_other_steps_ran(step, lines)

    def test_credential_error_message_is_returned(self):
        for step, function in self.STEPS.items():
            with self.subTest(step):
                credentials.forget_db_key()
                Credential.objects.all().delete()
                error = credentials.CredentialError("The step could not finish")
                with mock.patch.object(credentials, function, side_effect=error):
                    errors, lines = self.bootstrap(FakeSecretManager())
                self.assertEqual(errors, [f"{step}: The step could not finish"])
                self.assert_other_steps_ran(step, lines)


@override_settings(OFE_CREDENTIAL_ENCRYPTION_KEY=None)
class DatabaseKeyVersionTests(TestCase):
    """Every enabled version of the database-key secret is used: the newest
    encrypts, any decrypts"""

    def setUp(self):
        credentials.forget_db_key()
        self.addCleanup(credentials.forget_db_key)
        self.owner = User.objects.create_user(username="key-owner", password="pass")

    def sealed_with(self, key, name):
        """A key credential encrypted with key alone"""
        credentials.use_db_key(key)
        credential = Credential.objects.create(name=name, owner=self.owner, detail=KEY_JSON)
        credentials.forget_db_key()
        return credential

    def readable(self, credential):
        return credentials.key_json(Credential.objects.get(pk=credential.pk)) == KEY_JSON

    def start(self, secrets, encrypted_rows_exist=True):
        return credentials.ensure_db_key(
            encrypted_rows_exist, http_get=FakeMetadata(), client=secrets
        )

    def test_rows_of_an_older_version_stay_readable(self):
        old, new = Fernet.generate_key(), Fernet.generate_key()
        before = self.sealed_with(old, "before-new-version")
        self.assertEqual(self.start(FakeSecretManager([old, new])), "present")
        self.assertTrue(self.readable(before))
        # New rows use the newest version
        created = Credential.objects.create(name="after", owner=self.owner, detail=KEY_JSON)
        token = created.detail[len(credentials.ENCRYPTED_PREFIX):].encode("ascii")
        self.assertEqual(Fernet(new).decrypt(token).decode("utf-8"), KEY_JSON)

    def test_a_process_that_loads_on_first_use_reads_every_version(self):
        old, new = Fernet.generate_key(), Fernet.generate_key()
        before = self.sealed_with(old, "before-new-version")
        secrets = FakeSecretManager([old, new])
        with mock.patch("requests.get", side_effect=FakeMetadata()), mock.patch.object(
            credentials, "_secret_client", return_value=secrets
        ):
            self.assertTrue(self.readable(before))

    def test_disabled_versions_are_skipped(self):
        keys = [Fernet.generate_key() for _ in range(3)]
        first = self.sealed_with(keys[0], "first")
        third = self.sealed_with(keys[2], "third")
        self.start(FakeSecretManager(keys, disabled={2}))
        self.assertTrue(self.readable(first))
        self.assertTrue(self.readable(third))

    def test_two_first_starts_never_make_rows_unreadable(self):
        secrets = FakeSecretManager()
        # The first start adds a version and encrypts a row with it
        self.assertEqual(self.start(secrets, encrypted_rows_exist=False), "created")
        first_row = Credential.objects.create(name="first", owner=self.owner, detail=KEY_JSON)
        credentials.forget_db_key()
        # A second start that found no version before the first added one
        # adds another, and encrypts a row with what it reads back
        secrets.stale_reads = 1
        self.assertEqual(self.start(secrets, encrypted_rows_exist=False), "created")
        second_row = Credential.objects.create(name="second", owner=self.owner, detail=KEY_JSON)
        credentials.forget_db_key()
        self.assertEqual(secrets.added, 2)
        # Every later start reads both rows, and adds nothing
        self.assertEqual(self.start(secrets), "present")
        self.assertEqual(secrets.added, 2)
        self.assertTrue(self.readable(first_row))
        self.assertTrue(self.readable(second_row))

    def test_an_unnamed_version_falls_back_to_the_latest(self):
        key = Fernet.generate_key()
        secrets = mock.Mock()
        secrets.access_secret_version.return_value = SimpleNamespace(
            name="", payload=SimpleNamespace(data=key)
        )
        with self.assertLogs(credentials.logger, level="WARNING"):
            keys = credentials.load_db_keys(http_get=FakeMetadata(), client=secrets)
        self.assertEqual(keys, [key])
        secrets.access_secret_version.assert_called_once_with(
            request={"name": f"{SECRET_NAME}/versions/latest"}
        )


class RefusingSecretManager(FakeSecretManager):
    """A Secret Manager client that answers every read with error"""

    def __init__(self, error):
        super().__init__()
        self.error = error

    def access_secret_version(self, request):
        raise self.error


@override_settings(OFE_CREDENTIAL_ENCRYPTION_KEY=None)
class DatabaseKeyErrorTests(TestCase):
    """A database key that cannot be read or used is a CredentialError,
    which the credential pages show as a form error and the start-up
    reports, with the cause chained and no key text in its message"""

    MALFORMED = b"not-a-fernet-key-but-secret-text"

    def setUp(self):
        credentials.forget_db_key()
        self.addCleanup(credentials.forget_db_key)

    def load(self, secrets):
        return credentials.load_db_keys(http_get=FakeMetadata(), client=secrets)

    def assert_credential_error(self, call, cause, *texts):
        with self.assertRaises(credentials.CredentialError) as caught:
            call()
        self.assertIsInstance(caught.exception.__cause__, cause)
        for text in texts:
            self.assertIn(text, str(caught.exception))
        self.assertNotIn(self.MALFORMED.decode("ascii"), str(caught.exception))

    def test_a_refused_read_is_a_credential_error(self):
        refused = PermissionDenied("Permission 'secretmanager.versions.access' denied")
        self.assert_credential_error(
            lambda: self.load(RefusingSecretManager(refused)),
            PermissionDenied,
            SECRET_NAME,
            "secretmanager.versions.access",
        )

    def test_a_secret_with_no_version_is_a_credential_error(self):
        self.assert_credential_error(
            lambda: self.load(FakeSecretManager()),
            NotFound,
            SECRET_NAME,
            # The wrapper's own text; the fake's NotFound also says "has no
            # versions"
            f"Secret {SECRET_NAME} does not exist or has no version, so "
            "stored keys cannot be encrypted or read",
        )

    def test_a_disabled_latest_version_is_a_credential_error(self):
        self.assert_credential_error(
            lambda: self.load(FakeSecretManager([Fernet.generate_key()], disabled={1})),
            FailedPrecondition,
            SECRET_NAME,
            "disabled or destroyed",
        )

    def test_no_application_default_credentials_is_a_credential_error(self):
        # The client library finds no credentials when its client is made
        with mock.patch(
            "google.cloud.secretmanager.SecretManagerServiceClient",
            side_effect=DefaultCredentialsError("Could not find default credentials"),
        ):
            self.assert_credential_error(
                lambda: credentials.load_db_keys(http_get=FakeMetadata()),
                DefaultCredentialsError,
                SECRET_NAME,
                "Could not find default credentials",
            )

    def test_a_malformed_key_is_a_credential_error(self):
        for keys in (self.MALFORMED, [Fernet.generate_key(), self.MALFORMED], [], None):
            with self.subTest(keys=keys):
                self.assert_credential_error(
                    lambda keys=keys: credentials.use_db_key(keys),
                    (ValueError, TypeError),
                    "database key",
                )

    def test_a_malformed_key_in_the_secret_fails_its_first_use(self):
        secrets = FakeSecretManager([self.MALFORMED])
        with mock.patch("requests.get", side_effect=FakeMetadata()), mock.patch(
            "google.cloud.secretmanager.SecretManagerServiceClient",
            return_value=secrets,
        ):
            for _ in range(2):
                # Nothing is cached: each use reads the secret again
                self.assert_credential_error(
                    lambda: credentials.seal(
                        Credential(kind=credentials.KIND_KEY, detail=KEY_JSON)
                    ),
                    ValueError,
                    "database key",
                )
            # Once the version holds a key, the next use reads and uses it
            secrets.versions[0] = Fernet.generate_key()
            credential = Credential(kind=credentials.KIND_KEY, detail=KEY_JSON)
            credentials.seal(credential)
        self.assertTrue(credentials.is_sealed(credential.detail))

    def test_a_malformed_key_setting_is_a_credential_error(self):
        with self.settings(OFE_CREDENTIAL_ENCRYPTION_KEY=self.MALFORMED.decode("ascii")):
            self.assert_credential_error(
                lambda: credentials.seal(
                    Credential(kind=credentials.KIND_KEY, detail=KEY_JSON)
                ),
                ValueError,
                "database key",
            )

    def test_a_refused_add_on_first_start_is_a_credential_error(self):
        secrets = FakeSecretManager()
        with mock.patch.object(
            secrets,
            "add_secret_version",
            side_effect=PermissionDenied("Permission 'secretmanager.versions.add' denied"),
        ):
            self.assert_credential_error(
                lambda: credentials.ensure_db_key(
                    False, http_get=FakeMetadata(), client=secrets
                ),
                PermissionDenied,
                SECRET_NAME,
                "secretmanager.versions.add",
            )

    def test_start_up_reports_each_as_its_error(self):
        User.objects.create_superuser(
            username="admin", password="admin-pass", email="admin@example.com"
        )
        cases = {
            "refused": (
                RefusingSecretManager(PermissionDenied("versions.access denied")),
                (SECRET_NAME, "versions.access denied"),
            ),
            "malformed": (
                FakeSecretManager([self.MALFORMED]),
                (SECRET_NAME, "database key"),
            ),
        }
        for case, (secrets, texts) in cases.items():
            with self.subTest(case):
                base_dir = tempfile.mkdtemp(prefix="ofe-bootstrap-")
                self.addCleanup(shutil.rmtree, base_dir, ignore_errors=True)
                errors = credentials.bootstrap(
                    Credential,
                    User,
                    http_get=FakeMetadata(),
                    client=secrets,
                    report=lambda line: None,
                    base_dir=base_dir,
                )
                self.assertEqual(len(errors), 1, errors)
                self.assertTrue(
                    errors[0].startswith("Database key or stored keys: "), errors
                )
                for text in texts:
                    self.assertIn(text, errors[0])
                self.assertNotIn(self.MALFORMED.decode("ascii"), errors[0])


class MetadataTimeoutTests(TestCase):
    """A metadata server that hangs or cannot be reached stops the read with
    a CredentialError, which the start-up reports, instead of blocking"""

    def failing(self, error):
        return mock.Mock(side_effect=error)

    def test_every_request_is_bounded(self):
        http_get = mock.Mock(return_value=mock.Mock(status_code=200, text="value"))
        credentials.metadata_value("project/project-id", http_get)
        self.assertEqual(
            http_get.call_args.kwargs["timeout"], credentials.METADATA_TIMEOUT_SECONDS
        )

    def test_timeout_is_a_credential_error(self):
        with self.assertRaisesMessage(
            credentials.CredentialError,
            "Cannot read project/project-id from the metadata server: read timed out",
        ):
            credentials.metadata_value(
                "project/project-id", self.failing(requests.Timeout("read timed out"))
            )

    def test_unreachable_server_is_a_credential_error(self):
        with self.assertRaisesMessage(
            credentials.CredentialError, "from the metadata server: refused"
        ):
            credentials.metadata_value(
                "project/project-id",
                self.failing(requests.ConnectionError("refused")),
            )

    def test_bootstrap_reports_it_as_its_error(self):
        lines = []
        with tempfile.TemporaryDirectory() as base:
            errors = credentials.bootstrap(
                Credential,
                User,
                http_get=self.failing(requests.Timeout("read timed out")),
                client=FakeSecretManager(),
                report=lines.append,
                base_dir=base,
            )
        self.assertEqual(len(errors), 2)
        self.assertIn("Database key or stored keys: Cannot read", errors[0])
        self.assertIn("Provisioning credential: Cannot read", errors[1])
        for error in errors:
            self.assertIn("from the metadata server: read timed out", error)


class LegacyKeyCopyTests(SimpleTestCase):
    """remove_legacy_key_copies deletes only old key copies, only under
    OFE's working directories"""

    def setUp(self):
        self.base = Path(tempfile.mkdtemp(prefix="ofe-legacy-keys-"))
        self.addCleanup(shutil.rmtree, self.base, ignore_errors=True)
        self.outside = Path(tempfile.mkdtemp(prefix="ofe-outside-"))
        self.addCleanup(shutil.rmtree, self.outside, ignore_errors=True)

    def write(self, relative, text="secret"):
        path = self.base / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
        return path

    def test_removes_each_kind_of_old_copy(self):
        copies = [
            self.write("clusters/cluster_1/cloud_credentials"),
            self.write("vpcs/vpc_2/cloud_credentials"),
            self.write("fs/fs_3/cloud_credentials"),
            self.write("workbenches/workbench_4/cloud_credentials"),
            self.write("images/image_5/cloud_credentials"),
        ]
        configs = [
            self.write("clusters/cluster_1/c1/primary/.gcloud_config/credentials.db"),
            self.write("vpcs/vpc_2/.gcloud_config/legacy_credentials/a/adc.json"),
            self.write("fs/fs_3/fs-a/primary/.gcloud_config/credentials.db"),
            self.write(
                "workbenches/workbench_4/terraform/google/.gcloud_config/credentials.db"
            ),
            self.write(
                "images/image_5/image_5/builder-env/.gcloud_config/credentials.db"
            ),
        ]
        keep = [
            self.write("clusters/cluster_1/cluster.yaml"),
            self.write("clusters/cluster_1/.ssh/id_rsa"),
            self.write("vpcs/vpc_2/terraform.tfstate"),
            self.write("clusters/cluster_1/cloud_credentials.bak"),
            # Only OFE's working directories are searched
            self.write("other/cloud_credentials"),
            self.write("other/.gcloud_config/credentials.db"),
        ]
        counts = credentials.remove_legacy_key_copies(self.base)
        self.assertEqual(
            counts, {"files": 5, "directories": 5, "skipped": 0, "failed": 0}
        )
        for path in copies:
            self.assertFalse(path.exists(), path)
        for path in configs:
            config_dir = next(p for p in path.parents if p.name == ".gcloud_config")
            self.assertFalse(config_dir.exists(), config_dir)
            self.assertTrue(config_dir.parent.is_dir())
        for path in keep:
            self.assertTrue(path.exists(), path)
        # A second run finds nothing
        self.assertEqual(
            credentials.remove_legacy_key_copies(self.base),
            {"files": 0, "directories": 0, "skipped": 0, "failed": 0},
        )

    def test_symbolic_links_are_never_followed_or_removed(self):
        target_file = self.outside / "cloud_credentials"
        target_file.write_text("outside")
        target_config = self.outside / ".gcloud_config"
        target_config.mkdir()
        (target_config / "credentials.db").write_text("outside")
        linked_tree = self.outside / "tree"
        (linked_tree / ".gcloud_config").mkdir(parents=True)
        (linked_tree / "cloud_credentials").write_text("outside")

        cluster = self.base / "clusters" / "cluster_1"
        cluster.mkdir(parents=True)
        (cluster / "cloud_credentials").symlink_to(target_file)
        (cluster / ".gcloud_config").symlink_to(target_config)
        (cluster / "linked").symlink_to(linked_tree)
        (self.base / "vpcs").symlink_to(linked_tree)

        counts = credentials.remove_legacy_key_copies(self.base)
        self.assertEqual(
            counts, {"files": 0, "directories": 0, "skipped": 2, "failed": 0}
        )
        self.assertTrue((cluster / "cloud_credentials").is_symlink())
        self.assertTrue((cluster / ".gcloud_config").is_symlink())
        self.assertEqual(target_file.read_text(), "outside")
        self.assertTrue((target_config / "credentials.db").exists())
        self.assertTrue((linked_tree / ".gcloud_config").is_dir())
        self.assertTrue((linked_tree / "cloud_credentials").exists())

    def test_terraform_cache_is_not_searched(self):
        cached = self.write("clusters/cluster_1/c1/primary/.terraform/cloud_credentials")
        credentials.remove_legacy_key_copies(self.base)
        self.assertTrue(cached.exists())

    def test_missing_base_dir_is_fine(self):
        self.assertEqual(
            credentials.remove_legacy_key_copies(self.base / "missing"),
            {"files": 0, "directories": 0, "skipped": 0, "failed": 0},
        )

    def test_count_not_contents_is_logged(self):
        self.write("clusters/cluster_1/cloud_credentials", text="KEYMATERIAL")
        with self.assertNoLogs(credentials.logger, level="INFO"):
            credentials.remove_legacy_key_copies(self.base)


@override_settings(OFE_CREDENTIAL_ENCRYPTION_KEY=None)
class CommandTests(TestCase):
    """manage.py ofe_bootstrap_credentials"""

    def setUp(self):
        credentials.forget_db_key()
        self.addCleanup(credentials.forget_db_key)
        User.objects.create_superuser(
            username="admin", password="admin-pass", email="admin@example.com"
        )

    def test_command_runs_every_step(self):
        secrets = FakeSecretManager()
        out = io.StringIO()
        with mock.patch("requests.get", side_effect=FakeMetadata()), mock.patch(
            "google.cloud.secretmanager.SecretManagerServiceClient",
            return_value=secrets,
        ):
            call_command("ofe_bootstrap_credentials", stdout=out)
        self.assertIn("Credentials are ready", out.getvalue())
        self.assertEqual(secrets.added, 1)
        self.assertEqual(
            Credential.objects.filter(kind=credentials.KIND_IMPERSONATE).count(), 1
        )

    def test_command_fails_when_a_step_fails(self):
        with mock.patch(
            "requests.get", side_effect=FakeMetadata(secret=None)
        ), mock.patch(
            "google.cloud.secretmanager.SecretManagerServiceClient",
            return_value=FakeSecretManager(),
        ):
            with self.assertRaisesRegex(CommandError, "ofe-db-key-secret"):
                call_command("ofe_bootstrap_credentials", stdout=io.StringIO())
        # The provisioning account is still registered
        self.assertEqual(Credential.objects.count(), 1)
