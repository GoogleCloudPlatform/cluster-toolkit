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

"""Tests for storing Terraform state in a GCS bucket.

Every resource the front end provisions must write its Terraform state to the
shared state bucket rather than to a terraform.tfstate on the server's local
disk. State on local disk is lost when the server is rebuilt, and a resource
whose state is lost cannot be destroyed by Terraform.

These tests are deliberately importable WITHOUT Django. The repository's
pre-commit configuration runs a bare `pytest` from the repository root on any
changed Python file, with no rootdir configuration and no DJANGO_SETTINGS_MODULE,
so a module that imported Django (or any ghpcfe module that does) at import time
would error during collection in CI. `utils` itself imports only the standard
library and PyYAML, so it is loaded here directly by path; it imports the
credentials module, which needs Django, only when a run acts as a credential,
and these tests replace that module with FakeCredentials. filesystem.py is
loaded the same way, with its Django-dependent imports (models, site settings,
credentials) stubbed.
"""

import ast
import contextlib
import importlib.util
import json
import os
import re
import subprocess
import sys
import tempfile
import types
import unittest
from pathlib import Path
from unittest import mock

import yaml

OFE_DIR = Path(__file__).resolve().parents[3]
CLUSTER_MANAGER = OFE_DIR / "website" / "ghpcfe" / "cluster_manager"


