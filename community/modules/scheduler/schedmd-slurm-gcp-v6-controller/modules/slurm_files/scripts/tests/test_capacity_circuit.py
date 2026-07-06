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

from datetime import datetime, timedelta, timezone
import json
from types import SimpleNamespace
import threading
from unittest import mock

import pytest

from common import TstCfg, TstNodeset, TstPartition
import capacity_circuit
import util


@pytest.fixture
def state_files(tmp_path, monkeypatch):
    monkeypatch.setattr(capacity_circuit, "STATE_FILE", tmp_path / "state.json")
    monkeypatch.setattr(capacity_circuit, "LOCK_FILE", tmp_path / "state.lock")
    monkeypatch.setattr(capacity_circuit, "_owned_nodes_cache", None)


def make_lookup() -> util.Lookup:
    cfg = TstCfg(
        slurm_cluster_name="c",
        cloud_parameters={"resume_timeout": 10},
        capacity_circuit_breaker={
            "enabled": True,
            "initial_cooldown_seconds": 10,
            "max_cooldown_seconds": 40,
            "probe_count": 1,
        },
        nodeset={
            "n": TstNodeset(
                nodeset_name="n",
                node_count_dynamic_max=4,
            )
        },
    )
    lkp = util.Lookup(cfg)
    lkp.slurm_nodes = mock.Mock(  # type: ignore[method-assign]
        return_value={
            "c-n-0": util.NodeState("ALLOCATED", frozenset({"CLOUD", "POWERING_UP"})),
            "c-n-1": util.NodeState("IDLE", frozenset({"CLOUD", "POWERED_DOWN"})),
            "c-n-2": util.NodeState("IDLE", frozenset({"CLOUD", "POWERED_DOWN"})),
            "c-n-3": util.NodeState("ALLOCATED", frozenset({"CLOUD"})),
        }
    )
    return lkp


def test_partition_resume_timeout_overrides_global_timeout():
    lkp = make_lookup()
    lkp.cfg.cloud_parameters["resume_timeout"] = 300
    lkp.cfg.partitions["p"] = TstPartition(
        partition_name="p",
        partition_nodeset=["n"],
        partition_conf={"ResumeTimeout": "600"},
    )

    assert capacity_circuit._resume_timeout_seconds(lkp, "n") == 600


def test_partition_resume_timeout_accepts_case_insensitive_infinite():
    lkp = make_lookup()
    lkp.cfg.partitions["p"] = TstPartition(
        partition_name="p",
        partition_nodeset=["n"],
        partition_conf={"resumetimeout": "INFINITE"},
    )

    assert (
        capacity_circuit._resume_timeout_seconds(lkp, "n")
        == capacity_circuit.SLURM_INFINITE_TIMEOUT_SECONDS
    )


def test_null_partition_resume_timeout_uses_global_timeout():
    lkp = make_lookup()
    lkp.cfg.partitions["p"] = TstPartition(
        partition_name="p",
        partition_nodeset=["n"],
        partition_conf={"ResumeTimeout": None},  # type: ignore[dict-item]
    )

    assert capacity_circuit._resume_timeout_seconds(lkp, "n") == 10


def test_null_partition_nodeset_lists_are_ignored():
    lkp = make_lookup()
    lkp.cfg.partitions["p"] = TstPartition(
        partition_name="p",
        partition_nodeset=None,  # type: ignore[arg-type]
        partition_nodeset_dyn=["n"],
        partition_conf={"ResumeTimeout": "25"},
    )

    assert capacity_circuit._resume_timeout_seconds(lkp, "n") == 25


@pytest.mark.parametrize(
    "value",
    [
        {"error": {"errors": [{"code": "ZONE_RESOURCE_POOL_EXHAUSTED"}]}},
        {"error": {"errors": [{"reason": "insufficientCapacity"}]}},
        [{"code": "REGION_RESOURCE_POOL_EXHAUSTED"}],
        [{"code": "VM_MIN_COUNT_NOT_REACHED"}],
    ],
)
def test_structured_capacity_errors(value):
    assert capacity_circuit.is_capacity_error(
        capacity_circuit.structured_error_codes(value)
    )


