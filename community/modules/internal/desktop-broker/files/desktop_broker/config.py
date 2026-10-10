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

"""Runtime configuration, validated once at startup.

Every invalid combination is rejected here rather than at the point of use, so
a misconfigured broker fails when the service starts instead of when the first
user tries to open a desktop.
"""

import json
from pathlib import Path

IDENTITY_MODES = frozenset({"trusted_proxy"})


class ConfigError(ValueError):
    """Configuration that cannot produce a working broker."""


def _require(value, name):
    text = str(value or "").strip()
    if not text:
        raise ConfigError(f"{name} is required.")
    return text


class Config:
    """Validated broker configuration.

    Attribute access rather than dict lookups, so a typo is an AttributeError at
    startup instead of a silent None deep inside a request.
    """

    def __init__(self, raw):
        self.raw = dict(raw)

        self.state_dir = Path(raw["state_dir"])
        self.log_dir = Path(raw["log_dir"])
        self.runtime_dir = Path(raw["runtime_dir"])

        self.proxy_secret = _require(raw.get("proxy_secret"), "proxy_secret")

        self.identity_mode = (
            str(raw.get("identity_mode") or "trusted_proxy").strip().lower()
        )
        if self.identity_mode not in IDENTITY_MODES:
            raise ConfigError(
                f"Unsupported identity_mode: {self.identity_mode!r}. Expected "
                f"one of: {', '.join(sorted(IDENTITY_MODES))}."
            )

        self.listen_host = raw.get("listen_host", "0.0.0.0")
        self.listen_port = int(raw["listen_port"])

        self.base_display_number = int(raw["base_display_number"])
        if self.base_display_number < 1:
            raise ConfigError(
                "base_display_number must be at least 1; display :0 is "
                "reserved for a physical console."
            )
        self.max_user_sessions = int(raw["max_user_sessions"])
        if self.max_user_sessions < 1:
            raise ConfigError("max_user_sessions must be at least 1.")
        self.session_idle_timeout_seconds = int(
            raw["session_idle_timeout_seconds"]
        )
        self.session_resolution = raw["session_resolution"]

        self.vnc_backend = (
            str(raw.get("vnc_backend") or "tigervnc").strip().lower()
        )
        self.gpu_acceleration = bool(raw.get("gpu_acceleration", False))

        self.novnc_dir = Path(_require(raw.get("novnc_dir"), "novnc_dir")).resolve()

    def display_number(self, slot):
        return self.base_display_number + int(slot)


def load(config_path):
    with open(config_path, "r", encoding="utf-8") as handle:
        return Config(json.load(handle))
