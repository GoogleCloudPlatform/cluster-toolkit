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
"""Customer-managed encryption keys, delegated to the Cluster Toolkit modules.

OFE is a GUI for blueprint generation, so it does not manage keys itself. It
collects a choice on the resource's form and emits module blocks:

    community/modules/security/kms-key      create a key for this resource
    community/modules/security/kms-key-iam  grant on it

Everything else is the modules' job. In particular OFE does not create keys
through its own Terraform, does not call the Cloud KMS API to grant access, and
does not poll for IAM propagation: `kms-key-iam` makes the grants, and Terraform
orders consumers behind it because they `use:` that module rather than the key.

`kms-key-iam` re-exports the key id under seven consumer-specific output names,
so a consumer that says `use: [cmek_key_iam]` picks up the right input with no
further wiring:

    disk_encryption_key        slurm nodesets, instance templates
    slurm_bucket_kms_key       slurm controller
    kms_key_name               filestore
    image_encryption_key       packer custom-image
    repository_kms_key_name    artifact registry
    encryption_key_name        cloudsql federation

The one thing the modules cannot know is which service agents a given kind of
resource needs, so that mapping lives here.
"""
import dataclasses
import json
import logging
import re
from typing import Iterable, Optional

logger = logging.getLogger(__name__)

# Blueprint ids OFE gives the two module instances. Fixed so the rest of the
# generator can reference them without parsing the emitted YAML.
KEY_MODULE_ID = "cmek_key"
KEY_IAM_MODULE_ID = "cmek_key_iam"

KEY_MODULE_SOURCE = "community/modules/security/kms-key"
KEY_IAM_MODULE_SOURCE = "community/modules/security/kms-key-iam"

# Used when nothing has been configured, so encryption works on a fresh
# deployment without anyone visiting the settings page first. A key ring is
# permanent and free to hold, so a predictable default costs nothing and the
# alternative -- refusing to create a key until someone sets a name -- makes
# the feature unusable out of the box.
DEFAULT_KEY_RING = "ofe-cmek"

# Whether a resource may be created without a customer-managed key. Deployment
# wide, set on the superuser-only encryption settings page.
ENFORCEMENT_REQUIRED = "required"
ENFORCEMENT_PREFERRED = "preferred"
ENFORCEMENT_DISABLED = "disabled"
ENFORCEMENT_MODES = (
    ENFORCEMENT_REQUIRED, ENFORCEMENT_PREFERRED, ENFORCEMENT_DISABLED,
)

# kms-key's deletion_policy. Required, with no module default, because DELETE
# and ABANDON are opposite and both irreversible.
DELETION_POLICY_DELETE = "DELETE"
DELETION_POLICY_ABANDON = "ABANDON"
DELETION_POLICIES = (DELETION_POLICY_DELETE, DELETION_POLICY_ABANDON)

# kms-key's protection_level. Chosen at creation and effectively immutable
# afterwards -- changing it needs a new key name.
PROTECTION_SOFTWARE = "SOFTWARE"
PROTECTION_HSM = "HSM"
PROTECTION_LEVELS = (PROTECTION_SOFTWARE, PROTECTION_HSM)

# kms-key's rotation_period floor: greater than one day, and it cannot be
# disabled. Whole seconds with a trailing "s", at most ten digits: int()
# refuses a few thousand, and ten already covers centuries.
_DURATION_RE = re.compile(r"^[0-9]{1,10}s$")
MIN_ROTATION_SECONDS = 86400

# Which kms-key-iam service agents each protected resource kind needs.
# cluster needs two: the Compute agent for the controller/login/nodeset
# disks, and the Storage agent for the controller's GCS bucket
# (schedmd-slurm-gcp-v6-controller's create_bucket defaults to true, so this
# bucket exists and is encrypted by the same key whenever a cluster's key
# reaches the controller via slurm_bucket_kms_key).
#
# An earlier draft of this table gave "compute-disk" no agents at all, on the
# theory that Compute Engine encrypts as its own service agent when
# disk_encryption_key_service_account is left unset (true) and therefore needs
# no grant (false: it is exactly that default agent kms-key-iam's `compute`
# option grants). Reproduced live: an OFE cluster failed at apply with
# `kmsPermissionDenied` on the controller disk and `Permission denied on Cloud
# KMS key` on the controller bucket, both because no agent had been granted.
SERVICE_AGENT_NAMES = {
    "storage-bucket": ["storage"],
    "pubsub-topic": ["pubsub"],
    "artifact-registry": ["artifactregistry"],
    "cloudsql-instance": ["cloudsql"],
    "filestore": ["filestore"],
    "secret": ["secretmanager"],
    "image": ["compute"],
    "snapshot": ["compute"],
    "cluster": ["compute", "storage"],
}

