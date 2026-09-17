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

from concurrent.futures import CancelledError, ThreadPoolExecutor, wait
from concurrent.futures.thread import _threads_queues, _worker
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
import http.client
import logging
import queue
import socket
import ssl
import sys
import threading
from typing import Any, Optional
import weakref

from googleapiclient.errors import HttpError  # type: ignore
import httplib2
import local_pubsub
import util

log = logging.getLogger()

# Name of the topic
TOPIC = "watch_delete_vm_op"

# Maximum message age before force-acknowledging (TTL: 6 hours)
MAX_MESSAGE_AGE_HOURS = 6
MAX_MESSAGE_AGE_SECONDS = MAX_MESSAGE_AGE_HOURS * 3600

# Concurrency and execution bounds for slurmsync watch cycle
MAX_WATCH_WORKERS = 16
SLURMSYNC_WATCH_TIMEOUT_SECONDS = 30.0

# HTTP status code categories
PERMANENT_HTTP_ERRORS = {400, 403, 404, 410}
TRANSIENT_HTTP_ERRORS = {401, 408, 429, 500, 502, 503, 504}

TRANSIENT_NETWORK_EXCEPTIONS = (
    socket.timeout,
    socket.error,
    socket.gaierror,
    TimeoutError,
    ConnectionError,
    ssl.SSLError,
    http.client.HTTPException,
    httplib2.HttpLib2Error,
)


@dataclass(frozen=True)
class WatchDeleteVmOp_Message:
    op_name: str
    zone: str
    node: str

    @classmethod
    def from_dict(cls, data: Any) -> "WatchDeleteVmOp_Message":
        """Constructs WatchDeleteVmOp_Message defensively from a dictionary payload."""
        if not isinstance(data, dict):
            raise ValueError(
                f"Expected dict for message payload, got {type(data).__name__}"
            )
        op_name = data.get("op_name")
        zone = data.get("zone")
        node = data.get("node")
        if not op_name or not node:
            raise ValueError(
                f"Missing required fields ('op_name', 'node') in message payload: {data}"
            )
        return cls(
            op_name=util.to_leaf_name(str(op_name)),
            zone=util.to_leaf_name(str(zone or "")),
            node=str(node).strip(),
        )


class WatchDeleteVmOp_Topic:
    def __init__(self, topic: local_pubsub.Topic) -> None:
        self._t = topic

    def publish(self, op: dict[str, Any], node: str) -> None:
        """Publishes a delete VM operation watch message defensively.

        Validates inputs defensively to avoid crashing suspend.py after VM
        deletion calls have already been dispatched.
        """
        if not node or not str(node).strip():
            log.error(
                "Cannot publish delete VM operation watch message: node name is missing or empty"
            )
            return

        if not isinstance(op, dict):
            log.error(
                f"Cannot publish delete VM operation watch message for node {node}: "
                f"expected dict for op, got {type(op).__name__}"
            )
            return

        op_name = op.get("name")
        if not op_name:
            log.error(
                f"Cannot publish delete VM operation watch message for node {node}: "
                f"missing 'name' in operation payload: {op}"
            )
            return

        zone = op.get("zone")
        if not zone:
            log.error(
                f"Cannot publish delete VM operation watch message for node {node}: "
                f"missing 'zone' in operation payload: {op}"
            )
            return

        op_type = op.get("operationType")
        if op_type and op_type != "delete":
            log.warning(
                f"Operation {op_name} for node {node} has unexpected operationType "
                f"'{op_type}' (expected 'delete')"
            )

        # Store leaf zone name for cleanliness and compactness
        leaf_zone = util.to_leaf_name(zone)
        leaf_op_name = util.to_leaf_name(op_name)

        if not leaf_op_name or not leaf_zone:
            log.error(
                f"Cannot publish delete VM operation watch message for node {node}: "
                f"sanitized op_name ('{leaf_op_name}') or zone ('{leaf_zone}') is empty"
            )
            return

        msg = WatchDeleteVmOp_Message(
            op_name=leaf_op_name, zone=leaf_zone, node=str(node).strip()
        )
        self._t.publish(data=asdict(msg))


def watch_delete_vm_op_topic() -> WatchDeleteVmOp_Topic:
    return WatchDeleteVmOp_Topic(local_pubsub.topic(TOPIC))


def _extract_http_status(e: HttpError) -> Optional[int]:
    """Extract integer HTTP status code from HttpError."""
    status = getattr(e, "status_code", None)
    if status is not None:
        try:
            return int(status)
        except (ValueError, TypeError):
            pass
    if hasattr(e, "resp") and hasattr(e.resp, "status"):
        try:
            return int(e.resp.status)
        except (ValueError, TypeError):
            pass
    return None


