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

"""Keeps LLDAP users and groups in line with the "lldap-config" VM metadata.

Runs as a systemd service on the LLDAP VM. It waits for changes of the
metadata key and applies the JSON document through LLDAP's GraphQL API:

- creates the POSIX custom attributes (uidNumber, gidNumber, homeDirectory,
  unixShell) if missing;
- creates groups (with gidNumber) and users, and updates their attributes;
- sets group memberships for the groups it manages;
- ensures the SSSD bind user exists, is in lldap_strict_readonly, and has the
  password stored in Secret Manager. On first boot the startup script keeps a
  new bind password unpublished; it is published to its secret only once the
  bind user exists, is in lldap_strict_readonly and can log in with it, so
  clients never get a bind password that does not work yet;
- gives a new user a random password and stores it in that user's secret
  after the account exists, has that password and has its group memberships.
  An existing user's password is never changed;
- users not in the config lose their access group membership, or are deleted
  when "cleanup" is true. The admin user, the bind user and members of
  lldap_admin are never touched; LLDAP's built-in groups are never deleted.

Passwords are set with /app/lldap_set_password inside the LLDAP container
(LLDAP uses the OPAQUE protocol, so they cannot be set through GraphQL).
"""

import json
import logging
import os
import secrets
import subprocess
import sys
import time
import urllib.error
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import lldap_gcp  # noqa: E402  pylint: disable=wrong-import-position

LLDAP_URL = "http://127.0.0.1:17170"
CONFIG_KEY = "instance/attributes/lldap-config"
BUILTIN_GROUPS = {"lldap_admin", "lldap_password_manager", "lldap_strict_readonly"}

PROJECT = os.environ["LLDAP_PROJECT"]
ADMIN_USER = os.environ.get("LLDAP_ADMIN_USER", "admin")
BIND_USER = os.environ.get("LLDAP_BIND_USER", "lldap_bind")
BIND_SECRET = os.environ.get("LLDAP_BIND_SECRET", "")
CONTAINER = os.environ.get("LLDAP_CONTAINER", "lldap")
BASE_DIR = os.environ.get("LLDAP_BASE_DIR", "/var/lib/lldap")
DATA_DIR = os.path.join(BASE_DIR, "data")  # mounted at /data in the container
SECRETS_DIR = os.path.join(BASE_DIR, "secrets")
PENDING_DIR = os.path.join(BASE_DIR, "state", "pending-passwords")
# Written by the startup script when the bind password is not yet published.
BIND_PENDING = os.path.join(BASE_DIR, "state", "bind-password-unpublished")

USER_ATTRIBUTES = {
    "uidNumber": "INTEGER",
    "gidNumber": "INTEGER",
    "homeDirectory": "STRING",
    "unixShell": "STRING",
}
GROUP_ATTRIBUTES = {"gidNumber": "INTEGER"}

log = logging.getLogger("lldap-sync")


def _read(path):
    with open(path, encoding="utf-8") as f:
        return f.read().strip()