def _load_utils():
    """Import cluster_manager/utils.py without importing the ghpcfe package."""
    spec = importlib.util.spec_from_file_location(
        "ofe_cluster_manager_utils", CLUSTER_MANAGER / "utils.py"
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


utils = _load_utils()

# The server's ambient credential variables, none of which may reach a run
# given no credential. Pinned here, and checked against
# credentials.AMBIENT_CREDENTIAL_VARIABLES, read from its source because the
# module needs Django (GetTerraformStateTests).
AMBIENT_CREDENTIAL_VARIABLES = (
    "GOOGLE_APPLICATION_CREDENTIALS",
    "GOOGLE_CREDENTIALS",
    "GOOGLE_BACKEND_CREDENTIALS",
    "GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT",
    "GOOGLE_CLOUD_KEYFILE_JSON",
    "GCLOUD_KEYFILE_JSON",
    "GOOGLE_OAUTH_ACCESS_TOKEN",
    "GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
    "CLOUDSDK_CONFIG",
    "CLOUDSDK_AUTH_ACCESS_TOKEN_FILE",
    "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
    "CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT",
    "CLOUDSDK_CORE_ACCOUNT",
    "CLOUDSDK_CORE_PROJECT",
)


def code_ambient_credential_variables():
    """credentials.AMBIENT_CREDENTIAL_VARIABLES, read from credentials.py
    without importing it"""
    tree = ast.parse(
        (CLUSTER_MANAGER / "credentials.py").read_text(encoding="utf-8")
    )
    for node in tree.body:
        if isinstance(node, ast.Assign) and any(
            isinstance(target, ast.Name)
            and target.id == "AMBIENT_CREDENTIAL_VARIABLES"
            for target in node.targets
        ):
            return ast.literal_eval(node.value)
    raise AssertionError("credentials.py has no AMBIENT_CREDENTIAL_VARIABLES")


PROVISIONING_EMAIL = "prov@proj.iam.gserviceaccount.com"
# The environment a run that acts as the credential gets
CREDENTIAL_ENV = {"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT": PROVISIONING_EMAIL}


def fake_credentials():
    """A stand-in for cluster_manager/credentials.py, which needs Django: runs
    act as an impersonated provisioning account, and a failed run is raised
    unchanged (no permission refusal in its logs)."""
    module = types.SimpleNamespace()

    @contextlib.contextmanager
    def tool_environment(credential, key_file=False, base_env=None):
        env = dict(os.environ if base_env is None else base_env)
        env.update(CREDENTIAL_ENV)
        yield env

    module.tool_environment = mock.MagicMock(side_effect=tool_environment)
    module.python_credentials = mock.MagicMock(name="python_credentials")
    module.shell_identity = mock.MagicMock(
        return_value=f"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT={PROVISIONING_EMAIL}"
    )
    module.explain_failed_run = mock.MagicMock(side_effect=lambda cpe, *_: cpe)
    module.permission_error_from_output = mock.MagicMock(return_value=None)
    module.PermissionDeniedRunError = subprocess.CalledProcessError
    # The variables a run given no credential drops from its environment
    module.AMBIENT_CREDENTIAL_VARIABLES = AMBIENT_CREDENTIAL_VARIABLES
    return module


def acting_as_credential(test):
    """Patch utils so that runs which act as a credential use
    fake_credentials(); returns the fake module and a credential."""
    module = fake_credentials()
    patcher = mock.patch.object(utils, "_credentials", return_value=module)
    patcher.start()
    test.addCleanup(patcher.stop)
    credential = mock.Mock(name="credential", kind="impersonate", project_id="proj")
    return module, credential


class TerraformStatePrefixTests(unittest.TestCase):
    """The prefix scheme partitions state per database and per resource.

    Every prefix is scoped under the database's state namespace, so a fresh
    database (whose primary keys restart at 1) cannot reuse an earlier
    database's prefixes. Below the namespace, prefixes mirror the
    per-resource working-directory names the application creates on disk.
    """

    NS = "20261015-0a1b2c3d"

    def test_prefix_per_resource_kind(self):
        expected = {
            "cluster": "clusters/cluster_7",
            "filesystem": "fs/fs_7",
            "vpc": "vpcs/vpc_7",
            "workbench": "workbenches/workbench_7",
            "image": "images/image_7",
        }
        for kind, prefix in expected.items():
            with self.subTest(kind=kind):
                self.assertEqual(
                    utils.terraform_state_prefix(kind, 7, namespace=self.NS),
                    f"{self.NS}/{prefix}",
                )

    def test_default_namespace_comes_from_the_database(self):
        with mock.patch.object(
            utils, "get_state_namespace", return_value=self.NS
        ) as get_ns:
            self.assertEqual(
                utils.terraform_state_prefix("cluster", 1),
                f"{self.NS}/clusters/cluster_1",
            )
        get_ns.assert_called_once_with()

    def test_same_id_in_different_databases_does_not_collide(self):
        self.assertNotEqual(
            utils.terraform_state_prefix("cluster", 1, namespace=self.NS),
            utils.terraform_state_prefix("cluster", 1, namespace="20261016-ffffffff"),
        )

    def test_new_namespace_format_and_uniqueness(self):
        first, second = utils.new_state_namespace(), utils.new_state_namespace()
        for ns in (first, second):
            self.assertRegex(ns, r"^\d{8}-[0-9a-f]{8}$")
        self.assertNotEqual(first, second)

    def test_every_provisioned_kind_has_a_prefix(self):
        # All five resource types the front end provisions with Terraform.
        self.assertEqual(
            set(utils.TERRAFORM_STATE_PREFIXES),
            {"cluster", "filesystem", "vpc", "workbench", "image"},
        )

    def test_prefixes_are_distinct(self):
        prefixes = [
            utils.terraform_state_prefix(kind, 1, namespace=self.NS)
            for kind in utils.TERRAFORM_STATE_PREFIXES
        ]
        self.assertEqual(len(prefixes), len(set(prefixes)))

    def test_unknown_kind_is_rejected_before_any_database_lookup(self):
        with mock.patch.object(utils, "get_state_namespace") as get_ns:
            with self.assertRaises(ValueError):
                utils.terraform_state_prefix("not-a-resource", 1)
        get_ns.assert_not_called()


class StateBucketConfigTests(unittest.TestCase):
    """The bucket name reaches the application through configuration.yaml."""

    def test_returns_configured_bucket(self):
        config = {"server": {"terraform_state_bucket": "my-deploy-tfstate-ab12"}}
        with mock.patch.object(utils, "load_config", return_value=config):
            self.assertEqual(
                utils.get_terraform_state_bucket(), "my-deploy-tfstate-ab12"
            )

    def test_missing_bucket_is_fatal(self):
        # Falling back to local state would reinstate exactly the risk the
        # bucket exists to remove, so this must raise rather than default.
        for server in ({}, {"terraform_state_bucket": ""}):
            with self.subTest(server=server):
                with mock.patch.object(
                    utils, "load_config", return_value={"server": server}
                ):
                    with self.assertRaises(RuntimeError):
                        utils.get_terraform_state_bucket()


class WriteBackendTfTests(unittest.TestCase):
    """Raw-template resources get a backend.tf at template-copy time."""

    BUCKET = "my-deploy-tfstate-ab12"

    def _write(self, target_dir, prefix):
        with mock.patch.object(
            utils, "get_terraform_state_bucket", return_value=self.BUCKET
        ):
            return utils.write_backend_tf(target_dir, prefix)

    def test_writes_gcs_backend_with_bucket_and_prefix(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self._write(tmp, "vpcs/vpc_3")
            self.assertEqual(path, Path(tmp) / "backend.tf")
            content = path.read_text()
            self.assertIn('backend "gcs"', content)
            self.assertIn(f'bucket = "{self.BUCKET}"', content)
            self.assertIn('prefix = "vpcs/vpc_3"', content)

    def test_is_idempotent(self):
        # create_subnet re-enters a VPC directory created earlier, so writing
        # the backend again must leave it unchanged rather than duplicating it.
        with tempfile.TemporaryDirectory() as tmp:
            first = self._write(tmp, "vpcs/vpc_3").read_text()
            second = self._write(tmp, "vpcs/vpc_3").read_text()
            self.assertEqual(first, second)
            self.assertEqual(first.count("backend"), 1)


class GetTerraformStateTests(unittest.TestCase):
    """State is read through `terraform state pull`, not from a local file."""

    STATE = {"version": 4, "outputs": {"vpc_id": {"value": "my-net"}}}

    def setUp(self):
        # credentials.py needs Django; its stand-in names the ambient
        # credential variables
        patcher = mock.patch.object(
            utils, "_credentials", return_value=fake_credentials()
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_parses_pulled_state(self):
        completed = subprocess.CompletedProcess(
            args=[], returncode=0, stdout=json.dumps(self.STATE).encode()
        )
        with mock.patch.object(subprocess, "run", return_value=completed) as run:
            state = utils.get_terraform_state("/some/dir", extra_env={"A": "b"})
        self.assertEqual(state["outputs"]["vpc_id"]["value"], "my-net")
        cmdline = run.call_args.args[0]
        self.assertEqual(cmdline, ["terraform", "state", "pull"])
        self.assertEqual(run.call_args.kwargs["env"]["A"], "b")

    def test_the_ambient_variables_are_pinned(self):
        code = code_ambient_credential_variables()
        self.assertEqual(len(AMBIENT_CREDENTIAL_VARIABLES), 14)
        self.assertEqual(len(code), len(set(code)))
        self.assertEqual(set(code), set(AMBIENT_CREDENTIAL_VARIABLES))

    def test_pull_without_a_credential_gets_no_ambient_credential(self):
        # As run_terraform without a credential: the server's own identity
        # never reaches the pull
        completed = subprocess.CompletedProcess(
            args=[], returncode=0, stdout=json.dumps(self.STATE).encode()
        )
        ambient = {
            name: f"ambient-{name.lower()}"
            for name in AMBIENT_CREDENTIAL_VARIABLES
        }
        with mock.patch.dict(os.environ, {**ambient, "KEEP_ME": "1"}), \
                mock.patch.object(
                    subprocess, "run", return_value=completed
                ) as run:
            utils.get_terraform_state("/some/dir", extra_env={"A": "b"})
        env = run.call_args.kwargs["env"]
        for name in ambient:
            with self.subTest(variable=name):
                # Not assertNotIn: its failure message would print the
                # whole environment, secrets included, into the test log
                self.assertFalse(name in env, f"{name} reached the pull")
        self.assertEqual(env["KEEP_ME"], "1")
        self.assertEqual(env["A"], "b")

    def test_the_pull_acts_as_the_credential(self):
        # The state bucket is read as the resource's provisioning credential
        module, credential = acting_as_credential(self)
        completed = subprocess.CompletedProcess(
            args=[], returncode=0, stdout=json.dumps(self.STATE).encode()
        )
        with mock.patch.object(subprocess, "run", return_value=completed) as run:
            utils.get_terraform_state("/some/dir", credential=credential)
        module.tool_environment.assert_called_once_with(credential)
        env = run.call_args.kwargs["env"]
        self.assertEqual(
            env["GOOGLE_IMPERSONATE_SERVICE_ACCOUNT"], PROVISIONING_EMAIL
        )

    def test_unparseable_state_raises(self):
        completed = subprocess.CompletedProcess(
            args=[], returncode=0, stdout=b"not json"
        )
        with mock.patch.object(subprocess, "run", return_value=completed):
            with self.assertRaises(RuntimeError):
                utils.get_terraform_state("/some/dir")


class RunTerraformInputTests(unittest.TestCase):
    """Unattended terraform runs must never wait for input.

    A backend change (e.g. the first init of a resource created before state
    moved to GCS) makes `terraform init` ask whether to migrate state. Without
    -input=false that prompt blocks the provisioning task forever; with it,
    Terraform fails and says why. validate and output reject the flag.
    """

    def _cmdline(self, command):
        with tempfile.TemporaryDirectory() as tmp:
            with mock.patch.object(subprocess, "run") as run, \
                    mock.patch.object(utils, "_credentials",
                                      return_value=fake_credentials()):
                utils.run_terraform(tmp, command)
        return run.call_args.args[0]

    def test_prompting_commands_get_input_false(self):
        for command in ("init", "plan", "apply", "destroy"):
            with self.subTest(command=command):
                self.assertIn("-input=false", self._cmdline(command))

    def test_commands_without_the_flag_do_not_get_it(self):
        for command in ("validate", "output"):
            with self.subTest(command=command):
                self.assertNotIn("-input=false", self._cmdline(command))


class BlueprintBackendTests(unittest.TestCase):
    """Cluster Toolkit blueprints carry terraform_backend_defaults.

    The Toolkit writes the backend into every Terraform deployment group it
    generates (pkg/config/expand.go, pkg/modulewriter/tfwriter.go), so setting
    it at the top of the blueprint is all the generated clusters, filesystems
    and image-builder environments need.
    """

    TEMPLATE = (
        OFE_DIR
        / "website"
        / "ghpcfe"
        / "templates"
        / "blueprint"
        / "cluster_config.yaml.j2"
    )

    def test_cluster_template_declares_backend(self):
        text = self.TEMPLATE.read_text()
        self.assertIn("terraform_backend_defaults:", text)
        self.assertIn("type: gcs", text)
        self.assertIn("bucket: {{ state_bucket }}", text)
        self.assertIn("prefix: {{ state_prefix }}", text)

    def test_cluster_backend_precedes_deployment_groups(self):
        text = self.TEMPLATE.read_text()
        self.assertLess(
            text.index("terraform_backend_defaults:"),
            text.index("deployment_groups:"),
        )

    def test_generators_declare_backend(self):
        # filesystem.py and image.py build their blueprints as inline f-strings
        # and cannot be imported without Django, so assert on their source.
        for name in ("filesystem.py", "image.py"):
            with self.subTest(module=name):
                text = (CLUSTER_MANAGER / name).read_text()
                self.assertIn("terraform_backend_defaults:", text)
                self.assertIn("get_terraform_state_bucket()", text)
                self.assertIn("terraform_state_prefix(", text)

    def test_single_terraform_group_per_blueprint(self):
        # An explicit prefix is applied by the Toolkit to EVERY deployment
        # group, so two Terraform groups in one blueprint would collide on a
        # single state object. Each blueprint must keep exactly one.
        self.assertEqual(self.TEMPLATE.read_text().count("- group:"), 1)
        fs_text = (CLUSTER_MANAGER / "filesystem.py").read_text()
        self.assertEqual(fs_text.count("- group:"), 1)
        # The image blueprint has two groups, but the second is a Packer group
        # and the Toolkit writes a backend only for Terraform groups. Modules
        # with no explicit kind default to terraform (pkg/config/config.go
        # addKindToModules), so the Terraform group is the one WITHOUT
        # "kind: packer".
        image_text = (CLUSTER_MANAGER / "image.py").read_text()
        self.assertEqual(image_text.count("- group:"), 2)
        self.assertEqual(image_text.count("kind: packer"), 1)


class RawTemplateBackendTests(unittest.TestCase):
    """Networks and workbenches get their backend written at copy time."""

    def test_backend_written_after_template_copy(self):
        for name in ("vpc.py", "workbenchinfo.py"):
            with self.subTest(module=name):
                text = (CLUSTER_MANAGER / name).read_text()
                self.assertIn("write_backend_tf(", text)
                self.assertLess(
                    text.index("shutil.copytree"),
                    text.index("write_backend_tf("),
                    "backend must be written before any terraform init",
                )

    def test_no_module_reads_a_local_state_file(self):
        for path in CLUSTER_MANAGER.glob("*.py"):
            with self.subTest(module=path.name):
                for line in path.read_text().splitlines():
                    if line.lstrip().startswith("#") or '"""' in line:
                        continue
                    # .terraform/terraform.tfstate is the backend record
                    # `terraform init` writes, not the resource's state.
                    if '".terraform"' in line:
                        continue
                    self.assertNotIn(
                        'terraform.tfstate"', line,
                        f"{path.name} still references a local state file",
                    )


class TerraformInitCredentialsTests(unittest.TestCase):
    """`terraform init` must authenticate as the provisioning credential.

    A GCS backend is contacted during init, so without the resource's
    credential (run_terraform's credential, passed with the rest of `cloud`)
    Terraform falls back to application default credentials -- on the
    server, the front end's own service account, a different identity from
    the one that provisions the resource and possibly in a different project.
    """

    PROVISIONING_MODULES = (
        "clusterinfo.py",
        "image.py",
        "vpc.py",
        "filesystem.py",
        "workbenchinfo.py",
    )

    def test_every_init_passes_credentials(self):
        # Matches a run_terraform(..., "init" ...) call and the rest of its
        # argument list, across line breaks.
        call = re.compile(
            r"run_terraform\((?P<args>[^()]*?\"init\"[^()]*?)\)", re.S
        )
        for name in self.PROVISIONING_MODULES:
            text = (CLUSTER_MANAGER / name).read_text()
            calls = list(call.finditer(text))
            self.assertTrue(calls, f"no terraform init call found in {name}")
            for match in calls:
                with self.subTest(module=name):
                    self.assertRegex(
                        match.group("args"),
                        r"\*\*cloud\b|\bcredential=",
                        f"{name}: terraform init must act as the resource's "
                        "credential, or it will contact the state bucket as "
                        "the wrong identity",
                    )


def _load_filesystem():
    """Import cluster_manager/filesystem.py without Django.

    The module's only Django-dependent imports are the ghpcfe models, the
    site settings and the credentials module, none of which
    write_filestore_yaml touches. They are stubbed in sys.modules for the
    duration of the import, and the module is placed in a throwaway package so
    its relative imports resolve to the stubs and to the already-loaded
    `utils`.
    """
    pkg = types.ModuleType("ofe_stub")
    pkg.__path__ = []
    manager = types.ModuleType("ofe_stub.cluster_manager")
    manager.__path__ = []
    manager.utils = utils
    credentials = types.ModuleType("ofe_stub.cluster_manager.credentials")
    manager.credentials = credentials
    models = types.ModuleType("ofe_stub.models")
    for name in ("GCPFilestoreFilesystem", "Filesystem", "FilesystemImpl"):
        setattr(models, name, mock.MagicMock())
    website = types.ModuleType("website")
    website.__path__ = []
    settings = types.ModuleType("website.settings")
    settings.SITE_NAME = "test-site"
    stubs = {
        "ofe_stub": pkg,
        "ofe_stub.cluster_manager": manager,
        "ofe_stub.cluster_manager.utils": utils,
        "ofe_stub.cluster_manager.credentials": credentials,
        "ofe_stub.models": models,
        "website": website,
        "website.settings": settings,
    }
    with mock.patch.dict(sys.modules, stubs):
        spec = importlib.util.spec_from_file_location(
            "ofe_stub.cluster_manager.filesystem",
            CLUSTER_MANAGER / "filesystem.py",
        )
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)
    return module


filesystem = _load_filesystem()


class FilesystemBlueprintYamlTests(unittest.TestCase):
    """The real Filestore blueprint writer emits the GCS state backend.

    Runs the real write_filestore_yaml against a stand-in filesystem object,
    so no database or Django settings are needed.
    """

    NS = "20261015-0a1b2c3d"

    def test_generated_filesystem_yaml_has_backend(self):
        fs = mock.MagicMock()
        fs.id = 12
        fs.name = "testfs"
        fs.cloud_credential.project_id = "proj"
        fs.cloud_region = "europe-west4"
        fs.cloud_zone = "europe-west4-a"
        fs.vpc.cloud_id = "testnet"
        fs.capacity = 1024
        fs.exports.first.return_value.export_name = "/share"
        fs.get_performance_tier_display.return_value = "BASIC_HDD"

        with tempfile.TemporaryDirectory() as tmp:
            with mock.patch.object(
                utils, "get_terraform_state_bucket", return_value="bucket-x"
            ), mock.patch.object(
                utils, "get_state_namespace", return_value=self.NS
            ):
                filesystem.write_filestore_yaml(fs, Path(tmp))
            doc = yaml.safe_load((Path(tmp) / "filesystem.yaml").read_text())

        backend = doc["terraform_backend_defaults"]
        self.assertEqual(backend["type"], "gcs")
        self.assertEqual(backend["configuration"]["bucket"], "bucket-x")
        self.assertEqual(
            backend["configuration"]["prefix"], f"{self.NS}/fs/fs_12"
        )
        terraform_groups = [
            g for g in doc["deployment_groups"]
            if any(m.get("kind") == "terraform" for m in g["modules"])
        ]
        self.assertEqual(len(terraform_groups), 1)
        self.assertEqual(len(doc["deployment_groups"]), 1)


class RunTerraformLockTimeoutTests(unittest.TestCase):
    """A second operation on one resource waits for the state lock.

    Terraform's default -lock-timeout is 0, so without the flag a second
    operation fails at once with "Error acquiring the state lock". Only the
    commands that take the lock get it.
    """

    def _cmdline(self, command):
        with tempfile.TemporaryDirectory() as tmp:
            with mock.patch.object(subprocess, "run") as run, \
                    mock.patch.object(utils, "_credentials",
                                      return_value=fake_credentials()):
                utils.run_terraform(tmp, command)
        return run.call_args.args[0]

    def test_locking_commands_get_lock_timeout(self):
        for command in ("plan", "apply", "destroy", "import", "refresh"):
            with self.subTest(command=command):
                self.assertIn("-lock-timeout=45m", self._cmdline(command))

    def test_other_commands_do_not_get_it(self):
        for command in ("init", "validate", "output"):
            with self.subTest(command=command):
                self.assertFalse(
                    any(a.startswith("-lock-timeout") for a in
                        self._cmdline(command))
                )


class StaleStateLockTests(unittest.TestCase):
    """A provably stale lock fails fast with the recovery command.

    A lock is stale only if Terraform was killed while holding it. It is
    reported only when it was taken on this host and no terraform process
    here runs in the resource's directory; otherwise Terraform runs and
    waits. The lock is never removed automatically.
    """

    HOST = "livetest1-server"
    GENERATION = 1760566563123456

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.dir = Path(tmp.name)
        self._write_backend("gcs")
        # Each run acts as the resource's credential, which the lock check
        # also reads the lock object as
        self.credentials, self.credential = acting_as_credential(self)
        for target, value in (
            ("socket.gethostname", f"{self.HOST}.c.example.internal"),
            ("_terraform_running_in", False),
        ):
            patcher = mock.patch.object(
                utils.socket if target.startswith("socket") else utils,
                target.split(".")[-1], return_value=value,
            )
            setattr(self, target.split(".")[-1].strip("_"), patcher.start())
            self.addCleanup(patcher.stop)

    def _write_backend(self, backend_type):
        marker = self.dir / ".terraform" / "terraform.tfstate"
        marker.parent.mkdir(exist_ok=True)
        marker.write_text(json.dumps({
            "version": 3,
            "backend": {
                "type": backend_type,
                "config": {"bucket": "state-bkt", "prefix": "ns/vpcs/vpc_1"},
            },
        }))

    def _client(self, who=None):
        """A storage client whose lock object is held by `who` (None: free)."""
        client = mock.MagicMock()
        if who is None:
            client.bucket.return_value.get_blob.return_value = None
            return client
        blob = mock.MagicMock()
        blob.generation = self.GENERATION
        blob.download_as_bytes.return_value = json.dumps({
            "ID": "8c0ab9f4-random-id",
            "Operation": "OperationTypeApply",
            "Who": who,
            "Version": "1.16.1",
            "Created": "2026-10-15T22:16:03.123456789Z",
            "Path": "gs://state-bkt/ns/vpcs/vpc_1/default.tflock",
        }).encode()
        client.bucket.return_value.get_blob.return_value = blob
        return client

    def _run(self, client, command="plan", expect_check=True):
        """Run command; the lock check must read the bucket as the
        credential, or (expect_check=False) not read it at all"""
        with mock.patch.object(utils, "_gcs_client", return_value=client) as gcs_client:
            with mock.patch.object(subprocess, "run") as run:
                utils.run_terraform(self.dir, command, credential=self.credential)
        if expect_check:
            gcs_client.assert_called_with(self.credential)
        else:
            gcs_client.assert_not_called()
        return run

    def test_no_lock_object_runs_terraform(self):
        client = self._client(None)
        run = self._run(client)
        run.assert_called_once()
        client.bucket.assert_called_with("state-bkt")
        client.bucket.return_value.get_blob.assert_called_with(
            "ns/vpcs/vpc_1/default.tflock"
        )

    def test_lock_held_by_another_host_runs_and_waits(self):
        run = self._run(self._client("gcluster@other-server"))
        run.assert_called_once()
        self.assertIn("-lock-timeout=45m", run.call_args.args[0])

    def test_lock_held_by_live_process_here_runs(self):
        self.terraform_running_in.return_value = True
        run = self._run(self._client(f"gcluster@{self.HOST}"))
        run.assert_called_once()

    def test_stale_lock_raises_before_terraform_runs(self):
        client = self._client(f"gcluster@{self.HOST}")
        with self.assertLogs(utils.logger, "ERROR"):
            with self.assertRaises(utils.TerraformStateLockError) as ctx:
                self._run(client)
        message = str(ctx.exception)
        for expected in (
            str(self.dir),
            f"Lock ID:   {self.GENERATION}",
            "OperationTypeApply",
            f"gcluster@{self.HOST}",
            "Age:",
            "2026-10-15T22:16:03",
            "no Terraform process on this server holds it",
            # The command acts as the credential the operation ran as
            f"cd {self.dir} && GOOGLE_IMPERSONATE_SERVICE_ACCOUNT={PROVISIONING_EMAIL}"
            f" terraform force-unlock -force {self.GENERATION}",
        ):
            with self.subTest(expected=expected):
                self.assertIn(expected, message)
        self.assertNotIn("Age:       unknown", message)
        self.assertEqual(ctx.exception.stderr, message.encode())
        client.bucket.return_value.get_blob.return_value.delete.assert_not_called()

    def test_stale_lock_not_reported_if_released_meanwhile(self):
        client = self._client(f"gcluster@{self.HOST}")
        blob = client.bucket.return_value.get_blob.return_value
        client.bucket.return_value.get_blob.side_effect = [blob, None]
        run = self._run(client)
        run.assert_called_once()

    def test_check_error_logs_warning_and_runs(self):
        with mock.patch.object(
            utils, "_gcs_client", side_effect=RuntimeError("storage down")
        ):
            with mock.patch.object(subprocess, "run") as run:
                with self.assertLogs(utils.logger, "WARNING") as logs:
                    utils.run_terraform(self.dir, "plan", credential=self.credential)
        run.assert_called_once()
        self.assertIn("storage down", "\n".join(logs.output))

    def test_non_gcs_backend_skips_check(self):
        self._write_backend("local")
        with mock.patch.object(utils, "_gcs_client") as gcs_client:
            with mock.patch.object(subprocess, "run") as run:
                utils.run_terraform(self.dir, "plan", credential=self.credential)
        gcs_client.assert_not_called()
        run.assert_called_once()

    def test_missing_backend_record_skips_check(self):
        (self.dir / ".terraform" / "terraform.tfstate").unlink()
        with mock.patch.object(utils, "_gcs_client") as gcs_client:
            with mock.patch.object(subprocess, "run") as run:
                utils.run_terraform(self.dir, "plan", credential=self.credential)
        gcs_client.assert_not_called()
        run.assert_called_once()

    def test_non_locking_command_skips_check(self):
        client = self._client(None)
        run = self._run(client, "init", expect_check=False)
        client.bucket.assert_not_called()
        run.assert_called_once()

    def test_lock_timeout_is_reported_from_stderr(self):
        stderr = (
            "\nError: Error acquiring the state lock\n\n"
            "Error message: writing \"gs://state-bkt/ns/vpcs/vpc_1/"
            "default.tflock\" failed: googleapi: Error 412\n"
            "Lock Info:\n"
            "  ID:        1760566999000001\n"
            "  Path:      gs://state-bkt/ns/vpcs/vpc_1/default.tflock\n"
            "  Operation: OperationTypeApply\n"
            "  Who:       gcluster@other-server\n"
            "  Version:   1.16.1\n"
            "  Created:   2026-10-15 22:16:03.123456789 +0000 UTC\n"
            "  Info:\n"
        )

        def fail(cmdline, **kwargs):
            kwargs["stderr"].write(stderr.encode())
            raise subprocess.CalledProcessError(1, cmdline)

        with mock.patch.object(
            utils, "_gcs_client", return_value=self._client(None)
        ):
            with mock.patch.object(subprocess, "run", side_effect=fail):
                with self.assertLogs(utils.logger, "ERROR"):
                    with self.assertRaises(
                        utils.TerraformStateLockError
                    ) as ctx:
                        utils.run_terraform(
                            self.dir, "plan", credential=self.credential
                        )
        message = str(ctx.exception)
        self.assertIn("waited 45m", message)
        self.assertIn("gcluster@other-server", message)
        self.assertIn("terraform force-unlock -force 1760566999000001", message)
        self.assertIsInstance(
            ctx.exception.__cause__, subprocess.CalledProcessError
        )

    def test_other_failures_are_reraised_unchanged(self):
        original = subprocess.CalledProcessError(1, ["terraform", "plan"])

        def fail(cmdline, **kwargs):
            kwargs["stderr"].write(b"Error: Invalid reference\n")
            raise original

        with mock.patch.object(
            utils, "_gcs_client", return_value=self._client(None)
        ):
            with mock.patch.object(subprocess, "run", side_effect=fail):
                with self.assertRaises(subprocess.CalledProcessError) as ctx:
                    utils.run_terraform(self.dir, "plan", credential=self.credential)
        self.assertIs(ctx.exception, original)

    def test_error_is_a_called_process_error(self):
        err = utils.TerraformStateLockError(["terraform", "plan"], "locked")
        self.assertIsInstance(err, subprocess.CalledProcessError)
        self.assertEqual(err.returncode, 1)
        self.assertEqual(str(err), "locked")


@unittest.skipUnless(Path("/proc/self/cwd").exists(), "needs Linux /proc")
class TerraformProcessScanTests(unittest.TestCase):
    """The process scan finds a process named terraform by working directory."""

    def test_finds_terraform_only_in_its_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            work, other = Path(tmp) / "work", Path(tmp) / "other"
            work.mkdir()
            other.mkdir()
            fake = Path(tmp) / "terraform"
            fake.symlink_to(sys.executable)
            proc = subprocess.Popen(
                [str(fake), "-c", "import time; time.sleep(30)"], cwd=work
            )
            try:
                self.assertTrue(utils._terraform_running_in(work))
                self.assertFalse(utils._terraform_running_in(other))
            finally:
                proc.kill()
                proc.wait()

    def test_unreadable_process_directory_counts_as_running(self):
        # Another user's terraform: its cwd can't be read, so it may hold the
        # lock and must not let the lock be called stale.
        with tempfile.TemporaryDirectory() as tmp:
            work, proc_root = Path(tmp) / "work", Path(tmp) / "proc"
            (proc_root / "4242").mkdir(parents=True)
            (proc_root / "4242" / "comm").write_text("terraform\n")
            (proc_root / "4242" / "cwd").symlink_to(work)
            work.mkdir()
            real_resolve = Path.resolve

            def resolve(path, strict=False):
                if path.name == "cwd":
                    raise PermissionError(13, "Permission denied", str(path))
                return real_resolve(path, strict=strict)

            with mock.patch.object(Path, "resolve", resolve):
                self.assertTrue(utils._terraform_running_in(work, proc_root=proc_root))


if __name__ == "__main__":
    unittest.main()
