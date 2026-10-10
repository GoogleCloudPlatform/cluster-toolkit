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

"""Session records, per-user locks and display-slot allocation."""

import asyncio
import hashlib
import json
import logging
import os
import threading

from ..errors import BrokerError

LOG = logging.getLogger("ghpc-desktop-broker")


class SessionStore:
    """One JSON record per user, in a root-only directory.

    Records name the user, their uid, their display number and - on a backend
    without one-time passwords - their RFB password, so the directory is 0700
    and each file 0600.
    """

    def __init__(self, state_dir, max_user_sessions):
        self.state_dir = state_dir
        self.max_user_sessions = int(max_user_sessions)
        self._user_locks = {}
        # Slot allocation reads every record, and the per-user lock does not
        # serialise different users, so two arriving together would otherwise
        # both see the same slot as free.
        self._allocation_lock = threading.Lock()

        self.state_dir.mkdir(parents=True, exist_ok=True)
        os.chmod(self.state_dir, 0o700)

    def path_for(self, email):
        digest = hashlib.sha256(email.lower().encode("utf-8")).hexdigest()
        return self.state_dir / f"{digest}.json"

    def load(self, email):
        path = self.path_for(email)
        if not path.exists():
            return None
        try:
            return json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as err:
            LOG.warning("Failed to read session file %s: %s", path, err)
            return None

    def save(self, session):
        path = self.path_for(session["email"])
        payload = json.dumps(session, sort_keys=True, indent=2)
        temp_path = path.with_suffix(".tmp")
        temp_path.write_text(payload, encoding="utf-8")
        os.chmod(temp_path, 0o600)
        temp_path.replace(path)

    def delete(self, email):
        try:
            self.path_for(email).unlink()
        except FileNotFoundError:
            pass

    def list(self):
        sessions = []
        for path in self.state_dir.glob("*.json"):
            try:
                sessions.append(json.loads(path.read_text(encoding="utf-8")))
            except (OSError, json.JSONDecodeError) as err:
                LOG.warning("Failed to parse desktop session state %s: %s", path, err)
        return sessions

    def user_lock(self, email):
        lock = self._user_locks.get(email)
        if lock is None:
            lock = asyncio.Lock()
            self._user_locks[email] = lock
        return lock

    @property
    def allocation_lock(self):
        return self._allocation_lock

    def allocate_slot(self, email):
        """Lowest free display slot, ignoring this user's own existing one."""
        used = {
            int(session["slot"])
            for session in self.list()
            if session.get("email") != email and session.get("slot") is not None
        }
        for slot in range(self.max_user_sessions):
            if slot not in used:
                return slot
        raise BrokerError(
            503,
            "The desktop node has reached its configured user-session capacity.",
        )