def _watch_op(lkp: util.Lookup, m: WatchDeleteVmOp_Message) -> bool:
    """Processes VM delete-operation.

    If operation is still running - returns False (try later / NACK).
    If operation failed - logs error & returns True (ACK).
    If operation succeeded - returns True (ACK).

    To avoid querying status for each op individually, uses list of VM instances
    as a primary filter.

    Returns True if message should be marked as processed (ACK), False to retry (NACK).
    """
    inst = lkp.instance(m.node)

    if not inst:
        log.debug(f"Stop watching op {m.op_name}, VM {m.node} appears to be deleted")
        return True  # ack, instance absent

    if inst.status == "TERMINATED":
        log.debug(f"Stop watching op {m.op_name}, VM {m.node} is TERMINATED")
        return True  # ack, instance terminated

    if inst.status == "STOPPING":
        log.debug(f"Skipping op {m.op_name}, VM {m.node} is STOPPING")
        return False  # try later / nack

    try:
        req = util.get_operation_req(lkp, m.op_name, zone=m.zone)
        op = util.ensure_execute(req)
    except HttpError as e:
        status = _extract_http_status(e)
        if status in PERMANENT_HTTP_ERRORS or (
            status is not None and 400 <= status < 500 and status not in TRANSIENT_HTTP_ERRORS
        ):
            log.warning(
                f"Permanent HTTP {status} error querying operation {m.op_name} "
                f"for node {m.node}: {e}. Acknowledging message to prevent queue starvation."
            )
            return True  # ack
        elif status in TRANSIENT_HTTP_ERRORS or (status is not None and status >= 500):
            log.warning(
                f"Transient HTTP {status} error querying operation {m.op_name} "
                f"for node {m.node}: {e}. Will retry in subsequent cycle."
            )
            return False  # nack
        else:
            log.warning(
                f"Unclassified HttpError ({status}) querying operation {m.op_name} "
                f"for node {m.node}: {e}. Acknowledging defensively."
            )
            return True  # defensive ack
    except TRANSIENT_NETWORK_EXCEPTIONS as e:
        log.warning(
            f"Transient network exception querying operation {m.op_name} "
            f"for node {m.node}: {e}. Will retry in subsequent cycle."
        )
        return False  # nack
    except Exception as e:
        log.exception(
            f"Unexpected error processing operation {m.op_name} for node {m.node}, "
            f"acknowledging to avoid queue poisoning: {e}"
        )
        return True  # defensive ack

    if not isinstance(op, dict) or op.get("status") != "DONE":
        status_str = op.get("status") if isinstance(op, dict) else "UNKNOWN"
        log.debug(f"Watching op {m.op_name} is still not done ({status_str})")
        return False  # try later / nack

    if "error" in op:
        log.error(
            f"Operation {m.op_name} to delete {m.node} finished with error: {op['error']}"
        )
    else:
        log.debug(f"Operation {m.op_name} to delete {m.node} successfully finished")
    return True  # ack


def _is_message_expired(m: local_pubsub.Message, now: datetime) -> bool:
    """Check if message exceeds TTL."""
    created = getattr(m, "created", None)
    if created is None:
        return False
    if isinstance(created, str):
        try:
            created = datetime.fromisoformat(created)
        except (ValueError, TypeError):
            return False
    if not isinstance(created, datetime):
        return False
    if created.tzinfo is None:
        created = created.replace(tzinfo=timezone.utc)
    if now.tzinfo is None:
        now = now.replace(tzinfo=timezone.utc)
    return (now - created).total_seconds() > MAX_MESSAGE_AGE_SECONDS


class _DaemonThreadPoolExecutor(ThreadPoolExecutor):
    """ThreadPoolExecutor that creates daemon worker threads.

    By default, standard ThreadPoolExecutor threads are non-daemon and are joined
    by Python's atexit handler (_python_exit), which can cause slurmsync to hang on
    exit if workers are stuck on long-running network requests or retries.
    This subclass marks worker threads as daemon, avoids registering them in the
    global atexit queue (_threads_queues), and defensively evicts them on non-waiting
    shutdown to ensure worker threads never block process termination or hold the
    slurmsync PID lock.
    """

    def _adjust_thread_count(self) -> None:
        if self._idle_semaphore.acquire(timeout=0):
            return

        def weakref_cb(_, q=self._work_queue):
            q.put(None)

        num_threads = len(self._threads)
        if num_threads < self._max_workers:
            thread_name = f"{self._thread_name_prefix or self}_{num_threads}"
            t = threading.Thread(
                name=thread_name,
                target=_worker,
                args=(
                    weakref.ref(self, weakref_cb),
                    self._work_queue,
                    self._initializer,
                    self._initargs,
                ),
                daemon=True,
            )
            t.start()
            self._threads.add(t)  # type: ignore[attr-defined]
            # Note: We intentionally do NOT register t in _threads_queues.
            # In standard ThreadPoolExecutor, _threads_queues[t] = self._work_queue
            # causes _python_exit to join worker threads on interpreter shutdown,
            # even for daemon threads. Omitting registration ensures that these
            # daemon threads will never block interpreter shutdown.

    def shutdown(self, wait: bool = True, *, cancel_futures: bool = False) -> None:
        if sys.version_info >= (3, 9):
            super().shutdown(wait=wait, cancel_futures=cancel_futures)
        else:
            if cancel_futures:
                while True:
                    try:
                        work_item = self._work_queue.get_nowait()
                    except queue.Empty:
                        break
                    if work_item is not None:
                        work_item.future.cancel()
            super().shutdown(wait=wait)
        if not wait:
            for t in list(self._threads):
                _threads_queues.pop(t, None)  # type: ignore[attr-defined]