# Every agent kms-key-iam can grant, by the short names it accepts. A resource
# kind's own list (above) is a floor: a cluster's key is also handed to Cloud
# SQL and Artifact Registry when the cluster uses them, so the caller adds those
# agents through `extra_service_agents`.
KMS_IAM_AGENT_NAMES = frozenset(
    agent for agents in SERVICE_AGENT_NAMES.values() for agent in agents)

# Cloud KMS requires key ring and CryptoKey ids to match this. Checked before
# any apply because both names are permanent: a key ring can never be deleted
# and a CryptoKey name can never be reused.
KMS_NAME_RE = re.compile(r"^[a-zA-Z0-9_-]{1,63}$")

# A bare service-account email. kms-key-iam adds the "serviceAccount:" prefix
# itself and rejects a value that already carries one. Only the characters a
# service-account email can hold, so nothing that would end a YAML flow list
# (",", "[", "]", "#") or carry a user:/group:/domain:/deleted: prefix gets in.
_SA_EMAIL_RE = re.compile(
    r"^[A-Za-z0-9._+-]+@[A-Za-z0-9.-]+\.gserviceaccount\.com$")

# A Google Cloud project id: 6 to 30 characters bare, or domain-scoped
# (example.com:proj), where the part after the colon may be as short as one.
# Checked because it is written into a blueprint, where anything else could
# change the YAML around it.
_PROJECT_ID_RE = re.compile(
    r"^(?:[a-z][a-z0-9-]{4,28}[a-z0-9]"
    r"|[a-z][a-z0-9.-]*[a-z0-9]:[a-z](?:[a-z0-9-]{0,28}[a-z0-9])?)$")

_KEY_PATH_RE = re.compile(
    r"^projects/(?P<project>[^/]+)/locations/(?P<location>[^/]+)"
    r"/keyRings/(?P<ring>[a-zA-Z0-9_-]{1,63})"
    r"/cryptoKeys/(?P<key>[a-zA-Z0-9_-]{1,63})$")


class CmekError(Exception):
    """Anything wrong with an encryption choice."""


class CmekConfigError(CmekError):
    """A setting OFE will not send to the modules."""


def _kind_of(value) -> str:
    """What a YAML value is, in the words an operator would use."""
    if isinstance(value, bool):
        return "true/false"
    if isinstance(value, (int, float)):
        return "a number"
    if isinstance(value, (list, tuple)):
        return "a list"
    if isinstance(value, dict):
        return "a mapping"
    return f"{value!r}"


def _text_setting(section: dict, name: str, default: str):
    """One setting of the `cmek:` section, refused unless it is text.

    YAML reads an unquoted project number as a number and yes/no as a
    boolean, and the checks after this one only work on text.
    """
    value = section.get(name, default)
    if value is None or isinstance(value, str):
        return value
    raise CmekConfigError(
        f"cmek.{name} must be text, not {_kind_of(value)}: quote it in "
        "configuration.yaml")


