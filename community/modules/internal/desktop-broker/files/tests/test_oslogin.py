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

"""Asking OS Login whether an account may have a desktop.

This is the authorisation decision, so what is tested is that both questions
are asked, that only a grant is cached, and that failures fail closed.
"""

import time

import pytest

from desktop_broker import oslogin
from desktop_broker.errors import BrokerError

SUBJECT = "112605920660124105986"


def profile(username="alice_example_com", os_type="LINUX", **extra):
    account = {"username": username, "uid": "1001", "primary": True,
               "homeDirectory": f"/home/{username}", **extra}
    if os_type is not None:
        account["operatingSystemType"] = os_type
    return {"loginProfiles": [{"name": SUBJECT, "posixAccounts": [account]}]}


class FakeDirectory(oslogin.Directory):
    """Answers from scripted metadata responses, and records what was asked."""

    def __init__(self, users=None, authorized=True, **kwargs):
        super().__init__(**kwargs)
        self.users = users if users is not None else profile()
        self.authorized = authorized
        self.asked = []

    def _get(self, path, params):
        self.asked.append((path, dict(params)))
        if path == "users":
            return self.users
        if self.authorized is None:
            return None
        return {"success": self.authorized}


def test_a_granted_account_resolves_after_both_questions():
    directory = FakeDirectory()
    account = directory.resolve("alice_example_com")
    assert account["homeDirectory"] == "/home/alice_example_com"
    assert directory.asked == [
        ("users", {"username": "alice_example_com"}),
        ("authorize", {"email": SUBJECT, "policy": "login"}),
    ]


def test_an_unknown_account_is_refused():
    directory = FakeDirectory(users=None)
    with pytest.raises(oslogin.ProfileNotFound) as excinfo:
        directory.resolve("nobody")
    assert excinfo.value.status == 403


@pytest.mark.parametrize("authorized", [False, None], ids=["denied", "404"])
def test_a_profile_without_login_access_is_refused(authorized):
    # The profile outlives the IAM grant, so it alone must not be enough.
    directory = FakeDirectory(authorized=authorized)
    with pytest.raises(oslogin.ProfileNotFound, match="does not allow"):
        directory.resolve("alice_example_com")


@pytest.mark.parametrize(
    "users",
    [profile(os_type="WINDOWS"), profile(username="mallory")],
    ids=["windows-only", "another-account"],
)
def test_only_a_matching_linux_account_is_accepted(users):
    with pytest.raises(oslogin.ProfileNotFound):
        FakeDirectory(users=users).resolve("alice_example_com")


def test_an_account_with_no_os_type_counts_as_linux():
    directory = FakeDirectory(users=profile(os_type=None))
    assert directory.resolve("alice_example_com")["username"] == "alice_example_com"


def test_a_grant_is_cached():
    directory = FakeDirectory()
    directory.resolve("alice_example_com")
    directory.resolve("alice_example_com")
    assert len(directory.asked) == 2


def test_revocation_takes_effect_once_the_cache_expires():
    directory = FakeDirectory(cache_seconds=0)
    directory.resolve("alice_example_com")
    directory.authorized = False
    time.sleep(0.01)
    with pytest.raises(oslogin.ProfileNotFound):
        directory.resolve("alice_example_com")


def test_a_refusal_is_not_cached():
    # So a user who has just been granted access need not wait.
    directory = FakeDirectory(authorized=False)
    with pytest.raises(oslogin.ProfileNotFound):
        directory.resolve("alice_example_com")
    directory.authorized = True
    assert directory.resolve("alice_example_com")["username"] == "alice_example_com"


class _Response:
    def __init__(self, body):
        self.body = body

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def read(self):
        if isinstance(self.body, Exception):
            raise self.body
        return self.body


def serve(monkeypatch, body=None, error=None):
    def urlopen(request, timeout):
        if error:
            raise error
        return _Response(body)

    monkeypatch.setattr(oslogin.urllib.request, "urlopen", urlopen)


def http_error(code):
    return oslogin.urllib.error.HTTPError("http://x", code, "", {}, None)


def test_a_404_is_an_absent_answer_not_an_error(monkeypatch):
    serve(monkeypatch, error=http_error(404))
    assert oslogin.Directory()._get("users", {"username": "nobody"}) is None


@pytest.mark.parametrize("code", [403, 500, 503])
def test_other_http_failures_are_a_clean_502(monkeypatch, code):
    serve(monkeypatch, error=http_error(code))
    with pytest.raises(BrokerError) as excinfo:
        oslogin.Directory()._get("users", {"username": "alice"})
    assert excinfo.value.status == 502


@pytest.mark.parametrize(
    "body",
    [
        b"<html>502 Bad Gateway</html>",
        b'{"loginProfiles": [',
        b"\xff\xfe not utf-8",
        TimeoutError("timed out"),
    ],
    ids=["html", "truncated", "not-utf8", "read-timeout"],
)
def test_an_unreadable_metadata_response_is_a_clean_502(monkeypatch, body):
    serve(monkeypatch, body=body)
    with pytest.raises(BrokerError) as excinfo:
        oslogin.Directory()._get("users", {"username": "alice"})
    assert excinfo.value.status == 502
