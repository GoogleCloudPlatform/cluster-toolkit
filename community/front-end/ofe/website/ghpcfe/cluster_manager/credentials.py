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

"""The one place where OFE reads a cloud credential.

A Credential is one of two kinds:

- "key": an uploaded service account key. Its JSON is stored encrypted
  (Fernet) with a key held in a Secret Manager secret, which the instance
  metadata names. Terraform gets the JSON in GOOGLE_CREDENTIALS, from memory.
  Tools that need a file (gcloud, Packer and the gcluster validators) get a
  mode-0600 file in a new mode-0700 directory, removed when the run ends,
  also after an error. The directory is on tmpfs (/dev/shm), so the key
  stays off persistent disk; only where /dev/shm is missing does it go to the
  system temp directory, which may be on disk, and a warning says so.
- "impersonate": no key. OFE acts as the named service account by
  impersonating it with the web server's own identity: Terraform through
  GOOGLE_IMPERSONATE_SERVICE_ACCOUNT, gcloud through
  CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT and Python clients through
  google.auth.impersonated_credentials. Packer has no such setting, so image
  builds need a key credential.

Every run gets its own environment dictionary; the server's own process
environment is never changed.

This module also checks, before any provisioning, that the credential's
identity has the permissions the operation needs (projects.testIamPermissions
and the resource-level equivalents), and turns a Terraform or Packer 403 into
the same "Identity X lacks permission Y needed to Z" message.
"""

import contextlib
import hashlib
import json
import logging
import os
import re
import shlex
import shutil
import subprocess
import tempfile
import threading
import warnings
from dataclasses import dataclass
from pathlib import Path

import requests
from cryptography.fernet import Fernet, InvalidToken, MultiFernet
from django.conf import settings
from django.core.exceptions import ValidationError
from django.core.validators import validate_email

logger = logging.getLogger(__name__)

KIND_KEY = "key"
KIND_IMPERSONATE = "impersonate"
KIND_CHOICES = (
    (KIND_KEY, "Uploaded service account key"),
    (KIND_IMPERSONATE, "Impersonated service account (no key)"),
)

CLOUD_PLATFORM_SCOPE = "https://www.googleapis.com/auth/cloud-platform"

# Stored key JSON starts with this; anything else is a legacy plaintext row.
ENCRYPTED_PREFIX = "fernet:"

METADATA_URL = "http://metadata.google.internal/computeMetadata/v1"
# The bound on one metadata request (metadata_value). Not a fix for a seen
# delay: the metadata server runs on the VM's host and answers locally, so a
# healthy request takes milliseconds. The bound exists so a metadata server
# that hangs cannot block the server's start (ofe_bootstrap_credentials) or
# a request that first needs the database key. A request that times out or
# fails raises CredentialError, which the bootstrap reports as its error.
# Permanent; revisit only if a metadata request is measured to need longer.
METADATA_TIMEOUT_SECONDS = 5
PROVISIONING_ACCOUNT_ATTRIBUTE = "ofe-provisioning-service-account"
DB_KEY_SECRET_ATTRIBUTE = "ofe-db-key-secret"

PROVISIONING_CREDENTIAL_NAME = "OFE provisioning account"

# The tmpfs where key files live while a tool runs
KEY_FILE_PARENT = Path("/dev/shm")

# Variables that would give a tool an identity other than the credential's.
# Every run starts without them.
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

# Permissions each operation needs in the credential's project, checked
# before gcluster or Terraform runs. Each list holds only permissions the
# operation's own resources use, so a check never refuses a working setup.
# A profiles cluster or image also creates its service accounts, custom roles
# and project bindings in the blueprint.
_PROFILE_ACCOUNT_CREATE = (
    "iam.serviceAccounts.create",
    "iam.roles.create",
    "resourcemanager.projects.setIamPolicy",
)
_PROFILE_ACCOUNT_DESTROY = (
    "iam.serviceAccounts.delete",
    "iam.roles.delete",
    "resourcemanager.projects.setIamPolicy",
)
PERMISSIONS = {
    "cluster_create": (
        "compute.instances.create",
        "compute.instanceTemplates.create",
        "compute.disks.create",
        "iam.serviceAccounts.actAs",
        "storage.buckets.create",
    ),
    "cluster_destroy": (
        "compute.instances.delete",
        "compute.instanceTemplates.delete",
    ),
    "image_build": (
        "compute.networks.create",
        "compute.subnetworks.create",
        "compute.instances.create",
        "compute.images.create",
        "iam.serviceAccounts.actAs",
        "storage.buckets.create",
    ),
    "image_destroy": (
        "compute.networks.delete",
        "compute.images.delete",
    ),
    "filesystem_create": ("file.instances.create",),
    "filesystem_destroy": ("file.instances.delete",),
    "vpc_create": (
        "compute.networks.create",
        "compute.subnetworks.create",
        "compute.routers.create",
        "compute.firewalls.create",
    ),
    "vpc_import": ("compute.networks.get", "compute.subnetworks.create"),
    "vpc_destroy": ("compute.networks.delete",),
    "vpc_import_destroy": ("compute.subnetworks.delete",),
    "workbench_create": (
        "notebooks.instances.create",
        "iam.serviceAccounts.create",
        "iam.serviceAccounts.actAs",
        "resourcemanager.projects.setIamPolicy",
    ),
    "workbench_destroy": (
        "notebooks.instances.delete",
        "iam.serviceAccounts.delete",
    ),
}

# What the impersonation check on the credential page asks for: the account
# can be impersonated and can see its project.
VALIDATION_PERMISSIONS = ("resourcemanager.projects.get",)

# A cross-project profiles cluster binds its accounts on these server
# resources in its blueprint, so its provisioning identity needs to read and
# set their IAM policies.
BUCKET_IAM_PERMISSIONS = (
    "storage.buckets.getIamPolicy",
    "storage.buckets.setIamPolicy",
)
TOPIC_IAM_PERMISSIONS = (
    "pubsub.topics.getIamPolicy",
    "pubsub.topics.setIamPolicy",
)
SUBSCRIPTION_IAM_PERMISSIONS = (
    "pubsub.subscriptions.getIamPolicy",
    "pubsub.subscriptions.setIamPolicy",
)


def permissions_for(operation_key, profiles=False):
    """The project permissions an operation needs (see PERMISSIONS)"""
    permissions = PERMISSIONS[operation_key]
    if profiles and operation_key in ("cluster_create", "image_build"):
        permissions = permissions + _PROFILE_ACCOUNT_CREATE
    if profiles and operation_key in ("cluster_destroy", "image_destroy"):
        permissions = permissions + _PROFILE_ACCOUNT_DESTROY
    return permissions