@pytest.mark.parametrize(
    "value",
    [
        {"error": {"code": 503, "message": "ZONE_RESOURCE_POOL_EXHAUSTED"}},
        {"error": {"errors": [{"code": "QUOTA_EXCEEDED"}]}},
    ],
)
def test_non_capacity_errors_do_not_trip(value):
    assert not capacity_circuit.is_capacity_error(
        capacity_circuit.structured_error_codes(value)
    )


def test_exception_classification_uses_structured_content_only():
    stockout = RuntimeError("request failed")
    stockout.content = json.dumps(  # type: ignore[attr-defined]
        {"error": {"errors": [{"reason": "resourcePoolExhausted"}]}}
    ).encode()
    message_only = RuntimeError("ZONE_RESOURCE_POOL_EXHAUSTED")

    assert capacity_circuit.is_capacity_exception(stockout)
    assert not capacity_circuit.is_capacity_exception(message_only)


def test_node_updates_have_bounded_scontrol_timeout(monkeypatch):
    lkp = make_lookup()
    run = mock.Mock(return_value=SimpleNamespace(returncode=0))
    monkeypatch.setattr(capacity_circuit.util, "run", run)

    assert capacity_circuit._update_nodes(
        ["c-n-0"],
        "drain",
        "capacity-circuit:n",
        lkp,
        resume_after=-1,
    )

    run.assert_called_once_with(
        f"{lkp.scontrol} update nodename=c-n-0 state=drain "
        "reason=capacity-circuit:n resumeafter=-1",
        check=False,
        timeout=capacity_circuit.SCONTROL_TIMEOUT_SECONDS,
    )


def test_trip_drains_powered_down_siblings_and_arms_failed_probe(
    state_files, monkeypatch
):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)

    trip = capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)

    assert trip is not None
    assert trip.cooldown_seconds == 10
    assert trip.probe_nodes == ("c-n-0",)
    assert trip.handled_nodes == ("c-n-0",)
    update.assert_any_call(
        ["c-n-1", "c-n-2"], "drain", trip.reason, lkp, resume_after=-1
    )
    update.assert_any_call(["c-n-0"], "down", trip.reason, lkp, resume_after=10)
    state = json.loads(capacity_circuit.STATE_FILE.read_text())
    record = state["nodesets"]["n"]
    assert record["owned_nodes"] == ["c-n-0", "c-n-1", "c-n-2"]
    assert record["failures"] == 1


@pytest.mark.parametrize("raise_error", [False, True])
def test_trip_only_handles_failed_nodes_after_successful_down(
    state_files, monkeypatch, raise_error
):
    lkp = make_lookup()

    def update(nodes, state, reason, lookup, resume_after=None):
        if state == "down" and nodes:
            if raise_error:
                raise OSError("scontrol unavailable")
            return False
        return True

    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)

    trip = capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)

    assert trip is not None
    assert trip.handled_nodes == ()
    state = json.loads(capacity_circuit.STATE_FILE.read_text())
    assert state["nodesets"]["n"]["target_nodes"] == [
        "c-n-0",
        "c-n-1",
        "c-n-2",
    ]


