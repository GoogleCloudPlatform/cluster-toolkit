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

"""Tests for the state bucket check made when a credential is registered.

A provisioning credential whose service account cannot use the Terraform
state bucket fails only later, when Terraform first reads or writes state.
check_state_bucket_access() asks the bucket which permissions that account
holds and returns a warning saying how to grant them.

validate_credential.py is loaded by path into a throwaway package, with the
Google client libraries, `utils` and `credentials` stubbed in sys.modules, so
these tests need no Django settings, credentials or network. The check acts
as the credential, whether it holds a key or impersonates its account,
through credentials.python_credentials.
"""

import importlib.util
import sys
import types
import unittest
from pathlib import Path
from unittest import mock

CLUSTER_MANAGER = Path(__file__).resolve().parents[1] / "cluster_manager"

EMAIL = "prov@other-project.iam.gserviceaccount.com"
# A registered credential (a Credential row), in another project
CREDENTIAL = mock.Mock(name="credential", project_id="other-project")


# Stand-ins for google.api_core.exceptions, raised by the tests' fake bucket.
class NotFound(Exception):
    pass


class GoogleAPICallError(Exception):
    pass


class GoogleAuthError(Exception):
    pass


def _load_validate_credential():
    google = types.ModuleType("google")
    google.__path__ = []
    cloud = types.ModuleType("google.cloud")
    cloud.__path__ = []
    storage = types.ModuleType("google.cloud.storage")
    storage.Client = mock.MagicMock()
    cloud.storage = storage
    api_core = types.ModuleType("google.api_core")
    api_core.__path__ = []
    exceptions = types.ModuleType("google.api_core.exceptions")
    exceptions.NotFound = NotFound
    exceptions.GoogleAPICallError = GoogleAPICallError
    api_core.exceptions = exceptions
    auth = types.ModuleType("google.auth")
    auth.__path__ = []
    auth_exceptions = types.ModuleType("google.auth.exceptions")
    auth_exceptions.GoogleAuthError = GoogleAuthError
    auth.exceptions = auth_exceptions
    pkg = types.ModuleType("ofe_cred_stub")
    pkg.__path__ = []
    utils = types.ModuleType("ofe_cred_stub.utils")
    utils.load_config = mock.MagicMock()
    pkg.utils = utils
    credentials = types.ModuleType("ofe_cred_stub.credentials")
    credentials.identity_email = mock.MagicMock()
    credentials.python_credentials = mock.MagicMock()
    pkg.credentials = credentials
    stubs = {
        "google": google,
        "google.cloud": cloud,
        "google.cloud.storage": storage,
        "google.api_core": api_core,
        "google.api_core.exceptions": exceptions,
        "google.auth": auth,
        "google.auth.exceptions": auth_exceptions,
        "ofe_cred_stub": pkg,
        "ofe_cred_stub.utils": utils,
        "ofe_cred_stub.credentials": credentials,
    }
    with mock.patch.dict(sys.modules, stubs):
        spec = importlib.util.spec_from_file_location(
            "ofe_cred_stub.validate_credential", CLUSTER_MANAGER / "validate_credential.py"
        )
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)
    return module


validate_credential = _load_validate_credential()