class CredentialError(Exception):
    """A credential cannot be used for what was asked"""


class PermissionGapError(CredentialError):
    """The credential's identity lacks permissions an operation needs"""

    def __init__(
        self, identity, permissions, operation, resource=None, project=None,
        detail=None,
    ):
        self.identity = identity
        self.permissions = tuple(permissions)
        self.operation = operation
        self.resource = resource
        self.project = project
        self.detail = detail
        super().__init__(self._message())

    def _message(self):
        if self.permissions:
            noun = "permission" if len(self.permissions) == 1 else "permissions"
            what = f"{noun} {', '.join(self.permissions)}"
        else:
            what = "a permission"
        message = f"Identity {self.identity} lacks {what} needed to {self.operation}"
        if self.resource:
            message += f" on resource {self.resource}"
        elif self.project:
            message += f" in project {self.project}"
        message += "."
        if self.detail:
            message += f" Google Cloud said: {self.detail}"
        return message


class PermissionDeniedRunError(subprocess.CalledProcessError):
    """A tool run that Google Cloud refused with a 403.

    It is still a CalledProcessError, so callers that clean up after a failed
    run keep doing so, and its text is the PermissionGapError message.
    """

    def __init__(self, cpe, gap):
        super().__init__(cpe.returncode, cpe.cmd, cpe.output, cpe.stderr)
        self.gap = gap

    def __str__(self):
        return str(self.gap)


class ImageBuildNeedsKeyError(CredentialError):
    """Image builds need an uploaded key"""


IMAGE_BUILD_NEEDS_KEY_MESSAGE = (
    "Credential {name} impersonates {email} and has no key, but image builds "
    "need an uploaded service account key: Packer, which builds the image, "
    "cannot impersonate a service account yet. Choose a credential of kind "
    "'Uploaded service account key' for this image."
)


def image_build_refusal(credential):
    """The message refusing an image build with credential, or None"""
    if credential.kind == KIND_IMPERSONATE:
        return IMAGE_BUILD_NEEDS_KEY_MESSAGE.format(
            name=credential.name, email=impersonation_target(credential)
        )
    return None


def check_image_build_credential(credential):
    """Raise ImageBuildNeedsKeyError for a credential Packer cannot use"""
    refusal = image_build_refusal(credential)
    if refusal:
        raise ImageBuildNeedsKeyError(refusal)


# ---------------------------------------------------------------------------
# Storage: what Credential.detail holds
# ---------------------------------------------------------------------------

_fernet_lock = threading.Lock()
_cached_fernet = None


def _fernet():
    """The Fernet used for stored keys.

    A deployed server reads the keys from the enabled versions of the Secret
    Manager secret its instance metadata names, once per process. The
    OFE_CREDENTIAL_ENCRYPTION_KEY setting, for tests and local development
    only, replaces that lookup.

    Raises CredentialError when the keys cannot be read or one is not a
    Fernet key; nothing is cached then, so the next use tries again.
    """
    configured = getattr(settings, "OFE_CREDENTIAL_ENCRYPTION_KEY", None)
    if configured:
        return _multi_fernet(
            configured, "the OFE_CREDENTIAL_ENCRYPTION_KEY setting"
        )
    global _cached_fernet  # pylint: disable=global-statement
    with _fernet_lock:
        if _cached_fernet is None:
            name = _db_key_secret()
            _cached_fernet = _multi_fernet(
                _read_db_keys(name), f"secret {name}"
            )
        return _cached_fernet


@contextlib.contextmanager
def _malformed_key_errors(source):
    """Raise the TypeError or ValueError of a database key from source that
    is missing or not a Fernet key as a CredentialError that names source
    but never the key, with the error as its cause"""
    try:
        yield
    except (TypeError, ValueError) as err:
        # Only the error's type is shown, as for any error raised while a
        # key is parsed
        raise CredentialError(
            f"The database key in {source} is missing or is not a Fernet key "
            f"({type(err).__name__}), so stored keys cannot be encrypted or "
            "read"
        ) from err


class _DatabaseKeys(MultiFernet):
    """Encrypts with the newest database key and decrypts with any of them;
    newest is that key's own Fernet, which tells a stored key still sealed
    with an older one"""

    def __init__(self, fernets):
        super().__init__(fernets)
        self.newest = fernets[0]


def _multi_fernet(keys, source="the server's configuration"):
    """Encrypts with the first of keys and decrypts with any of them.

    Raises CredentialError when keys is empty or one of them is not a
    Fernet key.
    """
    if isinstance(keys, (bytes, str)):
        keys = [keys]
    with _malformed_key_errors(source):
        return _DatabaseKeys([Fernet(key) for key in keys])


def use_db_key(keys, source="the server's configuration"):
    """Use keys (one key, or a list newest first, read from source) for this
    process's stored credentials (set at start-up)"""
    global _cached_fernet  # pylint: disable=global-statement
    with _fernet_lock:
        _cached_fernet = _multi_fernet(keys, source)


def forget_db_key():
    """Drop the cached key, so the next use reads it again"""
    global _cached_fernet  # pylint: disable=global-statement
    with _fernet_lock:
        _cached_fernet = None


def is_sealed(detail):
    """True when detail holds an encrypted key"""
    return bool(detail) and detail.startswith(ENCRYPTED_PREFIX)


def _encrypt(text):
    token = _fernet().encrypt(text.encode("utf-8")).decode("ascii")
    return ENCRYPTED_PREFIX + token


_UNDECRYPTABLE = "A stored key cannot be decrypted with the server's database key"


def _decrypt(detail):
    try:
        return (
            _fernet()
            .decrypt(detail[len(ENCRYPTED_PREFIX):].encode("ascii"))
            .decode("utf-8")
        )
    except InvalidToken as err:
        raise CredentialError(_UNDECRYPTABLE) from err


def _resealed(detail):
    """The sealed detail encrypted with the newest database key, or None
    when it already is.

    Raises CredentialError when no enabled database key decrypts it, and
    ValueError or TypeError for text that is not a token TKFE wrote.
    """
    keys = _fernet()
    token = detail[len(ENCRYPTED_PREFIX):].encode("ascii")
    try:
        keys.newest.decrypt(token)
        return None
    except InvalidToken:
        pass
    try:
        # Decrypts with whichever key sealed it and encrypts with the newest
        return ENCRYPTED_PREFIX + keys.rotate(token).decode("ascii")
    except InvalidToken as err:
        raise CredentialError(_UNDECRYPTABLE) from err


def _parse_key_text(text):
    """The key JSON as a dict, or None when text is not a JSON object"""
    try:
        info = json.loads(text)
    except (TypeError, ValueError):
        return None
    return info if isinstance(info, dict) else None


