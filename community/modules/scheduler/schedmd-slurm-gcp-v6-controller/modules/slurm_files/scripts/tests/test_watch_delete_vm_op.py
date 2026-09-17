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

from dataclasses import asdict
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import socket
import unittest
from unittest.mock import MagicMock, patch

from googleapiclient.errors import HttpError  # type: ignore
import httplib2
import pytest

from common import TstCfg, TstNodeset
import local_pubsub
import util
import watch_delete_vm_op
from watch_delete_vm_op import (
    MAX_MESSAGE_AGE_HOURS,
    WatchDeleteVmOp_Message,
    WatchDeleteVmOp_Topic,
    _watch_op,
    watch_vm_delete_ops,
)


def make_http_error(status: int, reason: str = "Error") -> HttpError:
    resp = httplib2.Response({"status": status, "reason": reason})
    resp.status = status
    resp.reason = reason
    return HttpError(resp, reason.encode("utf-8"))


# ==============================================================================
# Topic Publication & Defensive Validation Tests
# ==============================================================================

def test_publish_valid(tmp_path):
    p = tmp_path / "topic"
    s = tmp_path / "staging"
    p.mkdir()
    s.mkdir()
    raw_topic = local_pubsub.Topic(p, s)
    topic = WatchDeleteVmOp_Topic(raw_topic)

    op = {
        "name": "https://.../operations/operation-1234/",
        "zone": "https://.../zones/us-central1-a/",
        "operationType": "delete",
    }
    topic.publish(op, "node-1")

    sub = local_pubsub.Subscription(p)
    msgs = sub.pull(10)
    assert len(msgs) == 1
    assert msgs[0].data == {
        "op_name": "operation-1234",
        "zone": "us-central1-a",
        "node": "node-1",
    }


def test_publish_defensive_missing_fields(tmp_path):
    """Ensure publish never crashes with AssertionError on invalid payloads."""
    p = tmp_path / "topic"
    s = tmp_path / "staging"
    p.mkdir()
    s.mkdir()
    raw_topic = local_pubsub.Topic(p, s)
    topic = WatchDeleteVmOp_Topic(raw_topic)

    # Empty node
    topic.publish({"name": "op1", "zone": "us-central1-a", "operationType": "delete"}, "")
    # Missing zone
    topic.publish({"name": "op2", "operationType": "delete"}, "node-1")
    # Missing name
    topic.publish({"zone": "us-central1-a", "operationType": "delete"}, "node-1")
    # Non-dict op
    topic.publish(None, "node-1")  # type: ignore

    sub = local_pubsub.Subscription(p)
    assert len(sub.pull(10)) == 0


def test_publish_unexpected_operation_type(tmp_path):
    """Unexpected operationType logs warning but publishes gracefully."""
    p = tmp_path / "topic"
    s = tmp_path / "staging"
    p.mkdir()
    s.mkdir()
    raw_topic = local_pubsub.Topic(p, s)
    topic = WatchDeleteVmOp_Topic(raw_topic)

    op = {"name": "op1", "zone": "us-central1-a", "operationType": "insert"}
    topic.publish(op, "node-1")

    sub = local_pubsub.Subscription(p)
    msgs = sub.pull(10)
    assert len(msgs) == 1
    assert msgs[0].data["node"] == "node-1"


def test_publish_sanitized_empty_leaf(tmp_path):
    """Ensure publish rejects op payload if name or zone sanitizes to empty string."""
    p = tmp_path / "topic"
    s = tmp_path / "staging"
    p.mkdir()
    s.mkdir()
    raw_topic = local_pubsub.Topic(p, s)
    topic = WatchDeleteVmOp_Topic(raw_topic)

    # Sanitized name is empty
    topic.publish({"name": "///", "zone": "us-central1-a", "operationType": "delete"}, "node-1")
    # Sanitized zone is empty
    topic.publish({"name": "op-1", "zone": "   ///   ", "operationType": "delete"}, "node-1")

    sub = local_pubsub.Subscription(p)
    assert len(sub.pull(10)) == 0