class CheckStateBucketAccessTests(unittest.TestCase):
    """Each case is answered with a warning, or None when nothing is missing."""

    BUCKET = "dep1-tfstate-abcd1234"

    def _check(self, granted=None, error=None, server=None, expect_client=True):
        """The warning for one case. The check must build its storage client
        from the registered credential, or (expect_client=False) build none."""
        if server is None:
            server = {"terraform_state_bucket": self.BUCKET, "deployment_name": "dep1"}
        bucket = mock.MagicMock()
        if error is not None:
            bucket.test_iam_permissions.side_effect = error
        else:
            bucket.test_iam_permissions.return_value = granted
        client = mock.MagicMock()
        client.bucket.return_value = bucket
        # What python_credentials returns for the registered credential
        creds = object()
        with mock.patch.object(
            validate_credential.utils, "load_config", return_value={"server": server}
        ), mock.patch.object(
            validate_credential.storage, "Client", return_value=client
        ) as client_cls, mock.patch.object(
            validate_credential.credentials,
            "python_credentials",
            return_value=creds,
        ) as python_credentials, mock.patch.object(
            validate_credential.credentials, "identity_email", return_value=EMAIL
        ):
            warning = validate_credential.check_state_bucket_access(CREDENTIAL)
        if expect_client:
            python_credentials.assert_called_once_with(CREDENTIAL)
            client_cls.assert_called_once()
            # The client acts as the credential: built with exactly what
            # python_credentials returned, never the server's own identity
            self.assertIs(client_cls.call_args.kwargs.get("credentials"), creds)
        else:
            python_credentials.assert_not_called()
            client_cls.assert_not_called()
        return warning, client_cls, client, bucket

    def test_full_access_gives_no_warning(self):
        warning, *_ = self._check(granted=list(validate_credential.STATE_BUCKET_PERMISSIONS))
        self.assertIsNone(warning)

    def test_checks_with_the_registered_credential_not_the_server_account(self):
        _, client_cls, client, bucket = self._check(
            granted=list(validate_credential.STATE_BUCKET_PERMISSIONS)
        )
        client_cls.assert_called_once_with(
            project="other-project", credentials=mock.ANY
        )
        client.bucket.assert_called_once_with(self.BUCKET)
        asked = bucket.test_iam_permissions.call_args.args[0]
        self.assertEqual(sorted(asked), sorted(validate_credential.STATE_BUCKET_PERMISSIONS))
        # Only asks; never reads or changes the bucket's IAM policy.
        bucket.get_iam_policy.assert_not_called()
        bucket.set_iam_policy.assert_not_called()

    def test_missing_permissions_give_the_exact_line_and_command(self):
        warning, *_ = self._check(granted=["storage.objects.get", "storage.objects.list"])
        self.assertIn(f"Service account {EMAIL}", warning)
        self.assertIn(f"gs://{self.BUCKET}", warning)
        self.assertIn("storage.objects.create, storage.objects.delete", warning)
        self.assertIn(
            "state_bucket_writers in tf/state_bucket/deployments/dep1/admin.auto.tfvars",
            warning,
        )
        self.assertIn(f'"serviceAccount:{EMAIL}",', warning)
        self.assertIn("terraform -chdir=tf/state_bucket/deployments/dep1 apply", warning)
        self.assertIn("will fail", warning)

    def test_no_access_at_all(self):
        warning, *_ = self._check(granted=[])
        self.assertIn(f'"serviceAccount:{EMAIL}",', warning)

    def test_missing_bucket_is_reported_not_raised(self):
        warning, *_ = self._check(error=NotFound("404 bucket does not exist"))
        self.assertIn(f"gs://{self.BUCKET}", warning)
        self.assertIn("was not found", warning)
        self.assertIn(EMAIL, warning)

    def test_unreadable_bucket_is_reported_not_raised(self):
        warning, *_ = self._check(error=GoogleAPICallError("503 backend error"))
        self.assertIn("Could not check", warning)
        self.assertIn("503 backend error", warning)

    def test_no_configured_bucket_is_reported_without_calling_the_api(self):
        warning, client_cls, *_ = self._check(
            granted=[], server={}, expect_client=False
        )
        self.assertIn("terraform_state_bucket", warning)
        client_cls.assert_not_called()

    def test_auth_failure_is_reported_not_raised(self):
        warning, *_ = self._check(error=GoogleAuthError("invalid_grant: account not found"))
        self.assertIn("Could not check", warning)
        self.assertIn("invalid_grant", warning)

    def test_single_short_attempt_inside_the_request(self):
        _, _, _, bucket = self._check(granted=list(validate_credential.STATE_BUCKET_PERMISSIONS))
        kwargs = bucket.test_iam_permissions.call_args.kwargs
        self.assertIsNone(kwargs["retry"])
        self.assertLessEqual(kwargs["timeout"], 30)

    def test_unknown_deployment_name_gives_a_placeholder_path(self):
        warning, *_ = self._check(granted=[], server={"terraform_state_bucket": self.BUCKET})
        self.assertIn("tf/state_bucket/deployments/<deployment_name>/admin.auto.tfvars", warning)
        self.assertNotIn("None", warning)