def _key_project(info):
    """The project_id of parsed key JSON, or "" when it is not a non-empty
    string"""
    project_id = (info or {}).get("project_id")
    return project_id if isinstance(project_id, str) and project_id else ""


def _is_plain_email(text):
    """True when text is an email address by the validation of the
    credential form's service-account field (Django's validate_email), with
    no quoted local part.

    Key JSON always holds double quotes, which a plain address never does,
    so it is never taken for an address.
    """
    if '"' in text:
        return False
    try:
        validate_email(text)
    except ValidationError:
        return False
    return True


def seal(credential):
    """Prepare a Credential for saving (Credential.save calls this).

    A key credential's JSON is encrypted, and its project_id is taken from the
    JSON. Text that is not JSON is still encrypted, because it may hold part of
    a key; using it later fails with a clear message. An impersonate
    credential stores only its service account email, which is not secret
    and so is not encrypted.

    Raises CredentialError, and nothing is changed, when an impersonate
    credential's detail is not a plain email address (see
    _is_plain_email): such text, key JSON above all, would otherwise be
    stored unencrypted.
    """
    if credential.kind == KIND_IMPERSONATE:
        email = (credential.detail or "").strip()
        if not _is_plain_email(email):
            # The detail may be a key, so it is never quoted
            raise CredentialError(
                f"Credential {credential.name} impersonates a service "
                "account, so it can hold only that account's email address; "
                "what was given is not one. Nothing was saved. To use a "
                "key, choose an uploaded key instead."
            )
        credential.detail = email
        return
    if credential.kind != KIND_KEY:
        raise CredentialError(f"Unknown credential kind {credential.kind!r}")
    detail = credential.detail or ""
    if is_sealed(detail):
        if not credential.project_id:
            credential.project_id = _key_project(
                _parse_key_text(_decrypt(detail))
            )
        return
    project_id = _key_project(_parse_key_text(detail))
    if project_id:
        credential.project_id = project_id
    credential.detail = _encrypt(detail)


def key_json(credential):
    """The key JSON text of a key credential"""
    if credential.kind != KIND_KEY:
        raise CredentialError(
            f"Credential {credential.name} has no key; it impersonates "
            f"{impersonation_target(credential)}"
        )
    detail = credential.detail or ""
    if is_sealed(detail):
        return _decrypt(detail)
    # A row saved before encryption, until the start-up fix encrypts it, or
    # an unsaved form
    return detail


def key_info(credential):
    """The key JSON of a key credential, as a dict"""
    info = _parse_key_text(key_json(credential))
    if info is None:
        raise CredentialError(
            f"Credential {credential.name} does not hold a service account key"
        )
    return info


def impersonation_target(credential):
    """The service account an impersonate credential acts as"""
    if credential.kind != KIND_IMPERSONATE:
        raise CredentialError(f"Credential {credential.name} holds a key")
    return (credential.detail or "").strip()


def identity_email(credential):
    """The service account email a credential acts as"""
    if credential.kind == KIND_IMPERSONATE:
        return impersonation_target(credential)
    return key_info(credential).get("client_email", "") or ""


def shell_identity(credential):
    """The variable setting, for a message, that makes a command typed by
    hand (such as terraform force-unlock) act as credential.

    The server keeps no key file: an impersonate credential is named by its
    service account, and for a key credential the reader supplies a key file
    of its account. credential may be None.
    """
    email = ""
    if credential is not None:
        try:
            if credential.kind == KIND_IMPERSONATE:
                return "GOOGLE_IMPERSONATE_SERVICE_ACCOUNT=" + shlex.quote(
                    impersonation_target(credential)
                )
            email = identity_email(credential)
        except CredentialError:
            email = ""
    placeholder = (
        f"<a key file of {email}>" if email
        else "<a key file of the resource's credential>"
    )
    return "GOOGLE_APPLICATION_CREDENTIALS=" + shlex.quote(placeholder)


def grafana_key_settings(credential):
    """(jsonData, secureJsonData) of a Grafana Cloud Monitoring datasource
    that signs in with a key credential's key"""
    info = key_info(credential)
    json_data = {
        "authenticationType": "jwt",
        "tokenUri": info["token_uri"],
        "clientEmail": info["client_email"],
        "defaultProject": info["project_id"],
    }
    return json_data, {"privateKey": info["private_key"]}


def cache_tag(credential):
    """A value that changes whenever the credential's content changes, for
    caches of what a credential can see"""
    digest = hashlib.sha256((credential.detail or "").encode("utf-8"))
    return (
        credential.pk,
        credential.kind,
        credential.project_id,
        digest.hexdigest()[:16],
    )


def key_project_id(text):
    """The project_id of key JSON text, or "" when it names none (or names
    something other than a string)"""
    return _key_project(_parse_key_text(text))


def validate_key_json(text):
    """True when text is a usable service account key"""
    info = _parse_key_text(text)
    if info is None:
        logger.info("Credential is not a JSON object")
        return False
    # A corrupted private key raises a UserWarning; other warnings are harmless
    with warnings.catch_warnings():
        warnings.simplefilter("ignore", ResourceWarning)
        warnings.simplefilter("error", UserWarning)
        try:
            _service_account_credentials(info)
        except Exception as err:  # pylint: disable=broad-except
            logger.info("Credential validation failed: %s", err)
            return False
    return True


# ---------------------------------------------------------------------------
# Runtime: what a tool or client needs to act as a credential
# ---------------------------------------------------------------------------


def _service_account_credentials(info, scopes=None):
    from google.oauth2 import service_account  # pylint: disable=import-outside-toplevel

    if scopes:
        return service_account.Credentials.from_service_account_info(
            info, scopes=list(scopes)
        )
    return service_account.Credentials.from_service_account_info(info)


def _impersonated_credentials(email, scopes, source_credentials=None):
    import google.auth  # pylint: disable=import-outside-toplevel
    from google.auth import impersonated_credentials  # pylint: disable=import-outside-toplevel

    if source_credentials is None:
        source_credentials, _ = google.auth.default(
            scopes=[CLOUD_PLATFORM_SCOPE]
        )
    return impersonated_credentials.Credentials(
        source_credentials=source_credentials,
        target_principal=email,
        target_scopes=list(scopes),
        lifetime=3600,
    )


def python_credentials(
    credential, scopes=(CLOUD_PLATFORM_SCOPE,), source_credentials=None
):
    """google.auth credentials that act as credential, for Python clients.

    An impersonate credential uses the server's own identity
    (Application Default Credentials, or source_credentials) to impersonate
    its service account.
    """
    if credential.kind == KIND_IMPERSONATE:
        return _impersonated_credentials(
            impersonation_target(credential), scopes, source_credentials
        )
    return _service_account_credentials(key_info(credential), scopes)