@dataclasses.dataclass(frozen=True)
class CmekSettings:
    """Deployment-wide encryption settings.

    Read from the `cmek:` section of configuration.yaml. Only three, because
    everything else is now a per-resource choice or a module default.
    """

    enforcement: str = ENFORCEMENT_PREFERRED
    key_project_id: str = ""
    key_ring_name: str = DEFAULT_KEY_RING

    @classmethod
    def from_config(cls, config) -> "CmekSettings":
        section = (config or {}).get("cmek") or {}
        if not isinstance(section, dict):
            raise CmekConfigError(
                "the cmek section of configuration.yaml must hold the "
                "settings enforcement, key_project_id and key_ring_name, "
                f"not {_kind_of(section)}")
        enforcement = _text_setting(
            section, "enforcement", ENFORCEMENT_PREFERRED)
        if enforcement not in ENFORCEMENT_MODES:
            raise CmekConfigError(
                f"cmek.enforcement must be one of "
                f"{', '.join(ENFORCEMENT_MODES)}, not {enforcement!r}")
        ring = _text_setting(section, "key_ring_name", "") or ""
        if ring and not KMS_NAME_RE.fullmatch(ring):
            raise CmekConfigError(
                f"cmek.key_ring_name {ring!r} is not a valid Cloud KMS key "
                "ring name (letters, digits, hyphen or underscore, up to 63 "
                "characters)")
        project = assert_project_id(
            _text_setting(section, "key_project_id", "") or "",
            what="cmek.key_project_id")
        return cls(
            enforcement=enforcement,
            key_project_id=project,
            key_ring_name=ring or DEFAULT_KEY_RING,
        )

    def to_config_section(self) -> dict:
        """The `cmek:` mapping to write back to configuration.yaml.

        Empty values are omitted rather than written as "", so the file keeps
        saying "unset" instead of gaining keys that mean nothing.
        """
        section = {"enforcement": self.enforcement}
        if self.key_project_id:
            section["key_project_id"] = self.key_project_id
        if self.key_ring_name:
            section["key_ring_name"] = self.key_ring_name
        return section

    @property
    def required(self) -> bool:
        return self.enforcement == ENFORCEMENT_REQUIRED

    @property
    def offered(self) -> bool:
        """Whether the encryption panel is shown at all."""
        return self.enforcement != ENFORCEMENT_DISABLED


def parse_key_path(path: str) -> dict:
    """Split a full CryptoKey resource name into its parts."""
    match = _KEY_PATH_RE.match((path or "").strip())
    if not match:
        raise CmekConfigError(
            f"{path!r} is not a full Cloud KMS key name of the form "
            "projects/PROJECT/locations/LOCATION/keyRings/RING/cryptoKeys/KEY")
    return match.groupdict()


def normalize_location(location: str) -> str:
    """A zone encrypts with its region's key ring; Cloud KMS has no zones."""
    text = (location or "").strip().lower()
    parts = text.split("-")
    # e.g. europe-west4-a -> europe-west4, but leave "us" and "europe" alone
    if len(parts) == 3 and len(parts[2]) == 1:
        return "-".join(parts[:2])
    return text


def assert_service_account_email(account: str) -> str:
    """Reject anything that is not a bare service-account email.

    kms-key-iam adds the "serviceAccount:" prefix itself and rejects a value
    that already carries one, so a principal string pasted into the form would
    fail Terraform validation partway through an apply. Catching it here names
    the setting the operator actually typed.
    """
    text = (account or "").strip()
    if text.startswith("serviceAccount:"):
        raise CmekConfigError(
            f"{text!r} must be a bare email address, not a principal string "
            '— drop the "serviceAccount:" prefix')
    if ":" in text.split("@", 1)[0]:
        raise CmekConfigError(
            f"{text!r} must be a bare email address, not a principal string "
            "— drop the type prefix (such as user:, group:, domain: or "
            "deleted:); kms-key-iam adds serviceAccount: itself")
    if text and not _SA_EMAIL_RE.fullmatch(text):
        raise CmekConfigError(
            f"{text!r} is not a service account email; it should look like "
            "name@project.iam.gserviceaccount.com")
    return text


def assert_project_id(value: str, what: str = "project id") -> str:
    """Reject anything that is not a Google Cloud project id.

    The key project is typed by hand and written into the blueprint, so a value
    that is not a project id is refused here rather than sent to the modules.
    """
    text = (value or "").strip()
    if text and not _PROJECT_ID_RE.fullmatch(text):
        raise CmekConfigError(
            f"{what} {text!r} is not a valid Google Cloud project id (6 to 30 "
            "lowercase letters, digits or hyphens, starting with a letter and "
            "not ending with a hyphen; after a domain such as example.com: it "
            "may be shorter)")
    return text


def assert_rotation_period(value: str) -> str:
    """Reject a rotation period kms-key would refuse.

    The module's own rule: whole seconds ending in "s", at least 86400s. Checked
    here so the operator sees the setting they typed rather than a Terraform
    validation failure partway through an apply.
    """
    text = (value or "").strip()
    if not text:
        return ""
    if not _DURATION_RE.match(text):
        raise CmekConfigError(
            f"{text!r} is not a duration of whole seconds ending in \"s\", "
            "for example 7776000s for 90 days")
    if int(text[:-1]) < MIN_ROTATION_SECONDS:
        raise CmekConfigError(
            f"a rotation period must be at least {MIN_ROTATION_SECONDS}s "
            f"(24 hours), not {text}")
    return text


