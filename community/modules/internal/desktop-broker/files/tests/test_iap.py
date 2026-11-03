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

"""Verifying IAP assertions on the TCP listener.

Assertions are signed here with a real ES256 key, so what is tested is the
verification itself: anything that is not a current assertion from IAP for
this backend service must be refused.
"""

import base64
import time
import types

import pytest

pytest.importorskip("cryptography")
pytest.importorskip("google.auth")

from cryptography.hazmat.primitives import serialization  # noqa: E402
from cryptography.hazmat.primitives.asymmetric import ec  # noqa: E402
from google.auth import jwt as google_jwt  # noqa: E402
from google.auth.crypt import es256  # noqa: E402

from desktop_broker import iap  # noqa: E402
from desktop_broker.errors import BrokerError  # noqa: E402

AUDIENCE = "/projects/123/global/backendServices/456"
SUBJECT = "112605920660124105986"


def _b64(number):
    return base64.urlsafe_b64encode(number.to_bytes(32, "big")).rstrip(b"=").decode()


class Key:
    def __init__(self, kid):
        self.kid = kid
        self.private = ec.generate_private_key(ec.SECP256R1())
        pem = self.private.private_bytes(
            serialization.Encoding.PEM,
            serialization.PrivateFormat.PKCS8,
            serialization.NoEncryption(),
        )
        self.signer = es256.ES256Signer.from_string(pem, key_id=kid)

    def jwk(self):
        numbers = self.private.public_key().public_numbers()
        return {"kty": "EC", "crv": "P-256", "kid": self.kid,
                "x": _b64(numbers.x), "y": _b64(numbers.y)}

    def sign(self, **overrides):
        now = int(time.time())
        claims = {"iss": iap.ISSUER, "aud": AUDIENCE,
                  "sub": f"accounts.google.com:{SUBJECT}",
                  "email": "alice@example.com", "iat": now, "exp": now + 600}
        claims.update(overrides)
        return google_jwt.encode(self.signer, claims).decode()


@pytest.fixture
def key():
    return Key("current")


@pytest.fixture
def published(monkeypatch, key):
    """The JWK set IAP publishes; tests may change it to simulate rotation."""
    keys = [key.jwk()]
    fetches = []

    def read_json(request, what):
        fetches.append(request.full_url)
        return {"keys": list(keys)}

    monkeypatch.setattr(iap, "_read_json", read_json)
    return types.SimpleNamespace(keys=keys, fetches=fetches)


@pytest.fixture
def verifier(published):
    verifier = iap.Verifier("c1-login")
    verifier._audience = AUDIENCE
    return verifier


def headers(assertion):
    return {iap.HEADER: assertion}


def test_a_valid_assertion_names_the_numeric_account(verifier, key):
    assert verifier.subject(headers(key.sign())) == SUBJECT


@pytest.mark.parametrize(
    "overrides",
    [
        {"aud": "/projects/123/global/backendServices/999"},
        {"iss": "https://accounts.google.com"},
        {"exp": int(time.time()) - 3600, "iat": int(time.time()) - 4200},
        {"sub": "accounts.google.com:"},
    ],
    ids=["other-backend", "other-issuer", "expired", "no-subject"],
)
def test_anything_but_a_current_assertion_for_this_backend_is_refused(
    verifier, key, overrides
):
    with pytest.raises(BrokerError) as excinfo:
        verifier.subject(headers(key.sign(**overrides)))
    assert excinfo.value.status == 401


def test_a_key_iap_does_not_publish_is_refused(verifier):
    with pytest.raises(BrokerError):
        verifier.subject(headers(Key("forged").sign()))


@pytest.mark.parametrize("assertion", ["", "not-a-jwt"], ids=["missing", "garbage"])
def test_a_missing_or_malformed_assertion_is_refused(verifier, assertion):
    with pytest.raises(BrokerError) as excinfo:
        verifier.subject(headers(assertion))
    assert excinfo.value.status == 401


def test_keys_are_cached_and_refetched_once_on_rotation(verifier, key, published):
    verifier.subject(headers(key.sign()))
    verifier.subject(headers(key.sign()))
    assert len(published.fetches) == 1

    rotated = Key("rotated")
    published.keys.append(rotated.jwk())
    assert verifier.subject(headers(rotated.sign())) == SUBJECT
    assert len(published.fetches) == 2


def test_the_audience_is_looked_up_from_the_backend_service(monkeypatch):
    metadata = {
        "instance/service-accounts/default/token": '{"access_token": "t"}',
        "project/project-id": "my-project",
        "project/numeric-project-id": "123",
    }
    asked = []

    def read_json(request, what):
        asked.append(request.full_url)
        return {"id": "456"}

    monkeypatch.setattr(iap, "_metadata", metadata.__getitem__)
    monkeypatch.setattr(iap, "_read_json", read_json)
    verifier = iap.Verifier("c1-login")
    assert verifier.audience() == AUDIENCE
    assert verifier.audience() == AUDIENCE
    assert asked == [
        "https://compute.googleapis.com/compute/v1/projects/my-project"
        "/global/backendServices/c1-login"
    ]


def test_a_failed_audience_lookup_is_retried(monkeypatch):
    # The broker can start before its backend service exists.
    monkeypatch.setattr(iap, "_metadata", lambda path: '{"access_token": "t"}')

    def missing(request, what):
        raise BrokerError(502, "not found")

    monkeypatch.setattr(iap, "_read_json", missing)
    verifier = iap.Verifier("c1-login")
    with pytest.raises(BrokerError):
        verifier.audience()
    monkeypatch.setattr(iap, "_read_json", lambda request, what: {"id": "456"})
    monkeypatch.setattr(iap, "_metadata", lambda path: {
        "project/numeric-project-id": "123"}.get(path, '{"access_token": "t"}'))
    assert verifier.audience() == AUDIENCE
