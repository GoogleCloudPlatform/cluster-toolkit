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

"""Joining a Google identity to a POSIX account through OS Login.

This is the authorisation decision: a user gets a desktop as the account OS
Login says they are, and gets none if OS Login does not know them. It is
re-evaluated on every hand-off rather than cached in a database, so revoking
access in IAM revokes it here.
"""

import json
import logging
import pwd
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

from .errors import BrokerError

LOG = logging.getLogger("ghpc-desktop-broker")

USERS_URL = (
    "http://metadata.google.internal/computeMetadata/v1/oslogin/users"
)
CACHE_SECONDS = 60


class Directory:
    """OS Login profiles for this instance, indexed for lookup."""

    def __init__(self, users_url=USERS_URL, cache_seconds=CACHE_SECONDS):
        self.users_url = users_url
        self.cache_seconds = cache_seconds
        self._cache = {"by_subject": {}, "by_username": {}, "expires_at": 0}

    def _fetch(self):
        query = urllib.parse.urlencode({"pagesize": 1024})
        request = urllib.request.Request(
            f"{self.users_url}?{query}",
            headers={"Metadata-Flavor": "Google"},
        )
        try:
            with urllib.request.urlopen(request, timeout=20) as response:
                return json.loads(response.read().decode("utf-8"))
        except urllib.error.HTTPError as err:
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

    def load(self, refresh=False):
        if (
            not refresh
            and time.time() < self._cache["expires_at"]
            and (self._cache["by_subject"] or self._cache["by_username"])
        ):
            return self._cache

        payload = self._fetch()
        by_subject = {}
        by_username = {}
        for profile in payload.get("loginProfiles", []):
            profile_name = profile.get("name")
            if not profile_name:
                continue

            accounts = [
                account
                for account in profile.get("posixAccounts", [])
                if account.get("operatingSystemType") in [None, "", "LINUX"]
            ]
            if not accounts:
                continue

            primary = [a for a in accounts if a.get("primary")]
            account = dict(primary[0] if primary else accounts[0])
            username = account.get("username")
            if not username:
                continue

            account["profile_name"] = profile_name
            # "name" on a login profile is the Google account's numeric
            # subject, which is exactly the "sub" claim on a verified ID token.
            # That makes it the only authoritative join between a Google
            # identity and a POSIX account: login profiles carry no email or
            # userName field of their own.
            by_subject[str(profile_name)] = account
            by_username[username] = account

        self._cache = {
            "by_subject": by_subject,
            "by_username": by_username,
            "expires_at": time.time() + self.cache_seconds,
        }
        return self._cache

    def resolve(self, login_uid, username_hint=None):
        """Find the POSIX account for an identity.

        Tries the numeric subject first because it is authoritative, then the
        username hint, which is only as trustworthy as the mode that supplied
        it. Refreshes once before giving up, so a user who has just been granted
        access does not have to wait out the cache.
        """
        if not login_uid and not username_hint:
            raise BrokerError(
                403,
                "Missing desktop OS Login identity. Sign in with Google and "
                "try again.",
            )

        for refresh in (False, True):
            profiles = self.load(refresh=refresh)
            if login_uid:
                account = profiles["by_subject"].get(str(login_uid))
                if account:
                    return account
            if username_hint:
                account = profiles["by_username"].get(username_hint)
                if account:
                    return account

        raise BrokerError(
            403, "No OS Login profile was found for this Google identity."
        )


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