def assert_protection_level(value: str) -> str:
    """Reject a protection level kms-key would refuse."""
    text = (value or "").strip()
    if text and text not in PROTECTION_LEVELS:
        raise CmekConfigError(
            f"protection level must be one of {', '.join(PROTECTION_LEVELS)}, "
            f"not {text!r}")
    return text


def service_agents_for(service: str) -> list:
    """Which kms-key-iam service agents this resource kind needs."""
    if service not in SERVICE_AGENT_NAMES:
        raise CmekConfigError(
            f"unknown protected service {service!r}; expected one of "
            f"{', '.join(sorted(SERVICE_AGENT_NAMES))}")
    return list(SERVICE_AGENT_NAMES[service])


def derive_key_name(prefix: str, identity: str) -> str:
    """Name a key OFE places itself.

    Never derived from the database id: that is a bare auto-increment integer,
    so two OFE installs sharing one project would collide on it and silently
    share a key. A cluster passes its cloud id. A Filestore or an image has no
    cloud id when its blueprint is written, so it passes its name plus a random
    suffix chosen once and stored with the resource.
    """
    name = f"{prefix}-{identity}".strip("-").lower()
    name = re.sub(r"[^a-z0-9_-]+", "-", name)[:63].strip("-")
    if not KMS_NAME_RE.fullmatch(name):
        raise CmekConfigError(
            f"cannot derive a valid Cloud KMS key name from {identity!r}")
    return name


def _yaml_scalar(value) -> str:
    """One value as YAML: strings double-quoted, so nothing in them is syntax.

    JSON's string form is a valid YAML double-quoted scalar, newlines and
    quotes escaped, so a typed value can never end the mapping it sits in.
    """
    if isinstance(value, (bool, int)):
        return json.dumps(value)
    return json.dumps(str(value))


def _yaml_settings(pairs, indent: int) -> str:
    pad = " " * indent
    lines = []
    for k, v in pairs:
        if v is None:
            continue
        if isinstance(v, (list, tuple)):
            text = "[" + ", ".join(_yaml_scalar(item) for item in v) + "]"
        else:
            text = _yaml_scalar(v)
        lines.append(f"{pad}  {k}: {text}\n")
    return "".join(lines)


def key_ring_path(project_id: str, location: str, key_ring_name: str) -> str:
    """The full id of a key ring, as `kms-key`'s key_ring_id expects it."""
    return (f"projects/{project_id}/locations/{normalize_location(location)}"
            f"/keyRings/{key_ring_name}")


def full_key_path(project_id: str, location: str, key_ring_name: str,
                  key_name: str) -> str:
    """The full id of a CryptoKey, in the form every OFE key field expects."""
    return (f"{key_ring_path(project_id, location, key_ring_name)}"
            f"/cryptoKeys/{key_name}")


def resource_key_path(resource) -> str:
    """The full path of a resource's key, or "" if it cannot be determined.

    Every key is created by the resource that uses it, so the path is always
    rebuilt from what the deploy wrote back to
    cmek_key_name/cmek_key_ring_name/cmek_key_project_id. Shared with the
    create-time uniqueness check (resources_sharing_key below) so the two never
    learn to disagree about what a resource's key path is.
    """
    if resource.cmek_key_name and resource.cmek_key_ring_name:
        location = normalize_location(
            getattr(resource, "cloud_zone", None)
            or getattr(resource, "cloud_region", None) or "")
        if not location:
            return ""
        return full_key_path(
            resource.cmek_key_project_id or "", location,
            resource.cmek_key_ring_name, resource.cmek_key_name)
    return ""


CONSOLE_KMS_ROOT = "https://console.cloud.google.com/security/kms"


def console_keyring_url(project: str, location: str, ring: str) -> str:
    """Cloud Console page for a key ring."""
    return (f"{CONSOLE_KMS_ROOT}/keyring/manage/{location}/{ring}"
            f"?project={project}")


def console_key_url(project: str, location: str, ring: str, key: str) -> str:
    """Cloud Console page for one CryptoKey.

    OFE only ever reads Cloud KMS state; anything an operator wants to change
    about a key -- including destroying one OFE abandoned -- is done in the
    Console, which is what these links are for.
    """
    return (f"{CONSOLE_KMS_ROOT}/key/manage/{location}/{ring}/{key}"
            f"?project={project}")


def _resource_project(resource) -> str:
    """The project of the credential a resource deploys with, or ""."""
    try:
        return getattr(resource, "project_id", "") or ""
    except (AttributeError, ValueError):
        return ""