def _private_dir(tmpfs=KEY_FILE_PARENT):
    """A new mode-0700 directory on tmpfs (or the temp dir without one)"""
    parent = None
    if tmpfs.is_dir() and os.access(tmpfs, os.W_OK | os.X_OK):
        parent = tmpfs
    else:
        logger.warning(
            "%s is not available, so a key file for this run is written to "
            "%s, which may be on persistent disk (mode-0700 directory, "
            "mode-0600 file, removed after the run)",
            tmpfs,
            tempfile.gettempdir(),
        )
    path = Path(tempfile.mkdtemp(prefix="ofe-credential-", dir=parent))
    os.chmod(path, 0o700)
    return path


def _write_private_file(path, text):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as fp:
        fp.write(text)


def _remove_private_dir(path):
    try:
        shutil.rmtree(path)
    except FileNotFoundError:
        pass
    except OSError as err:
        logger.error("Could not remove credential directory %s: %s", path, err)


@contextlib.contextmanager
def tool_environment(credential, key_file=False, base_env=None):
    """Yield the environment for one tool run that acts as credential.

    The result is a new dict: base_env (default: the server's environment)
    without any ambient credential, plus

    - an empty gcloud configuration directory of the run's own, and the
      credential's project as gcloud's project;
    - impersonate: GOOGLE_IMPERSONATE_SERVICE_ACCOUNT (Terraform) and
      CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT (gcloud);
    - key: GOOGLE_CREDENTIALS, the key JSON for Terraform. With key_file, for
      tools that only read a file (gcloud, Packer and the gcluster
      validators), also a mode-0600 copy in a mode-0700 directory on tmpfs
      (the system temp directory where there is no /dev/shm), named by
      GOOGLE_APPLICATION_CREDENTIALS and
      CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE.

    The directory, with any key file, is removed when the block ends, also on
    an exception.
    """
    env = dict(os.environ if base_env is None else base_env)
    for variable in AMBIENT_CREDENTIAL_VARIABLES:
        env.pop(variable, None)
    if credential.kind not in (KIND_KEY, KIND_IMPERSONATE):
        raise CredentialError(f"Unknown credential kind {credential.kind!r}")
    # Read the credential before making anything that needs cleaning up
    if credential.kind == KIND_IMPERSONATE:
        email = impersonation_target(credential)
        if not email:
            raise CredentialError(
                f"Credential {credential.name} names no service account"
            )
        key_text = None
    else:
        key_text = key_json(credential)
        if _parse_key_text(key_text) is None:
            raise CredentialError(
                f"Credential {credential.name} does not hold a service "
                "account key"
            )

    workdir = _private_dir()
    try:
        gcloud_config = workdir / "gcloud"
        gcloud_config.mkdir(mode=0o700)
        env["CLOUDSDK_CONFIG"] = str(gcloud_config)
        if credential.project_id:
            env["CLOUDSDK_CORE_PROJECT"] = credential.project_id
        if key_text is None:
            env["GOOGLE_IMPERSONATE_SERVICE_ACCOUNT"] = email
            env["CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT"] = email
        else:
            env["GOOGLE_CREDENTIALS"] = key_text
            if key_file:
                path = workdir / "key.json"
                _write_private_file(path, key_text)
                env["GOOGLE_APPLICATION_CREDENTIALS"] = str(path)
                env["CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE"] = str(path)
        yield env
    finally:
        _remove_private_dir(workdir)


# ---------------------------------------------------------------------------
# Preflight: does the identity have what the operation needs?
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class ResourceCheck:
    """Permissions to test on one resource.

    kind is "project", "bucket", "topic" or "subscription"; name is the
    project id, bucket name, or full topic or subscription path.
    fallback_project, for a subscription OFE has not created yet, is the
    project whose policy a new subscription inherits.
    """

    kind: str
    name: str
    permissions: tuple
    fallback_project: str = ""

    @property
    def label(self):
        if self.kind == "bucket":
            return f"gs://{self.name}"
        return self.name


def _test_project(creds, check):
    import googleapiclient.discovery  # pylint: disable=import-outside-toplevel

    service = googleapiclient.discovery.build(
        "cloudresourcemanager", "v1", credentials=creds, cache_discovery=False
    )
    response = (
        service.projects()
        .testIamPermissions(
            resource=check.name, body={"permissions": list(check.permissions)}
        )
        .execute()
    )
    return set(response.get("permissions", []))


def _test_bucket(creds, check):
    from google.cloud import storage  # pylint: disable=import-outside-toplevel

    # project=None: a bucket's permissions need no project
    client = storage.Client(project=None, credentials=creds)
    return set(client.bucket(check.name).test_iam_permissions(list(check.permissions)))


def _test_topic(creds, check):
    from google.cloud import pubsub_v1  # pylint: disable=import-outside-toplevel

    client = pubsub_v1.PublisherClient(credentials=creds)
    response = client.test_iam_permissions(
        request={"resource": check.name, "permissions": list(check.permissions)}
    )
    return set(response.permissions)


def _test_subscription(creds, check):
    from google.api_core.exceptions import NotFound  # pylint: disable=import-outside-toplevel
    from google.cloud import pubsub_v1  # pylint: disable=import-outside-toplevel

    client = pubsub_v1.SubscriberClient(credentials=creds)
    try:
        response = client.test_iam_permissions(
            request={
                "resource": check.name,
                "permissions": list(check.permissions),
            }
        )
        return set(response.permissions)
    except NotFound:
        if not check.fallback_project:
            raise
    # Not created yet: a new subscription has no policy of its own, so the
    # project's policy decides
    return _test_project(
        creds,
        ResourceCheck("project", check.fallback_project, check.permissions),
    )


DEFAULT_TESTERS = {
    "project": _test_project,
    "bucket": _test_bucket,
    "topic": _test_topic,
    "subscription": _test_subscription,
}


def preflight(
    credential, operation, permissions, resources=(), testers=None,
    creds=None,
):
    """Stop before provisioning when the identity lacks a permission.

    Tests permissions in the credential's project, then each ResourceCheck,
    as the credential's identity. Raises PermissionGapError naming the
    identity, the missing permissions, the operation and, for a resource, the
    resource; or CredentialError when the check itself cannot run (for
    example, the server cannot impersonate the account).
    """
    testers = {**DEFAULT_TESTERS, **(testers or {})}
    identity = identity_email(credential)
    if not credential.project_id:
        raise CredentialError(
            f"Credential {credential.name} has no project id, so the "
            f"permissions needed to {operation} cannot be checked"
        )
    if creds is None:
        creds = python_credentials(credential)
    checks = [ResourceCheck("project", credential.project_id, tuple(permissions))]
    checks.extend(resources)
    for check in checks:
        if not check.permissions:
            continue
        try:
            granted = testers[check.kind](creds, check)
        except Exception as err:  # pylint: disable=broad-except
            raise CredentialError(
                f"Could not check the permissions of identity {identity} "
                f"needed to {operation} on {check.label}: {err}"
            ) from err
        missing = [p for p in check.permissions if p not in granted]
        if missing:
            if check.kind == "project":
                raise PermissionGapError(
                    identity, missing, operation, project=check.name
                )
            raise PermissionGapError(
                identity, missing, operation, resource=check.label
            )
    logger.info("Identity %s has the permissions needed to %s", identity, operation)