def test_watch_delete_vm_op_message_from_dict():
    """Verify WatchDeleteVmOp_Message.from_dict normalizes URLs and handles invalid payloads."""
    # Valid dict with full self-link URLs
    msg = WatchDeleteVmOp_Message.from_dict({
        "op_name": "https://.../operations/op-123/",
        "zone": "https://.../zones/us-central1-a/",
        "node": "node-1",
        "extra_field": "ignore_me",  # Forward compatibility
    })
    assert msg.op_name == "op-123"
    assert msg.zone == "us-central1-a"
    assert msg.node == "node-1"

    # Non-dict payload
    with pytest.raises(ValueError, match="Expected dict"):
        WatchDeleteVmOp_Message.from_dict("invalid-json")

    # Missing node
    with pytest.raises(ValueError, match="Missing required fields"):
        WatchDeleteVmOp_Message.from_dict({"op_name": "op-1", "zone": "z1"})

    # Missing op_name
    with pytest.raises(ValueError, match="Missing required fields"):
        WatchDeleteVmOp_Message.from_dict({"node": "node-1", "zone": "z1"})


# ==============================================================================
# _watch_op Status & Error Classification Tests
# ==============================================================================

def test_watch_op_fast_path_instance_absent():
    lkp = MagicMock()
    lkp.instance.return_value = None
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    # Instance absent -> ACK (True) with 0 API calls
    assert _watch_op(lkp, msg) is True
    lkp.compute.zoneOperations.assert_not_called()


def test_watch_op_fast_path_instance_terminated():
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="TERMINATED")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    # Instance TERMINATED -> ACK (True) with 0 API calls
    assert _watch_op(lkp, msg) is True
    lkp.compute.zoneOperations.assert_not_called()


def test_watch_op_fast_path_instance_stopping():
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="STOPPING")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    # Instance STOPPING -> NACK (False) with 0 API calls
    assert _watch_op(lkp, msg) is False
    lkp.compute.zoneOperations.assert_not_called()


def test_watch_op_success_done():
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    with patch("util.ensure_execute", return_value={"status": "DONE"}):
        assert _watch_op(lkp, msg) is True


def test_watch_op_done_with_error():
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    with patch("util.ensure_execute", return_value={"status": "DONE", "error": {"code": "GCE_ERROR"}}):
        assert _watch_op(lkp, msg) is True


def test_watch_op_in_progress():
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    with patch("util.ensure_execute", return_value={"status": "RUNNING"}):
        assert _watch_op(lkp, msg) is False


@pytest.mark.parametrize("status_code", [400, 403, 404, 410])
def test_watch_op_permanent_http_errors(status_code):
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    with patch("util.ensure_execute", side_effect=make_http_error(status_code)):
        # Permanent error -> ACK (True)
        assert _watch_op(lkp, msg) is True


@pytest.mark.parametrize("status_code", [401, 408, 429, 500, 502, 503, 504])
def test_watch_op_transient_http_errors(status_code):
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    with patch("util.ensure_execute", side_effect=make_http_error(status_code)):
        # Transient error -> NACK (False)
        assert _watch_op(lkp, msg) is False


def test_watch_op_network_timeouts():
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    for exc in [socket.timeout("timed out"), ConnectionResetError(), TimeoutError()]:
        with patch("util.ensure_execute", side_effect=exc):
            assert _watch_op(lkp, msg) is False


def test_watch_op_httplib2_errors():
    """Verify httplib2.ServerNotFoundError and HttpLib2Error are treated as transient."""
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    for exc in [
        httplib2.ServerNotFoundError("Unable to find server"),
        httplib2.HttpLib2Error("Connection reset"),
    ]:
        with patch("util.ensure_execute", side_effect=exc):
            assert _watch_op(lkp, msg) is False


def test_watch_op_non_dict_or_none_response():
    """Verify non-dict or None op response from ensure_execute does not crash and returns False."""
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    for invalid_op in [None, "", "error-string", 123]:
        with patch("util.ensure_execute", return_value=invalid_op):
            assert _watch_op(lkp, msg) is False


