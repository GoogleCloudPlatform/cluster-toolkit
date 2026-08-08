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

"""Customer-managed encryption key (CMEK) helpers for OFE.

A CMEK key is chosen per resource and stored on that resource's model.
This module only parses and checks the chosen key name, and turns it
into the blueprint template context; Cloud KMS owns everything else.

Key selection is deliberately not a policy engine. Enforcement is an
organization policy (constraints/gcp.restrictNonCmekServices), key
rotation is a Cloud KMS setting, and which key protects a resource is
readable from that resource's own API. Duplicating any of that here
could only ever agree with Google or be wrong.

Granting the key to the service agents that encrypt with it is done
declaratively by the blueprint, via the kms-key-iam Cluster Toolkit
module, not by OFE at runtime.
"""

import dataclasses
import re

CRYPTO_KEY_RE = re.compile(
    r"^projects/(?P<project>[^/]+)"
    r"/locations/(?P<location>[^/]+)"
    r"/keyRings/(?P<key_ring>[^/]+)"
    r"/cryptoKeys/(?P<key>[^/]+)$"
)

ENCRYPTER_DECRYPTER_ROLE = "roles/cloudkms.cryptoKeyEncrypterDecrypter"

_ZONE_RE = re.compile(r"^[a-z]+-[a-z0-9]+\d-[a-z]$")  # e.g. us-central1-a


class CmekError(Exception):
    """Base class for CMEK failures."""

    def __init__(self, message, *, key_name=None, remediation=None):
        super().__init__(message)
        self.key_name = key_name
        self.remediation = remediation

    def __str__(self):
        parts = [super().__str__()]
        if self.key_name:
            parts.append(f"key: {self.key_name}")
        if self.remediation:
            parts.append(self.remediation)
        return ". ".join(parts)


class CmekConfigError(CmekError):
    """The supplied key name is malformed or unusable for the resource."""


@dataclasses.dataclass(frozen=True)
class CryptoKeyName:
    """Parsed form of a full CryptoKey resource name."""

    project: str
    location: str
    key_ring: str
    key: str

    def __str__(self):
        return (
            f"projects/{self.project}/locations/{self.location}"
            f"/keyRings/{self.key_ring}/cryptoKeys/{self.key}"
        )


def parse_crypto_key_name(name) -> CryptoKeyName:
    """Parse `projects/P/locations/L/keyRings/R/cryptoKeys/K`.

    Raises CmekConfigError for anything else, including bare key IDs,
    key-version names, and names with trailing components.
    """
    if not isinstance(name, str):
        raise CmekConfigError(
            f"CryptoKey name must be a string, got {type(name).__name__}",
            key_name=name,
            remediation="Supply the full CryptoKey resource name.",
        )
    match = CRYPTO_KEY_RE.match(name.strip())
    if not match:
        raise CmekConfigError(
            f"Not a full CryptoKey resource name: {name!r}",
            key_name=name,
            remediation=(
                "Use the form projects/KEY_PROJECT/locations/LOCATION/"
                "keyRings/KEY_RING/cryptoKeys/KEY (no key version suffix)."
            ),
        )
    return CryptoKeyName(
        project=match.group("project"),
        location=match.group("location"),
        key_ring=match.group("key_ring"),
        key=match.group("key"),
    )


def normalize_location(location) -> str:
    """Normalize a resource location: zones collapse to their region.

    `us-central1-a` -> `us-central1`; regions and multi-regions are
    returned lower-cased and otherwise unchanged.
    """
    loc = location.strip().lower()
    if _ZONE_RE.match(loc):
        return loc.rsplit("-", 1)[0]
    return loc


def check_key_location(key_name, location) -> CryptoKeyName:
    """Validate `key_name` and check it can serve a resource in `location`.

    Cloud KMS refuses to encrypt a resource with a key from a different
    region, and does so only once the resource is being created. Checking
    here turns a late, opaque apply failure into an immediate, specific
    one. A `global` key serves any location.
    """
    parsed = parse_crypto_key_name(key_name)
    key_location = parsed.location.lower()
    if key_location == "global":
        return parsed
    resource_location = normalize_location(location)
    if key_location != resource_location:
        raise CmekConfigError(
            f"CryptoKey is in {parsed.location} but the resource is in "
            f"{resource_location}",
            key_name=str(parsed),
            remediation=(
                f"Use a key in {resource_location} (or a global key). A "
                "Cloud KMS key cannot encrypt a resource in another region."
            ),
        )
    return parsed


# Blueprint module ids for the two modules a CMEK-enabled blueprint emits:
# one to look the key up, one to grant on it.
KEY_MODULE_ID = "cmek_key"
IAM_MODULE_ID = "cmek_key_iam"


def blueprint_context(key_name, *, region, zone, service_agents=()):
    """the `cmek` template context for a blueprint.

    Returns None when there is no key, so the blueprint renders without
    any CMEK settings.

    The key values are **blueprint references to the kms-key-iam module**,
    not the key name itself. That is what makes the encrypted resources
    depend on the grants: gcluster turns `$(cmek_key_iam.disk_encryption_key)`
    into a Terraform reference, so nothing is created before its service
    agent can use the key, and on destroy the resource goes before the
    grant. Writing the key name in directly would encrypt the resources
    just the same and fail intermittently, because the grants would be
    unordered with respect to them.

    The parsed key parts are returned alongside, for the
    pre-existing-kms-key module block that resolves the key.

    `disk_encryption_key_service_account` is deliberately never set. With
    it unset, Compute Engine performs the encryption request as the
    project's Compute Engine service agent -- a stable identity that
    already exists. Naming the per-cluster service account instead would
    mean granting a service account that this same blueprint creates,
    which is why doing so previously required applying the blueprint in
    two phases with an IAM grant wedged between them.
    """
    if not key_name:
        return None
    # the Slurm configuration bucket is regional and the disks are
    # zonal, so one key has to serve both; check the stricter of the two.
    parsed = check_key_location(key_name, zone or region)

    def ref(output):
        return f"$({IAM_MODULE_ID}.{output})"

    return {
        # for the pre-existing-kms-key module block
        "key_project": parsed.project,
        "key_location": parsed.location,
        "key_ring": parsed.key_ring,
        "key_name": parsed.key,
        "service_agents": sorted(service_agents),
        # references, so consumers are ordered behind the grants
        "disk_key": ref("disk_encryption_key"),
        "bucket_key": ref("slurm_bucket_kms_key"),
        "artifact_registry_key": ref("crypto_key_id"),
        "secret_key": ref("crypto_key_id"),
        "cloudsql_key": ref("crypto_key_id"),
    }
