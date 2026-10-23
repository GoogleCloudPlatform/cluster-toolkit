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

"""Tests of lldap_sync.apply() against an in-memory fake LLDAP.

HTTP, podman and Secret Manager calls are replaced; no network is used.
"""

import copy
import importlib
import logging
import os
import sys

import pytest

FILES_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if FILES_DIR not in sys.path:
    sys.path.insert(0, FILES_DIR)

BASE_CFG = {
    "access_group": "cluster_users",
    "access_group_gid": 10000,
    "cleanup": False,
    "groups": {"hpc": {"gid": 20001}},
    "users": {
        "alice": {"uid": 30001, "gid": 20001, "groups": ["hpc", "cluster_users"],
                  "shell": "/bin/bash", "email": "alice@x.io", "display_name": "Alice",
                  "disabled": False, "secret": "sec-alice"},
        "bob": {"uid": 30002, "gid": 10000, "groups": [], "shell": "/bin/zsh",
                "email": "bob@noemail.invalid", "display_name": "bob",
                "disabled": True, "secret": "sec-bob"},
    },
}


class FakeLldap:
    """In-memory state of an LLDAP server."""

    def __init__(self):
        self.users = {
            "admin": {"email": "admin@x", "displayName": "", "attrs": {}, "groups": {"lldap_admin"}},
            "manual": {"email": "m@x", "displayName": "M", "attrs": {}, "groups": {"cluster_users"}},
            "otheradmin": {"email": "o@x", "displayName": "O", "attrs": {},
                           "groups": {"lldap_admin", "cluster_users"}},
        }
        self.groups = {1: {"name": "lldap_admin", "attrs": {}},
                       2: {"name": "lldap_password_manager", "attrs": {}},
                       3: {"name": "lldap_strict_readonly", "attrs": {}},
                       4: {"name": "cluster_users", "attrs": {}},
                       5: {"name": "stale", "attrs": {}}}
        self.next_id = 6
        self.schema = {"user": set(), "group": set()}
        self.passwords = {"admin": "adminpw"}
        self.password_sets = []
        self.secret_adds = []
        self.secrets_with_versions = set()
        self.secret_values = {}
        self.add_snapshots = []
        self.fail_membership = set()
        self.calls = []
        self.fail_create = set()

    def gid(self, name):
        return next(i for i, g in self.groups.items() if g["name"] == name)