def test_watch_op_unhandled_exception():
    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")
    msg = WatchDeleteVmOp_Message("op1", "us-central1-a", "node-1")

    with patch("util.ensure_execute", side_effect=RuntimeError("unexpected bug")):
        # Defensive fallback -> ACK (True) to prevent poison queue
        assert _watch_op(lkp, msg) is True


# ==============================================================================
# watch_vm_delete_ops Integration, TTL & Concurrency Tests
# ==============================================================================

def test_watch_vm_delete_ops_ttl_pruning(tmp_path):
    p = tmp_path / "topic"
    s = tmp_path / "staging"
    p.mkdir()
    s.mkdir()

    sub = local_pubsub.Subscription(p)
    raw_topic = local_pubsub.Topic(p, s)

    # Message 1: 7 hours old (expired)
    with patch("util.now", return_value=datetime.now(timezone.utc) - timedelta(hours=7)):
        raw_topic.publish(asdict(WatchDeleteVmOp_Message("op-expired", "us-central1-a", "node-expired")))

    # Message 2: 1 hour old (fresh)
    with patch("util.now", return_value=datetime.now(timezone.utc) - timedelta(hours=1)):
        raw_topic.publish(asdict(WatchDeleteVmOp_Message("op-fresh", "us-central1-a", "node-fresh")))

    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")

    with patch("local_pubsub.subscription", return_value=sub):
        with patch("util.ensure_execute", return_value={"status": "RUNNING"}):
            watch_vm_delete_ops(lkp)

    # Expired message must be ACKed (deleted from disk)
    # Fresh message must be NACKed (still on disk)
    remaining = local_pubsub.Subscription(p).pull(10)
    assert len(remaining) == 1
    assert remaining[0].data["node"] == "node-fresh"


def test_watch_vm_delete_ops_legacy_full_urls(tmp_path):
    """Verify legacy messages on disk with full URLs are resolved seamlessly."""
    p = tmp_path / "topic"
    s = tmp_path / "staging"
    p.mkdir()
    s.mkdir()

    sub = local_pubsub.Subscription(p)
    raw_topic = local_pubsub.Topic(p, s)

    # Simulate legacy message with full selfLink URL and trailing slash
    raw_topic.publish({
        "op_name": "https://.../operations/operation-legacy/",
        "zone": "https://www.googleapis.com/compute/v1/projects/my-proj/zones/us-central1-a/",
        "node": "legacy-node",
    })

    lkp = MagicMock()
    lkp.project = "my-proj"
    lkp.instance.return_value = MagicMock(status="RUNNING")

    with patch("local_pubsub.subscription", return_value=sub):
        with patch("util.ensure_execute", return_value={"status": "DONE"}) as mock_exec:
            watch_vm_delete_ops(lkp)
            assert mock_exec.call_count == 1

    # Verify zone was normalized to leaf name
    lkp.compute.zoneOperations().get.assert_called_with(
        project="my-proj", zone="us-central1-a", operation="operation-legacy"
    )
    # Message must be ACKed and deleted from disk
    assert len(local_pubsub.Subscription(p).pull(10)) == 0


