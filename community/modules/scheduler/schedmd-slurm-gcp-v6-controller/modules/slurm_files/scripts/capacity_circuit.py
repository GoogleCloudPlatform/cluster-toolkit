#!/slurm/python/venv/bin/python3.13

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

"""Temporarily remove GCE nodesets from scheduling after capacity errors."""

from __future__ import annotations

import fcntl
import json
import logging
import os
import shlex
import uuid
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any, Iterable, Iterator, Mapping, Optional, Sequence

import util


log = logging.getLogger()

CAPACITY_ERROR_CODES = frozenset(
    {
        "REGION_RESOURCE_POOL_EXHAUSTED",
        "RESOURCE_POOL_EXHAUSTED",
        "VM_MIN_COUNT_NOT_REACHED",
        "ZONE_RESOURCE_POOL_EXHAUSTED",
        "ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS",
        "insufficientCapacity",
        "resourcePoolExhausted",
    }
)
REASON_PREFIX = "capacity-circuit:"
STATE_FILE = util.slurmdirs.state / "capacity_circuits.json"
LOCK_FILE = util.slurmdirs.state / "capacity_circuits.lock"
TRANSIENT_INSTANCE_STATES = frozenset({"PROVISIONING", "STAGING"})
TRANSIENT_SLURM_BASE_STATES = frozenset({"ALLOCATED", "MIXED"})
TRANSIENT_SLURM_FLAGS = frozenset({"CONFIGURING", "POWERING_UP"})
SCONTROL_TIMEOUT_SECONDS = 30
SLURM_INFINITE_TIMEOUT_SECONDS = 65533
_owned_nodes_cache: Optional[
    tuple[Optional[tuple[int, int, int, int, int]], dict[str, frozenset[str]]]
] = None


@dataclass(frozen=True)
class CircuitConfig:
    enabled: bool
    initial_cooldown_seconds: int
    max_cooldown_seconds: int
    probe_count: int


@dataclass(frozen=True)
class CircuitTrip:
    nodeset_name: str
    generation: int
    reason: str
    cooldown_seconds: int
    probe_nodes: tuple[str, ...]
    handled_nodes: tuple[str, ...]


def _value(mapping: Any, key: str, default: Any) -> Any:
    if mapping is None or key not in mapping or mapping[key] is None:
        return default
    return mapping[key]


def get_config(lkp: Optional[util.Lookup] = None) -> CircuitConfig:
    lkp = lkp or util.lookup()
    raw = getattr(lkp.cfg, "capacity_circuit_breaker", {})
    return CircuitConfig(
        enabled=bool(_value(raw, "enabled", False)),
        initial_cooldown_seconds=int(_value(raw, "initial_cooldown_seconds", 300)),
        max_cooldown_seconds=int(_value(raw, "max_cooldown_seconds", 1800)),
        probe_count=int(_value(raw, "probe_count", 1)),
    )


def _collect_structured_codes(value: Any) -> set[str]:
    codes: set[str] = set()
    if isinstance(value, Mapping):
        for key, child in value.items():
            if key in ("code", "reason") and isinstance(child, str):
                codes.add(child)
            else:
                codes.update(_collect_structured_codes(child))
    elif isinstance(value, list):
        for child in value:
            codes.update(_collect_structured_codes(child))
    return codes


def structured_error_codes(value: Any) -> set[str]:
    """Return structured error codes/reasons without inspecting message text."""
    return _collect_structured_codes(value)


def exception_error_codes(exc: Exception) -> set[str]:
    """Extract structured codes from a googleapiclient-style exception."""
    content = getattr(exc, "content", None)
    if isinstance(content, bytes):
        content = content.decode(errors="replace")
    if not isinstance(content, str):
        return set()
    try:
        return structured_error_codes(json.loads(content))
    except json.JSONDecodeError:
        return set()


def is_capacity_error(codes: Iterable[str]) -> bool:
    return not CAPACITY_ERROR_CODES.isdisjoint(codes)


def is_capacity_exception(exc: Exception) -> bool:
    return is_capacity_error(exception_error_codes(exc))


def _empty_state() -> dict[str, Any]:
    return {"nodesets": {}}