def validate_impersonation(email, project_id, testers=None, source_credentials=None):
    """Check that the server can impersonate email and use project_id.

    Returns None when it can, or a message saying why not.
    """
    from ..models import Credential  # pylint: disable=import-outside-toplevel

    # Unsaved: only what the checks read
    target = Credential(
        name=email, kind=KIND_IMPERSONATE, detail=email, project_id=project_id
    )
    try:
        creds = python_credentials(target, source_credentials=source_credentials)
        preflight(
            target,
            f"use project {project_id}",
            VALIDATION_PERMISSIONS,
            testers=testers,
            creds=creds,
        )
    except CredentialError as err:
        return str(err)
    except Exception as err:  # pylint: disable=broad-except
        return f"The server cannot impersonate {email}: {err}"
    return None


_DENIED = re.compile(r"\bError 403\b|\b403 Forbidden\b|PERMISSION_DENIED|\bcode = 403\b")
_PERMISSION_TOKEN = r"([a-z][a-zA-Z0-9]*(?:\.[a-zA-Z0-9]+){2,})"
_PERMISSION_PATTERNS = (
    re.compile(r"(?i:permissions?) ['\"]?" + _PERMISSION_TOKEN + r"['\"]? (?:denied|is required)"),
    re.compile(r"(?i:required) ['\"]?" + _PERMISSION_TOKEN + r"['\"]? permission"),
    re.compile(r"(?i:permission) ['\"]" + _PERMISSION_TOKEN + r"['\"]"),
)
_RESOURCE_PATTERN = re.compile(r"(?:on resource|for) ['\"]([^'\"]+)['\"]")


def permission_error_from_output(text, credential, operation):
    """A PermissionGapError for tool output that shows a 403, else None"""
    if not text or not _DENIED.search(text):
        return None
    permissions = []
    for pattern in _PERMISSION_PATTERNS:
        for match in pattern.finditer(text):
            if match.group(1) not in permissions:
                permissions.append(match.group(1))
    resource_match = _RESOURCE_PATTERN.search(text)
    resource = resource_match.group(1) if resource_match else None
    detail = None
    if not permissions:
        line = next(
            (ln.strip() for ln in text.splitlines() if _DENIED.search(ln)), ""
        )
        detail = line[:300]
    try:
        identity = identity_email(credential)
    except CredentialError:
        identity = credential.name
    return PermissionGapError(
        identity,
        permissions,
        operation,
        resource=resource,
        project=credential.project_id,
        detail=detail,
    )


def explain_failed_run(cpe, log_files, credential, operation):
    """The error to raise for a failed tool run.

    When its logs show a 403, a PermissionDeniedRunError with the
    "Identity X lacks permission Y needed to Z" message, which is also added
    to the first log file so the log the page shows ends with it; otherwise
    cpe unchanged.
    """
    texts = []
    for log_file in log_files:
        try:
            texts.append(Path(log_file).read_text(encoding="utf-8", errors="replace"))
        except OSError:
            continue
    gap = permission_error_from_output("\n".join(texts), credential, operation)
    if gap is None:
        return cpe
    logger.error("%s", gap)
    if log_files:
        try:
            with open(log_files[0], "a", encoding="utf-8") as fp:
                fp.write(f"\nOFE: {gap}\n")
        except OSError as err:
            logger.warning("Could not add the permission message to %s: %s", log_files[0], err)
    return PermissionDeniedRunError(cpe, gap)


# ---------------------------------------------------------------------------
# Refusals in the log a resource's page shows
# ---------------------------------------------------------------------------

# The Terraform log page of a cluster or filesystem shows, for its Terraform
# directory, the log of the first of these runs that has one
_TERRAFORM_LOG_PHASES = ("destroy", "apply", "plan", "init")

# Key text that could reach a refusal through an error Google returned: a PEM
# block (to its end, or to the end of a cut-off text) and a key file's
# "private_key" field
_PRIVATE_KEY_BLOCK = re.compile(
    r"-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|\Z)",
    re.DOTALL,
)
_PRIVATE_KEY_FIELD = re.compile(r"""(["']private_key["']\s*:\s*)["'][^"']*["']""")


def _without_private_keys(text):
    text = _PRIVATE_KEY_BLOCK.sub("[private key removed]", text)
    return _PRIVATE_KEY_FIELD.sub(r'\1"[removed]"', text)


def _run_log(directory, run):
    """A run's stderr log when it has text, else its stdout log, else None"""
    stderr = directory / f"{run}_log.stderr"
    if stderr.is_file() and stderr.stat().st_size:
        return stderr
    stdout = directory / f"{run}_log.stdout"
    if stdout.exists():
        return stdout
    return None


def shown_log_file(terraform_dir):
    """The log file the Terraform log page shows for terraform_dir, or None.

    The newest Terraform run's log; when Terraform has not run there, the log
    of gcluster, which writes a deployment group's Terraform directory to
    <working directory>/<deployment>/<group> and its own log to the working
    directory.
    """
    terraform_dir = Path(terraform_dir)
    for phase in _TERRAFORM_LOG_PHASES:
        found = _run_log(terraform_dir, f"terraform_{phase}")
        if found:
            return found
    return _run_log(terraform_dir.parent.parent, "ghpc_create")


def record_refusal(terraform_dir, err, phase):
    """Add "OFE: <err>" to the end of the log the resource's Terraform log
    page shows, so a refused operation says why on the page as well as in
    the Django log.

    err is the refusal (a PermissionGapError or CredentialError), whose text
    names the identity, permission, operation and resource; any private key
    text in it is removed. Earlier content is kept. When no log exists yet,
    the line starts the Terraform log of phase ("apply" or "destroy") in an
    existing Terraform directory, else gcluster's log in its working
    directory, which is created if needed. The Terraform directory itself is
    never created here: gcluster refuses to write to a deployment directory
    it did not create. Returns the file written, or None.
    """
    terraform_dir = Path(terraform_dir)
    target = shown_log_file(terraform_dir)
    if target is None:
        if terraform_dir.is_dir():
            target = terraform_dir / f"terraform_{phase}_log.stderr"
        else:
            target = terraform_dir.parent.parent / "ghpc_create_log.stderr"
    try:
        target.parent.mkdir(parents=True, exist_ok=True)
        with open(target, "a", encoding="utf-8") as fp:
            fp.write(f"\nOFE: {_without_private_keys(str(err))}\n")
    except OSError as log_err:
        logger.warning("Could not add the refusal to %s: %s", target, log_err)
        return None
    return target


