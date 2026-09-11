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

"""Shared fixtures. Puts the broker package on sys.path for the whole suite."""

import sys
from pathlib import Path

import pytest

FILES_DIR = Path(__file__).resolve().parent.parent
if str(FILES_DIR) not in sys.path:
    sys.path.insert(0, str(FILES_DIR))




def base_config(tmp_path, **overrides):
    """The minimum a Config needs, plus whatever a test wants to change."""
    raw = {
        "state_dir": str(tmp_path / "state"),
        "log_dir": str(tmp_path / "log"),
        "runtime_dir": str(tmp_path / "run"),
        "novnc_dir": str(tmp_path / "novnc"),
        "proxy_secret": "test-proxy-secret",
        "identity_mode": "trusted_proxy",
        "listen_port": 6080,
        "base_display_number": 1,
        "max_user_sessions": 32,
        "session_idle_timeout_seconds": 43200,
        "session_resolution": "1920x1080",
        "vnc_backend": "tigervnc",
    }
    raw.update(overrides)
    return raw




@pytest.fixture
def novnc_raw(tmp_path):
    return base_config(tmp_path)
