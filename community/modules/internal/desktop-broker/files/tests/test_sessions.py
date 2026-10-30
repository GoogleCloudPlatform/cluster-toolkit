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
    assert stat.S_IMODE(os.stat(store.state_dir).st_mode) == 0o700


def test_records_round_trip(store):
    store.save({"username": "alice", "slot": 0, "uid": 1001})
    assert store.load("alice")["uid"] == 1001


def test_record_files_are_not_group_readable(store):
    store.save({"username": "a", "slot": 0})
    mode = stat.S_IMODE(os.stat(store.path_for("a")).st_mode)
    assert mode == 0o600


def test_missing_record_is_none(store):
    assert store.load("nobody") is None


@pytest.mark.parametrize("contents", ["{not json", "[]", "null"], ids=["corrupt", "list", "null"])
def test_a_file_that_is_not_a_record_is_skipped(store, contents):
    store.save({"username": "b", "slot": 1})
    store.path_for("a").write_text(contents, encoding="utf-8")
    assert store.load("a") is None
    assert [record["username"] for record in store.list()] == ["b"]


def test_delete_is_idempotent(store):
    store.delete("never")


def test_slots_are_allocated_lowest_first(store):
    assert store.allocate_slot("a") == 0
    store.save({"username": "a", "slot": 0})
    assert store.allocate_slot("c") == 1


def test_a_user_does_not_block_their_own_slot(store):
    store.save({"username": "a", "slot": 2})
    # Reconnecting must not consume a second slot.
    assert store.allocate_slot("a") == 0


def test_gaps_are_reused(store):
    for slot, username in enumerate(["a", "c", "e"]):
        store.save({"username": username, "slot": slot})
    store.delete("c")
    assert store.allocate_slot("newcomer") == 1


def test_capacity_is_enforced(store):
    for slot in range(4):
        store.save({"username": f"u{slot}", "slot": slot})
    with pytest.raises(BrokerError, match="user-session capacity") as excinfo:
        store.allocate_slot("one_too_many")
    assert excinfo.value.status == 503


def test_touch_refreshes_last_accessed(store):
    store.save({"username": "a", "slot": 0, "last_accessed": 1})
    store.touch("a")
    assert store.load("a")["last_accessed"] > 1


def test_touch_does_not_resurrect_a_deleted_record(store):
    store.touch("gone")
    assert store.load("gone") is None