def test_watch_vm_delete_ops_concurrency_and_batching(tmp_path):
    """Verify ThreadPoolExecutor processes bulk messages in parallel with batch ACK/NACK."""
    p = tmp_path / "topic"
    s = tmp_path / "staging"
    p.mkdir()
    s.mkdir()

    sub = local_pubsub.Subscription(p)
    raw_topic = local_pubsub.Topic(p, s)

    # Publish 20 messages
    for i in range(20):
        raw_topic.publish(asdict(WatchDeleteVmOp_Message(f"op-{i}", "us-central1-a", f"node-{i}")))

    lkp = MagicMock()
    # Even nodes are TERMINATED (fast path ACK)
    # Odd nodes < 10 are DONE (API ACK)
    # Odd nodes >= 10 are RUNNING (API NACK)
    def fake_instance(name):
        idx = int(name.split("-")[1])
        if idx % 2 == 0:
            return MagicMock(status="TERMINATED")
        return MagicMock(status="RUNNING")

    lkp.instance.side_effect = fake_instance

    def fake_ensure_execute(req):
        op_name = req._op_name
        idx = int(op_name.split("-")[1])
        if idx < 10:
            return {"status": "DONE"}
        return {"status": "RUNNING"}

    def fake_get_operation_req(lkp, name, zone=None):
        req = MagicMock()
        req._op_name = name
        return req

    with patch("local_pubsub.subscription", return_value=sub):
        with patch("util.get_operation_req", side_effect=fake_get_operation_req):
            with patch("util.ensure_execute", side_effect=fake_ensure_execute):
                watch_vm_delete_ops(lkp)

    # 10 even nodes + 5 odd nodes (< 10) = 15 ACKed (deleted)
    # 5 odd nodes (>= 10) = 5 NACKed (still on disk)
    remaining = local_pubsub.Subscription(p).pull(20)
    assert len(remaining) == 5
    for m in remaining:
        idx = int(m.data["node"].split("-")[1])
        assert idx >= 10 and idx % 2 == 1


def test_is_message_expired_edge_cases():
    """Verify _is_message_expired handles naive/aware timestamps, string dates, and boundary cases."""
    from watch_delete_vm_op import _is_message_expired, MAX_MESSAGE_AGE_HOURS

    now_utc = datetime.now(timezone.utc)
    now_naive = datetime.now(timezone.utc).replace(tzinfo=None)

    # m.created is None
    m_none = MagicMock(created=None)
    assert _is_message_expired(m_none, now_utc) is False

    # m.created is naive datetime
    m_naive = MagicMock(
        created=datetime.now(timezone.utc).replace(tzinfo=None) - timedelta(hours=7)
    )
    assert _is_message_expired(m_naive, now_utc) is True

    # now is naive datetime
    m_aware = MagicMock(created=now_utc - timedelta(hours=7))
    assert _is_message_expired(m_aware, now_naive) is True

    # m.created is ISO string
    m_str = MagicMock(created=(now_utc - timedelta(hours=7)).isoformat())
    assert _is_message_expired(m_str, now_utc) is True

    # Boundary test: exactly at TTL minus 10 seconds -> not expired
    m_boundary_fresh = MagicMock(
        created=now_utc - timedelta(hours=MAX_MESSAGE_AGE_HOURS, seconds=-10)
    )
    assert _is_message_expired(m_boundary_fresh, now_utc) is False

    # Boundary test: at TTL plus 10 seconds -> expired
    m_boundary_expired = MagicMock(
        created=now_utc - timedelta(hours=MAX_MESSAGE_AGE_HOURS, seconds=10)
    )
    assert _is_message_expired(m_boundary_expired, now_utc) is True


def test_watch_vm_delete_ops_empty_queue():
    """Verify watch_vm_delete_ops returns immediately when no messages are queued."""
    sub = MagicMock()
    sub.pull.return_value = []
    lkp = MagicMock()

    with patch("local_pubsub.subscription", return_value=sub):
        watch_vm_delete_ops(lkp)

    sub.ack.assert_not_called()
    sub.modify_ack_deadline.assert_not_called()
    lkp.instance.assert_not_called()


def test_watch_vm_delete_ops_corrupted_payload(tmp_path):
    """Verify unparsable message payloads are removed from disk to prevent queue poison."""
    p = tmp_path / "topic"
    s = tmp_path / "staging"
    p.mkdir()
    s.mkdir()

    sub = local_pubsub.Subscription(p)
    raw_topic = local_pubsub.Topic(p, s)

    # Publish corrupted message (missing node/op_name)
    raw_topic.publish({"bad_payload": 123})

    lkp = MagicMock()

    with patch("local_pubsub.subscription", return_value=sub):
        watch_vm_delete_ops(lkp)

    # Corrupted message must be acknowledged and removed
    remaining = local_pubsub.Subscription(p).pull(10)
    assert len(remaining) == 0