def test_only_probe_failure_extends_backoff(state_files, monkeypatch):
    lkp = make_lookup()
    monkeypatch.setattr(capacity_circuit, "_update_nodes", mock.Mock(return_value=True))

    first = capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    unrelated = capacity_circuit.trip(["c-n-3"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    retry = capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)

    assert first is not None and first.cooldown_seconds == 10
    assert unrelated is not None and unrelated.cooldown_seconds == 10
    assert unrelated.probe_nodes == ()
    assert retry is not None and retry.cooldown_seconds == 20
    assert retry.probe_nodes == ("c-n-0",)


def test_unrelated_failure_does_not_redrain_released_probe(state_files, monkeypatch):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    with capacity_circuit._locked_state(write=True) as state:
        state["nodesets"]["n"]["probe_released"] = True
    lkp.slurm_nodes = mock.Mock(  # type: ignore[method-assign]
        return_value={
            "c-n-0": util.NodeState("IDLE", frozenset({"CLOUD", "POWERED_DOWN"})),
            "c-n-1": util.NodeState("IDLE", frozenset({"CLOUD", "POWERED_DOWN"})),
            "c-n-2": util.NodeState("IDLE", frozenset({"CLOUD", "POWERED_DOWN"})),
            "c-n-3": util.NodeState("ALLOCATED", frozenset({"CLOUD"})),
        }
    )
    update.reset_mock()

    trip = capacity_circuit.trip(["c-n-3"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)

    assert trip is not None
    assert trip.probe_nodes == ()
    assert all("c-n-0" not in call.args[0] for call in update.call_args_list)


def test_successful_probe_closes_and_resumes_owned_nodes(state_files, monkeypatch):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    monkeypatch.setattr(
        capacity_circuit,
        "_circuit_owned_resumable_nodes",
        lambda nodes, _: ("c-n-1", "c-n-2"),
    )
    update.reset_mock()

    assert not capacity_circuit.close_if_probe_succeeded(["c-n-3"], lkp)
    assert capacity_circuit.close_if_probe_succeeded(["c-n-0"], lkp)

    update.assert_called_once_with(
        ("c-n-1", "c-n-2"),
        "resume",
        "capacity-circuit:n:closed",
        lkp,
    )
    assert not capacity_circuit.STATE_FILE.exists()


def test_failed_node_fallback_uses_plain_reason_after_close(state_files, monkeypatch):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    trip = capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    assert trip is not None
    with capacity_circuit._locked_state(write=True) as state:
        state["nodesets"].pop("n")
    update.reset_mock()

    capacity_circuit.finish_failed_nodes(
        trip,
        ["c-n-0"],
        "GCP Error: stockout",
        lkp,
    )

    update.assert_called_once_with(
        ["c-n-0"],
        "down",
        "GCP Error: stockout",
        lkp,
    )


def test_failed_node_fallback_uses_current_circuit_generation(state_files, monkeypatch):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    trip = capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    assert trip is not None
    with capacity_circuit._locked_state(write=True) as state:
        record = state["nodesets"]["n"]
        record["generation"] = 2
        record["reason"] = "capacity-circuit:n:attempt=2:cooldown=20s"
        record["cooldown_seconds"] = 20
    update.reset_mock()

    capacity_circuit.finish_failed_nodes(
        trip,
        ["c-n-0"],
        "GCP Error: stockout",
        lkp,
    )

    update.assert_any_call(
        ["c-n-0"],
        "down",
        "capacity-circuit:n:attempt=2:cooldown=20s; GCP Error: stockout",
        lkp,
        resume_after=20,
    )


def test_close_only_resumes_nodes_still_owned_by_circuit(monkeypatch):
    lkp = make_lookup()
    response = {
        "nodes": [
            {
                "name": "c-n-0",
                "reason": "capacity-circuit:n:attempt=1",
                "state": ["DOWN", "CLOUD", "POWERED_DOWN"],
            },
            {
                "name": "c-n-1",
                "reason": "operator maintenance",
                "state": ["IDLE", "DRAIN", "CLOUD"],
            },
            {
                "name": "c-n-2",
                "reason": "capacity-circuit:n:attempt=1",
                "state": ["ALLOCATED", "CLOUD"],
            },
        ]
    }
    monkeypatch.setattr(
        capacity_circuit.util,
        "run",
        mock.Mock(
            return_value=SimpleNamespace(returncode=0, stdout=json.dumps(response))
        ),
    )

    assert capacity_circuit._circuit_owned_resumable_nodes(
        ["c-n-0", "c-n-1", "c-n-2"], lkp
    ) == ("c-n-0",)


def test_close_ignores_targets_removed_by_nodeset_resize(monkeypatch):
    lkp = make_lookup()
    response = {
        "nodes": [
            {
                "name": "c-n-0",
                "reason": "capacity-circuit:n:attempt=1",
                "state": ["DOWN", "CLOUD", "POWERED_DOWN"],
            },
            {
                "name": "c-other-0",
                "reason": "capacity-circuit:other:attempt=1",
                "state": ["DOWN", "CLOUD", "POWERED_DOWN"],
            },
        ]
    }
    run = mock.Mock(
        return_value=SimpleNamespace(returncode=0, stdout=json.dumps(response))
    )
    monkeypatch.setattr(capacity_circuit.util, "run", run)

    assert capacity_circuit._circuit_owned_resumable_nodes(
        ["c-n-0", "c-n-99"], lkp
    ) == ("c-n-0",)
    run.assert_called_once_with(
        f"{lkp.scontrol} show nodes --json",
        check=False,
        timeout=capacity_circuit.SCONTROL_TIMEOUT_SECONDS,
    )


def test_close_retains_circuit_when_show_nodes_fails(monkeypatch):
    lkp = make_lookup()
    run = mock.Mock(return_value=SimpleNamespace(returncode=1, stdout=""))
    monkeypatch.setattr(capacity_circuit.util, "run", run)

    assert capacity_circuit._circuit_owned_resumable_nodes(["c-n-0"], lkp) is None
    run.assert_called_once_with(
        f"{lkp.scontrol} show nodes --json",
        check=False,
        timeout=capacity_circuit.SCONTROL_TIMEOUT_SECONDS,
    )


def test_close_rejects_valid_json_without_node_data(monkeypatch):
    lkp = make_lookup()
    monkeypatch.setattr(
        capacity_circuit.util,
        "run",
        mock.Mock(return_value=SimpleNamespace(returncode=0, stdout="{}")),
    )

    assert capacity_circuit._circuit_owned_resumable_nodes(["c-n-0"], lkp) is None


def test_trip_waits_for_in_progress_close(state_files, monkeypatch):
    lkp = make_lookup()
    monkeypatch.setattr(capacity_circuit, "_update_nodes", mock.Mock(return_value=True))
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    monkeypatch.setattr(
        capacity_circuit,
        "_circuit_owned_resumable_nodes",
        lambda nodes, _: ("c-n-1", "c-n-2"),
    )
    trip_started = threading.Event()
    trip_finished = threading.Event()
    trip_thread = None

    def concurrent_trip():
        trip_started.set()
        capacity_circuit.trip(["c-n-3"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
        trip_finished.set()

    def update_nodes(nodes, state, reason, lookup, resume_after=None):
        nonlocal trip_thread
        if state == "resume":
            trip_thread = threading.Thread(target=concurrent_trip)
            trip_thread.start()
            assert trip_started.wait(timeout=1)
            assert not trip_finished.wait(timeout=0.05)
        return True

    monkeypatch.setattr(capacity_circuit, "_update_nodes", update_nodes)

    assert capacity_circuit._close_nodeset("n", lkp)
    assert trip_thread is not None
    trip_thread.join(timeout=1)
    assert trip_finished.is_set()
    state = json.loads(capacity_circuit.STATE_FILE.read_text())
    assert "n" in state["nodesets"]


def test_stale_close_does_not_close_reopened_circuit(state_files, monkeypatch):
    lkp = make_lookup()
    monkeypatch.setattr(capacity_circuit, "_update_nodes", mock.Mock(return_value=True))
    first = capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    assert first is not None
    first_state = json.loads(capacity_circuit.STATE_FILE.read_text())
    first_id = first_state["nodesets"]["n"]["circuit_id"]
    monkeypatch.setattr(
        capacity_circuit,
        "_circuit_owned_resumable_nodes",
        lambda nodes, _: tuple(nodes),
    )
    assert capacity_circuit._close_nodeset("n", lkp, first_id, 1)

    second = capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    assert second is not None
    second_state = json.loads(capacity_circuit.STATE_FILE.read_text())
    second_id = second_state["nodesets"]["n"]["circuit_id"]
    assert second_id != first_id

    assert not capacity_circuit._close_nodeset("n", lkp, first_id, 1)
    final_state = json.loads(capacity_circuit.STATE_FILE.read_text())
    assert final_state["nodesets"]["n"]["circuit_id"] == second_id


def test_reconcile_releases_probe_after_cooldown(state_files, monkeypatch):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    now = datetime(2026, 1, 1, tzinfo=timezone.utc)
    monkeypatch.setattr(capacity_circuit, "_now", lambda: now)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    monkeypatch.setattr(
        capacity_circuit,
        "_now",
        lambda: now + timedelta(seconds=11),
    )
    lkp.instance = mock.Mock(return_value=None)  # type: ignore[method-assign]
    lkp.slurm_nodes()["c-n-0"] = util.NodeState(
        "DOWN", frozenset({"CLOUD", "POWERED_DOWN"})
    )
    update.reset_mock()

    capacity_circuit.reconcile(lkp)

    update.assert_any_call(
        ("c-n-0",),
        "resume",
        mock.ANY,
        lkp,
    )
    state = json.loads(capacity_circuit.STATE_FILE.read_text())
    assert state["nodesets"]["n"]["probe_released"] is True


def test_stale_reconcile_does_not_release_rearmed_probe(state_files, monkeypatch):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    with capacity_circuit._locked_state(write=True) as state:
        record = state["nodesets"]["n"]
        circuit_id = record["circuit_id"]
        record["generation"] = 2
        record["probe_after"] = (
            datetime.now(timezone.utc) - timedelta(seconds=1)
        ).isoformat()
    update.reset_mock()

    assert not capacity_circuit._release_probe("n", circuit_id, 1, lkp)
    update.assert_not_called()


def test_stale_reconcile_does_not_drain_after_close(state_files, monkeypatch):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    with capacity_circuit._locked_state(write=True) as state:
        circuit_id = state["nodesets"]["n"]["circuit_id"]
        state["nodesets"].pop("n")
    update.reset_mock()

    assert not capacity_circuit._drain_new_nodes("n", circuit_id, 1, ["c-n-3"], lkp)
    update.assert_not_called()


def test_reconcile_does_not_close_for_provisioning_probe(state_files, monkeypatch):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    lkp.instance = mock.Mock(  # type: ignore[method-assign]
        return_value=SimpleNamespace(status="PROVISIONING")
    )
    close = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_close_nodeset", close)

    capacity_circuit.reconcile(lkp)

    close.assert_not_called()


def test_reconcile_fully_resets_after_probe_grace_period(state_files, monkeypatch):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    now = datetime(2026, 1, 1, tzinfo=timezone.utc)
    monkeypatch.setattr(capacity_circuit, "_now", lambda: now)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    monkeypatch.setattr(
        capacity_circuit,
        "_now",
        lambda: now + timedelta(seconds=21),
    )
    lkp.instance = mock.Mock(return_value=None)  # type: ignore[method-assign]
    lkp.slurm_nodes()["c-n-0"] = util.NodeState(
        "DOWN", frozenset({"CLOUD", "POWERED_DOWN"})
    )
    close = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_close_nodeset", close)

    capacity_circuit.reconcile(lkp)

    close.assert_called_once_with("n", lkp, mock.ANY, 1)


def test_reconcile_defers_reset_for_powering_up_probe(state_files, monkeypatch):
    lkp = make_lookup()
    monkeypatch.setattr(capacity_circuit, "_update_nodes", mock.Mock(return_value=True))
    now = datetime(2026, 1, 1, tzinfo=timezone.utc)
    monkeypatch.setattr(capacity_circuit, "_now", lambda: now)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    monkeypatch.setattr(capacity_circuit, "_now", lambda: now + timedelta(seconds=21))
    lkp.instance = mock.Mock(return_value=None)  # type: ignore[method-assign]
    close = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_close_nodeset", close)

    capacity_circuit.reconcile(lkp)

    close.assert_not_called()


def test_reconcile_drains_new_sibling_while_probe_is_powering_up(
    state_files, monkeypatch
):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    lkp.instance = mock.Mock(return_value=None)  # type: ignore[method-assign]
    lkp.slurm_nodes()["c-n-3"] = util.NodeState(
        "IDLE", frozenset({"CLOUD", "POWERED_DOWN"})
    )
    update.reset_mock()

    capacity_circuit.reconcile(lkp)

    update.assert_called_once_with(
        ["c-n-3"],
        "drain",
        mock.ANY,
        lkp,
        resume_after=-1,
    )


def test_reconcile_hard_resets_stuck_powering_up_probe(state_files, monkeypatch):
    lkp = make_lookup()
    monkeypatch.setattr(capacity_circuit, "_update_nodes", mock.Mock(return_value=True))
    now = datetime(2026, 1, 1, tzinfo=timezone.utc)
    monkeypatch.setattr(capacity_circuit, "_now", lambda: now)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    monkeypatch.setattr(capacity_circuit, "_now", lambda: now + timedelta(seconds=31))
    lkp.instance = mock.Mock(return_value=None)  # type: ignore[method-assign]
    close = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_close_nodeset", close)

    capacity_circuit.reconcile(lkp)

    close.assert_called_once_with("n", lkp, mock.ANY, 1)


def test_reconcile_resets_terminal_probe_instance(state_files, monkeypatch):
    lkp = make_lookup()
    monkeypatch.setattr(capacity_circuit, "_update_nodes", mock.Mock(return_value=True))
    now = datetime(2026, 1, 1, tzinfo=timezone.utc)
    monkeypatch.setattr(capacity_circuit, "_now", lambda: now)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    monkeypatch.setattr(capacity_circuit, "_now", lambda: now + timedelta(seconds=21))
    lkp.instance = mock.Mock(  # type: ignore[method-assign]
        return_value=SimpleNamespace(status="TERMINATED")
    )
    lkp.slurm_nodes()["c-n-0"] = util.NodeState(
        "DOWN", frozenset({"CLOUD", "POWERED_DOWN"})
    )
    close = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_close_nodeset", close)

    capacity_circuit.reconcile(lkp)

    close.assert_called_once_with("n", lkp, mock.ANY, 1)


def test_disabled_circuit_is_noop(state_files, monkeypatch):
    lkp = make_lookup()
    lkp.cfg.capacity_circuit_breaker["enabled"] = False
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)

    assert (
        capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp) is None
    )
    assert not capacity_circuit.owns_node("c-n-0", lkp)
    update.assert_not_called()


def test_owns_node_reuses_unchanged_state(state_files, monkeypatch):
    lkp = make_lookup()
    monkeypatch.setattr(capacity_circuit, "_update_nodes", mock.Mock(return_value=True))
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    read_state = mock.Mock(wraps=capacity_circuit._read_state_unlocked)
    monkeypatch.setattr(capacity_circuit, "_read_state_unlocked", read_state)

    assert capacity_circuit.owns_node("c-n-0", lkp)
    assert capacity_circuit.owns_node("c-n-1", lkp)
    assert capacity_circuit.owns_node("c-n-2", lkp)
    assert read_state.call_count == 1


def test_owns_node_reloads_state_written_by_another_process(state_files, monkeypatch):
    lkp = make_lookup()
    monkeypatch.setattr(capacity_circuit, "_update_nodes", mock.Mock(return_value=True))
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    assert capacity_circuit.owns_node("c-n-0", lkp)

    replacement = capacity_circuit.STATE_FILE.with_suffix(".replacement")
    replacement.write_text(json.dumps({"nodesets": {}}))
    replacement.replace(capacity_circuit.STATE_FILE)

    assert not capacity_circuit.owns_node("c-n-0", lkp)


def test_disabling_circuit_resumes_owned_nodes(state_files, monkeypatch):
    lkp = make_lookup()
    update = mock.Mock(return_value=True)
    monkeypatch.setattr(capacity_circuit, "_update_nodes", update)
    capacity_circuit.trip(["c-n-0"], {"ZONE_RESOURCE_POOL_EXHAUSTED"}, lkp)
    monkeypatch.setattr(
        capacity_circuit,
        "_circuit_owned_resumable_nodes",
        lambda nodes, _: ("c-n-0", "c-n-1", "c-n-2"),
    )
    lkp.cfg.capacity_circuit_breaker["enabled"] = False
    update.reset_mock()

    capacity_circuit.reconcile(lkp)

    update.assert_called_once_with(
        ("c-n-0", "c-n-1", "c-n-2"),
        "resume",
        "capacity-circuit:n:closed",
        lkp,
    )
    assert not capacity_circuit.owns_node("c-n-0", lkp)
