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

"""Minimal Google Cloud helpers for the LLDAP server, using only the stdlib.

Authentication uses the VM service account token from the metadata server.
Used as a library by lldap_sync.py and as a CLI by the startup and backup
scripts:

  lldap_gcp.py secret-get    PROJECT SECRET OUTFILE   # exit 3 if no version
  lldap_gcp.py secret-add    PROJECT SECRET INFILE
  lldap_gcp.py secret-ensure PROJECT SECRET OUTFILE   # read, or generate+add
  lldap_gcp.py secret-sync   PROJECT SECRET INFILE    # add if latest differs
  lldap_gcp.py gcs-upload    BUCKET OBJECT INFILE
  lldap_gcp.py metadata      PATH                     # e.g. instance/hostname
"""

import base64
import json
import os
import secrets
import string
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

METADATA_URL = "http://metadata.google.internal/computeMetadata/v1/"
SECRETS_URL = "https://secretmanager.googleapis.com/v1/"
GCS_UPLOAD_URL = "https://storage.googleapis.com/upload/storage/v1/b/"

_token_cache = {"token": None, "expiry": 0.0}


class NotFound(Exception):
    """The requested resource (e.g. a secret version) does not exist."""


def metadata_get(path, params=None, timeout=30):
    """Returns (body, etag) of a metadata server path."""
    url = METADATA_URL + path
    if params:
        url += "?" + urllib.parse.urlencode(params)
    req = urllib.request.Request(url, headers={"Metadata-Flavor": "Google"})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return resp.read().decode("utf-8"), resp.headers.get("ETag", "")


def access_token():
    now = time.time()
    if _token_cache["token"] and now < _token_cache["expiry"] - 60:
        return _token_cache["token"]
    body, _ = metadata_get("instance/service-accounts/default/token")
    data = json.loads(body)
    _token_cache["token"] = data["access_token"]
    _token_cache["expiry"] = now + int(data.get("expires_in", 300))
    return _token_cache["token"]


def _request(method, url, data=None, headers=None, attempts=6):
    """Authenticated request with retries on transient errors."""
    delay = 2
    for attempt in range(attempts):
        hdrs = {"Authorization": "Bearer " + access_token()}
        hdrs.update(headers or {})
        req = urllib.request.Request(url, data=data, method=method, headers=hdrs)
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                return resp.read()
        except urllib.error.HTTPError as err:
            if err.code == 404:
                raise NotFound(url) from err
            if err.code in (429, 500, 502, 503, 504) and attempt < attempts - 1:
                time.sleep(delay)
                delay = min(delay * 2, 60)
                continue
            detail = err.read().decode("utf-8", "replace")
            raise RuntimeError(f"{method} {url}: HTTP {err.code}: {detail}") from err
        except urllib.error.URLError as err:
            if attempt < attempts - 1:
                time.sleep(delay)
                delay = min(delay * 2, 60)
                continue
            raise RuntimeError(f"{method} {url}: {err}") from err
    raise RuntimeError(f"{method} {url}: retries exhausted")


def _secret_url(project, secret):
    return f"{SECRETS_URL}projects/{project}/secrets/{secret}"


def secret_get(project, secret):
    """Returns the latest version's payload as bytes; raises NotFound."""
    body = _request("GET", _secret_url(project, secret) + "/versions/latest:access")
    return base64.b64decode(json.loads(body)["payload"]["data"])


def secret_add(project, secret, payload):
    body = json.dumps({"payload": {"data": base64.b64encode(payload).decode("ascii")}})
    _request(
        "POST",
        _secret_url(project, secret) + ":addVersion",
        data=body.encode("utf-8"),
        headers={"Content-Type": "application/json"},
    )


def secret_has_version(project, secret):
    """True if the secret has at least one ENABLED version (needs versions.list)."""
    url = _secret_url(project, secret) + "/versions"
    token = ""
    while True:
        query = "?" + urllib.parse.urlencode({"pageToken": token}) if token else ""
        data = json.loads(_request("GET", url + query) or b"{}")
        if any(v.get("state") == "ENABLED" for v in data.get("versions", [])):
            return True
        token = data.get("nextPageToken", "")
        if not token:
            return False


def random_password(length=32):
    alphabet = string.ascii_letters + string.digits
    return "".join(secrets.choice(alphabet) for _ in range(length))


def gcs_upload(bucket, name, path):
    url = (GCS_UPLOAD_URL + urllib.parse.quote(bucket, safe="") + "/o?"
           + urllib.parse.urlencode({"uploadType": "media", "name": name}))
    with open(path, "rb") as f:
        data = f.read()
    _request("POST", url, data=data, headers={"Content-Type": "application/octet-stream"})


def _write_private(path, data):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "wb") as f:
        f.write(data)


def main(argv):
    if len(argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    cmd, args = argv[1], argv[2:]
    if cmd == "secret-get" and len(args) == 3:
        try:
            _write_private(args[2], secret_get(args[0], args[1]))
        except NotFound:
            return 3
        return 0
    if cmd == "secret-add" and len(args) == 3:
        with open(args[2], "rb") as f:
            secret_add(args[0], args[1], f.read())
        return 0
    if cmd == "secret-ensure" and len(args) == 3:
        try:
            value = secret_get(args[0], args[1])
        except NotFound:
            value = random_password().encode("ascii")
            secret_add(args[0], args[1], value)
            print(f"generated a new value for secret {args[1]}", file=sys.stderr)
        _write_private(args[2], value)
        return 0
    if cmd == "secret-sync" and len(args) == 3:
        with open(args[2], "rb") as f:
            value = f.read()
        try:
            current = secret_get(args[0], args[1])
        except NotFound:
            current = None
        if current != value:
            secret_add(args[0], args[1], value)
            print(f"published a new version of secret {args[1]}", file=sys.stderr)
        return 0
    if cmd == "gcs-upload" and len(args) == 3:
        gcs_upload(args[0], args[1], args[2])
        return 0
    if cmd == "metadata" and len(args) == 1:
        body, _ = metadata_get(args[0])
        print(body)
        return 0
    print(__doc__, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