def test_watch_vm_delete_ops_cancelled_future(tmp_path):
    """Verify cancelled futures in ThreadPoolExecutor are NACKed, not ACKed."""
    p = tmp_path / "topic"
    s = tmp_path / "staging"
    p.mkdir()
    s.mkdir()

    sub = local_pubsub.Subscription(p)
    raw_topic = local_pubsub.Topic(p, s)
    raw_topic.publish(asdict(WatchDeleteVmOp_Message("op-cancel", "us-central1-a", "node-cancel")))

    lkp = MagicMock()
    lkp.instance.return_value = MagicMock(status="RUNNING")

    mock_future = MagicMock()
    mock_future.cancelled.return_value = True

    with patch("local_pubsub.subscription", return_value=sub):
        with patch("watch_delete_vm_op._DaemonThreadPoolExecutor") as mock_exe_cls:
            mock_exe = MagicMock()
            mock_exe_cls.return_value = mock_exe
            mock_exe.submit.return_value = mock_future
            with patch("watch_delete_vm_op.wait", return_value=([mock_future], [])):
                watch_vm_delete_ops(lkp)

    # The message must still be in the queue (NACKed)
    remaining = local_pubsub.Subscription(p).pull(10)
    assert len(remaining) == 1
    assert remaining[0].data["node"] == "node-cancel"


def test_daemon_thread_pool_executor():
    """Verify _DaemonThreadPoolExecutor creates daemon threads and cleans up cleanly on shutdown."""
    import threading
    import time
    from concurrent.futures.thread import _threads_queues
    from watch_delete_vm_op import _DaemonThreadPoolExecutor

    # Test 1: Verify daemon status, _threads_queues exclusion, and non-blocking shutdown while tasks run
    exe = _DaemonThreadPoolExecutor(max_workers=1, thread_name_prefix="test_daemon")
    started = threading.Event()
    unblock = threading.Event()

    def running_task():
        started.set()
        unblock.wait(timeout=2)
        return threading.current_thread().daemon

    f_running = exe.submit(running_task)
    f_pending = exe.submit(lambda: "queued")

    assert started.wait(timeout=2) is True
    # Verify active worker threads are daemon and not tracked by _threads_queues
    for t in exe._threads:
        assert t.daemon is True
        assert t not in _threads_queues

    # Non-waiting shutdown should return immediately without blocking on the running task
    t0 = time.time()
    exe.shutdown(wait=False, cancel_futures=True)
    shutdown_duration = time.time() - t0
    assert shutdown_duration < 0.2, f"Shutdown took {shutdown_duration}s, expected immediate return"

    # Queued pending future should be cancelled by cancel_futures=True
    assert f_pending.cancelled() is True

    # New task submissions should be rejected after shutdown
    try:
        exe.submit(lambda: None)
        assert False, "Expected RuntimeError when submitting after shutdown"
    except RuntimeError:
        pass

    # Unblock and allow the running task to finish cleanly
    unblock.set()
    assert f_running.result(timeout=2) is True

    # Verify active worker threads remain absent from _threads_queues
    for t in exe._threads:
        assert t not in _threads_queues


def test_daemon_thread_pool_executor_py38_fallback():
    """Verify _DaemonThreadPoolExecutor.shutdown fallback path for Python < 3.9."""
    import threading
    from unittest.mock import patch
    from watch_delete_vm_op import _DaemonThreadPoolExecutor

    exe = _DaemonThreadPoolExecutor(max_workers=1, thread_name_prefix="test_daemon_py38")
    started = threading.Event()
    unblock = threading.Event()

    def running_task():
        started.set()
        unblock.wait(timeout=2)
        return "done"

    f_running = exe.submit(running_task)
    f_pending = exe.submit(lambda: "queued")
    assert started.wait(timeout=2) is True

    with patch("sys.version_info", (3, 8, 10)):
        exe.shutdown(wait=False, cancel_futures=True)

    assert f_pending.cancelled() is True
    unblock.set()
    assert f_running.result(timeout=2) == "done"
