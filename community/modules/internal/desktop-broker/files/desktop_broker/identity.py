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

"""Establishing who is asking: the kernel's word, not the client's.

The broker listens only on a unix socket, so every request comes from a process
on this host, and SO_PEERCRED names the account that process runs as. A browser
reaches the socket through an SSH forward ("ssh -L 6080:<socket>"), and sshd
opens that connection as the user who logged in, after OS Login has
authenticated them. A local user connecting directly is likewise only ever
themselves. Nothing a client sends can change the answer, so there is no shared
secret to hold and no header to forge.
"""

import pwd
import socket
import struct

from .errors import BrokerError

# struct ucred: pid_t pid, uid_t uid, gid_t gid. uid_t and gid_t are unsigned,
# and OS Login hands out uids above 2**31, so they must not be read as int.
_UCRED = struct.Struct("iII")


def peer_uid(transport):
    """The uid of the process at the other end of a unix-socket connection."""
    connection = transport.get_extra_info("socket") if transport else None
    try:
        _, uid, _ = _UCRED.unpack(
            connection.getsockopt(
                socket.SOL_SOCKET, socket.SO_PEERCRED, _UCRED.size
            )
        )
    except (AttributeError, OSError, struct.error) as err:
        # AttributeError covers a platform without SO_PEERCRED and a missing
        # socket alike: either way there is no identity to go on.
        raise BrokerError(403, "Could not identify the connecting user.") from err
    return uid


def resolve(uid):
    """The account a desktop is started for.

    Root is refused outright rather than left to fail the OS Login check: a
    desktop running as root is never what was meant.
    """
    if uid == 0:
        raise BrokerError(403, "Desktop sessions are not started for root.")
    try:
        return pwd.getpwuid(uid).pw_name
    except KeyError as err:
        raise BrokerError(
            403, "The connecting user has no account on this host."
        ) from err