def _read_state_unlocked() -> dict[str, Any]:
    try:
        state = json.loads(STATE_FILE.read_text())
    except FileNotFoundError:
        return _empty_state()
    except (json.JSONDecodeError, OSError) as exc:
        log.error("Unable to read capacity circuit state %s: %s", STATE_FILE, exc)
        return _empty_state()
    if not isinstance(state, dict) or not isinstance(state.get("nodesets"), dict):
        log.error("Ignoring malformed capacity circuit state in %s", STATE_FILE)
        return _empty_state()
    return state


def _write_state_unlocked(state: dict[str, Any]) -> None:
    global _owned_nodes_cache
    _owned_nodes_cache = None
    STATE_FILE.parent.mkdir(parents=True, exist_ok=True)
    if not state.get("nodesets"):
        STATE_FILE.unlink(missing_ok=True)
        return
    temporary = STATE_FILE.with_name(f"{STATE_FILE.name}.{os.getpid()}.tmp")
    with temporary.open("w", encoding="utf-8") as stream:
        json.dump(state, stream, indent=2, sort_keys=True)
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, STATE_FILE)


@contextmanager
def _locked_state(write: bool) -> Iterator[dict[str, Any]]:
    LOCK_FILE.parent.mkdir(parents=True, exist_ok=True)
    with LOCK_FILE.open("a+", encoding="utf-8") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX if write else fcntl.LOCK_SH)
        state = _read_state_unlocked()
        try:
            yield state
            if write:
                _write_state_unlocked(state)
        finally:
            fcntl.flock(lock, fcntl.LOCK_UN)


def _now() -> datetime:
    return datetime.now(timezone.utc)


def _parse_time(value: str) -> datetime:
    return datetime.fromisoformat(value)


def _dynamic_nodes(nodeset: Any, lkp: util.Lookup) -> list[str]:
    _, dynamic = lkp.nodenames(nodeset)
    return list(dynamic)


def _powered_down_nodes(nodeset: Any, lkp: util.Lookup) -> list[str]:
    states = lkp.slurm_nodes()
    result = []
    for node in _dynamic_nodes(nodeset, lkp):
        state = states.get(node)
        if state is None or "POWERED_DOWN" not in state.flags:
            continue
        if state.base == "DOWN" or "DRAIN" in state.flags:
            continue
        result.append(node)
    return result


def _reason(nodeset_name: str, failures: int, cooldown: int) -> str:
    return f"{REASON_PREFIX}{nodeset_name}:attempt={failures}:cooldown={cooldown}s"


def _resume_timeout_seconds(lkp: util.Lookup, nodeset_name: str) -> int:
    cloud_parameters = getattr(lkp.cfg, "cloud_parameters", {})
    global_timeout = max(
        1,
        int(_value(cloud_parameters, "resume_timeout", 300)),
    )
    timeouts = [global_timeout]
    for partition in getattr(lkp.cfg, "partitions", {}).values():
        partition_nodesets = set(getattr(partition, "partition_nodeset", []) or [])
        partition_nodesets.update(getattr(partition, "partition_nodeset_dyn", []) or [])
        if nodeset_name not in partition_nodesets:
            continue
        partition_conf = getattr(partition, "partition_conf", {})
        timeout: Any = global_timeout
        for key, value in partition_conf.items():
            if str(key).casefold() == "resumetimeout":
                timeout = global_timeout if value is None else value
                break
        if isinstance(timeout, str) and timeout.strip().casefold() == "infinite":
            timeouts.append(SLURM_INFINITE_TIMEOUT_SECONDS)
        else:
            timeouts.append(max(1, int(timeout)))
    return max(timeouts)


def _update_nodes(
    nodes: Sequence[str],
    state: str,
    reason: str,
    lkp: util.Lookup,
    resume_after: Optional[int] = None,
) -> bool:
    if not nodes:
        return True
    command = f"{lkp.scontrol} update nodename={util.to_hostlist(nodes)} state={state}"
    if state.lower() != "resume":
        command += f" reason={shlex.quote(reason)}"
    if resume_after is not None:
        command += f" resumeafter={resume_after}"
    return (
        util.run(
            command,
            check=False,
            timeout=SCONTROL_TIMEOUT_SECONDS,
        ).returncode
        == 0
    )


