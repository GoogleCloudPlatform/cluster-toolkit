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

"""Identity resolution.

Only trusted_proxy exists so far: the shared secret is checked, then the
identity is taken from headers. What is tested is the secret comparison and
which inputs are allowed to influence the answer.
"""

import pytest
from conftest import base_config

from desktop_broker.identity import resolver as identity
from desktop_broker.config import Config
from desktop_broker.errors import BrokerError
from desktop_broker.identity import common, trusted_proxy


def presented(**overrides):
    values = {key: "" for key in identity.HEADERS}
    values.update(overrides)
    return values


def test_headers_are_read_case_insensitively():
    # aiohttp gives a case-insensitive mapping; a plain dict here proves we
    # look up the documented names rather than lower-casing by accident.
    extracted = identity.presented_from_headers(
        {"X-Cluster-Desktop-Email": " user@example.com "}
    )
    assert extracted["email"] == "user@example.com"


def test_default_oslogin_username_transform():
    assert (
        common.default_oslogin_username("user.name@example.com")
        == "user_name_example_com"
    )


def test_wrong_secret_is_rejected_before_any_verification(tmp_path):
    resolver = identity.Resolver(Config(base_config(tmp_path)))
    with pytest.raises(BrokerError, match="proxy secret") as excinfo:
        resolver.resolve(presented(secret="wrong", email="a@b.com"))
    assert excinfo.value.status == 403


def test_correct_secret_reaches_the_configured_mode(tmp_path):
    resolver = identity.Resolver(Config(base_config(tmp_path)))
    resolved = resolver.resolve(
        presented(secret="test-proxy-secret", email="User@Example.com")
    )
    assert resolved["email"] == "user@example.com"


# -- trusted_proxy --------------------------------------------------------


def test_trusted_proxy_requires_an_email(tmp_path):
    config = Config(base_config(tmp_path))
    with pytest.raises(BrokerError, match="identity header"):
        trusted_proxy.resolve(presented(), config)


def test_trusted_proxy_prefers_an_explicit_username(tmp_path):
    config = Config(base_config(tmp_path))
    resolved = trusted_proxy.resolve(
        presented(email="a.b@example.com", username="custom_name"), config
    )
    # Deriving from the email is only correct where the OS Login username
    # format is untouched, so an explicit header must win.
    assert resolved["username_hint"] == "custom_name"


def test_trusted_proxy_derives_a_username_when_absent(tmp_path):
    config = Config(base_config(tmp_path))
    resolved = trusted_proxy.resolve(presented(email="a.b@example.com"), config)
    assert resolved["username_hint"] == "a_b_example_com"
