import json
import os
from pathlib import Path

STATE_FILE = "state.json"

class StateManager:
    def __init__(self, state_file=STATE_FILE):
        self.state_file = state_file
        self._ensure_state_file()

    def get_default_state(self):
        return {
            "CURRENT_STATE": "INITIALIZATION",
            "pr_number": None,
            "rc_branch": None,
            "version_branch": None,
            "bugs": [],
            "test_run_ids": [],
            "test_retries": 0,
            "on_call_ldap": None,
            "on_call_github": None,
            "pr_active_timestamp": None
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