@pytest.fixture(name="env")
def fixture_env(tmp_path, monkeypatch):
    for d in ("data", "secrets", "state/pending-passwords"):
        (tmp_path / d).mkdir(parents=True)
    (tmp_path / "secrets" / "admin_password").write_text("adminpw")
    (tmp_path / "secrets" / "bind_password").write_text("bindpw")
    monkeypatch.setenv("LLDAP_PROJECT", "p")
    monkeypatch.setenv("LLDAP_BASE_DIR", str(tmp_path))
    monkeypatch.setenv("LLDAP_BIND_SECRET", "sec-bind")
    import lldap_sync  # pylint: disable=import-outside-toplevel
    sync = importlib.reload(lldap_sync)
    fake = FakeLldap()

    class FakeApi(sync.Api):
        def gql(self, query, variables=None):
            raise AssertionError("unexpected raw GraphQL call")

        def schema(self):
            return {"userSchema": {"attributes": [{"name": n} for n in fake.schema["user"]]},
                    "groupSchema": {"attributes": [{"name": n} for n in fake.schema["group"]]}}

        def add_attribute(self, kind, name, attr_type):
            fake.schema[kind].add(name.lower())

        def groups(self):
            return [{"id": i, "displayName": g["name"],
                     "attributes": [{"name": k, "value": v} for k, v in g["attrs"].items()]}
                    for i, g in fake.groups.items()]

        def create_group(self, name, attributes):
            fake.groups[fake.next_id] = {
                "name": name, "attrs": {a["name"].lower(): a["value"] for a in attributes}}
            fake.next_id += 1

        def update_group(self, group_id, attributes):
            fake.groups[group_id]["attrs"].update({a["name"].lower(): a["value"] for a in attributes})

        def delete_group(self, group_id):
            fake.calls.append(("delete_group", fake.groups[group_id]["name"]))
            del fake.groups[group_id]

        def users(self):
            return [{"id": u, "email": d["email"], "displayName": d["displayName"],
                     "groups": [{"id": fake.gid(g), "displayName": g} for g in d["groups"]],
                     "attributes": [{"name": k, "value": v} for k, v in d["attrs"].items()]}
                    for u, d in fake.users.items()]

        def create_user(self, user):
            if user["id"] in fake.fail_create:
                raise RuntimeError("GraphQL error: simulated")
            assert user["id"] not in fake.users and user.get("email")
            fake.users[user["id"]] = {
                "email": user["email"], "displayName": user.get("displayName", ""), "groups": set(),
                "attrs": {a["name"].lower(): a["value"] for a in user.get("attributes", [])}}

        def update_user(self, user):
            d = fake.users[user["id"]]
            d["email"] = user["email"]
            d["displayName"] = user["displayName"]
            d["attrs"].update({a["name"].lower(): a["value"] for a in user["insertAttributes"]})
            fake.calls.append(("update_user", user["id"]))

        def delete_user(self, user_id):
            fake.calls.append(("delete_user", user_id))
            del fake.users[user_id]

        def add_member(self, user_id, group_id):
            if user_id in fake.fail_membership:
                raise RuntimeError("GraphQL error: simulated membership failure")
            fake.users[user_id]["groups"].add(fake.groups[group_id]["name"])

        def remove_member(self, user_id, group_id):
            fake.users[user_id]["groups"].discard(fake.groups[group_id]["name"])

    def set_password(token, username, password):
        fake.passwords[username] = password
        fake.password_sets.append(username)

    def secret_add(project, secret, payload):
        fake.secret_adds.append((secret, payload.decode()))
        fake.secrets_with_versions.add(secret)
        fake.secret_values[secret] = payload
        # State of LLDAP at the moment the secret is published.
        fake.add_snapshots.append((secret, copy.deepcopy(fake.users), dict(fake.passwords)))

    def secret_get(project, secret):
        if secret not in fake.secret_values:
            raise sync.lldap_gcp.NotFound(secret)
        return fake.secret_values[secret]

    monkeypatch.setattr(sync, "Api", FakeApi)
    monkeypatch.setattr(sync, "wait_for_lldap", lambda timeout=600: None)
    monkeypatch.setattr(sync, "login", lambda u, p: "tok" if fake.passwords.get(u) == p else None)
    monkeypatch.setattr(sync, "set_password", set_password)
    monkeypatch.setattr(sync.lldap_gcp, "secret_add", secret_add)
    monkeypatch.setattr(sync.lldap_gcp, "secret_get", secret_get)
    monkeypatch.setattr(sync.lldap_gcp, "secret_has_version",
                        lambda project, secret: secret in fake.secrets_with_versions)
    return sync, fake, tmp_path


def test_first_sync_creates_everything(env):
    sync, fake, base = env
    sync.apply(copy.deepcopy(BASE_CFG))
    assert fake.schema == {"user": {"uidnumber", "gidnumber", "homedirectory", "unixshell"},
                           "group": {"gidnumber"}}
    assert fake.groups[fake.gid("hpc")]["attrs"]["gidnumber"] == ["20001"]
    assert fake.groups[fake.gid("cluster_users")]["attrs"]["gidnumber"] == ["10000"]
    alice = fake.users["alice"]
    assert alice["attrs"]["uidnumber"] == ["30001"]
    assert alice["attrs"]["homedirectory"] == ["/home/alice"]
    assert alice["groups"] == {"hpc", "cluster_users"}
    assert fake.users["bob"]["groups"] == set()  # disabled: not in the access group
    assert fake.users["lldap_bind"]["groups"] == {"lldap_strict_readonly"}
    assert fake.passwords["lldap_bind"] == "bindpw"
    assert sorted(s for s, _ in fake.secret_adds) == ["sec-alice", "sec-bob"]
    assert dict(fake.secret_adds)["sec-alice"] == fake.passwords["alice"]
    assert os.listdir(base / "state" / "pending-passwords") == []