def resolved_key_path(resource, settings: CmekSettings) -> str:
    """The full path a resource's typed key name will deploy to, or "".

    Resolved the way the deploy resolves it (resolve_created_key), so a
    resource that has not been deployed yet, whose blank project and key ring
    the deploy has not yet filled in, is compared by where its key will
    actually live. A deployed resource has all three fields written back, so
    this gives the same path as resource_key_path. A blank key name gives "":
    OFE derives one unique to the resource.
    """
    if not resource.cmek_key_name:
        return ""
    location = normalize_location(
        getattr(resource, "cloud_zone", None)
        or getattr(resource, "cloud_region", None) or "")
    resolved = resolve_created_key(
        {"key_name": resource.cmek_key_name,
         "key_ring_name": resource.cmek_key_ring_name,
         "key_project_id": resource.cmek_key_project_id},
        settings, _resource_project(resource))
    if not (location and resolved["key_ring_name"]
            and resolved["key_project_id"]):
        return ""
    return full_key_path(
        resolved["key_project_id"], location, resolved["key_ring_name"],
        resolved["key_name"])


def resources_sharing_key(key_path: str, exclude=None,
                          settings: Optional[CmekSettings] = None) -> list:
    """Other live, encrypted resources whose key resolves to key_path.

    kms-key-iam's service agents (compute, storage, filestore, ...) are
    scoped to the project, not the resource, so two resources sharing one key
    end up with the identical (member, role) grant on it -- destroying either
    resource's kms-key-iam removes that grant for both, not just the one being
    torn down. Each resource therefore gets its own key, and this
    reports the collisions the per-resource key name cannot rule out: two
    resources given the same name and key ring by hand. Used to refuse that at
    creation, before either has been deployed.

    Only live resources count. A torn-down one still holds its key fields, and
    treating it as a sharer would refuse a key that is genuinely free.

    With `settings`, each resource's path is resolved as the deploy will
    resolve it, so two resources not yet deployed that were given the same
    name with the project and key ring left blank are seen to collide. Without
    it, only the stored fields are compared.
    """
    if not key_path:
        return []
    from ..models import (  # pylint: disable=import-outside-toplevel
        Cluster, GCPFilestoreFilesystem, Image,
    )

    kind_labels = {Cluster: "Cluster", GCPFilestoreFilesystem: "Filestore",
                   Image: "Image"}
    sharers = []
    for model, label in kind_labels.items():
        rows = model.objects.filter(cmek_enabled=True).exclude(
            cloud_state="xm")
        if model is Cluster:
            rows = rows.exclude(status="d")
        for row in rows:
            if exclude is not None and type(row) is type(exclude) \
                    and row.pk == exclude.pk:
                continue
            path = (resolved_key_path(row, settings) if settings
                    else resource_key_path(row))
            if path == key_path:
                sharers.append(f"{label} {row.name!r}")
    return sharers


def ring_already_exists(credentials: str, deployed: dict,
                        location: str) -> bool:
    """Whether the ring a key OFE creates will live in is already there.

    Answered against Cloud KMS rather than OFE's own records, because the ring
    outlives every deployment that used it and may have been created outside
    OFE entirely.
    """
    # Imported here so this module stays free of GCP clients: everything else
    # in it is pure blueprint text, and its tests run without credentials.
    from . import cloud_info

    location = normalize_location(location)
    try:
        return cloud_info.kms_key_ring_exists(
            credentials, deployed["key_project_id"], location,
            deployed["key_ring_name"])
    except (cloud_info.KeyRingCheckError, CmekError):
        raise
    except Exception as err:  # pylint: disable=broad-except
        # Building the client can fail before Cloud KMS is asked anything
        # (a credential it cannot use, no network), and that leaves the
        # answer just as unknown as an error from Cloud KMS itself. Raised
        # the same way, so every caller has a message to show.
        name = key_ring_path(
            deployed["key_project_id"], location, deployed["key_ring_name"])
        raise cloud_info.KeyRingCheckError(
            f"Cannot check the Cloud KMS key ring {name}: {err}",
            None) from err


