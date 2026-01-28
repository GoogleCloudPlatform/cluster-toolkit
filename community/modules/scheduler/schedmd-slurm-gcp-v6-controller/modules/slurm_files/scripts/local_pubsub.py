# Copyright 2026 Google LLC
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


"""
Implementation of message queue that mimics interface of GCP (PubSub)[https://cloud.google.com/pubsub]

Messages are stored on controller state disk (to survive controller re-creation) with following layout:

/<controller_state_disk_mount>/<pubsub_folder>
├- <TOPIC>
|  └- <MESSAGE_ID>
└- .staging
   └- <TOPIC>
      └- <MESSAGE_ID>

One message is one immutable file, that will be deleted after acknowledgement.
NOTE: Implementation assumes that both `<TOPIC>` and `.staging/<TOPIC>` are on the same disk device,
so it can rely on atomic "move / rename" operation.
"""
from typing import Any
import util
import json
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
import os
import uuid

import logging
log = logging.getLogger()


@dataclass(frozen=True)
class Message:
    id: str
    created: datetime
    data: Any

    def to_json(self) -> dict[str, str]:
        return dict(
            id=self.id,
            created=self.created.isoformat(),
            data=self.data)
    
    @classmethod
    def from_json(cls, data: dict[str, str]) -> 'Message':
        return cls(
            id=data['id'],
            created=datetime.fromisoformat(data['created']),
            data=data['data'])

class Topic:
    """
    Acts as PubSub topic (https://cloud.google.com/pubsub/docs/reference/rest/v1/projects.topics).
    We can have multiple instances of 
    """
    def __init__(self, path: Path, staging: Path) -> None:
        self._path = path
        self._staging = staging
    
    def _gen_id(self, created: datetime) -> str:
        ts = created.strftime("%Y_%m_%d-%H_%M_%S")
        suf = str(uuid.uuid4())[:8]
        return f"{ts}-{suf}"
    
    def publish(self, data: Any) -> None:
        created = util.now()
        id = self._gen_id(created)
        msg = Message(id=id, created=created, data=data)
        
        staged = self._staging / msg.id
        dst = self._path / msg.id

        # Write to stagin area first then perform atomic move
        # to prevent "reads of partial writes"
        staged.write_text(json.dumps(msg.to_json()))
        util.chown_slurm(staged)
        staged.rename(dst)


class Subscription:
    """
    Acts as PubSub subscription (https://cloud.google.com/pubsub/docs/reference/rest/v1/projects.subscriptions)
    with following settings:

    ```
    ackDeadlineSeconds = +Inf     # don't resend message that was already being delivered but not acked yet
    retainAckedMessages = False   # don't persist messages that were already acked 
    enableMessageOrdering = True  # delivers messages in chronoligical order
    messageRetentionDuration = +Inf # don't expire messages  
    deadLetterPolicy = None         # "deadlettering" is disabled, subscriber should take care of any poisonous messages
    retryPolicy = {                 # NACKed message will be re-delievered after some time
        minimumBackoff = 30s        # NOTE: Practically there is no timer, but Subscription instance will not try to re-deliver NACKed messages.
        maximumBackoff = 30s        # Assumes that slurmsync runs every 30+ sec.
    }
    ```

    IMPORTANT: Should only be run as part of slurmsync, 
    this is our way to ensure that at most one instance exists at a time.
    There is no concurancy safeguards in place, avoid multithreaded `pull`,
    while multithreaded `ack` & `modify_ack_deadline` are OK.
    """

    def __init__(self, path: Path) -> None:
        self._path: Path = path
        # contains ALL messages pulled by this subscription instance
        # both acked, nacked, and still being processed
        # used to prevent double delivery within lifetime of subscription (slurmsync)
        self._pulled: set[str] = set()

    def _delete(self, id: str) -> None:
        log.debug(f"removing {id}")
        try:
            os.unlink(self._path / id)
        except:
            log.exception(f"Failed to remove message {id}")
    
    def _read_msg(self, id: str) -> Message | None:
        try:
            with open(self._path / id, 'r') as f:
                content = json.loads(f.read())
                return Message.from_json(content)
        except Exception:
            log.exception(f"Failed to read message {id}")
            self._delete(id) # delete message to reduce "deadlettering"
            return None

    def pull(self, max_messages: int) -> list[Message]:
        if not self._path.exists():
            log.warning(f"Topic {self._path} does not exist")
            return []
        res = []
        ls = sorted(os.listdir(self._path))
        for name in ls:
            msg = self._read_msg(name)
            if msg is not None and msg.id not in self._pulled:
                self._pulled.add(msg.id)
                res.append(msg)

            if len(res) >= max_messages:
                break
        return res


    def ack(self, ids: list[str]) -> None:
        for id in ids:
            self._delete(id)


    def modify_ack_deadline(self, ids: list[str], deadline: int) -> None:
        """
        Modifies the ack deadline for a specific message. 
        IMPORTANT: Only accepts deadline=0, which is a way to NACK
        Any other values are also meaningless due to ackDeadlineSeconds==+Inf
        """
        assert deadline == 0 # no op, next subscriber (slurmsync) will pick this up


# Topics and Subscriptions are singletons
# TODO: consider making thread-safe
_topics = {}
_subscriptions = {}

def _make_path(name: str) -> Path:
    p = util.slurmdirs.state / "pubsub" / name
    p.mkdir(parents=True, exist_ok=True)
    util.chown_slurm(p)
    return p

def _make_staging_path(name: str) -> Path:
    p = util.slurmdirs.state / "pubsub" / ".staging" / name
    p.mkdir(parents=True, exist_ok=True)
    util.chown_slurm(p)
    return p

def topic(name: str) -> Topic:
    if name not in _topics:
        _topics[name] = Topic(_make_path(name), _make_staging_path(name))
    return _topics[name]

def subscription(name: str) -> Subscription:
    if name not in _subscriptions:
        _subscriptions[name] = Subscription(_make_path(name))
    return _subscriptions[name]