def _post_json(url, payload, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(
        url, data=json.dumps(payload).encode("utf-8"), headers=headers, method="POST")
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.loads(resp.read().decode("utf-8"))


def wait_for_lldap(timeout=600):
    deadline = time.time() + timeout
    while True:
        try:
            with urllib.request.urlopen(LLDAP_URL + "/", timeout=10):
                return
        except (urllib.error.URLError, OSError):
            if time.time() > deadline:
                raise RuntimeError("LLDAP did not answer on " + LLDAP_URL)
            time.sleep(5)


def login(username, password):
    """Returns a JWT, or None if the credentials are rejected."""
    try:
        data = _post_json(LLDAP_URL + "/auth/simple/login",
                          {"username": username, "password": password})
    except urllib.error.HTTPError as err:
        if err.code in (400, 401, 403, 404):
            return None
        raise
    return data["token"]


class Api:
    """Thin GraphQL client that fails on any GraphQL error."""

    def __init__(self, token):
        self.token = token

    def gql(self, query, variables=None):
        resp = _post_json(LLDAP_URL + "/api/graphql",
                          {"query": query, "variables": variables or {}}, self.token)
        if resp.get("errors"):
            raise RuntimeError("GraphQL error: " + "; ".join(
                e.get("message", str(e)) for e in resp["errors"]))
        return resp["data"]

    def schema(self):
        return self.gql("{ schema { userSchema { attributes { name } }"
                        " groupSchema { attributes { name } } } }")["schema"]

    def add_attribute(self, kind, name, attr_type):
        field = "addUserAttribute" if kind == "user" else "addGroupAttribute"
        self.gql(
            "mutation($name: String!, $type: AttributeType!) { " + field +
            "(name: $name, attributeType: $type, isList: false, isVisible: true,"
            " isEditable: false) { ok } }",
            {"name": name, "type": attr_type})

    def groups(self):
        return self.gql("{ groups { id displayName attributes { name value } } }")["groups"]

    def create_group(self, name, attributes):
        return self.gql(
            "mutation($r: CreateGroupInput!) { createGroupWithDetails(request: $r) { id } }",
            {"r": {"displayName": name, "attributes": attributes}})["createGroupWithDetails"]["id"]

    def update_group(self, group_id, attributes):
        self.gql("mutation($g: UpdateGroupInput!) { updateGroup(group: $g) { ok } }",
                 {"g": {"id": group_id, "insertAttributes": attributes}})

    def delete_group(self, group_id):
        self.gql("mutation($id: Int!) { deleteGroup(groupId: $id) { ok } }", {"id": group_id})

    def users(self):
        return self.gql("{ users { id email displayName groups { id displayName }"
                        " attributes { name value } } }")["users"]

    def create_user(self, user):
        self.gql("mutation($u: CreateUserInput!) { createUser(user: $u) { id } }", {"u": user})

    def update_user(self, user):
        self.gql("mutation($u: UpdateUserInput!) { updateUser(user: $u) { ok } }", {"u": user})

    def delete_user(self, user_id):
        self.gql("mutation($id: String!) { deleteUser(userId: $id) { ok } }", {"id": user_id})

    def add_member(self, user_id, group_id):
        self.gql("mutation($u: String!, $g: Int!) { addUserToGroup(userId: $u, groupId: $g) { ok } }",
                 {"u": user_id, "g": group_id})

    def remove_member(self, user_id, group_id):
        self.gql("mutation($u: String!, $g: Int!) { removeUserFromGroup(userId: $u, groupId: $g) { ok } }",
                 {"u": user_id, "g": group_id})


def set_password(token, username, password):
    """Registers a password with lldap_set_password inside the container.

    The password and the token are written to files on the data volume, so
    neither appears on the host command line of `podman exec`. Inside the
    container the token is still passed as `--token` to lldap_set_password
    (the tool has no environment or file option for it), so it is briefly
    visible in that process's argv, which root and, unless /proc is mounted
    with hidepid, other local users on the VM can read. The password is passed
    through the LLDAP_USER_PASSWORD environment variable and is not in argv.
    """
    suffix = secrets.token_hex(8)
    files = {"pw": f".pw-{suffix}", "tok": f".tok-{suffix}"}
    try:
        for key, value in (("pw", password), ("tok", token)):
            fd = os.open(os.path.join(DATA_DIR, files[key]),
                         os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, "w", encoding="utf-8") as f:
                f.write(value)
        script = ('LLDAP_USER_PASSWORD="$(cat "$1")" exec /app/lldap_set_password '
                  '--base-url http://127.0.0.1:17170 --token "$(cat "$2")" --username "$3"')
        result = subprocess.run(
            ["podman", "exec", CONTAINER, "/bin/sh", "-c", script, "sh",
             "/data/" + files["pw"], "/data/" + files["tok"], username],
            capture_output=True, text=True, check=False, timeout=120)
        if result.returncode != 0:
            raise RuntimeError(f"lldap_set_password failed for {username}: {result.stderr.strip()}")
    finally:
        for name in files.values():
            try:
                os.unlink(os.path.join(DATA_DIR, name))
            except FileNotFoundError:
                pass


def _attr_map(entity):
    return {a["name"].lower(): a["value"] for a in entity.get("attributes", [])}


def _pending_path(username):
    return os.path.join(PENDING_DIR, username)


class Failures:
    """Collects per-entry errors so one bad entry does not stop the others."""

    def __init__(self):
        self.items = []

    def run(self, what, func, *args, **kwargs):
        """Calls func; returns True on success, False (and records) on error."""
        try:
            func(*args, **kwargs)
            return True
        except Exception as err:  # pylint: disable=broad-except
            log.error("%s failed: %s", what, err)
            self.items.append(what)
            return False


def ensure_schema(api):
    schema = api.schema()
    have_user = {a["name"].lower() for a in schema["userSchema"]["attributes"]}
    have_group = {a["name"].lower() for a in schema["groupSchema"]["attributes"]}
    for name, attr_type in USER_ATTRIBUTES.items():
        if name.lower() not in have_user:
            api.add_attribute("user", name, attr_type)
            log.info("created user attribute %s", name)
    for name, attr_type in GROUP_ATTRIBUTES.items():
        if name.lower() not in have_group:
            api.add_attribute("group", name, attr_type)
            log.info("created group attribute %s", name)


def ensure_group(api, existing, name, gid):
    attrs = [{"name": "gidNumber", "value": [str(gid)]}]
    if name not in existing:
        api.create_group(name, attrs)
        log.info("created group %s (gid %s)", name, gid)
    elif _attr_map(existing[name]).get("gidnumber") != [str(gid)]:
        api.update_group(existing[name]["id"], attrs)
        log.info("updated gid of group %s to %s", name, gid)


def create_user(api, token, username, fields, password=None):
    """Creates a user and sets its initial password.

    For regular users the password is generated here and kept in a 0600
    pending file on the data disk until publish_pending() stores it in the
    user's secret, so an interrupted creation completes with the same
    password on the next run.
    """
    if password is None:
        pending = _pending_path(username)
        if os.path.exists(pending):
            password = _read(pending)
        else:
            password = lldap_gcp.random_password()
            fd = os.open(pending, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            with os.fdopen(fd, "w", encoding="utf-8") as f:
                f.write(password)
    api.create_user(fields)
    set_password(token, username, password)
    log.info("created user %s", username)


def publish_pending(username, secret_id):
    """Stores a new user's initial password in their secret, once.

    Only called after the account exists, has the password and has its group
    memberships. A version is added only if the secret has none, so retries
    never add duplicate versions.
    """
    pending = _pending_path(username)
    if not lldap_gcp.secret_has_version(PROJECT, secret_id):
        lldap_gcp.secret_add(PROJECT, secret_id, _read(pending).encode("ascii"))
        log.info("published initial password of %s to %s", username, secret_id)
    os.unlink(pending)


def publish_bind_password(bind_password):
    """Publishes the bind password after the bind user works (first boot)."""
    if not os.path.exists(BIND_PENDING):
        return
    if login(BIND_USER, bind_password) is None:
        raise RuntimeError("bind user cannot log in yet; not publishing its password")
    try:
        current = lldap_gcp.secret_get(PROJECT, BIND_SECRET).decode("ascii")
    except lldap_gcp.NotFound:
        current = None
    if current is None:
        lldap_gcp.secret_add(PROJECT, BIND_SECRET, bind_password.encode("ascii"))
        log.info("published the bind password to %s", BIND_SECRET)
    elif current != bind_password:
        raise RuntimeError(f"secret {BIND_SECRET} already holds a different bind password; "
                           "restart the VM so the startup script adopts it")
    os.unlink(BIND_PENDING)


def sync_user(api, token, existing, username, u):
    attrs = [
        {"name": "uidNumber", "value": [str(u["uid"])]},
        {"name": "gidNumber", "value": [str(u["gid"])]},
        {"name": "homeDirectory", "value": [f"/home/{username}"]},
        {"name": "unixShell", "value": [u["shell"]]},
    ]
    if username not in existing:
        create_user(api, token, username, {
            "id": username, "email": u["email"], "displayName": u["display_name"],
            "attributes": attrs})
        return
    if os.path.exists(_pending_path(username)):
        # Creation was interrupted before the password was published. The
        # password was never handed out, so setting it again is not a reset.
        set_password(token, username, _read(_pending_path(username)))
        log.info("completed initial password of %s", username)
    cur = existing[username]
    cur_attrs = _attr_map(cur)
    changed = (cur["email"].lower() != u["email"].lower()
               or cur["displayName"] != u["display_name"]
               or any(cur_attrs.get(a["name"].lower()) != a["value"] for a in attrs))
    if changed:
        api.update_user({"id": username, "email": u["email"],
                         "displayName": u["display_name"], "insertAttributes": attrs})
        log.info("updated user %s", username)


def warn_if_secret_empty(username, secret_id):
    """An existing user whose secret has no version (e.g. removed from the
    blueprint, which deleted the secret, and added again while the account
    stayed in LLDAP) keeps the old password; it is never reset."""
    try:
        if not lldap_gcp.secret_has_version(PROJECT, secret_id):
            log.warning("user %s exists in LLDAP but secret %s has no version; the existing "
                        "password is kept and no initial password is available", username, secret_id)
    except Exception as err:  # pylint: disable=broad-except
        log.warning("could not list versions of secret %s: %s", secret_id, err)


def sync_memberships(api, user, wanted, managed, group_ids):
    current = {g["displayName"] for g in user["groups"]}
    for name in sorted(wanted - current):
        api.add_member(user["id"], group_ids[name])
        log.info("added %s to %s", user["id"], name)
    for name in sorted((current & managed) - wanted):
        api.remove_member(user["id"], group_ids[name])
        log.info("removed %s from %s", user["id"], name)


def apply(cfg):
    """Applies cfg. Errors in single users or groups are logged and skipped;
    if any occurred, raises at the end so the caller retries."""
    wait_for_lldap()
    token = login(ADMIN_USER, _read(os.path.join(SECRETS_DIR, "admin_password")))
    if not token:
        raise RuntimeError("admin login failed; check the admin password secret")
    api = Api(token)
    ensure_schema(api)
    failures = Failures()

    access_group = cfg["access_group"]
    wanted_groups = {name: g["gid"] for name, g in cfg["groups"].items()}
    wanted_groups[access_group] = cfg["access_group_gid"]
    managed = set(wanted_groups)
    existing_groups = {g["displayName"]: g for g in api.groups()}
    for name, gid in sorted(wanted_groups.items()):
        failures.run(f"group {name}", ensure_group, api, existing_groups, name, gid)
    group_ids = {g["displayName"]: g["id"] for g in api.groups()}

    existing = {u["id"]: u for u in api.users()}

    # SSSD bind user: read-only. Its password file comes from Secret Manager,
    # or on first boot is unpublished until the bind user works.
    bind_password = _read(os.path.join(SECRETS_DIR, "bind_password"))
    bind_ok = True
    if BIND_USER not in existing:
        bind_ok = failures.run(f"user {BIND_USER}", create_user, api, token, BIND_USER, {
            "id": BIND_USER, "email": f"{BIND_USER}@noemail.invalid",
            "displayName": "SSSD bind user"}, password=bind_password)
    elif login(BIND_USER, bind_password) is None:
        bind_ok = failures.run(f"user {BIND_USER} password", set_password, token,
                               BIND_USER, bind_password)
        if bind_ok:
            log.info("reset bind user password to the value in Secret Manager")
    if bind_ok:
        bind_user = next(u for u in api.users() if u["id"] == BIND_USER)
        bind_ok = failures.run(f"memberships of {BIND_USER}", sync_memberships, api, bind_user,
                               {"lldap_strict_readonly"}, managed, group_ids)
    if bind_ok:
        failures.run(f"publish {BIND_USER} password", publish_bind_password, bind_password)

    user_ok = {}
    for username, u in sorted(cfg["users"].items()):
        if username in existing and not os.path.exists(_pending_path(username)):
            warn_if_secret_empty(username, u["secret"])
        user_ok[username] = failures.run(f"user {username}", sync_user, api, token,
                                         existing, username, u)

    users = {u["id"]: u for u in api.users()}
    for username, u in sorted(cfg["users"].items()):
        if username not in users:
            continue
        ok = failures.run(f"memberships of {username}", sync_memberships, api, users[username],
                          set(u["groups"]), managed, group_ids)
        if ok and user_ok.get(username) and os.path.exists(_pending_path(username)):
            failures.run(f"publish password of {username}", publish_pending,
                         username, u["secret"])

    protected = {ADMIN_USER, BIND_USER}
    protected |= {uid for uid, usr in users.items()
                  if any(g["displayName"] == "lldap_admin" for g in usr["groups"])}
    for uid, usr in sorted(users.items()):
        if uid in cfg["users"] or uid in protected:
            continue
        if cfg.get("cleanup"):
            if failures.run(f"delete user {uid}", api.delete_user, uid):
                log.info("deleted user %s (not in config)", uid)
        elif any(g["displayName"] == access_group for g in usr["groups"]):
            if failures.run(f"remove {uid} from {access_group}", api.remove_member,
                            uid, group_ids[access_group]):
                log.info("removed %s from %s (not in config)", uid, access_group)

    if cfg.get("cleanup"):
        for name, gid in sorted(group_ids.items()):
            if name not in managed and name not in BUILTIN_GROUPS:
                if failures.run(f"delete group {name}", api.delete_group, gid):
                    log.info("deleted group %s (not in config)", name)

    if failures.items:
        raise RuntimeError(f"{len(failures.items)} entries failed: {', '.join(failures.items)}")
    log.info("sync complete: %d users, %d groups", len(cfg["users"]), len(wanted_groups))


def main():
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    os.makedirs(PENDING_DIR, mode=0o700, exist_ok=True)
    last_etag = ""
    healthy = False
    while True:
        try:
            if healthy:
                # Returns on change, or after timeout_sec with the current value;
                # either way the config is re-applied, which repairs drift.
                body, etag = lldap_gcp.metadata_get(
                    CONFIG_KEY,
                    {"wait_for_change": "true", "last_etag": last_etag, "timeout_sec": "3600"},
                    timeout=3700)
            else:
                body, etag = lldap_gcp.metadata_get(CONFIG_KEY)
        except Exception:  # pylint: disable=broad-except
            log.exception("reading %s failed", CONFIG_KEY)
            time.sleep(10)
            continue
        last_etag = etag
        try:
            apply(json.loads(body))
            healthy = True
        except Exception:  # pylint: disable=broad-except
            log.exception("sync failed; retrying in 60 seconds")
            healthy = False
            time.sleep(60)


if __name__ == "__main__":
    main()