def trip(
    failed_nodes: Sequence[str],
    error_codes: Iterable[str],
    lkp: Optional[util.Lookup] = None,
) -> Optional[CircuitTrip]:
    """Open or re-arm the circuit for the failed nodes' nodeset."""
    lkp = lkp or util.lookup()
    config = get_config(lkp)
    error_codes = set(error_codes)
    if not config.enabled or not failed_nodes or not is_capacity_error(error_codes):
        return None

    nodeset_names = {lkp.node_nodeset_name(node) for node in failed_nodes}
    if len(nodeset_names) != 1:
        raise ValueError("capacity circuit trip must contain exactly one nodeset")
    nodeset_name = next(iter(nodeset_names))
    nodeset = lkp.cfg.nodeset[nodeset_name]
    powered_down = _powered_down_nodes(nodeset, lkp)
    now = _now()

    intent_persisted = False
    result: Optional[CircuitTrip] = None
    handled: set[str] = set()
    try:
        with _locked_state(write=True) as state:
            circuits = state["nodesets"]
            previous = circuits.get(nodeset_name)
            previous_probes = (
                set(previous.get("probe_nodes", [])) if previous else set()
            )
            rearm = previous is None or bool(previous_probes.intersection(failed_nodes))

            failures = int(previous.get("failures", 0)) if previous else 0
            if rearm:
                failures += 1
            failures = max(failures, 1)
            cooldown = min(
                config.initial_cooldown_seconds * (2 ** min(failures - 1, 30)),
                config.max_cooldown_seconds,
            )
            probes = (
                tuple(sorted(failed_nodes)[: config.probe_count]) if rearm else tuple()
            )
            owned = set(previous.get("owned_nodes", [])) if previous else set()
            target = set(previous.get("target_nodes", [])) if previous else set()
            target.update(failed_nodes)
            target.update(powered_down)
            generation = int(previous.get("generation", 0)) + 1 if previous else 1
            circuit_id = (
                str(previous.get("circuit_id"))
                if previous and previous.get("circuit_id")
                else uuid.uuid4().hex
            )
            reason = _reason(nodeset_name, failures, cooldown)
            probe_after = now + timedelta(seconds=cooldown)
            resume_timeout = _resume_timeout_seconds(lkp, nodeset_name)
            reset_grace = max(cooldown, resume_timeout)
            reset_after = probe_after + timedelta(seconds=reset_grace)
            # A released half-open probe must remain schedulable while unrelated
            # requests fail. Only a failure of that probe re-arms the circuit.
            siblings = sorted(
                set(powered_down).difference(
                    failed_nodes,
                    previous_probes,
                    owned,
                )
            )

            record = {
                "circuit_id": circuit_id,
                "cooldown_seconds": cooldown,
                "error_codes": sorted(error_codes),
                "failures": failures,
                "generation": generation,
                "opened_at": previous.get("opened_at", now.isoformat())
                if previous
                else now.isoformat(),
                "owned_nodes": sorted(owned),
                "probe_after": probe_after.isoformat()
                if rearm
                else previous.get("probe_after"),
                "probe_nodes": list(probes) if rearm else list(previous_probes),
                "probe_released": False
                if rearm
                else bool(previous.get("probe_released", False)),
                "reason": reason,
                "reset_after": reset_after.isoformat()
                if rearm
                else previous.get("reset_after"),
                "hard_reset_after": (
                    reset_after + timedelta(seconds=resume_timeout)
                ).isoformat()
                if rearm
                else previous.get("hard_reset_after"),
                "target_nodes": sorted(target),
            }
            circuits[nodeset_name] = record
            result = CircuitTrip(
                nodeset_name,
                generation,
                reason,
                cooldown,
                probes,
                tuple(),
            )
            # Persist intent before changing Slurm state. Reconciliation can finish
            # an interrupted transition, and slurmsync will preserve it meanwhile.
            _write_state_unlocked(state)
            intent_persisted = True

            if _update_nodes(siblings, "drain", reason, lkp, resume_after=-1):
                owned.update(siblings)

            probe_failures = set(probes).intersection(failed_nodes)
            regular_failures = sorted(set(failed_nodes).difference(probe_failures))
            if _update_nodes(regular_failures, "down", reason, lkp):
                owned.update(regular_failures)
                handled.update(regular_failures)
            if _update_nodes(
                sorted(probe_failures),
                "down",
                reason,
                lkp,
                resume_after=cooldown,
            ):
                owned.update(probe_failures)
                handled.update(probe_failures)
            record["owned_nodes"] = sorted(owned)
    except Exception:
        if not intent_persisted or result is None:
            raise
        log.exception(
            "Capacity circuit intent was saved but its initial node transition was interrupted"
        )

    assert result is not None
    result = CircuitTrip(
        result.nodeset_name,
        result.generation,
        result.reason,
        result.cooldown_seconds,
        result.probe_nodes,
        tuple(sorted(handled)),
    )

    log.warning(
        "Opened GCE capacity circuit for nodeset %s for %ss; probe nodes=%s",
        result.nodeset_name,
        result.cooldown_seconds,
        util.to_hostlist(result.probe_nodes) if result.probe_nodes else "already armed",
    )
    return result