# ---------------------------------------------------------------------------
# Start-up: database key, existing rows and the provisioning credential
# ---------------------------------------------------------------------------


def metadata_value(path, http_get=None):
    """A value from the metadata server, or "" when it is not set"""
    http_get = http_get or requests.get
    try:
        response = http_get(
            f"{METADATA_URL}/{path}",
            headers={"Metadata-Flavor": "Google"},
            timeout=METADATA_TIMEOUT_SECONDS,
        )
    except requests.RequestException as err:
        raise CredentialError(
            f"Cannot read {path} from the metadata server: {err}"
        ) from err
    if response.status_code == 404:
        return ""
    if response.status_code != 200:
        raise CredentialError(
            f"The metadata server answered {response.status_code} for {path}"
        )
    return response.text.strip()


def _secret_name(project_id, secret):
    if secret.startswith("projects/"):
        return secret
    return f"projects/{project_id}/secrets/{secret}"


def _secret_client():
    from google.cloud import secretmanager  # pylint: disable=import-outside-toplevel

    return secretmanager.SecretManagerServiceClient()


def _db_key_secret(http_get=None):
    secret = metadata_value(
        f"instance/attributes/{DB_KEY_SECRET_ATTRIBUTE}", http_get
    )
    if not secret:
        raise CredentialError(
            f"Instance metadata has no {DB_KEY_SECRET_ATTRIBUTE}, so stored "
            "keys cannot be encrypted or read"
        )
    project_id = metadata_value("project/project-id", http_get)
    return _secret_name(project_id, secret)


def _version_number(name):
    """The number that ends a secret version's resource name, or None"""
    match = re.fullmatch(r".*/versions/([0-9]+)", name or "")
    return int(match.group(1)) if match else None


def _enabled_db_keys(name, client):
    """The key of every enabled version of secret name, newest first.

    Reads versions/latest, whose answer names its version number, then each
    lower number. Only secretmanager.versions.access is needed, which is all
    the server's account has on the secret (no versions.list). A disabled or
    destroyed version is skipped. Raises NotFound when the secret has no
    version.
    """
    # pylint: disable-next=import-outside-toplevel
    from google.api_core.exceptions import FailedPrecondition, NotFound

    latest = client.access_secret_version(
        request={"name": f"{name}/versions/latest"}
    )
    keys = [latest.payload.data]
    number = _version_number(latest.name)
    if number is None:
        logger.warning(
            "Secret Manager did not name the version it returned for %s, so "
            "only its latest version is used",
            name,
        )
        return keys
    for older in range(number - 1, 0, -1):
        try:
            response = client.access_secret_version(
                request={"name": f"{name}/versions/{older}"}
            )
        except (FailedPrecondition, NotFound):
            continue  # disabled or destroyed
        keys.append(response.payload.data)
    return keys


@contextlib.contextmanager
def _secret_errors(name):
    """Raise an error of Secret Manager or of Google authentication, met
    while the database-key secret name is used, as a CredentialError that
    names the secret, with the error as its cause. The credential pages show
    a CredentialError as a form error, and the start-up reports it."""
    # pylint: disable-next=import-outside-toplevel
    from google.api_core.exceptions import (
        FailedPrecondition,
        GoogleAPIError,
        NotFound,
    )
    from google.auth.exceptions import GoogleAuthError  # pylint: disable=import-outside-toplevel

    unusable = "so stored keys cannot be encrypted or read"
    try:
        yield
    except NotFound as err:
        raise CredentialError(
            f"Secret {name} does not exist or has no version, {unusable}"
        ) from err
    except FailedPrecondition as err:
        raise CredentialError(
            f"The latest version of secret {name} is disabled or destroyed, "
            f"{unusable}: {err}"
        ) from err
    except (GoogleAPIError, GoogleAuthError) as err:
        raise CredentialError(
            f"Secret {name} cannot be used, {unusable}: {err}"
        ) from err


def _read_db_keys(name, client=None):
    """The database keys from the enabled versions of secret name, newest
    first; any error reading them is a CredentialError"""
    with _secret_errors(name):
        return _enabled_db_keys(name, client or _secret_client())


def load_db_keys(http_get=None, client=None):
    """The database keys from the enabled versions of the metadata's
    secret, newest first. Raises CredentialError when they cannot be read."""
    return _read_db_keys(_db_key_secret(http_get), client)


def check_db_keys():
    """Read the database keys now, as the first use of a stored key would,
    and keep them for this process (see _fernet).

    Run before a loop over stored keys, so that a Secret Manager outage, a
    disabled latest version or a missing secret is one error rather than
    every row failing to decrypt. Raises CredentialError when the keys
    cannot be read.
    """
    _fernet()


def ensure_db_key(encrypted_rows_exist, http_get=None, client=None):
    """Make sure the database-key secret has a version, and use it.

    Adds a new Fernet key only when the secret has no version yet and no
    stored key is encrypted; a database with encrypted keys and no secret
    version has lost its key, which a new one cannot fix. Returns "present"
    or "created".

    Every enabled version is used: new keys are encrypted with the newest,
    and a stored key encrypted with any of them can be read. So a version
    added later never makes earlier rows unreadable, and neither do two
    first starts that each add a version.

    Raises CredentialError when the secret cannot be read or written, or a
    version does not hold a Fernet key.
    """
    from google.api_core.exceptions import NotFound  # pylint: disable=import-outside-toplevel

    name = _db_key_secret(http_get)
    status = "present"
    with _secret_errors(name):
        client = client or _secret_client()
        try:
            keys = _enabled_db_keys(name, client)
        except NotFound:
            if encrypted_rows_exist:
                raise CredentialError(
                    f"Secret {name} has no version, but the database holds "
                    "encrypted keys. Restore the secret's version; a new key "
                    "cannot decrypt them."
                ) from None
            client.add_secret_version(
                request={
                    "parent": name,
                    "payload": {"data": Fernet.generate_key()},
                }
            )
            # Read back every version: another first start may have added
            # one at the same time, and rows encrypted with either must stay
            # readable
            keys = _enabled_db_keys(name, client)
            status = "created"
    if len(keys) > 1:
        logger.info("Secret %s has %d enabled database keys", name, len(keys))
    use_db_key(keys, f"secret {name}")
    return status


