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

import json
import os
from pathlib import Path

STATE_FILE = "state.json"

class StateManager:
    def __init__(self, state_file=STATE_FILE):
        self.state_file = state_file
        self._ensure_state_file()

    def get_default_state(self):
        import time
        return {
            "CURRENT_STATE": "INITIALIZATION",
            "pr_number": None,
            "rc_branch": None,
            "version_branch": None,
            "bugs": [],
            "bugs_details": [],
            "test_run_ids": [],
            "test_retries": 0,
            "on_call_ldap": None,
            "on_call_github": None,
            "pr_active_timestamp": None,
            "paused": False,
            "created_at": None,
            "merged_at": None,
            "cancelled_from_state": None,
            "trigger_type": None,
            "version_pr_number": None,
            "backport_pr_number": None,
            "reviewer_assigned": False,
            "seen_comments": 0
        }

    def _ensure_state_file(self):
        if not os.path.exists(self.state_file):
            self.write_state(self.get_default_state())

    def reset_state(self):
        self.write_state(self.get_default_state())

    def read_state(self):
        with open(self.state_file, 'r') as f:
            return json.load(f)

    def write_state(self, state_dict):
        with open(self.state_file, 'w') as f:
            json.dump(state_dict, f, indent=4)

    def update_state(self, **kwargs):
        state = self.read_state()
        state.update(kwargs)
        self.write_state(state)

    def set_current_state(self, new_state):
        self.update_state(CURRENT_STATE=new_state)

    def get_current_state(self):
        return self.read_state().get("CURRENT_STATE")

    def archive_to_history(self):
        import time
        state = self.read_state()
        history_file = Path(self.state_file).parent / "history.json"
        
        history = []
        if history_file.exists():
            try:
                with open(history_file, 'r') as f:
                    history = json.load(f)
            except Exception:
                pass
                
        bugs_list = state.get("bugs_details", [])
        
        fixed_count = 0
        deprioritized_count = 0
        for bug in bugs_list:
            if bug.get("status", "").upper() in ["CLOSED", "FIXED", "VERIFIED"]:
                fixed_count += 1
            elif bug.get("priority", "").upper() in ["P2", "P3", "P4"] or bug.get("status", "").upper() == "DEPRIORITIZED":
                deprioritized_count += 1
                
        record = {
            "release_id": state.get('rc_branch') or 'unknown',
            "created_at": state.get("created_at"),
            "merged_at": state.get("merged_at") or time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "pr_number": state.get("pr_number"),
            "on_call_ldap": state.get("on_call_ldap"),
            "bugs": bugs_list,
            "bug_stats": {
                "total": len(bugs_list),
                "fixed": fixed_count,
                "deprioritized": deprioritized_count
            }
        }
        
        history.append(record)
        with open(history_file, 'w') as f:
            json.dump(history, f, indent=4)
