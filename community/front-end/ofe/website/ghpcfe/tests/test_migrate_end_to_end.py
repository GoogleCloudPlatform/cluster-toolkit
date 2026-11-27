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

"""Upgrading a server whose resources still keep local Terraform state.

The server's start-up (manage.py ofe_bootstrap_credentials) deletes the
plaintext key files older releases kept in each run-time directory. The
operator then runs script/migrate_tf_state_to_gcs.sh --yes, which must
still run every Terraform command as each resource's stored credential,
through the real manage.py ofe_terraform_as_credential, with no key file.

The test runs both, in that order, on one run-time tree: the start-up
command in this process (the metadata server and Secret Manager are the
fakes test_credential_bootstrap uses), then the real script, in a copy of
this website tree so that its paths resolve to the run-time tree, with the
test interpreter as the application's. The database the command reads is a
file copy of the test database, with the test's database key. Only
Terraform is replaced, by a stand-in on PATH that records its arguments and
environment.
"""

import io
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
from pathlib import Path
from unittest import mock

from django.conf import settings
from django.core.management import call_command
from django.db import connection
from django.test import TransactionTestCase

from ghpcfe.cluster_manager import credentials
from ghpcfe.cluster_manager import utils
from ghpcfe.models import Credential, User, VirtualNetwork
from ghpcfe.tests.test_credential_bootstrap import (
    FakeMetadata,
    FakeSecretManager,
)
from ghpcfe.tests.test_credentials import ACCOUNT, KEY_JSON, PROJECT

OFE_DIR = Path(__file__).resolve().parents[3]
WEBSITE_DIR = OFE_DIR / "website"
MIGRATE_SCRIPT = OFE_DIR / "script" / "migrate_tf_state_to_gcs.sh"

BUCKET = "e2e-state-bucket"
IMPERSONATED_PROJECT = "impersonated-project"
STATE = '{"version": 4, "resources": [{"type": "google_compute_network"}]}'

# Variables that give Terraform, its GCS backend or gcloud an identity
# other than the stored credential's. Listed here, not taken from
# credentials.AMBIENT_CREDENTIAL_VARIABLES, so that a variable the code
# forgets is still set in the server's environment and caught.
IDENTITY_VARIABLES = (
    "GOOGLE_APPLICATION_CREDENTIALS",
    "GOOGLE_CREDENTIALS",
    "GOOGLE_CLOUD_KEYFILE_JSON",
    "GCLOUD_KEYFILE_JSON",
    "GOOGLE_OAUTH_ACCESS_TOKEN",
    "GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
    "GOOGLE_BACKEND_CREDENTIALS",
    "GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT",
    "CLOUDSDK_CONFIG",
    "CLOUDSDK_AUTH_ACCESS_TOKEN_FILE",
    "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
    "CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT",
    "CLOUDSDK_CORE_ACCOUNT",
    "CLOUDSDK_CORE_PROJECT",
)

# The stand-in for terraform. It appends one JSON line per run to the log:
# its arguments, its whole environment, and the old key copies and key
# files that exist while it runs. init -migrate-state empties the local
# state, as the real one does after copying it to the backend; plan
# -detailed-exitcode reports no changes.
TERRAFORM_STAND_IN = """#!{python}
import json, os, sys
from pathlib import Path

args = sys.argv[1:]
config = os.environ.get("CLOUDSDK_CONFIG", "")
run_dir = Path(config).parent if config else None
record = {{
    "argv": args,
    "env": dict(os.environ),
    "old_key_copies": sorted(
        str(path)
        for name in ("cloud_credentials", ".gcloud_config")
        for path in Path({vpcs!r}).rglob(name)
    ),
    "run_dir_files": (
        sorted(os.listdir(run_dir)) if run_dir and run_dir.is_dir() else None
    ),
    "config_files": (
        sorted(os.listdir(config)) if config and Path(config).is_dir() else None
    ),
}}
with open({log!r}, "a", encoding="utf-8") as fp:
    fp.write(json.dumps(record) + "\\n")
subcommand = next((arg for arg in args if not arg.startswith("-")), None)
chdir = next((arg[len("-chdir="):] for arg in args if arg.startswith("-chdir=")), "")
if subcommand == "version":
    print("Terraform v1.9.0 (stand-in)")
elif subcommand == "init" and "-migrate-state" in args:
    Path(chdir, "terraform.tfstate").write_text(
        '{{"version": 4, "resources": []}}', encoding="utf-8"
    )
"""