def test_undeclared_users_without_cleanup(env):
    sync, fake, _ = env
    sync.apply(copy.deepcopy(BASE_CFG))
    assert "cluster_users" not in fake.users["manual"]["groups"]
    assert "manual" in fake.users
    assert "stale" in {g["name"] for g in fake.groups.values()}
    assert "cluster_users" in fake.users["otheradmin"]["groups"]  # lldap_admin member untouched


def test_reapply_never_resets_passwords(env):
    sync, fake, _ = env
    cfg = copy.deepcopy(BASE_CFG)
    sync.apply(cfg)
    before = dict(fake.passwords)
    fake.secret_adds.clear()
    fake.password_sets.clear()
    fake.calls.clear()
    sync.apply(cfg)
    assert fake.secret_adds == [] and fake.password_sets == [] and fake.passwords == before
    assert not any(c[0] == "update_user" for c in fake.calls)

    fake.passwords["alice"] = "changed-by-user"
    cfg["users"]["bob"].update(disabled=False, groups=["cluster_users"])
    sync.apply(cfg)
    assert fake.passwords["alice"] == "changed-by-user"
    assert fake.users["bob"]["groups"] == {"cluster_users"}


def test_bind_password_resynced(env):
    sync, fake, _ = env
    sync.apply(copy.deepcopy(BASE_CFG))
    fake.passwords["lldap_bind"] = "drifted"
    sync.apply(copy.deepcopy(BASE_CFG))
    assert fake.passwords["lldap_bind"] == "bindpw"


def test_cleanup_protects_admin_bind_and_builtins(env):
    sync, fake, _ = env
    cfg = copy.deepcopy(BASE_CFG)
    sync.apply(cfg)
    cfg["cleanup"] = True
    del cfg["users"]["bob"]
    sync.apply(cfg)
    assert "bob" not in fake.users and "manual" not in fake.users
    assert {"admin", "lldap_bind", "otheradmin", "alice"} <= set(fake.users)
    names = {g["name"] for g in fake.groups.values()}
    assert "stale" not in names
    assert {"lldap_admin", "lldap_password_manager", "lldap_strict_readonly",
            "hpc", "cluster_users"} <= names


def test_interrupted_creation_completes_with_stored_password(env, monkeypatch):
    sync, fake, _ = env
    good = sync.set_password

    def failing(token, username, password):
        raise RuntimeError("boom")

    monkeypatch.setattr(sync, "set_password", failing)
    with pytest.raises(RuntimeError):
        sync.apply(copy.deepcopy(BASE_CFG))
    monkeypatch.setattr(sync, "set_password", good)
    sync.apply(copy.deepcopy(BASE_CFG))
    adds = dict(fake.secret_adds)
    assert len(fake.secret_adds) == 2
    assert fake.passwords["alice"] == adds["sec-alice"]


def test_one_bad_user_does_not_block_others(env):
    sync, fake, _ = env
    cfg = copy.deepcopy(BASE_CFG)
    cfg["cleanup"] = True
    fake.fail_create.add("alice")
    with pytest.raises(RuntimeError, match="user alice"):
        sync.apply(cfg)
    assert "alice" not in fake.users
    assert "bob" in fake.users and fake.users["bob"]["groups"] == set()
    assert "manual" not in fake.users  # cleanup still ran
    assert "stale" not in {g["name"] for g in fake.groups.values()}


def test_warns_when_existing_user_secret_is_empty(env, caplog):
    sync, fake, _ = env
    sync.apply(copy.deepcopy(BASE_CFG))
    fake.secrets_with_versions.discard("sec-alice")  # user removed and re-added
    with caplog.at_level(logging.WARNING, logger="lldap-sync"):
        sync.apply(copy.deepcopy(BASE_CFG))
    assert any("sec-alice has no version" in r.getMessage() for r in caplog.records)
    assert fake.secret_adds.count(("sec-alice", fake.passwords["alice"])) == 1


