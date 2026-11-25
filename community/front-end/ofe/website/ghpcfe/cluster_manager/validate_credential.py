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

"""Cloud credential validation routines"""

import logging

from google.api_core import exceptions as google_exceptions
from google.auth import exceptions as auth_exceptions
from google.cloud import storage

from . import credentials
from . import utils

logger = logging.getLogger(__name__)

# What Terraform's GCS backend needs to read, write and lock a resource's
# state in the state bucket.
STATE_BUCKET_PERMISSIONS = (
    "storage.objects.create",
    "storage.objects.delete",
    "storage.objects.get",
    "storage.objects.list",
)


def validate_credential(cloud_provider, credential_detail):
    """communicate with a cloud provider to validate a credential"""

    validated = False
    if cloud_provider == "GCP":
        validated = credentials.validate_key_json(credential_detail)

    return validated


def validate_impersonation(cloud_provider, email, project_id):
    """Check that the server can impersonate email and use project_id.

    Returns None when it can, or the reason it cannot.
    """
    if cloud_provider != "GCP":
        return f"Unsupported cloud provider {cloud_provider}"
    return credentials.validate_impersonation(email, project_id)


def check_state_bucket_access(credential):
    """Return a warning if a GCP credential cannot use the state bucket.

    credential is a Credential, with a key or impersonated. Asks the bucket
    (testIamPermissions), acting as the credential, which of
    STATE_BUCKET_PERMISSIONS its service account holds: this writes
    nothing, needs no permission and includes project-level roles. The
    front end never grants access itself; the warning tells an
    administrator how. Returns None if every permission is held.
    """
    email = credentials.identity_email(credential)
    server = utils.load_config()["server"]
    bucket = server.get("terraform_state_bucket")
    if not bucket:
        return (
            f"Could not check whether service account {email} can use the "
            "Terraform state bucket: the server configuration names no "
            "terraform_state_bucket. Provisioning will fail until it does."
        )

    client = storage.Client(
        project=credential.project_id or None,
        credentials=credentials.python_credentials(credential),
    )
    # One try with a short timeout: this runs inside the registration request.
    try:
        granted = client.bucket(bucket).test_iam_permissions(
            list(STATE_BUCKET_PERMISSIONS), timeout=30, retry=None
        )
    except google_exceptions.NotFound:
        return (
            f"Could not check whether service account {email} can use the "
            f"Terraform state bucket gs://{bucket}: the bucket was not found. "
            "Provisioning will fail until it exists and this account can use it."
        )
    except (google_exceptions.GoogleAPICallError,
            auth_exceptions.GoogleAuthError) as err:
        logger.warning("State bucket access check for %s failed: %s", email, err)
        return (
            f"Could not check whether service account {email} can use the "
            f"Terraform state bucket gs://{bucket}: {err}"
        )

    missing = sorted(set(STATE_BUCKET_PERMISSIONS) - set(granted))
    if not missing:
        return None
    deployment = server.get("deployment_name") or "<deployment_name>"
    sb_dir = f"tf/state_bucket/deployments/{deployment}"
    return (
        f"Service account {email} cannot read and write the Terraform state "
        f"bucket gs://{bucket} (missing {', '.join(missing)}), so provisioning "
        "with this credential will fail until it can. On the machine TKFE was "
        "deployed from, add this line to state_bucket_writers in "
        f"{sb_dir}/admin.auto.tfvars: "
        f'"serviceAccount:{email}", '
        f"then run: terraform -chdir={sb_dir} apply"
    )
