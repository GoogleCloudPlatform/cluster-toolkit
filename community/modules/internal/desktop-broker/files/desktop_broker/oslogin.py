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

"""Whether OS Login grants an account, and its POSIX details.

This is the authorisation decision: a user gets a desktop only as an account
OS Login knows and currently allows to log in to this instance. It is asked
per user, the same two questions the guest agent asks when someone signs in
over SSH, and re-asked on every hand-off once a short cache expires, so
revoking access in IAM revokes it here.
"""

import json
import logging
import pwd
import socket
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

from .errors import BrokerError

LOG = logging.getLogger("ghpc-desktop-broker")

METADATA_ROOT = "http://metadata.google.internal/computeMetadata/v1/oslogin"
CACHE_SECONDS = 60
# An ID missing from the cached listing refetches it at most this often: every
# request from a signed-in user without OS Login access would otherwise list
# every profile again.
SUBJECTS_MIN_REFRESH_SECONDS = 60


class ProfileNotFound(BrokerError):
    """OS Login has no profile for the account, or no longer grants it access."""


class Directory:
    """Per-user OS Login lookups, with successful answers cached briefly."""

    def __init__(self, metadata_root=METADATA_ROOT, cache_seconds=CACHE_SECONDS):
        self.metadata_root = metadata_root
        self.cache_seconds = cache_seconds
        self._cache = {}
        self._subjects = {}
        self._subjects_expire_at = 0
        self._subjects_fetched_at = 0

    def _get(self, path, params):
        """JSON from an OS Login metadata endpoint, or None if it answers 404."""
        query = urllib.parse.urlencode(params)
        request = urllib.request.Request(
            f"{self.metadata_root}/{path}?{query}",
            headers={"Metadata-Flavor": "Google"},
        )
        try:
            with urllib.request.urlopen(request, timeout=20) as response:
                return json.loads(response.read().decode("utf-8"))
        except urllib.error.HTTPError as err:
            if err.code == 404:
                return None
            if err.code == 403:
                raise BrokerError(
                    502,
                    "Desktop OS Login metadata lookup was denied. Check the "
                    "desktop host OS Login and metadata configuration.",
                ) from err
            raise BrokerError(
                502, f"OS Login metadata lookup failed with HTTP {err.code}."
            ) from err
        except urllib.error.URLError as err:
            raise BrokerError(
                502, f"OS Login metadata lookup failed: {err.reason}"
            ) from err
        except (TimeoutError, socket.timeout) as err:
            # A read that stalls after the connection opened is not a URLError.
            # socket.timeout is TimeoutError only from Python 3.10.
            raise BrokerError(502, "OS Login metadata lookup timed out.") from err
        except ValueError as err:
            # JSONDecodeError and UnicodeDecodeError both: an HTML error page
            # from an intercepting proxy, say, or a truncated response.
            raise BrokerError(
                502, "OS Login metadata returned an unreadable response."
            ) from err

    def _lookup(self, username):
        # Asked by name rather than read from the full profile listing: the
        # listing can lag behind, and miss an account that has just signed in.
        payload = self._get("users", {"username": username}) or {}
        for profile in payload.get("loginProfiles") or []:
            for account in profile.get("posixAccounts") or []:
                if account.get("username") == username and account.get(
                    "operatingSystemType"
                ) in (None, "", "LINUX"):
                    subject = profile.get("name")
                    break
            else:
                continue
            break
        else:
            raise ProfileNotFound(
                403, "No OS Login profile was found for this account."
            )

        # A profile outlives the IAM grant that lets it log in, so having one
        # is not enough. "email" takes the numeric subject, as the guest agent
        # passes it.
        verdict = self._get("authorize", {"email": subject, "policy": "login"})
        if not (verdict or {}).get("success"):
            raise ProfileNotFound(
                403, "OS Login does not allow this account to log in here."
            )
        return dict(account)

    def _usernames_by_subject(self):
        usernames = {}
        params = {"pagesize": 1024}
        seen = set()
        while True:
            payload = self._get("users", params) or {}
            for profile in payload.get("loginProfiles") or []:
                linux = [
                    account
                    for account in profile.get("posixAccounts") or []
                    if account.get("operatingSystemType") in (None, "", "LINUX")
                    and account.get("username")
                ]
                if profile.get("name") and linux:
                    primary = [a for a in linux if a.get("primary")] or linux
                    usernames[str(profile["name"])] = primary[0]["username"]
            # The metadata server never sends an empty token: it ends a listing
            # with "LAST_PAGE_TOKEN", then answers "0" indefinitely. "0" is the
            # end marker the guest agent's NSS module stops on. A repeated
            # token stops it too, so it can never loop.
            token = payload.get("nextPageToken")
            if not token or token == "0" or token in seen:
                return usernames
            seen.add(token)
            params = {"pagesize": 1024, "pagetoken": token}

    def username_for_subject(self, subject):
        """The POSIX username of the Google account with this numeric ID.

        For callers that arrive through IAP, which names the Google account
        rather than a local one. Only the name is decided here: whether that
        account may log in is still asked by resolve(), on every hand-off.
        Profiles carry no email, so the numeric ID is the only join. The
        listing is cached, but refetched for an ID it lacks, at most once a
        minute, so an account that has just been granted access waits a
        minute at most.
        """
        subject = str(subject)
        now = time.time()
        missing = (
            subject not in self._subjects
            and now - self._subjects_fetched_at >= SUBJECTS_MIN_REFRESH_SECONDS
        )
        if missing or now >= self._subjects_expire_at:
            self._subjects = self._usernames_by_subject()
            self._subjects_fetched_at = now
            self._subjects_expire_at = now + self.cache_seconds
        username = self._subjects.get(subject)
        if username:
            return username
        raise ProfileNotFound(
            403,
            "No OS Login account was found for your Google account. Ask an "
            "administrator for OS Login access to this cluster.",
        )

    def resolve(self, username):
        """The OS Login POSIX account for a username, if it may log in.

        Only a granted answer is cached, so a refusal is re-checked on the next
        request and a user just granted access need not wait.
        """
        cached = self._cache.get(username)
        if cached and time.time() < cached["expires_at"]:
            return cached["account"]
        account = self._lookup(username)
        self._cache[username] = {
            "account": account,
            "expires_at": time.time() + self.cache_seconds,
        }
        return account


def lookup_passwd(username):
    """The local passwd entry for a username, prompting nsswitch if needed."""
    try:
        return pwd.getpwnam(username)
    except KeyError:
        # getent forces the OS Login NSS module to populate its cache; a
        # freshly granted account is often not in it yet.
        result = subprocess.run(
            ["getent", "passwd", username],
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        if result.returncode != 0:
            raise BrokerError(
                403,
                "Your OS Login account is not available on this desktop host.",
            )
        try:
            return pwd.getpwnam(username)
        except KeyError as err:
            raise BrokerError(
                403, "Your OS Login account could not be resolved locally."
            ) from err
