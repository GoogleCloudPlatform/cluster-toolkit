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

"""Joining a Google identity to a POSIX account.

This is the authorisation decision, so the indexing and the lookup order are
both load-bearing.
"""

import pytest

from desktop_broker import oslogin
from desktop_broker.errors import BrokerError

PROFILES = {
    "loginProfiles": [
        {
            "name": "117253417745780664166",
            "posixAccounts": [
                {
                    "username": "alice_example_com",
                    "uid": 1001,
                    "homeDirectory": "/home/alice_example_com",
                    "operatingSystemType": "LINUX",
                    "primary": True,
                },
                {
                    "username": "alice_secondary",
                    "uid": 1002,
                    "operatingSystemType": "LINUX",
                },
            ],
        },
        {
            "name": "220000000000000000001",
            "posixAccounts": [
                {"username": "bob_example_com", "uid": 1003}
            ],
        },
        # No POSIX accounts at all: must not appear in either index.
        {"name": "330000000000000000002", "posixAccounts": []},
        # Windows-only account: not usable for a Linux desktop.
        {
            "name": "440000000000000000003",
            "posixAccounts": [
                {"username": "win_user", "operatingSystemType": "WINDOWS"}
            ],
        },
    ]
}


class FakeDirectory(oslogin.Directory):
    def __init__(self, payload=None, **kwargs):
        super().__init__(**kwargs)
        self.payload = payload if payload is not None else PROFILES
        self.fetches = 0

    def _fetch(self):
        self.fetches += 1
        return self.payload


def test_profiles_index_by_subject_and_username():
    cache = FakeDirectory().load()
    assert "117253417745780664166" in cache["by_subject"]
    assert "alice_example_com" in cache["by_username"]


def test_primary_account_wins():
    account = FakeDirectory().resolve("117253417745780664166")
    assert account["username"] == "alice_example_com"


def test_first_account_used_when_none_is_primary():
    account = FakeDirectory().resolve("220000000000000000001")
    assert account["username"] == "bob_example_com"


def test_profiles_without_posix_accounts_are_skipped():
    cache = FakeDirectory().load()
    assert "330000000000000000002" not in cache["by_subject"]


def test_non_linux_accounts_are_skipped():
    cache = FakeDirectory().load()
    assert "win_user" not in cache["by_username"]


def test_subject_is_preferred_over_a_username_hint():
    # The subject is authoritative; the hint is only as trustworthy as the mode
    # that supplied it.
    account = FakeDirectory().resolve(
        "117253417745780664166", username_hint="bob_example_com"
    )
    assert account["username"] == "alice_example_com"


def test_username_hint_used_when_no_subject():
    account = FakeDirectory().resolve("", username_hint="bob_example_com")
    assert account["username"] == "bob_example_com"


def test_no_identity_at_all_is_rejected():
    with pytest.raises(BrokerError, match="Missing desktop OS Login identity"):
        FakeDirectory().resolve("", None)


def test_unknown_identity_is_rejected():
    with pytest.raises(BrokerError, match="No OS Login profile"):
        FakeDirectory().resolve("999", username_hint="nobody")


def test_lookup_refreshes_once_before_giving_up():
    """A just-granted account should not have to wait out the cache."""
    directory = FakeDirectory()
    with pytest.raises(BrokerError):
        directory.resolve("999")
    assert directory.fetches == 2


def test_results_are_cached_between_lookups():
    directory = FakeDirectory()
    directory.resolve("117253417745780664166")
    directory.resolve("117253417745780664166")
    assert directory.fetches == 1


def test_cache_expiry_forces_a_refetch():
    directory = FakeDirectory(cache_seconds=-1)
    directory.resolve("117253417745780664166")
    directory.resolve("117253417745780664166")
    assert directory.fetches == 2