def resolve_created_key(chosen: dict, settings: CmekSettings,
                        project_id: str) -> dict:
    """Name, ring and project a key OFE creates will actually use.

    Both the blueprint and the record stored on the resource come from here, so
    the detail page reports what was deployed rather than recomputing it from
    settings that may since have changed.
    """
    ring = (chosen.get("key_ring_name") or "").strip() or settings.key_ring_name
    project = ((chosen.get("key_project_id") or "").strip()
               or settings.key_project_id or project_id)
    return {
        "key_name": (chosen.get("key_name") or "").strip(),
        "key_ring_name": ring,
        "key_project_id": project,
    }


def key_modules_yaml(
    *,
    service: str,
    project_id: str,
    location: str,
    settings: CmekSettings,
    key_name: str = "",
    key_project_id: str = "",
    key_ring_name: str = "",
    deletion_policy: str = "",
    protection_level: str = "",
    rotation_period: str = "",
    encryption_service_account: str = "",
    ring_exists: bool = False,
    custom_service_accounts: Optional[Iterable[str]] = None,
    extra_service_agents: Optional[Iterable[str]] = None,
    iam_outputs: Optional[Iterable[str]] = None,
    indent: int = 2,
) -> str:
    """The blueprint blocks for one resource's key.

    Emits `kms-key`, then `kms-key-iam`. Every resource creates its own key:
    kms-key-iam's service agents are project-scoped, so two resources sharing
    a key would share one (member, role) grant that either could destroy.
    Consumers reference `kms_key_iam` and not the key, so Terraform
    creates the key and grants on it before anything encrypts with it, and on
    destroy removes the consumers before the grants they rely on.

    The key ring is the one place sharing is safe, and expected: a ring can
    never be deleted, so `ring_exists` adopts one that is already there.
    """
    location = normalize_location(location)
    pad = " " * indent
    blocks = []

    resolved = resolve_created_key(
        {"key_name": key_name,
         "key_ring_name": key_ring_name,
         "key_project_id": key_project_id},
        settings, project_id)
    name = resolved["key_name"]
    ring = resolved["key_ring_name"]
    project = resolved["key_project_id"]
    for label, value in (("key name", name), ("key ring name", ring)):
        if not value:
            raise CmekConfigError(
                f"a {label} is required to create a key; set it on the "
                "resource or as a deployment default")
        if not KMS_NAME_RE.fullmatch(value):
            raise CmekConfigError(
                f"{value!r} is not a valid Cloud KMS {label} (letters, "
                "digits, hyphen or underscore, up to 63 characters)")
    policy = (deletion_policy or "").strip() or DELETION_POLICY_ABANDON
    if policy not in DELETION_POLICIES:
        raise CmekConfigError(
            f"deletion policy must be one of {', '.join(DELETION_POLICIES)}"
            f", not {policy!r}")
    blocks.append(
        f"{pad}- id: {KEY_MODULE_ID}\n"
        f"{pad}  source: {KEY_MODULE_SOURCE}\n"
        f"{pad}  settings:\n"
        + _yaml_settings([
            ("project_id", project),
            ("location", location),
            # Cloud KMS key rings can never be deleted, so a ring shared
            # by several resources exists already for all but the first.
            # kms-key creates a ring from key_ring_name and adopts one
            # from key_ring_id, and takes exactly one of the two; both
            # still require project_id and location to agree with it.
            ("key_ring_id",
             key_ring_path(project, location, ring) if ring_exists
             else None),
            ("key_ring_name", None if ring_exists else ring),
            ("key_name", name),
            # required, with no module default
            ("deletion_policy", policy),
            # omitted when unset, so kms-key's own defaults apply rather
            # than OFE restating them and being able to disagree
            ("protection_level", assert_protection_level(protection_level)
             or None),
            ("rotation_period", assert_rotation_period(rotation_period)
             or None),
        ], indent + 2)
    )

    # kms-key-iam grants on exactly one key, so it is rendered per key. OFE
    # creates the key, so OFE administers its access too: there is no longer a
    # case where something else holds the grants.
    iam_settings = [("project_id", project_id)]
    agents = service_agents_for(service)
    for agent in extra_service_agents or ():
        if agent not in KMS_IAM_AGENT_NAMES:
            raise CmekConfigError(
                f"unknown service agent {agent!r}; expected one of "
                f"{', '.join(sorted(KMS_IAM_AGENT_NAMES))}")
        if agent not in agents:
            agents.append(agent)
    iam_settings.append(("service_agents", agents))
    extra = [assert_service_account_email(a)
             for a in (custom_service_accounts or []) if a]
    account = assert_service_account_email(encryption_service_account)
    if account:
        extra.append(account)
    if extra:
        iam_settings.append(("custom_service_accounts", extra))
    iam_block = (
        f"{pad}- id: {KEY_IAM_MODULE_ID}\n"
        f"{pad}  source: {KEY_IAM_MODULE_SOURCE}\n"
        f"{pad}  use: [{KEY_MODULE_ID}]\n"
        f"{pad}  settings:\n"
        + _yaml_settings(iam_settings, indent + 2)
    )
    if iam_outputs:
        # A consumer in another deployment group cannot `use:` this module
        # unless the output it needs is promoted to the group's own outputs --
        # the same reason the image blueprint's startup-script module declares
        # `outputs: [startup_script]`.
        iam_block += f"{pad}  outputs: [{', '.join(iam_outputs)}]\n"
    blocks.append(iam_block)
    return "\n".join(blocks)


