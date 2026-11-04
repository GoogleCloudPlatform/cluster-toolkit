# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Identity from an Identity-Aware Proxy assertion, for the TCP listener.

A load balancer cannot reach a unix socket, and a local relay onto it would be
seen by the kernel as the relay's account rather than the user's. So a desktop
published through an HTTPS load balancer gets a second listener, and on it the
caller is whoever IAP says they are: IAP signs an assertion onto every request
it forwards, in "x-goog-iap-jwt-assertion". As on the socket, there is no
shared secret and no header taken on trust.

The audience is the security boundary. It names the backend service the
assertion was minted for, so without it an assertion for any other
IAP-protected service would verify here. It contains that backend service's
numeric ID, which does not exist until the backend service does, so it is
looked up by name through the Compute API at first use rather than passed in.

Imported only when the TCP listener is configured: it needs google-auth and
cryptography, which only such a broker installs.
"""

import base64
import json
import logging
import time
import urllib.error
import urllib.parse
import urllib.request

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec
from google.auth import jwt as google_jwt

from .errors import BrokerError

LOG = logging.getLogger("ghpc-desktop-broker")

HEADER = "x-goog-iap-jwt-assertion"
ISSUER = "https://cloud.google.com/iap"
# IAP signs with ES256 and publishes its keys as a JWK set, separately from
# Google's OIDC certificates.
CERTS_URL = "https://www.gstatic.com/iap/verify/public_key-jwk"
CERTS_CACHE_SECONDS = 3600
ALGORITHM = "ES256"
# An unknown key ID refetches the keys at most this often: anyone who can reach
# the port can send one.
CERTS_MIN_REFRESH_SECONDS = 60
# Leeway on "iat" and "exp" for clocks that differ from IAP's.
CLOCK_SKEW_SECONDS = 60

METADATA_ROOT = "http://metadata.google.internal/computeMetadata/v1"
BACKEND_SERVICE_URL = (
    "https://compute.googleapis.com/compute/v1/projects/{project}"
    "/global/backendServices/{name}"
)
AUDIENCE = "/projects/{project_number}/global/backendServices/{backend_id}"


def _read_json(request, what):
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as err:
        raise BrokerError(502, f"{what} failed with HTTP {err.code}.") from err
    except (urllib.error.URLError, OSError, ValueError) as err:
        raise BrokerError(502, f"{what} failed: {err}") from err


def _metadata(path):
    request = urllib.request.Request(
        f"{METADATA_ROOT}/{path}", headers={"Metadata-Flavor": "Google"}
    )
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            return response.read().decode("utf-8").strip()
    except (urllib.error.URLError, OSError) as err:
        raise BrokerError(502, f"Metadata lookup for {path} failed: {err}") from err


def _ec_public_key_pem(jwk):
    """An ES256 JWK as the PEM key google.auth.jwt.decode expects."""
    if jwk.get("kty") != "EC" or jwk.get("crv") != "P-256":
        raise ValueError(f"unsupported key {jwk.get('kid')!r}")

    def coordinate(value):
        return int.from_bytes(
            base64.urlsafe_b64decode(value + "=" * (-len(value) % 4)), "big"
        )

    key = ec.EllipticCurvePublicNumbers(
        coordinate(jwk["x"]), coordinate(jwk["y"]), ec.SECP256R1()
    ).public_key()
    return key.public_bytes(
        serialization.Encoding.PEM,
        serialization.PublicFormat.SubjectPublicKeyInfo,
    )


class Verifier:
    """Verifies IAP assertions for one backend service.

    The audience and IAP's signing keys are both cached: the backend service's
    ID never changes, and the keys rotate slowly enough that refetching for an
    unknown key ID, at most once a minute, is enough.
    """

    def __init__(self, backend_service, certs_url=CERTS_URL):
        self.backend_service = backend_service
        self.certs_url = certs_url
        self._audience = None
        self._certs = {}
        self._certs_expire_at = 0
        self._certs_fetched_at = 0

    def audience(self):
        # Not cached on failure, so a broker that starts before its backend
        # service exists recovers on the first request after it does.
        if self._audience is None:
            self._audience = self._lookup_audience()
            LOG.info("IAP audience for this broker is %s", self._audience)
        return self._audience

    def _lookup_audience(self):
        token = json.loads(_metadata("instance/service-accounts/default/token"))
        request = urllib.request.Request(
            BACKEND_SERVICE_URL.format(
                project=urllib.parse.quote(_metadata("project/project-id")),
                name=urllib.parse.quote(self.backend_service),
            ),
            headers={"Authorization": f"Bearer {token['access_token']}"},
        )
        backend = _read_json(
            request,
            f"Looking up backend service {self.backend_service!r} (the instance "
            "service account needs compute.backendServices.get)",
        )
        backend_id = str(backend.get("id") or "").strip()
        if not backend_id:
            raise BrokerError(
                502, f"Backend service {self.backend_service!r} returned no id."
            )
        return AUDIENCE.format(
            project_number=_metadata("project/numeric-project-id"),
            backend_id=backend_id,
        )

    def _keys(self, refresh=False):
        now = time.time()
        if refresh and now - self._certs_fetched_at < CERTS_MIN_REFRESH_SECONDS:
            refresh = False
        if refresh or not self._certs or now >= self._certs_expire_at:
            payload = _read_json(
                urllib.request.Request(self.certs_url), "Fetching IAP signing keys"
            )
            try:
                self._certs = {
                    jwk["kid"]: _ec_public_key_pem(jwk) for jwk in payload["keys"]
                }
            except (KeyError, TypeError, ValueError) as err:
                raise BrokerError(502, "Could not read IAP signing keys.") from err
            self._certs_fetched_at = now
            self._certs_expire_at = now + CERTS_CACHE_SECONDS
        return self._certs

    def subject(self, headers):
        """The numeric Google account ID the request was made by.

        That, not the email, is what OS Login profiles are keyed on.
        """
        assertion = str(headers.get(HEADER) or "").strip()
        if not assertion:
            raise BrokerError(
                401,
                "Missing IAP assertion. This port is only reachable through "
                "Identity-Aware Proxy.",
            )
        try:
            header = google_jwt.decode_header(assertion)
        except ValueError as err:
            raise BrokerError(401, "Malformed IAP assertion.") from err
        if not isinstance(header, dict):
            raise BrokerError(401, "Malformed IAP assertion.")
        # google-auth picks the verifier from the token's own header, and an
        # RS256 one fails on IAP's EC keys with a TypeError rather than a
        # refusal. IAP only signs with ES256.
        if header.get("alg") != ALGORITHM:
            raise BrokerError(401, "IAP assertion has an unexpected algorithm.")
        key_id = header.get("kid")

        certs = self._keys()
        if key_id not in certs:
            certs = self._keys(refresh=True)
        try:
            # Checks the signature, "aud", "exp" and "iat", but not "iss".
            claims = google_jwt.decode(
                assertion,
                certs=certs,
                audience=self.audience(),
                clock_skew_in_seconds=CLOCK_SKEW_SECONDS,
            )
        except ValueError as err:
            LOG.warning("Rejected IAP assertion: %s", err)
            raise BrokerError(401, "IAP assertion could not be verified.") from err

        if claims.get("iss") != ISSUER:
            raise BrokerError(401, "IAP assertion has an unexpected issuer.")
        # "accounts.google.com:1234"; OS Login keys profiles on the bare number.
        subject = str(claims.get("sub") or "").rsplit(":", 1)[-1]
        if not subject.isdigit():
            raise BrokerError(401, "IAP assertion names no Google account.")
        return subject