def finish_failed_nodes(
    circuit_trip: CircuitTrip,
    failed_nodes: Sequence[str],
    failure_reason: str,
    lkp: Optional[util.Lookup] = None,
) -> None:
    """Finish failed-node handling without racing circuit close or re-arm."""
    if not failed_nodes:
        return
    lkp = lkp or util.lookup()
    intent_persisted = False
    try:
        with _locked_state(write=True) as state:
            record = state["nodesets"].get(circuit_trip.nodeset_name)
            if not record:
                if not _update_nodes(failed_nodes, "down", failure_reason, lkp):
                    log.error("Unable to mark failed nodes DOWN: %s", failed_nodes)
                return

            target = set(record.get("target_nodes", []))
            target.update(failed_nodes)
            record["target_nodes"] = sorted(target)
            _write_state_unlocked(state)
            intent_persisted = True

            reason = f"{record.get('reason', REASON_PREFIX)}; {failure_reason}"
            probes = set(record.get("probe_nodes", [])).intersection(failed_nodes)
            regular = sorted(set(failed_nodes).difference(probes))
            owned = set(record.get("owned_nodes", []))
            if _update_nodes(regular, "down", reason, lkp):
                owned.update(regular)
            else:
                log.error("Unable to mark circuit nodes DOWN: %s", regular)
            if _update_nodes(
                sorted(probes),
                "down",
                reason,
                lkp,
                resume_after=int(record.get("cooldown_seconds", 0)),
            ):
                owned.update(probes)
            else:
                log.error("Unable to mark circuit probes DOWN: %s", sorted(probes))
            record["owned_nodes"] = sorted(owned)
    except Exception:
        if not intent_persisted:
            raise
        log.exception(
            "Capacity circuit retained failed-node ownership after an interrupted transition"
        )


def _state_version() -> Optional[tuple[int, int, int, int, int]]:
    try:
        stat = STATE_FILE.stat()
    except FileNotFoundError:
        return None
    return (
        stat.st_dev,
        stat.st_ino,
        stat.st_size,
        stat.st_mtime_ns,
        stat.st_ctime_ns,
    )


def _owned_nodes_by_nodeset() -> dict[str, frozenset[str]]:
    global _owned_nodes_cache
    version = _state_version()
    if _owned_nodes_cache is not None and _owned_nodes_cache[0] == version:
        return _owned_nodes_cache[1]
    if version is None:
        _owned_nodes_cache = (None, {})
        return _owned_nodes_cache[1]

    with _locked_state(write=False) as state:
        owned = {
            nodeset_name: frozenset(record.get("target_nodes", []))
            for nodeset_name, record in state["nodesets"].items()
        }
        # Writers replace the state file atomically while holding the same lock,
        # so this version describes the state that was just parsed.
        version = _state_version()
    _owned_nodes_cache = (version, owned)
    return owned