def is_frozen(resource) -> bool:
    """Whether a resource is deployed, so its encryption can no longer change.

    Turning encryption on or off, or changing the key, on a deployed resource
    would mean recreating its disks, bucket or share. A resource is frozen
    once an apply with its choice has been started (`cmek_applied`, stored
    by record_apply_started before the apply runs, so a failed first apply
    counts too; an image stores it once its build succeeds) or an
    administrator has revoked its key (`cmek_revoked`); both are stored, so
    they hold whatever state the record is in later. An unsaved resource has
    nothing deployed.

    Starting a cluster and creating a Filestore reset the record to "nm"
    before the blueprint is regenerated, so those flows take a snapshot
    first, in the instance attribute `_cmek_deployed`, and it counts here
    too. It lives on that one instance only; nothing stores it.

    Resources deployed before the stored flags existed fall back to their
    state: a Filestore is deployed once its cloud_state leaves "nm"; a cluster
    once it is managed, running or being destroyed. An image has no update
    view, so it is frozen only through the flags or the snapshot.

    The flags are compared with `is True`, so a stand-in object whose every
    attribute exists (a MagicMock) is never frozen by accident.

    A failed first apply puts the record back to cloud_state "nm" (and a
    cluster to status "e"), but `cmek_applied` is already stored, so the
    resource stays frozen: a retry applies the same choice over whatever the
    failed apply created. To change the choice, destroy the resource and
    create it again.
    """
    if resource is None or getattr(resource, "pk", None) is None:
        return False
    if getattr(resource, "cmek_revoked", False) is True \
            or getattr(resource, "cmek_applied", False) is True \
            or getattr(resource, "_cmek_deployed", False) is True:
        return True
    from ..models import Image  # pylint: disable=import-outside-toplevel

    if isinstance(resource, Image):
        return False
    cloud_state = getattr(resource, "cloud_state", None)
    if hasattr(resource, "status"):
        return (cloud_state in ("m", "re", "dm")
                or getattr(resource, "status", None) in ("r", "re"))
    return bool(cloud_state) and cloud_state != "nm"


def record_deployed(resource) -> None:
    """Store `cmek_applied` on a deployed resource that does not have it yet.

    A resource deployed before the flag existed is frozen only by its state
    (is_frozen), and a failed start or reconfigure puts that state back to
    "not managed". Called on entry to those flows, while the state (or the
    `_cmek_deployed` snapshot) still says deployed, so that a failure later
    in the flow cannot unfreeze it.
    """
    if getattr(resource, "cmek_applied", False) is True \
            or not is_frozen(resource):
        return
    resource.cmek_applied = True
    resource.save(update_fields=["cmek_applied"])


def record_apply_started(resource) -> None:
    """Store `cmek_applied` just before a `terraform apply` runs.

    An apply that fails part way can already have created disks, a bucket,
    a share or the key itself with the generated choice, so a retry must
    generate the same choice whatever the enforcement is by then. Stored
    whether or not a key is used, on its own, before the apply starts.

    The flags are re-read first: a key revoked since the flow began is
    refused (CmekConfigError) and nothing is stored or applied.
    """
    if resource is None:
        return
    keep_flags_current(resource)
    assert_not_revoked(resource)
    if getattr(resource, "cmek_applied", False) is True:
        return
    resource.cmek_applied = True
    resource.save(update_fields=["cmek_applied"])


