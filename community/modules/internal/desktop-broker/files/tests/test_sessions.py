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

"""Session records and slot allocation."""

import os
import stat

import pytest

from desktop_broker.errors import BrokerError
from desktop_broker.sessions.store import SessionStore


@pytest.fixture
def store(tmp_path):
    return SessionStore(tmp_path / "state", max_user_sessions=4)


def test_state_directory_is_root_only(store):
    mode = stat.S_IMODE(os.stat(store.state_dir).st_mode)
    # Records name uids and, on the tcp transport, RFB passwords.
    assert mode == 0o700


def test_records_round_trip(store):
    store.save({"email": "a@b.com", "slot": 0, "username": "a_b_com"})
    assert store.load("a@b.com")["username"] == "a_b_com"


def test_record_files_are_not_group_readable(store):
    store.save({"email": "a@b.com", "slot": 0})
    mode = stat.S_IMODE(os.stat(store.path_for("a@b.com")).st_mode)
    assert mode == 0o600


def test_email_lookup_is_case_insensitive(store):
    store.save({"email": "User@Example.com", "slot": 0})
    assert store.path_for("user@example.com") == store.path_for("User@Example.com")


def test_missing_record_is_none(store):
    assert store.load("nobody@example.com") is None


def test_corrupt_record_does_not_raise(store):
    path = store.path_for("a@b.com")
    path.write_text("{not json", encoding="utf-8")
    assert store.load("a@b.com") is None


def test_delete_is_idempotent(store):
    store.delete("never@existed.com")


def test_slots_are_allocated_lowest_first(store):
    assert store.allocate_slot("a@b.com") == 0
    store.save({"email": "a@b.com", "slot": 0})
    assert store.allocate_slot("c@d.com") == 1


def test_a_user_does_not_block_their_own_slot(store):
    store.save({"email": "a@b.com", "slot": 2})
    # Reconnecting must not consume a second slot.
    assert store.allocate_slot("a@b.com") == 0


def test_gaps_are_reused(store):
    for slot, email in enumerate(["a@b.com", "c@d.com", "e@f.com"]):
        store.save({"email": email, "slot": slot})
    store.delete("c@d.com")
    assert store.allocate_slot("new@example.com") == 1


def test_capacity_is_enforced(store):
    for slot in range(4):
        store.save({"email": f"u{slot}@example.com", "slot": slot})
    with pytest.raises(BrokerError, match="user-session capacity") as excinfo:
        store.allocate_slot("one.too.many@example.com")
    assert excinfo.value.status == 503