def owns_node(node: str, lkp: Optional[util.Lookup] = None) -> bool:
    lkp = lkp or util.lookup()
    try:
        nodeset_name = lkp.node_nodeset_name(node)
    except Exception:
        return False
    return node in _owned_nodes_by_nodeset().get(nodeset_name, ())


def _circuit_owned_resumable_nodes(
    nodes: Sequence[str], lkp: util.Lookup
) -> Optional[tuple[str, ...]]:
    if not nodes:
        return tuple()
    result = util.run(
        f"{lkp.scontrol} show nodes --json",
        check=False,
        timeout=SCONTROL_TIMEOUT_SECONDS,
    )
    if result.returncode != 0:
        return None
    try:
        response = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        log.error("Unable to parse Slurm node state while closing circuit: %s", exc)
        return None
    if not isinstance(response, Mapping) or not isinstance(response.get("nodes"), list):
        log.error("Unexpected Slurm node response while closing circuit")
        return None
    node_data = response["nodes"]
    if not all(isinstance(node, Mapping) for node in node_data):
        log.error("Unexpected Slurm node entry while closing circuit")
        return None
    target = set(nodes)
    resumable = []
    for node in node_data:
        if node.get("name") not in target:
            continue
        reason = node.get("reason") or ""
        states = set(node.get("state") or [])
        if reason.startswith(REASON_PREFIX) and states.intersection(
            {"DOWN", "DRAIN", "DRAINING"}
        ):
            resumable.append(node["name"])
    return tuple(sorted(resumable))


def _close_nodeset(
    nodeset_name: str,
    lkp: util.Lookup,
    expected_circuit_id: Optional[str] = None,
    expected_generation: Optional[int] = None,
) -> bool:
    with _locked_state(write=True) as state:
        record = state["nodesets"].get(nodeset_name)
        if not record:
            return False
        if (
            expected_circuit_id is not None
            and record.get("circuit_id") != expected_circuit_id
        ):
            return False
        if (
            expected_generation is not None
            and int(record.get("generation", 0)) != expected_generation
        ):
            return False
        target = tuple(record.get("target_nodes", []))
        reason = f"{REASON_PREFIX}{nodeset_name}:closed"
        resumable = _circuit_owned_resumable_nodes(target, lkp)
        if resumable is None:
            log.error(
                "Unable to inspect capacity circuit nodes for nodeset %s",
                nodeset_name,
            )
            return False
        if not _update_nodes(resumable, "resume", reason, lkp):
            log.error("Unable to close capacity circuit for nodeset %s", nodeset_name)
            return False
        state["nodesets"].pop(nodeset_name, None)
    log.info("Closed GCE capacity circuit for nodeset %s", nodeset_name)
    return True


def close_if_probe_succeeded(
    nodes: Sequence[str], lkp: Optional[util.Lookup] = None
) -> bool:
    lkp = lkp or util.lookup()
    if not get_config(lkp).enabled or not nodes:
        return False
    closed = False
    for nodeset_name in {lkp.node_nodeset_name(node) for node in nodes}:
        with _locked_state(write=False) as state:
            record = state["nodesets"].get(nodeset_name)
            probes = set(record.get("probe_nodes", [])) if record else set()
            circuit_id = record.get("circuit_id") if record else None
            generation = int(record.get("generation", 0)) if record else None
        if probes.intersection(nodes):
            closed = _close_nodeset(nodeset_name, lkp, circuit_id, generation) or closed
    return closed


def _release_probe(
    nodeset_name: str,
    circuit_id: Optional[str],
    generation: int,
    lkp: util.Lookup,
) -> bool:
    """Release a due probe unless the circuit changed since reconciliation."""
    with _locked_state(write=True) as state:
        record = state["nodesets"].get(nodeset_name)
        if (
            not record
            or record.get("circuit_id") != circuit_id
            or int(record.get("generation", 0)) != generation
        ):
            return False
        probe_after = record.get("probe_after")
        probes = tuple(record.get("probe_nodes", []))
        if (
            not probes
            or not probe_after
            or record.get("probe_released", False)
            or _now() < _parse_time(probe_after)
        ):
            return False
        if not _update_nodes(
            probes,
            "resume",
            record.get("reason", REASON_PREFIX),
            lkp,
        ):
            return False
        record["probe_released"] = True
    return True


