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

"""Entrypoint: python3 -m desktop_broker.main --config /path/to/config.json

Not __main__.py, for the same reason there are no __init__.py files here:
gcluster embeds these modules with go:embed, which skips underscore-prefixed
names, so a file called __main__.py would never reach a deployed host.
"""

import argparse
import logging

from aiohttp import web

from . import app as broker_app
from . import config as broker_config

LOG = logging.getLogger("ghpc-desktop-broker")


def main(argv=None):
    parser = argparse.ArgumentParser(prog="desktop_broker.main")
    parser.add_argument(
        "--config",
        required=True,
        help="Path to the desktop broker JSON config file.",
    )
    args = parser.parse_args(argv)

    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )

    try:
        config = broker_config.load(args.config)
        app = broker_app.build(config)
    except broker_config.ConfigError as err:
        LOG.error("Desktop broker configuration is invalid: %s", err)
        raise SystemExit(1) from err

    web.run_app(
        app,
        host=config.listen_host,
        port=config.listen_port,
        access_log=LOG,
    )


if __name__ == "__main__":
    main()