def keep_flags_current(resource) -> None:
    """Re-read `cmek_revoked` and `cmek_applied` before a full save.

    The long flows (start, reconfigure, apply, destroy, an image build) load
    the record once and save all of it at the end, so a key revoked while
    they ran would be written back as not revoked. Both flags are only ever
    set to True, each by a save of that field alone, so the stored value is
    always the one to keep. A Filesystem's flags live on its Filestore row.
    Anything without the flags, or not saved yet, is left alone.
    """
    if resource is not None and not hasattr(resource, "cmek_revoked"):
        resource = getattr(resource, "gcpfilestorefilesystem", None)
    if resource is None or getattr(resource, "pk", None) is None:
        return
    # pylint: disable-next=import-outside-toplevel
    from django.core.exceptions import ObjectDoesNotExist

    try:
        resource.refresh_from_db(fields=["cmek_revoked", "cmek_applied"])
    except ObjectDoesNotExist:
        pass


def assert_not_revoked(resource) -> None:
    """Refuse to start, change or re-apply a resource whose key was revoked.

    Revoking is final in OFE: restoring the key versions in the Cloud Console
    does not clear `cmek_revoked`. Not called by resource_settings, so the
    detail page and the forms still show what the resource was encrypted with.
    """
    if getattr(resource, "cmek_revoked", False) is True:
        raise CmekConfigError(
            f"The encryption key for '{getattr(resource, 'name', resource)}' "
            "was revoked by an administrator, so this resource can no longer "
            "be started, changed or re-applied. Destroy it and create a new "
            "one. (Restoring the key versions in the Cloud Console does not "
            "clear this.)")


def resource_settings(resource, settings: CmekSettings) -> Optional[dict]:
    """The key arguments for a resource, or None when it is not encrypted.

    Reads the `cmek_*` fields the resource forms write. Enforcement wins: with
    `required` a resource cannot opt out, and with `disabled` no key is ever
    emitted. Except on a deployed resource (is_frozen), whose stored choice
    stands either way: enforcement changed later must not encrypt it, nor
    strip the key from a blueprint that already uses one.
    """
    if is_frozen(resource):
        enabled = bool(getattr(resource, "cmek_enabled", False))
    else:
        if not settings.offered:
            return None
        enabled = bool(getattr(resource, "cmek_enabled", False))
        if settings.required:
            enabled = True
    if not enabled:
        return None
    return {
        "key_name": getattr(resource, "cmek_key_name", "") or "",
        "key_project_id": getattr(resource, "cmek_key_project_id", "") or "",
        "key_ring_name": getattr(resource, "cmek_key_ring_name", "") or "",
        "deletion_policy": getattr(resource, "cmek_deletion_policy", "") or "",
        "protection_level": getattr(resource, "cmek_protection_level", "") or "",
        "rotation_period": getattr(resource, "cmek_rotation_period", "") or "",
        "encryption_service_account": getattr(
            resource, "cmek_encryption_service_account", "") or "",
    }


def describe(resource, settings: CmekSettings) -> Optional[dict]:
    """What to show on a resource's detail page, or None when unencrypted.

    Reports only what OFE decided. Where OFE leaves a kms-key setting unset the
    module's own default is named as such, rather than printed as though OFE had
    chosen it. The live key state is deliberately absent: it would need a Cloud
    KMS call the modules already make unnecessary.
    """
    chosen = resource_settings(resource, settings)
    if not chosen:
        return {"encrypted": False}

    out = {
        "encrypted": True,
        "how": "Customer-managed key, created for this resource",
        "encryption_service_account": chosen["encryption_service_account"],
        # Blank means OFE did not set it, so the value is kms-key's default.
        "protection_level": (chosen["protection_level"]
                             or f"{PROTECTION_SOFTWARE} (module default)"),
        "rotation_period": (chosen["rotation_period"]
                            or "7776000s (module default)"),
        "key_path": "",
    }

    name = chosen["key_name"]
    ring = chosen["key_ring_name"] or settings.key_ring_name
    project = (chosen["key_project_id"] or settings.key_project_id
               or getattr(resource, "project_id", "") or "")
    location = normalize_location(
        getattr(resource, "cloud_zone", None)
        or getattr(resource, "cloud_region", None) or "")
    out["key_name"] = name
    out["key_ring_name"] = ring
    out["key_project_id"] = project
    out["teardown"] = (
        "The key is destroyed when this resource is torn down, and anything "
        "still encrypted with it becomes unreadable"
        if chosen["deletion_policy"] == DELETION_POLICY_DELETE
        else "The key is left in place when this resource is torn down")
    if name and ring and project and location:
        out["key_path"] = full_key_path(project, location, ring, name)
    return out
