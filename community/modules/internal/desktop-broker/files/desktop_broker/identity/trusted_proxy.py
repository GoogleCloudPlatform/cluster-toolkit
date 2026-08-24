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

"""Identity taken from request headers, unverified.

Only safe where an authenticating proxy is the sole route to the broker; the
shared proxy secret is otherwise the only control.
"""

from ..errors import BrokerError
from .common import default_oslogin_username, identity


def resolve(presented, config):
    email = presented["email"].lower()
    if not email:
        raise BrokerError(403, "Missing desktop user identity header.")
    # The proxy is trusted to have authenticated the user, so it is also the
    # best source for the POSIX account name. Deriving one from the email is
    # only correct for projects that have not customised the OS Login username
    # format, so prefer an explicit header.
    username_hint = presented["username"] or default_oslogin_username(email)
    return identity(
        email=email,
        login_uid=presented["login_uid"],
        username_hint=username_hint,
    )
