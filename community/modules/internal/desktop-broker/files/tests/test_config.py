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

"""Configuration validation.

Every case here is a misconfiguration that would otherwise surface as a broken
desktop for a user rather than a failure to start.
"""

import pytest
from conftest import base_config

from desktop_broker.config import Config, ConfigError



def test_proxy_secret_required(tmp_path):
    with pytest.raises(ConfigError, match="proxy_secret is required"):
        Config(base_config(tmp_path, proxy_secret=""))



def test_unknown_identity_mode_rejected(tmp_path):
    with pytest.raises(ConfigError, match="Unsupported identity_mode"):
        Config(base_config(tmp_path, identity_mode="password"))









def test_display_zero_rejected(tmp_path):
    with pytest.raises(ConfigError, match="display :0 is"):
        Config(base_config(tmp_path, base_display_number=0))


def test_zero_sessions_rejected(tmp_path):
    with pytest.raises(ConfigError, match="max_user_sessions"):
        Config(base_config(tmp_path, max_user_sessions=0))


def test_defaults(tmp_path):
    config = Config(base_config(tmp_path))
    assert config.identity_mode == "trusted_proxy"
    assert config.vnc_backend == "tigervnc"


def test_novnc_dir_required(tmp_path):
    raw = base_config(tmp_path)
    del raw["novnc_dir"]
    with pytest.raises(ConfigError, match="novnc_dir is required"):
        Config(raw)


def test_display_number_follows_the_slot(tmp_path):
    config = Config(base_config(tmp_path, base_display_number=1))
    assert config.display_number(0) == 1
    assert config.display_number(31) == 32