def test_failed_creation_retries_do_not_add_secret_versions(env):
    sync, fake, _ = env
    fake.fail_create.add("alice")
    for _ in range(3):
        with pytest.raises(RuntimeError, match="user alice"):
            sync.apply(copy.deepcopy(BASE_CFG))
    fake.fail_create.discard("alice")
    sync.apply(copy.deepcopy(BASE_CFG))
    alice_adds = [pw for s, pw in fake.secret_adds if s == "sec-alice"]
    assert alice_adds == [fake.passwords["alice"]]


def _mark_bind_unpublished(base):
    (base / "state" / "bind-password-unpublished").write_text("")


def test_bind_password_published_only_when_bind_user_works(env):
    sync, fake, base = env
    _mark_bind_unpublished(base)
    sync.apply(copy.deepcopy(BASE_CFG))
    snaps = [snap for snap in fake.add_snapshots if snap[0] == "sec-bind"]
    assert len(snaps) == 1
    _, users_then, passwords_then = snaps[0]
    assert "lldap_strict_readonly" in users_then["lldap_bind"]["groups"]
    assert passwords_then["lldap_bind"] == "bindpw"
    assert fake.secret_values["sec-bind"] == b"bindpw"
    assert not (base / "state" / "bind-password-unpublished").exists()
    # Later runs do not publish it again.
    sync.apply(copy.deepcopy(BASE_CFG))
    assert [s for s, _ in fake.secret_adds].count("sec-bind") == 1


def test_bind_password_not_published_before_readonly_membership(env):
    sync, fake, base = env
    _mark_bind_unpublished(base)
    fake.fail_membership.add("lldap_bind")
    with pytest.raises(RuntimeError, match="memberships of lldap_bind"):
        sync.apply(copy.deepcopy(BASE_CFG))
    assert "sec-bind" not in fake.secret_values
    assert (base / "state" / "bind-password-unpublished").exists()
    fake.fail_membership.discard("lldap_bind")
    sync.apply(copy.deepcopy(BASE_CFG))
    assert fake.secret_values["sec-bind"] == b"bindpw"


def test_bind_password_not_published_when_login_fails(env, monkeypatch):
    sync, fake, base = env
    _mark_bind_unpublished(base)
    monkeypatch.setattr(sync, "login",
                        lambda u, p: None if u == "lldap_bind" else "tok")
    monkeypatch.setattr(sync, "set_password", lambda t, u, p: None)
    with pytest.raises(RuntimeError, match="publish lldap_bind password"):
        sync.apply(copy.deepcopy(BASE_CFG))
    assert "sec-bind" not in fake.secret_values


def test_bind_secret_with_other_value_is_not_overwritten(env):
    sync, fake, base = env
    _mark_bind_unpublished(base)
    fake.secret_values["sec-bind"] = b"something-else"
    with pytest.raises(RuntimeError, match="publish lldap_bind password"):
        sync.apply(copy.deepcopy(BASE_CFG))
    assert fake.secret_values["sec-bind"] == b"something-else"
    assert (base / "state" / "bind-password-unpublished").exists()


def test_user_password_published_after_account_is_ready(env):
    sync, fake, _ = env
    sync.apply(copy.deepcopy(BASE_CFG))
    snap = next(snap for snap in fake.add_snapshots if snap[0] == "sec-alice")
    _, users_then, passwords_then = snap
    assert users_then["alice"]["groups"] == {"hpc", "cluster_users"}
    assert passwords_then["alice"] == fake.secret_values["sec-alice"].decode()


def test_user_password_not_published_when_membership_fails(env):
    sync, fake, base = env
    fake.fail_membership.add("alice")
    with pytest.raises(RuntimeError, match="memberships of alice"):
        sync.apply(copy.deepcopy(BASE_CFG))
    assert "sec-alice" not in fake.secret_values
    pending = base / "state" / "pending-passwords" / "alice"
    assert pending.exists()
    fake.fail_membership.discard("alice")
    sync.apply(copy.deepcopy(BASE_CFG))
    assert fake.secret_values["sec-alice"].decode() == fake.passwords["alice"]
    assert not pending.exists()