def _row_now(credential_model, pk):
    """After a write that matched no row because the row was edited or
    deleted after it was read: "deleted", "current" (the row as it is now
    is sealed with the newest database key) or "changed" (it is not: it is
    sealed with an older key, is plaintext, or cannot be read)"""
    now = (
        credential_model.objects.filter(pk=pk)
        .values_list("detail", flat=True)
        .first()
    )
    if now is None:
        return "deleted"
    try:
        current = is_sealed(now) and _resealed(now) is None
    except (CredentialError, ValueError, TypeError):
        # Not readable either: it is not written, and the next run, which
        # reads it again, reports it as unreadable
        current = False
    return "current" if current else "changed"


def _reseal_row(credential_model, credential):
    """Re-seal a sealed row with the newest database key if an older one
    sealed it. Returns "current" (already sealed with the newest key),
    "resealed", or, when the row was edited or deleted after it was read
    and so is left as it is now, what _row_now says of it.

    The write matches the text that was read, so an edit made meanwhile is
    never overwritten. Raises as _resealed does.
    """
    old = credential.detail
    new = _resealed(old)
    if new is None:
        return "current"
    if credential_model.objects.filter(pk=credential.pk, detail=old).update(
        detail=new
    ):
        credential.detail = new
        return "resealed"
    return _row_now(credential_model, credential.pk)


def fix_existing_credentials(credential_model):
    """Encrypt plaintext keys, re-seal keys sealed with an older database
    key, and fill project_id; safe to run again.

    A row whose stored key cannot be decrypted (including text that is not
    a Fernet token at all, such as non-ASCII text), or whose project id
    cannot be read from it (the key is not JSON, or names no project as a
    non-empty string), counts as unreadable and is logged by credential id
    (never its key text); the other rows are still done. Every encrypted
    row is decrypted once, so a key the server can no longer read is
    reported at start-up rather than when a resource first uses it.

    An encrypted row that only an older enabled version of the database-key
    secret decrypts is encrypted again with the newest (rotation). Every
    write (re-seal, encryption of a plaintext key, project id) is made only
    if the row still holds the text and project id that were read. A row
    edited while this runs is left as the edit wrote it; if that text is not
    sealed with the newest key, it counts as changed and the next run seals
    it. A row deleted while this runs is skipped and not counted.

    The database keys are read on the first use of a stored key. Callers
    read them before (ensure_db_key at start-up, check_db_keys in the
    ofe_rotate_credentials command), so that keys that cannot be read are
    one error and not every row counted as unreadable.

    Returns counts: encrypted, resealed, project_filled, unchanged (sealed
    with the newest key and given a project id already), changed and
    changed_ids, unreadable and unreadable_ids (the ids of those rows).
    """
    counts = {
        "encrypted": 0,
        "resealed": 0,
        "project_filled": 0,
        "unchanged": 0,
        "changed": 0,
        "changed_ids": [],
        "unreadable": 0,
        "unreadable_ids": [],
    }

    def unreadable(credential, reason):
        counts["unreadable"] += 1
        counts["unreadable_ids"].append(credential.pk)
        logger.warning(
            "Credential %s was skipped: its stored key cannot be read (%s)",
            credential.pk,
            reason,
        )

    def edited_meanwhile(credential, state):
        """Count a row that was edited or deleted after it was read, and so
        was not written; state is what _row_now says of it"""
        if state == "deleted":
            logger.info(
                "Credential %s was deleted while this ran", credential.pk
            )
        elif state == "current":
            counts["unchanged"] += 1
        else:
            counts["changed"] += 1
            counts["changed_ids"].append(credential.pk)
            logger.warning(
                "Credential %s changed while this ran, and its stored key "
                "is not sealed with the newest database key; run this again "
                "to seal it",
                credential.pk,
            )

    for credential in credential_model.objects.filter(kind=KIND_KEY):
        sealed = is_sealed(credential.detail)
        if sealed:
            try:
                state = _reseal_row(credential_model, credential)
            except CredentialError as err:
                # Its text names no key
                unreadable(credential, str(err))
                continue
            except (ValueError, TypeError) as err:
                # Not a token TKFE wrote (UnicodeEncodeError for non-ASCII
                # text, among others). The error's text may quote part of
                # the row, so only its type is logged.
                unreadable(credential, type(err).__name__)
                continue
            if state in ("changed", "deleted"):
                edited_meanwhile(credential, state)
                continue
            if state == "resealed":
                counts["resealed"] += 1
                logger.info(
                    "Re-sealed the stored key of credential %s with the "
                    "newest database key",
                    credential.pk,
                )
            if credential.project_id:
                if state == "current":
                    counts["unchanged"] += 1
                continue
        old_detail, old_project = credential.detail, credential.project_id
        try:
            # As Credential.save() would: a plaintext key is encrypted, and
            # project_id is filled from the key
            seal(credential)
        except (CredentialError, ValueError, TypeError) as err:
            # CredentialError's own text names no key; any other error's
            # text may quote part of the row, so only its type is logged
            if isinstance(err, CredentialError):
                reason = str(err)
            else:
                reason = type(err).__name__
            unreadable(credential, reason)
            continue
        values = {
            field: getattr(credential, field)
            for field, old in (("detail", old_detail), ("project_id", old_project))
            if getattr(credential, field) != old
        }
        # Written only if the row still holds what was read, as a re-seal is,
        # so an edit made meanwhile (which sealed its own key and set its own
        # project id) is never overwritten, and a deleted row is skipped
        if values and not credential_model.objects.filter(
            pk=credential.pk, detail=old_detail, project_id=old_project
        ).update(**values):
            edited_meanwhile(credential, _row_now(credential_model, credential.pk))
            continue
        if not sealed:
            counts["encrypted"] += 1
            logger.info("Encrypted the stored key of credential %s", credential.pk)
        if isinstance(credential.project_id, str) and credential.project_id:
            counts["project_filled"] += 1
        else:
            # The key is not a JSON object, or names no project as a
            # string, so no resource can use it
            unreadable(credential, "it is not a key that names a project")
    return counts


def sealed_rows_exist(credential_model):
    """True when any stored key is encrypted"""
    return credential_model.objects.filter(
        kind=KIND_KEY, detail__startswith=ENCRYPTED_PREFIX
    ).exists()