class MigrateAfterStartUpTests(TransactionTestCase):
    """ofe_bootstrap_credentials, then migrate_tf_state_to_gcs.sh --yes"""

    def setUp(self):
        credentials.forget_db_key()
        self.addCleanup(credentials.forget_db_key)
        self.root = Path(tempfile.mkdtemp(prefix="ofe-migrate-e2e-"))
        self.addCleanup(shutil.rmtree, self.root, ignore_errors=True)

        # The OFE tree the script runs in: the script, this website (the
        # real management command and cluster_manager code) and the
        # configuration the script's own Python reads the bucket from. It
        # sits as deep as an installed server's tree
        # (/opt/gcluster/cluster-toolkit/community/front-end/ofe):
        # website/settings.py looks four levels above website/ for the log
        # folder, which must stay inside this test's own root.
        self.ofe = (
            self.root / "gcluster" / "cluster-toolkit" / "community"
            / "front-end" / "ofe"
        )
        (self.ofe / "script").mkdir(parents=True)
        shutil.copy2(MIGRATE_SCRIPT, self.ofe / "script" / MIGRATE_SCRIPT.name)
        shutil.copytree(
            WEBSITE_DIR,
            self.ofe / "website",
            ignore=shutil.ignore_patterns(
                "__pycache__", ".venv*", ".secret_key", "*.sqlite3"
            ),
        )
        (self.ofe / "configuration.yaml").write_text(
            f'config:\n  server:\n    terraform_state_bucket: "{BUCKET}"\n',
            encoding="utf-8",
        )

        bin_dir = self.root / "bin"
        bin_dir.mkdir()
        self.terraform_log = self.root / "terraform-runs.jsonl"
        terraform = bin_dir / "terraform"
        terraform.write_text(
            TERRAFORM_STAND_IN.format(
                python=sys.executable,
                vpcs=str(self.ofe / "vpcs"),
                log=str(self.terraform_log),
            ),
            encoding="utf-8",
        )
        terraform.chmod(0o755)
        self.bin_dir = bin_dir

        # One admin owns everything, so no Role rows are needed
        self.admin = User.objects.create_superuser(
            username="admin", password="admin-pass", email="admin@example.com"
        )
        # A key saved before encryption, as an older release left it
        self.key_credential = Credential.objects.create(
            name="legacy-key", owner=self.admin, detail="{}"
        )
        Credential.objects.filter(pk=self.key_credential.pk).update(
            detail=KEY_JSON, project_id=""
        )
        self.impersonate_credential = Credential.objects.create(
            name="keyless",
            owner=self.admin,
            kind=credentials.KIND_IMPERSONATE,
            detail=ACCOUNT,
            project_id=IMPERSONATED_PROJECT,
        )
        self.key_vpc = self.make_vpc("net-key", self.key_credential)
        self.impersonate_vpc = self.make_vpc(
            "net-keyless", self.impersonate_credential
        )

    def make_vpc(self, name, credential):
        """A network with local state and the old key copies beside it"""
        vpc = VirtualNetwork.objects.create(
            name=name,
            cloud_id=name,
            cloud_credential=credential,
            cloud_region="us-central1",
            cloud_state="m",
        )
        run_dir = self.ofe / "vpcs" / f"vpc_{vpc.id}"
        run_dir.mkdir(parents=True)
        (run_dir / "terraform.tfstate").write_text(STATE, encoding="utf-8")
        (run_dir / credentials.LEGACY_KEY_FILE).write_text(
            KEY_JSON, encoding="utf-8"
        )
        gcloud = run_dir / credentials.LEGACY_GCLOUD_CONFIG
        gcloud.mkdir()
        (gcloud / "application_default_credentials.json").write_text(
            KEY_JSON, encoding="utf-8"
        )
        return vpc

    def start_server(self):
        """The start-up command, with the backend's run-time tree here"""
        out = io.StringIO()
        with mock.patch.dict(utils.g_config, {"baseDir": self.ofe}), mock.patch(
            "requests.get", side_effect=FakeMetadata()
        ), mock.patch(
            "google.cloud.secretmanager.SecretManagerServiceClient",
            return_value=FakeSecretManager(),
        ):
            call_command("ofe_bootstrap_credentials", stdout=out)
        return out.getvalue()

    def database_file(self):
        """A file copy of the test database, and settings that use it"""
        path = self.root / "db.sqlite3"
        connection.ensure_connection()
        target = sqlite3.connect(path)
        try:
            connection.connection.backup(target)
        finally:
            target.close()
        (self.ofe / "website" / "e2e_settings.py").write_text(
            "from ghpcfe.tests.settings import *  # pylint: disable=wildcard-import\n"
            "DATABASES = {'default': {'ENGINE': 'django.db.backends.sqlite3', "
            f"'NAME': {str(path)!r}}}}}\n"
            "OFE_CREDENTIAL_ENCRYPTION_KEY = "
            f"{settings.OFE_CREDENTIAL_ENCRYPTION_KEY!r}\n",
            encoding="utf-8",
        )

    def ambient(self):
        """A value for every variable that gives Terraform, its GCS backend
        or gcloud an identity (the list here, and any the code drops), and
        a key file for those that name one"""
        decoy = self.root / "ambient-key.json"
        decoy.write_text(KEY_JSON, encoding="utf-8")
        values = {
            name: f"ambient-{name.lower()}"
            for name in (
                *IDENTITY_VARIABLES, *credentials.AMBIENT_CREDENTIAL_VARIABLES
            )
        }
        for name in (
            "GOOGLE_APPLICATION_CREDENTIALS",
            "GOOGLE_BACKEND_CREDENTIALS",
            "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
        ):
            values[name] = str(decoy)
        return values

    def migrate(self):
        env = {
            "PATH": f"{self.bin_dir}:/usr/bin:/bin",
            "HOME": str(self.root),
            "LC_ALL": "C",
            "OFE_PYTHON": sys.executable,
            "DJANGO_SETTINGS_MODULE": "e2e_settings",
            "PYTHONDONTWRITEBYTECODE": "1",
            "TMPDIR": tempfile.gettempdir(),
            **self.ambient(),
        }
        return subprocess.run(
            ["bash", str(self.ofe / "script" / MIGRATE_SCRIPT.name), "--yes"],
            env=env,
            cwd=self.ofe,
            capture_output=True,
            text=True,
            timeout=300,
            check=False,
        )

    def terraform_runs(self):
        return [
            json.loads(line)
            for line in self.terraform_log.read_text(encoding="utf-8").splitlines()
        ]

    def test_migration_runs_as_each_stored_credential_after_start_up(self):
        run_dirs = {
            vpc.pk: self.ofe / "vpcs" / f"vpc_{vpc.pk}"
            for vpc in (self.key_vpc, self.impersonate_vpc)
        }

        # 1. The server starts: the old key copies are deleted and the
        #    legacy key is encrypted
        report = self.start_server()
        self.assertIn("Old key copies on disk: 2 files and 2 gcloud", report)
        self.assertIn("Credentials are ready", report)
        for run_dir in run_dirs.values():
            self.assertFalse((run_dir / credentials.LEGACY_KEY_FILE).exists())
            self.assertFalse((run_dir / credentials.LEGACY_GCLOUD_CONFIG).exists())
            self.assertTrue((run_dir / "terraform.tfstate").is_file())
        stored = Credential.objects.get(pk=self.key_credential.pk)
        self.assertTrue(credentials.is_sealed(stored.detail))
        self.assertEqual(stored.project_id, PROJECT)

        # 2. The operator migrates the state
        self.database_file()
        result = self.migrate()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Migrated: 2   Skipped: 0   Failed: 0", result.stdout)

        # Each network: version (the check before any change), then init
        # -migrate-state and plan in its directory, as its own credential
        runs = self.terraform_runs()
        self.assertEqual(len(runs), 6, [run["argv"] for run in runs])
        expected = {
            self.key_vpc.pk: self.assert_runs_as_key,
            self.impersonate_vpc.pk: self.assert_runs_as_impersonation,
        }
        for index in range(0, 6, 3):
            version, init, plan = runs[index:index + 3]
            run_dir = init["argv"][0][len("-chdir="):]
            (pk,) = [pk for pk, path in run_dirs.items() if str(path) == run_dir]
            self.assertEqual(version["argv"], ["version"])
            self.assertEqual(
                init["argv"],
                [f"-chdir={run_dir}", "init", "-migrate-state", "-force-copy",
                 "-no-color"],
            )
            self.assertEqual(
                plan["argv"],
                [f"-chdir={run_dir}", "plan", "-detailed-exitcode", "-no-color"],
            )
            for run in (version, init, plan):
                with self.subTest(vpc=pk, argv=run["argv"]):
                    expected[pk](run["env"])
                    self.assert_no_key_file_or_ambient_identity(run)
        self.assertEqual(
            {int(run["argv"][0].rsplit("_", 1)[1]) for run in runs[1::3]},
            set(run_dirs),
        )

        # The state moved; the original is kept; no key copy came back
        for run_dir in run_dirs.values():
            self.assertFalse((run_dir / "terraform.tfstate").exists())
            self.assertEqual(
                (run_dir / "terraform.tfstate.premigration").read_text(
                    encoding="utf-8"
                ),
                STATE,
            )
            self.assertIn(
                f'bucket = "{BUCKET}"',
                (run_dir / "backend.tf").read_text(encoding="utf-8"),
            )
        self.assertEqual(list(self.ofe.rglob(credentials.LEGACY_KEY_FILE)), [])

    def assert_runs_as_key(self, env):
        self.assertEqual(json.loads(env["GOOGLE_CREDENTIALS"]), json.loads(KEY_JSON))
        self.assertEqual(env["CLOUDSDK_CORE_PROJECT"], PROJECT)
        self.assertNotIn("GOOGLE_IMPERSONATE_SERVICE_ACCOUNT", env)
        self.assertNotIn("CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT", env)

    def assert_runs_as_impersonation(self, env):
        self.assertEqual(env["GOOGLE_IMPERSONATE_SERVICE_ACCOUNT"], ACCOUNT)
        self.assertEqual(env["CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT"], ACCOUNT)
        self.assertEqual(env["CLOUDSDK_CORE_PROJECT"], IMPERSONATED_PROJECT)
        self.assertNotIn("GOOGLE_CREDENTIALS", env)

    def assert_no_key_file_or_ambient_identity(self, run):
        env = run["env"]
        ambient = self.ambient()
        # No key file at all, and nothing of the ambient identity
        for name in (
            "GOOGLE_APPLICATION_CREDENTIALS",
            "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
            "GOOGLE_BACKEND_CREDENTIALS",
            "GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT",
            "GOOGLE_CLOUD_KEYFILE_JSON",
            "GCLOUD_KEYFILE_JSON",
            "GOOGLE_OAUTH_ACCESS_TOKEN",
            "CLOUDSDK_AUTH_ACCESS_TOKEN_FILE",
            "CLOUDSDK_CORE_ACCOUNT",
        ):
            # A short message: the environment holds the stored key
            self.assertFalse(name in env, f"{name} reached Terraform")
        for name, value in ambient.items():
            self.assertFalse(
                env.get(name) == value, f"the ambient {name} reached Terraform"
            )
        # The run's own empty gcloud configuration, alone in its directory
        self.assertEqual(run["config_files"], [])
        self.assertEqual(run["run_dir_files"], ["gcloud"])
        self.assertTrue(
            Path(env["CLOUDSDK_CONFIG"]).parent.name.startswith("ofe-credential-")
        )
        self.assertFalse(Path(env["CLOUDSDK_CONFIG"]).exists())
        # The old key copies were already gone
        self.assertEqual(run["old_key_copies"], [])
