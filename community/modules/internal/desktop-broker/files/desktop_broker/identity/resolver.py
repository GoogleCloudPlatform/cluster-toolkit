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

"""Establishing who is asking.

One module per trust model, each returning the same identity shape. Adding a
model means adding a module and an entry in _RESOLVERS - nothing else in the
broker changes.

Named rather than living in a package __init__: gcluster embeds these modules
with go:embed, which silently skips files whose names start with an underscore,
so an __init__.py would never reach a deployed host.
"""

import hmac
import logging

from ..errors import BrokerError
from . import trusted_proxy

LOG = logging.getLogger("ghpc-desktop-broker")

_RESOLVERS = {
    "trusted_proxy": trusted_proxy.resolve,
}

# Headers the broker reads. Named once here so the set is auditable.
HEADERS = {
    "secret": "X-Cluster-Desktop-Secret",
    "email": "X-Cluster-Desktop-Email",
    "login_uid": "X-Cluster-Desktop-Login-Uid",
    "username": "X-Cluster-Desktop-Username",
}


def presented_from_headers(headers):
    """Extract every header the resolvers may look at."""
    return {
        key: str(headers.get(header, "") or "").strip()
        for key, header in HEADERS.items()
    }


class Resolver:
    """Applies the configured trust model to a request's headers."""

    def __init__(self, config):
        self.config = config
        self._resolve = _RESOLVERS[config.identity_mode]

        if config.identity_mode == "trusted_proxy":
            LOG.warning(
                "identity_mode=trusted_proxy: desktop identity is taken from "
                "request headers with no token verification. Only use this "
                "where an authenticating proxy is the sole route to this "
                "broker."
            )

    def resolve(self, presented):
        """Verify the shared secret, then apply the configured trust model."""
        if not hmac.compare_digest(
            presented["secret"].encode("utf-8"),
            self.config.proxy_secret.encode("utf-8"),
        ):
            raise BrokerError(403, "Missing or invalid desktop proxy secret.")
        return self._resolve(presented, self.config)
