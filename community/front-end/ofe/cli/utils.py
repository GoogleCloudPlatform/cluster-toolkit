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

"""Useful util functions for the cli"""

from pathlib import Path
import json
import yaml
import requests
import sys

g_config = {"server": {}, "loaded": False}


def load_config():
    if not g_config["loaded"]:
        config_file = str(Path.home()) + "/.ghpcfe/config"
        p = Path(config_file)
        if p.is_file():
            with p.open("r", encoding="utf-8") as f:
                g_config.update(yaml.safe_load(f)["config"])
                g_config["loaded"] = True
        else:
            invoc = sys.argv[0]
            if invoc.startswith("python"):
                invoc = f"{sys.argv[0]} {sys.argv[1]}"
            print(
                "Please first initialise this application by calling "
                f"`{invoc} config`."
            )
    return g_config


def get_model_state(config, table, key=None):
    url = f"{config['server']['url']}/api/{table}/"
    if key:
        url += f"{key}/"
    headers = {"Authorization": f"Token {config['server']['accessKey']}"}
    resp = requests.get(url, headers=headers)
    if not resp.ok:
        resp.raise_for_status()
    state = resp.json()
    return json.dumps(state)


def model_create(config, table, data):
    url = f"{config['server']['url']}/api/{table}/"
    headers = {"Authorization": f"Token {config['server']['accessKey']}"}
    resp = requests.post(url, data=data, headers=headers)
    if not resp.ok:
        resp.raise_for_status()
    state = resp.json()
    return json.dumps(state)


def print_json(json_str):
    parsed = json.loads(json_str)
    print(json.dumps(parsed, indent=4))


def notimplementedyet(placeholder_function):
    """Decorator method for unimplemented feature warnings"""

    def notimpl_version(*args, **kwargs):
        print(
            f"Error: The function {placeholder_function.__name__} is not yet "
            "implemented in the cli. Please use the web frontend to perform "
            "this action."
        )

    return notimpl_version