def _drain_new_nodes(
    nodeset_name: str,
    circuit_id: Optional[str],
    generation: int,
    nodes: Sequence[str],
    lkp: util.Lookup,
) -> bool:
    """Add and drain newly eligible nodes unless the circuit changed."""
    if not nodes:
        return False
    with _locked_state(write=True) as state:
        record = state["nodesets"].get(nodeset_name)
        if (
            not record
            or record.get("circuit_id") != circuit_id
            or int(record.get("generation", 0)) != generation
        ):
            return False
        probes = set(record.get("probe_nodes", []))
        candidates = sorted(set(nodes).difference(probes))
        if not candidates:
            return False
        target = set(record.get("target_nodes", []))
        target.update(candidates)
        record["target_nodes"] = sorted(target)
        # Persist ownership before changing Slurm state so a process failure
        # cannot leave an untracked drain behind.
        _write_state_unlocked(state)
        reason = _reason(
            nodeset_name,
            int(record.get("failures", 1)),
            int(record.get("cooldown_seconds", 0)),
        )
        if not _update_nodes(
            candidates,
            "drain",
            reason,
            lkp,
            resume_after=-1,
        ):
            return False
        owned = set(record.get("owned_nodes", []))
        owned.update(candidates)
        record["owned_nodes"] = sorted(owned)
    return True


def _probe_is_in_flight(probes: Sequence[str], lkp: util.Lookup) -> bool:
    states = lkp.slurm_nodes()
    for probe in probes:
        state = states.get(probe)
        if state is None:
            continue
        if state.base in TRANSIENT_SLURM_BASE_STATES:
            return True
        if not TRANSIENT_SLURM_FLAGS.isdisjoint(state.flags):
            return True
    return False


def reconcile(lkp: Optional[util.Lookup] = None) -> None:
    """Recover circuit state and release probes whose cooldown has elapsed."""
    lkp = lkp or util.lookup()
    if not STATE_FILE.exists():
        return
    with _locked_state(write=False) as state:
        snapshot = dict(state["nodesets"])

    if not get_config(lkp).enabled:
        for nodeset_name in snapshot:
            _close_nodeset(nodeset_name, lkp)
        return

    for nodeset_name, record in snapshot.items():
        circuit_id = record.get("circuit_id")
        generation = int(record.get("generation", 0))
        probes = tuple(record.get("probe_nodes", []))
        probe_instances = [lkp.instance(node) for node in probes]
        if any(
            instance is not None and instance.status == "RUNNING"
            for instance in probe_instances
        ):
            _close_nodeset(nodeset_name, lkp, circuit_id, generation)
            continue

        now = _now()
        reset_after = record.get("reset_after")
        reset_due = bool(reset_after and now >= _parse_time(reset_after))
        probe_in_flight = _probe_is_in_flight(probes, lkp)
        transient_instance = any(
            instance is not None and instance.status in TRANSIENT_INSTANCE_STATES
            for instance in probe_instances
        )
        reset_deferred = False
        if reset_due:
            hard_reset_after = record.get("hard_reset_after", reset_after)
            if (
                (probe_in_flight or transient_instance)
                and hard_reset_after
                and now < _parse_time(hard_reset_after)
            ):
                log.info(
                    "Deferring capacity circuit reset for in-flight probe in nodeset %s",
                    nodeset_name,
                )
                reset_deferred = True
            else:
                log.info(
                    "Closing capacity circuit for nodeset %s after probe grace period",
                    nodeset_name,
                )
                _close_nodeset(nodeset_name, lkp, circuit_id, generation)
                continue
        if (
            not reset_deferred
            and not probe_in_flight
            and not any(instance is not None for instance in probe_instances)
        ):
            _release_probe(nodeset_name, circuit_id, generation, lkp)

        nodeset = lkp.cfg.nodeset.get(nodeset_name)
        if nodeset is None:
            continue
        newly_powered_down = set(_powered_down_nodes(nodeset, lkp)).difference(
            record.get("owned_nodes", [])
        )
        _drain_new_nodes(
            nodeset_name,
            circuit_id,
            generation,
            sorted(newly_powered_down),
            lkp,
        )
