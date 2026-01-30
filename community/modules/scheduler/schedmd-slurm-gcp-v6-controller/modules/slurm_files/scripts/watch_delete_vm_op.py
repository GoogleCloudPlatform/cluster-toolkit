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

from typing import Any


from dataclasses import dataclass, asdict
import util
import local_pubsub 

import logging
log = logging.getLogger()

# Name of the topic
TOPIC = "watch_delete_vm_op"

@dataclass(frozen=True)
class WatchDeleteVmOp_Message:
    op_name: str
    zone: str
    node: str

class WatchDeleteVmOp_Topic:
    def __init__(self, topic: local_pubsub.Topic) -> None:
        self._t = topic

    def publish(self, op: dict[str, Any], node: str) -> None:
        assert op.get("operationType") == "delete"
        assert op.get("zone")
        assert node

        msg = WatchDeleteVmOp_Message(op_name=op["name"], zone=op["zone"], node=node)
        self._t.publish(data=asdict(msg))


def watch_delete_vm_op_topic() -> WatchDeleteVmOp_Topic:
    return WatchDeleteVmOp_Topic(local_pubsub.topic(TOPIC))


def _watch_op(lkp: util.Lookup, m: WatchDeleteVmOp_Message) -> bool:
    """
    Processes VM delete-operation.
    If operation is still running - do nothing
    If operation failed - log error & remove op from watch list
    If operation is done - remove op from watch list do nothing
    
    To avoid querying status for each op individually, use list of VM instances as 
    a source of data. Don't query op for instance X if instance X is not present 
    (presumably deleted).
    NOTE: This optimization can lead to false-positives -
    absence of error-logs in case op failed, but VM got deleted by other means.

    Returns True if message should be marked as processed (ack).
    """

    inst = lkp.instance(m.node)
    
    if not inst:
        log.debug(f"Stop watching op {m.op_name}, VM {m.node} appears to be deleted")
        return True # ack, potentially false-positive

    if inst.status == "TERMINATED":
        log.debug(f"Stop watching op {m.op_name}, VM {m.node} is TERMINATED")
        return True # ack, potentially false-positive

    if inst.status == "STOPPING":
        log.debug(f"Skipping op {m.op_name}, VM {m.node} is STOPPING")
        return False # try later

    try:
        op = util.get_operation_req(lkp, m.op_name, zone=m.zone).execute()
    except:
        # TODO: consider less conservative handling, but be careful not to cause deadlettering.
        log.exception(f"Failed to get operation {m.op_name}, will not retry")
        return True # ack (remove)

    if op["status"] != "DONE":
        log.debug(f"Watching op {m.op_name} is still not done ({op['status']})")
        return False # try later

    if "error" in op:
        log.error(f"Operation {m.op_name} to delete {m.node} finished with error: {op['error']}")
    else:
        log.debug(f"Operation {m.op_name} to delete {m.node} successfully finished")
    return True # ack


def watch_vm_delete_ops(lkp: util.Lookup) -> None:
    sub = local_pubsub.subscription(TOPIC)

    # Pull once instead of "pulling until empty", motivation:
    # Bulk of cases processed by `_watch_op` relies on freshness of `lkp.instances`,
    # `lkp.instances` are fetched once during run of `slurmsync`.
    # Therefore we shouldn't try to re-process messages that has been already NACKed in this run,
    # since they will be handled with the same `lkp.instance` as a previous attempt.
    msgs = sub.pull(max_messages=1000) # 1000 is arbitrary number to be adjusted if needed.
    log.debug(f"Processing {len(msgs)} delete VM operations")
    # TODO: handle messages in butches to improve latency
    for m in msgs:
        try:
            dm = WatchDeleteVmOp_Message(**m.data)
            ack = _watch_op(lkp, dm)
        except Exception:
            log.exception(f"Failed to process the message {m.id}, removing")
            ack = True
        if ack:
            sub.ack([m.id])
        else:
            sub.modify_ack_deadline([m.id], deadline=0) # NACK
        

        
        