def watch_vm_delete_ops(lkp: util.Lookup) -> None:
    """Polls and monitors VM deletion operations with concurrency and TTL handling."""
    sub = local_pubsub.subscription(TOPIC)

    msgs = sub.pull(max_messages=1000)
    if not msgs:
        return

    log.debug(f"Processing {len(msgs)} delete VM operations")

    now = util.now()
    to_ack: list[str] = []
    to_nack: list[str] = []
    pending_api_checks: list[tuple[local_pubsub.Message, WatchDeleteVmOp_Message]] = []

    try:
        # Phase 1: In-memory evaluation and fast-path pruning
        for m in msgs:
            # Check TTL
            if _is_message_expired(m, now):
                created = getattr(m, "created", None)
                if created is not None and hasattr(created, "isoformat"):
                    created_str = created.isoformat()
                else:
                    created_str = str(created or "unknown")
                log.error(
                    f"Message {m.id} (created {created_str}) exceeded maximum TTL of "
                    f"{MAX_MESSAGE_AGE_HOURS}h. Acknowledging to prevent queue starvation."
                )
                to_ack.append(m.id)
                continue

            # Unpack message payload
            try:
                dm = WatchDeleteVmOp_Message.from_dict(m.data)
            except Exception:
                log.exception(
                    f"Failed to parse payload for message {m.id}, removing from queue: {m.data}"
                )
                to_ack.append(m.id)
                continue

            # In-memory fast path
            inst = lkp.instance(dm.node)
            if not inst:
                log.debug(
                    f"Stop watching op {dm.op_name}, VM {dm.node} appears to be deleted"
                )
                to_ack.append(m.id)
                continue

            if inst.status == "TERMINATED":
                log.debug(
                    f"Stop watching op {dm.op_name}, VM {dm.node} is TERMINATED"
                )
                to_ack.append(m.id)
                continue

            if inst.status == "STOPPING":
                log.debug(f"Skipping op {dm.op_name}, VM {dm.node} is STOPPING")
                to_nack.append(m.id)
                continue

            pending_api_checks.append((m, dm))

        # Phase 2: Parallel GCE API polling for remaining operations
        if pending_api_checks:
            num_workers = min(MAX_WATCH_WORKERS, len(pending_api_checks))
            exe = _DaemonThreadPoolExecutor(
                max_workers=num_workers,
                thread_name_prefix="watch_delete_vm_op",
            )
            try:
                future_to_msg = {
                    exe.submit(_watch_op, lkp, dm): m
                    for m, dm in pending_api_checks
                }

                done, not_done = wait(
                    future_to_msg.keys(), timeout=SLURMSYNC_WATCH_TIMEOUT_SECONDS
                )

                for future in done:
                    m = future_to_msg[future]
                    if future.cancelled():
                        log.warning(
                            f"Watch worker for message {m.id} was cancelled. Retrying next cycle."
                        )
                        to_nack.append(m.id)
                        continue
                    try:
                        ack = future.result()
                    except CancelledError:
                        log.warning(
                            f"Watch worker for message {m.id} was cancelled. Retrying next cycle."
                        )
                        ack = False
                    except Exception:
                        log.exception(
                            f"Unhandled exception in watch worker for message {m.id}"
                        )
                        ack = True  # Defensive ACK
                    if ack:
                        to_ack.append(m.id)
                    else:
                        to_nack.append(m.id)

                for future in not_done:
                    m = future_to_msg[future]
                    log.warning(
                        f"Watch query for message {m.id} timed out after "
                        f"{SLURMSYNC_WATCH_TIMEOUT_SECONDS}s. NACKing for next cycle."
                    )
                    to_nack.append(m.id)
            finally:
                exe.shutdown(wait=False, cancel_futures=True)
    finally:
        # Phase 3: Atomic Batch ACK and NACK execution
        if to_ack:
            sub.ack(to_ack)
        if to_nack:
            sub.modify_ack_deadline(to_nack, deadline=0)