def register_provisioning_credential(credential_model, user_model, email, project_id):
    """Create the impersonate credential for the provisioning account.

    Does nothing when email is empty or a credential already impersonates
    it. The credential belongs to the first admin user. Returns
    (credential or None, created).
    """
    email = (email or "").strip()
    if not email:
        return None, False
    existing = (
        credential_model.objects.filter(kind=KIND_IMPERSONATE, detail=email)
        .order_by("pk")
        .first()
    )
    if existing is not None:
        return existing, False
    owner = user_model.objects.filter(is_superuser=True).order_by("pk").first()
    if owner is None:
        raise CredentialError(
            "No admin user exists to own the provisioning credential"
        )
    if not project_id:
        raise CredentialError("The server's project id is unknown")
    credential = credential_model.objects.create(
        name=PROVISIONING_CREDENTIAL_NAME,
        owner=owner,
        kind=KIND_IMPERSONATE,
        detail=email,
        project_id=project_id,
    )
    return credential, True


# Code from before encrypted storage wrote a plaintext copy of the key in
# each working directory (LEGACY_KEY_FILE), and gcloud kept another in the
# configuration directory it activated in a Terraform directory
# (LEGACY_GCLOUD_CONFIG). These are the working directories, under the
# backend's baseDir, of clusters, filesystems, networks, workbenches and
# images.
LEGACY_KEY_FILE = "cloud_credentials"
LEGACY_GCLOUD_CONFIG = ".gcloud_config"
WORKING_DIRECTORIES = ("clusters", "fs", "vpcs", "workbenches", "images")
# Terraform's own plugin and module cache, where no key copy was written
_SKIPPED_DIRECTORIES = (".terraform",)


def remove_legacy_key_copies(base_dir):
    """Delete the plaintext key copies older code left on disk.

    Removes only files named LEGACY_KEY_FILE and directories named
    LEGACY_GCLOUD_CONFIG under base_dir's WORKING_DIRECTORIES. Symbolic links
    are never followed or removed: one with either name is counted as
    skipped. Safe to run again. Returns counts: files, directories, skipped,
    failed.
    """
    counts = {"files": 0, "directories": 0, "skipped": 0, "failed": 0}

    def remove(path, remover, kind):
        try:
            remover(path)
            counts[kind] += 1
        except OSError as err:
            counts["failed"] += 1
            logger.warning("Could not remove old key copy %s: %s", path, err)

    for name in WORKING_DIRECTORIES:
        root = Path(base_dir) / name
        if root.is_symlink() or not root.is_dir():
            continue
        for dirpath, dirnames, filenames in os.walk(root, followlinks=False):
            here = Path(dirpath)
            descend = []
            for dirname in dirnames:
                path = here / dirname
                if dirname == LEGACY_GCLOUD_CONFIG:
                    if path.is_symlink():
                        counts["skipped"] += 1
                    else:
                        remove(path, shutil.rmtree, "directories")
                elif dirname not in _SKIPPED_DIRECTORIES:
                    descend.append(dirname)
            dirnames[:] = descend
            if LEGACY_KEY_FILE in filenames:
                path = here / LEGACY_KEY_FILE
                if path.is_symlink() or not path.is_file():
                    counts["skipped"] += 1
                else:
                    remove(path, os.unlink, "files")
    return counts


def _backend_base_dir():
    from . import utils  # pylint: disable=import-outside-toplevel

    return utils.load_config()["baseDir"]


def _step_error(step, err):
    """The error-list entry for a bootstrap step that raised err.

    The list is shown to superusers on the web pages. A CredentialError's
    message is written by OFE's credential code, which quotes names, paths,
    HTTP statuses and Google's error messages but never key text, so it is
    kept. Any other exception's text could quote stored data, so only its
    type is returned; the full error and traceback go to the server log.
    """
    if isinstance(err, CredentialError):
        return f"{step}: {err}"
    logger.error("%s: unexpected error", step, exc_info=err)
    return (
        f"{step}: unexpected error ({type(err).__name__}); see the server log"
    )


def bootstrap(
    credential_model, user_model, http_get=None, client=None, report=None,
    base_dir=None,
):
    """The start-up steps, run by the ofe_bootstrap_credentials command.

    1. Plaintext key copies that older code left in the working directories
       under base_dir (default: the backend's baseDir) are deleted.
    2. The database-key secret gets a version if it has none.
    3. Existing key rows are encrypted, re-sealed with the newest database
       key if an older one sealed them, and get their project_id.
    4. The provisioning account named in metadata gets an impersonate
       credential, if none impersonates it yet.

    report(text) receives progress lines (never key material). Returns the
    list of errors; each step runs even if an earlier one failed, except
    that rows are not touched without the key.
    """
    report = report or logger.info
    errors = []
    try:
        counts = remove_legacy_key_copies(
            base_dir if base_dir is not None else _backend_base_dir()
        )
        report(
            "Old key copies on disk: {files} files and {directories} gcloud "
            "configuration directories removed, {skipped} symbolic links "
            "skipped".format(**counts)
        )
        if counts["failed"]:
            errors.append(
                f"Old key copies on disk: {counts['failed']} could not be "
                "removed; see the warnings above"
            )
    except Exception as err:  # pylint: disable=broad-except
        errors.append(_step_error("Old key copies on disk", err))
    try:
        status = ensure_db_key(
            sealed_rows_exist(credential_model), http_get=http_get, client=client
        )
        report(f"Database key: {status}")
        counts = fix_existing_credentials(credential_model)
        report(
            "Stored keys: {encrypted} encrypted, {project_filled} given a "
            "project id, {unchanged} already done".format(**counts)
        )
        report(
            "Stored keys sealed with an older database key: {resealed} "
            "re-sealed with the newest, {changed} changed meanwhile and left "
            "for the next start".format(**counts)
        )
        if counts["unreadable"]:
            ids = ", ".join(str(pk) for pk in counts["unreadable_ids"])
            errors.append(
                f"Stored keys: {counts['unreadable']} could not be read and "
                f"were skipped (credential ids: {ids}). The server cannot "
                "decrypt them with its database key, or they are damaged. If "
                "a version of the database-key secret was disabled, enable it "
                "again and restart the server; otherwise delete each of these "
                "credentials in TKFE and add it again"
            )
    except Exception as err:  # pylint: disable=broad-except
        errors.append(_step_error("Database key or stored keys", err))

    try:
        email = metadata_value(
            f"instance/attributes/{PROVISIONING_ACCOUNT_ATTRIBUTE}", http_get
        )
        if not email:
            report("No provisioning account in instance metadata")
        else:
            project_id = metadata_value("project/project-id", http_get)
            credential, created = register_provisioning_credential(
                credential_model, user_model, email, project_id
            )
            if created:
                report(f"Registered credential {credential.pk} for {email}")
            else:
                report(f"Credential {credential.pk} already impersonates {email}")
    except Exception as err:  # pylint: disable=broad-except
        errors.append(_step_error("Provisioning credential", err))
    return errors
