#!/slurm/python/venv/bin/python3.13

# Copyright (C) SchedMD LLC.
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

import argparse
import fcntl
import json
import logging
import re
import sys
import shlex
import subprocess
from datetime import datetime, timedelta
from itertools import chain
from pathlib import Path
from dataclasses import dataclass
from typing import Dict, Tuple, List, Optional, Protocol, Any
from functools import lru_cache

import util
from util import (
    batch_execute,
    ensure_execute,
    execute_with_futures,
    FutureReservation,
    install_custom_scripts,
    run,
    separate,
    to_hostlist,
    NodeState,
    chunked,
    dirs,
)
from util import lookup
from suspend import suspend_nodes
import conf
import watch_delete_vm_op
import repair

log = logging.getLogger()

TOT_REQ_CNT = 1000
_MAINTENANCE_SBATCH_SCRIPT_PATH = dirs.custom_scripts / "perform_maintenance.sh"
_UPDATE_MSG = "*** slurm configuration was updated ***"

class NodeAction(Protocol):
    def apply(self, nodes:List[str]) -> None:
        ...

    def __hash__(self):
        ...

@dataclass(frozen=True)
class NodeActionPowerUp():
    def apply(self, nodes:List[str]) -> None:
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} instances to resume ({hostlist})")
        run(f"{lookup().scontrol} update nodename={hostlist} state=power_up")

@dataclass(frozen=True)
class NodeActionIdle():
    def apply(self, nodes:List[str]) -> None:
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} nodes to idle ({hostlist})")
        run(f"{lookup().scontrol} update nodename={hostlist} state=resume")

@dataclass(frozen=True)
class NodeActionPowerDown():
    reason: Optional[str] = None

    def apply(self, nodes:List[str]) -> None:
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} instances to power down ({hostlist})")
        cmd = f"{lookup().scontrol} update nodename={hostlist} state=power_down"
        if self.reason:
            cmd += f" reason={shlex.quote(self.reason)}"
        run(cmd)


@dataclass(frozen=True)
class NodeActionPowerDownForce():
    reason: Optional[str] = None

    def apply(self, nodes:List[str]) -> None:
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} instances to power down ({hostlist})")
        cmd = f"{lookup().scontrol} update nodename={hostlist} state=power_down_force"
        if self.reason:
            cmd += f" reason={shlex.quote(self.reason)}"
        run(cmd)

@dataclass(frozen=True)
class NodeActionDeleteMIG():
    def apply(self, nodes: List[str]) -> None:
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} mig-managed instances to delete ({hostlist})")
        suspend_nodes(nodes)

@dataclass(frozen=True)
class NodeActionDelete():
    def apply(self, nodes:List[str]) -> None:
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} instances to delete ({hostlist})")
        suspend_nodes(nodes)

@dataclass(frozen=True)
class NodeActionPrempt():
    def apply(self, nodes:List[str]) -> None:
        NodeActionDown(reason="Preempted instance").apply(nodes)
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} instances restarted ({hostlist})")
        start_instances(nodes)

@dataclass(frozen=True)
class NodeActionUnchanged():
    def apply(self, nodes:List[str]) -> None:
        pass

@dataclass(frozen=True)
class NodeActionDown():
    reason: str

    def apply(self, nodes: List[str]) -> None:
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} nodes set down ({hostlist}) with reason={self.reason}")
        run(f"{lookup().scontrol} update nodename={hostlist} state=down reason={shlex.quote(self.reason)}")

@dataclass(frozen=True)
class NodeActionRepair():
    reason: str
    def apply(self, nodes: List[str]) -> None:
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} nodes to repair ({hostlist}) with reason={self.reason}")
        for node in nodes:
            op_id = repair.call_rr_api(node, self.reason)
            if op_id:
                repair.store_operation(node, op_id, self.reason)

@dataclass(frozen=True)
class NodeActionUnknown():
    slurm_state: Optional[NodeState]
    instance_state: Optional[str]

    def apply(self, nodes:List[str]) -> None:
        hostlist = util.to_hostlist(nodes)    
        log.error(f"{len(nodes)} nodes have unexpected {self.slurm_state} and instance state:{self.instance_state}, ({hostlist})")

def start_instance_op(node: str) -> Any:
    inst = lookup().instance(node)
    assert inst

    return lookup().compute.instances().start(
        project=lookup().project,
        zone=inst.zone,
        instance=inst.name,
    )


def start_instances(node_list):
    log.info("{} instances to start ({})".format(len(node_list), ",".join(node_list)))
    ops = {node: start_instance_op(node) for node in node_list}

    done, failed = batch_execute(ops)


def _find_dynamic_node_status() -> NodeAction:
    # TODO: cover more cases:
    # * delete dead dynamic nodes
    # * delete orhpaned instances
    return NodeActionUnchanged()  # don't touch dynamic nodes

def get_fr_action(fr: FutureReservation, state:Optional[NodeState]) -> Optional[NodeAction]:
    now = util.now()
    if state is None:
        return None # handle like any other node
    if fr.start_time < now < fr.end_time:
        return None # handle like any other node
    
    if state.base == "DOWN":
        return NodeActionUnchanged()
    if fr.start_time >= now:
        msg = f"Waiting for reservation:{fr.name} to start at {fr.start_time}" 
    else:
        msg = f"Reservation:{fr.name} is after its end-time"
    return NodeActionDown(reason=msg)


_MIG_REPAIRING_REASON = "MIG Auto-Healing instance repair in progress"
_MIG_REPAIR_ERROR_PREFIX = "MIG Auto-Healing failed"
_MIG_ERROR_PREFIX = "MIG Error"
_MIG_REASON_MAX_LEN = 200
_MIG_REPAIRING_ACTIONS = frozenset(("REPAIRING", "RESTARTING", "RECREATING"))
_MIG_TRANSIENT_CAPACITY_CODES = frozenset((
    "ZONE_RESOURCE_POOL_EXHAUSTED",
    "ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS",
    "RESOURCE_POOL_EXHAUSTED",
    "INSUFFICIENT_RESOURCE_CAPACITY",
))
_POWER_DOWN_FLAGS = frozenset(("POWERED_DOWN", "POWERING_DOWN"))


def _sanitize_mig_reason_part(text: str) -> str:
    """Normalize a GCE error field into a single-line Slurm node reason fragment.

    Square brackets are converted to parentheses because ``get_node_reason``
    splits on ``[`` to strip the user/timestamp suffix appended by ``scontrol``.
    """
    cleaned = re.sub(r"[\r\n\t\x00-\x1f\x7f]+", " ", text)
    cleaned = cleaned.replace("[", "(").replace("]", ")")
    return " ".join(cleaned.split())


def _format_mig_error_reason(action: str, code: str, message: str) -> str:
    """Format a GCE managed instance ``lastAttempt`` error for Slurm ``sinfo``."""
    safe_code = _sanitize_mig_reason_part(code) or "UNKNOWN"
    safe_msg = _sanitize_mig_reason_part(message)
    prefix = (
        _MIG_REPAIR_ERROR_PREFIX
        if action in _MIG_REPAIRING_ACTIONS
        else _MIG_ERROR_PREFIX
    )
    reason = f"{prefix} ({safe_code}): {safe_msg}" if safe_msg else f"{prefix} ({safe_code})"
    if len(reason) > _MIG_REASON_MAX_LEN:
        reason = reason[: _MIG_REASON_MAX_LEN - 3].rstrip() + "..."
    return reason


def _is_mig_managed_reason(reason: Optional[str]) -> bool:
    """Return True when ``reason`` was set by slurmsync MIG auto-healing/error logic."""
    if not reason:
        return False
    return "MIG Auto-Healing" in reason or reason.startswith(_MIG_ERROR_PREFIX)


def _can_update_mig_reason(current_reason: Optional[str]) -> bool:
    """Return True if a DOWN node's reason may be updated with a newer MIG error.

    Preserves manual operator reasons (e.g. admin maintenance notes or hardware checks).
    """
    if not current_reason:
        return True
    if _is_mig_managed_reason(current_reason):
        return True
    return current_reason.strip().lower() in (
        "unbacked instance",
        "instance terminated",
        "not responding",
        "node unexpectedly rebooted",
    )


def _action_for_mig_error(
    short_nodename: str,
    state: NodeState,
    mig_err: Tuple[str, str, str],
) -> NodeAction:
    """Return a NodeAction updating ``short_nodename`` with a formatted MIG error reason."""
    err_reason = _format_mig_error_reason(*mig_err)
    if state.base != "DOWN":
        return NodeActionDown(reason=err_reason)
    cur_reason = get_node_reason(short_nodename)
    if _can_update_mig_reason(cur_reason) and cur_reason != err_reason:
        return NodeActionDown(reason=err_reason)
    return NodeActionUnchanged()


def _is_instance_managed_by_mig(
    lkp: util.Lookup,
    inst: Optional[util.Instance],
    short_nodename: str,
    region: str,
    mig_name: str,
) -> bool:
    """Return True if ``inst`` is managed by ``mig_name`` (fails safe on API error)."""
    if util.instance_created_by_mig(inst) == mig_name:
        return True
    try:
        return short_nodename in lkp.get_mig_instance_names(lkp.project, region, mig_name)
    except Exception as e:
        log.debug(f"Could not verify MIG membership for {short_nodename} in {mig_name}: {e}")
        return True


_node_reasons_cache: Optional[Dict[str, Optional[str]]] = None


def _normalize_reason(reason: Optional[str]) -> Optional[str]:
    if reason and "[" in reason:
        return reason.split("[")[0].strip()
    return reason


def _load_all_node_reasons() -> Optional[Dict[str, Optional[str]]]:
    """Fetch reasons for all Slurm nodes in a single ``scontrol show nodes --json`` call."""
    try:
        result = run(f"{lookup().scontrol} show nodes --json")
        data = json.loads(result.stdout)
        reasons: Dict[str, Optional[str]] = {}
        for node in data.get("nodes", []):
            name = node.get("name")
            if name:
                normalized = _normalize_reason(node.get("reason"))
                reasons[name] = normalized
                reasons.setdefault(name.split(".")[0], normalized)
        return reasons
    except (subprocess.CalledProcessError, json.JSONDecodeError) as e:
        log.debug(f"Bulk scontrol show nodes --json failed, will query per-node: {e}")
    except Exception as e:
        log.debug(f"Unexpected error prefetching node reasons: {e}")
    return None


def get_node_reason(nodename: str) -> Optional[str]:
    """Get the reason for a node's state using JSON output."""
    if _node_reasons_cache is not None and nodename in _node_reasons_cache:
        return _node_reasons_cache[nodename]
    try:
        # Use --json to get structured data
        result = run(f"{lookup().scontrol} show node {nodename} --json")
        data = json.loads(result.stdout)

        # Access the reason field directly from the JSON structure
        nodes = data.get('nodes', [])
        if nodes:
            return _normalize_reason(nodes[0].get('reason'))
    except (subprocess.CalledProcessError, json.JSONDecodeError) as e:
        log.error(f"Failed to execute scontrol or parse its JSON output for node {nodename}: {e}")
    except Exception as e:
        log.error(f"An unexpected error occurred while getting reason for node {nodename}: {e}")
    return None


def get_node_action(nodename: str) -> NodeAction:
    """Determine node/instance status that requires action"""
    lkp = lookup()
    state = lkp.node_state(nodename)
    if lkp.node_is_gke(nodename):
        return NodeActionUnchanged()

    if lkp.node_is_fr(nodename):
        fr = lkp.future_reservation(lkp.node_nodeset(nodename))
        assert fr
        if action := get_fr_action(fr, state):
            return action

    if lkp.node_is_dyn(nodename):
        return _find_dynamic_node_status()

    # split below is workaround for VMs whose hostname is FQDN
    short_nodename = nodename.split(".")[0]
    inst = lkp.instance(short_nodename)

    # For MIG compute nodes, detect active GCE Auto-Healing repairs and surface MIG slot errors
    if lkp.is_node_mig(nodename) and not lkp.is_tpu_node(nodename):
        try:
            mig_name = lkp.node_mig_name(nodename)
            region = lkp.node_region(nodename)
            repairing = lkp.get_mig_repairing_instances(lkp.project, region, mig_name)
            mig_errors = lkp.get_mig_instance_errors(lkp.project, region, mig_name)
            mig_err = mig_errors.get(short_nodename)
            in_power_down = state is not None and bool(state.flags & _POWER_DOWN_FLAGS)

            if short_nodename in repairing:
                if state is None:
                    return NodeActionUnchanged()
                if not in_power_down:
                    if mig_err is not None:
                        return _action_for_mig_error(short_nodename, state, mig_err)
                    if state.base != "DOWN":
                        return NodeActionDown(reason=_MIG_REPAIRING_REASON)
                    return NodeActionUnchanged()
            elif mig_err is not None:
                err_action, err_code, err_msg = mig_err
                is_healthy_or_booting = (
                    inst is not None
                    and inst.status in ("RUNNING", "PROVISIONING", "STAGING")
                    and err_action == "NONE"
                )
                if not is_healthy_or_booting and state is not None and not in_power_down:
                    if "POWERING_UP" in state.flags and err_code.upper() in _MIG_TRANSIENT_CAPACITY_CODES:
                        log.warning(
                            f"MIG {mig_name} slot {short_nodename} queued on capacity ({err_code}): {err_msg}"
                        )
                        return NodeActionUnchanged()
                    return _action_for_mig_error(short_nodename, state, mig_err)

            if inst and inst.status == "RUNNING" and state is not None and state.base == "DOWN" and not in_power_down:
                if _is_mig_managed_reason(get_node_reason(short_nodename)):
                    log.info(f"{short_nodename} recovered by MIG auto-healing; resuming node to idle")
                    return NodeActionIdle()
        except Exception as e:
            log.debug(f"Failed to check managed instance repair status for {nodename}: {e}")

    power_flags = frozenset(
        ("POWER_DOWN", "POWERING_UP", "POWERING_DOWN", "POWERED_DOWN")
    ) & (state.flags if state is not None else set())

    if state is not None and "DRAIN" in state.flags:
        reason = get_node_reason(nodename)
        if reason in repair.REPAIR_REASONS:
            if repair.is_node_being_repaired(nodename):
                return NodeActionUnchanged()
            if inst:
                return NodeActionRepair(reason=reason)

    if (state is None) and (inst is None):
        # Should never happen
        return NodeActionUnknown(None, None)
    if inst is None:
        assert state is not None # to keep type-checker happy
        if "POWERING_UP" in state.flags:
            return NodeActionUnchanged()
        if state.base == "DOWN" and "POWERED_DOWN" in state.flags:
            return NodeActionIdle()
        if "POWERING_DOWN" in state.flags:
            return NodeActionIdle()
        if "COMPLETING" in state.flags:
            return NodeActionDown(reason="Unbacked instance")
        if state.base != "DOWN" and not power_flags:
            return NodeActionDown(reason="Unbacked instance")
        if state.base == "DOWN" and not power_flags:
            return NodeActionPowerDown()
        if "NOT_RESPONDING" in state.flags:
            return NodeActionPowerDown()
        if "POWERED_DOWN" in state.flags and lkp.is_static_node(nodename):
            return NodeActionPowerUp()
    elif (
        state is not None
        and "POWERED_DOWN" not in state.flags
        and "POWERING_DOWN" not in state.flags
        and inst.status == "TERMINATED"
    ):
        if lkp.is_tpu_node(nodename):
            termination_reason = (
                "Spot instance preempted"
                if inst.scheduling.preemptible
                else "Instance terminated"
            )
            return NodeActionPowerDownForce(reason=termination_reason)
        if lkp.is_node_mig(nodename):
            mig_name = lkp.node_mig_name(nodename)
            region = lkp.node_region(nodename)
            if _is_instance_managed_by_mig(lkp, inst, short_nodename, region, mig_name):
                # MIG auto-healing (default_action_on_failure=REPAIR) recreates the VM
                # (delete + insert). Do not start it; wait for RUNNING, then resume via
                # the auto-healing branch above.
                if state.base != "DOWN":
                    return NodeActionDown(reason=_MIG_REPAIRING_REASON)
                return NodeActionUnchanged()
            log.info(
                f"{short_nodename} is a legacy standalone VM (outside MIG {mig_name}); "
                "using legacy TERMINATED handling."
            )
        if inst.scheduling.preemptible:
            return NodeActionPrempt()
        if state.base != "DOWN":
            return NodeActionDown(reason="Instance terminated")
    elif (state is None or "POWERED_DOWN" in state.flags) and inst.status == "RUNNING":
        log.info("%s is potential orphan node", nodename)
        threshold = timedelta(seconds=90)
        age = util.now() - inst.creation_timestamp
        log.info(f"{nodename} state: {state}, age: {age}")
        if age < threshold:
            log.info(f"{nodename} not marked as orphan, it started less than {threshold.seconds}s ago ({age.seconds}s)")
            return NodeActionUnchanged()
        if lkp.is_flex_node(nodename) or lkp.is_tpu_node(nodename):
            return NodeActionDeleteMIG()
        return NodeActionDelete()
    elif (
        (state is None or "POWERED_DOWN" in state.flags)
        and inst.status == "TERMINATED"
        and lkp.is_tpu_node(nodename)
    ):
        return NodeActionDeleteMIG()
    elif state is None:
        # if state is None here, the instance exists but it's not in Slurm
        return NodeActionUnknown(slurm_state=state, instance_state=inst.status)
    elif (lkp.is_flex_node(nodename) or lkp.is_tpu_node(nodename)) and "POWERING_UP" in state.flags:
        threshold = timedelta(seconds=int(lkp.cfg.compute_startup_scripts_timeout) * 2) #extra buffer for unexpectedly long startup scripts
        if util.now() - inst.creation_timestamp > threshold:
            log.info(f"{nodename} was unable to join the cluster after {threshold.seconds}s, potential failure on VM startup. Powering down...")
            return NodeActionPowerDownForce()
    return NodeActionUnchanged()


def delete_resource_policies(links: list[str], lkp: util.Lookup) -> None:
    requests = {}
    for link in links:
        name = util.trim_self_link(link)
        region = util.parse_self_link(link).region
        requests[name] = lkp.compute.resourcePolicies().delete(project=lkp.project, region=region, resourcePolicy=name)

    def swallow_err(_: str) -> None:
        pass

    done, failed = batch_execute(requests, log_err=swallow_err)
    if failed:
        # Filter out resourceInUseByAnotherResource errors , they are expected to happen
        def ignore_err(e) -> bool:
            return "resourceInUseByAnotherResource" in str(e)

        failures = [f"{n}: {e}" for n, (_, e) in failed.items() if not ignore_err(e)]
        if failures:
            log.error(f"some placement groups failed to delete: {failures}")
    log.info(
        f"deleted {len(done)} of {len(links)} placement groups ({to_hostlist(done.keys())})"
    )



@lru_cache
def _get_resource_policies_in_region(lkp: util.Lookup, region: str) -> list[Any]:
    res = []
    act = lkp.compute.resourcePolicies()
    op = act.list(project=lkp.project, region=region)
    prefix = f"{lkp.cfg.slurm_cluster_name}-slurmgcp-"
    while op is not None:
        result = ensure_execute(op)
        res.extend([p for p in result.get("items", []) if p.get("name", "").startswith(prefix)])
        op = act.list_next(op, result)
    return res


@lru_cache
def _get_resource_policies(lkp: util.Lookup) -> list[Any]:
    res = []
    for region in lkp.cluster_regions():
        res.extend(_get_resource_policies_in_region(lkp, region))
    return res

def sync_placement_groups():
    """Delete placement policies that are for jobs that have completed/terminated"""
    keep_states = frozenset(
        [
            "RUNNING",
            "CONFIGURING",
            "STOPPED",
            "SUSPENDED",
            "COMPLETING",
            "PENDING",
        ]
    )

    lkp = lookup()
    keep_jobs = {
        str(job.id)
        for job in lkp.get_jobs()
        if job.job_state in keep_states
    }
    keep_jobs.add("0")  # Job 0 is a placeholder for static node placement

    to_delete = []
    pg_regex = re.compile(
        rf"{lkp.cfg.slurm_cluster_name}-slurmgcp-managed-(?P<ns>[^\s\-]+)-(?P<job_id>\d+)-(?P<index>\d+)"
    )
    tpu_wp_regex = re.compile(
        rf"{lkp.cfg.slurm_cluster_name}-slurmgcp-(?P<ns>.+?)-wp-\d+(?:-\d+)*$"
    )

    for pg in _get_resource_policies(lkp):
        name = pg["name"]

        # Keep reusable TPU workload policies unless their nodeset was removed
        if mtch := tpu_wp_regex.match(name):
            ns_name = mtch.group("ns")
            if ns_name not in lkp.cfg.nodeset:
                log.info(f"Nodeset {ns_name} is not found, deleting its TPU workload policy {name}")
                to_delete.append(pg["selfLink"])
            continue
        if (mtch := pg_regex.match(name)) is None:
            log.warning(f"Unexpected resource policy {name=}")
            continue
        if mtch.group("job_id") not in keep_jobs:
            to_delete.append(pg["selfLink"])

    if to_delete:
        delete_resource_policies(to_delete, lkp)


def sync_instances():
    global _node_reasons_cache
    compute_instances = {
        name for name, inst in lookup().instances().items() if inst.role == "compute"
    }
    slurm_nodes = set(lookup().slurm_nodes().keys())
    log.debug(f"reconciling {len(compute_instances)} GCP instances and {len(slurm_nodes)} Slurm nodes.")

    _node_reasons_cache = _load_all_node_reasons()
    try:
        for action, nodes in util.groupby_unsorted(list(compute_instances | slurm_nodes), get_node_action):
            action.apply(list(nodes))
    finally:
        _node_reasons_cache = None


def reconfigure_slurm() -> bool:
    """Fetch new config and regenerate Slurm config files.

    Returns True if config was updated and a scontrol reconfigure is needed.
    On the controller, the reconfigure is deferred until after topology
    regeneration to avoid issuing two sequential scontrol reconfigure calls
    in the same sync cycle (which can cause a socket timeout).
    """
    if lookup().cfg.hybrid:
        # terraform handles generating the config.yaml, don't do it here
        return False

    upd, cfg_new = util.fetch_config()
    if not upd:
        log.debug("No changes in config detected.")
        return False
    log.debug("Changes in config detected. Reconfiguring Slurm now.")
    util.update_config(cfg_new)

    if lookup().is_controller:
        conf.get_generator(lookup()).generate_configs()

        # Defer slurmctld restart and scontrol reconfigure:
        # Running them now would cause slurmctld to temporarily load new node defs with old topology.
        # This causes slurmctld to hang/timeout when update_topology tries to reconfigure it subsequently.
        log.info("Config files regenerated. Deferring scontrol reconfigure until after topology update.")
        log.debug("Done.")
        return True
    elif lookup().instance_role_safe == "compute":
        log.info("Restarting slurmd to make changes take effect.")
        run("systemctl restart slurmd")
        util.run(f"wall '{_UPDATE_MSG}'", timeout=30)
        log.debug("Done.")
    elif lookup().is_login_node:
        log.info("Restarting sackd to make changes take effect.")
        run("systemctl restart sackd")
        util.run(f"wall '{_UPDATE_MSG}'", timeout=30)
        log.debug("Done.")
    return False


def _generate_topology(lkp: util.Lookup) -> Tuple[bool, Any]:
    return conf.get_generator(lkp).generate_topology_data()

def update_topology(lkp: util.Lookup) -> Tuple[bool, Any]:
    """Regenerate topology files.

    Returns (updated, summary) so the caller can decide when to
    issue the scontrol reconfigure and persist the summary.
    """
    updated, summary = _generate_topology(lkp) # type: ignore[attr-defined]

    if updated:
        log.info("Topology configuration updated.")
    return updated, summary


def delete_reservation(lkp: util.Lookup, reservation_name: str) -> None:
    util.run(f"{lkp.scontrol} delete reservation {reservation_name}")


def create_reservation(lkp: util.Lookup, reservation_name: str, node: str, start_time: datetime) -> None:
    # Format time to be compatible with slurm reservation.
    formatted_start_time = start_time.strftime('%Y-%m-%dT%H:%M:%S')
    
    util.run(f"{lkp.scontrol} create reservation user=slurm starttime={formatted_start_time} duration=180 nodes={node} reservationname={reservation_name} flags=maint,ignore_jobs")


def get_slurm_reservation_maintenance(lkp: util.Lookup) -> Dict[str, datetime]:
    res = util.run(f"{lkp.scontrol} show reservation --json")
    all_reservations = json.loads(res.stdout)
    reservation_map = {}

    for reservation in all_reservations['reservations']:
        name = reservation.get('name')
        nodes = reservation.get('node_list')
        time_epoch = reservation.get('start_time', {}).get('number')

        if name is None or nodes is None or time_epoch is None:
          continue

        if reservation.get('node_count') != 1:
          continue

        if name != f"{nodes}_maintenance":
          continue

        reservation_map[name] = datetime.fromtimestamp(time_epoch)

    return reservation_map

@lru_cache
def get_upcoming_maintenance(lkp: util.Lookup) -> Dict[str, Tuple[str, datetime]]:
    upc_maint_map = {}

    for node, inst in lkp.instances().items():
        if inst.resource_status.upcoming_maintenance:
          upc_maint_map[node + "_maintenance"] = (node, inst.resource_status.upcoming_maintenance.window_start_time)

    return upc_maint_map


def sync_maintenance_reservation(lkp: util.Lookup) -> None:
    upc_maint_map = get_upcoming_maintenance(lkp)  # map reservation_name -> (node_name, time)
    log.debug(f"upcoming-maintenance-vms: {upc_maint_map}")

    curr_reservation_map = get_slurm_reservation_maintenance(lkp)  # map reservation_name -> time
    log.debug(f"curr-reservation-map: {curr_reservation_map}")

    del_reservation = set(curr_reservation_map.keys() - upc_maint_map.keys())
    create_reservation_map = {}

    for res_name, (node, start_time) in upc_maint_map.items():
      try:
        enabled = lkp.node_nodeset(node).enable_maintenance_reservation
      except Exception:
        enabled = False

      if not enabled:
          if res_name in curr_reservation_map:
            del_reservation.add(res_name)
          continue

      if res_name in curr_reservation_map:
        diff = curr_reservation_map[res_name] - start_time
        if abs(diff) <= timedelta(seconds=1):
          continue
        else:
          del_reservation.add(res_name)
          create_reservation_map[res_name] = (node, start_time)
      else:
        create_reservation_map[res_name] = (node, start_time)

    log.debug(f"del-reservation: {del_reservation}")
    for res_name in del_reservation:
      delete_reservation(lkp, res_name)

    log.debug(f"create-reservation-map: {create_reservation_map}")
    for res_name, (node, start_time) in create_reservation_map.items():
      create_reservation(lkp, res_name, node, start_time)


def delete_maintenance_job(job_name: str) -> None:
    util.run(f"scancel --name={job_name}")


def create_maintenance_job(job_name: str, node: str) -> None:
    util.run(f"sbatch --job-name={job_name} --nodelist={node} {_MAINTENANCE_SBATCH_SCRIPT_PATH}")


def get_slurm_maintenance_job(lkp: util.Lookup) -> Dict[str, str]:
    jobs = {}

    for job in lkp.get_jobs():
        if job.name is None or job.required_nodes is None or job.job_state is None:
          continue

        if job.name != f"{job.required_nodes}_maintenance":
          continue

        if job.job_state != "PENDING":
          continue

        jobs[job.name] = job.required_nodes

    return jobs


def sync_opportunistic_maintenance(lkp: util.Lookup) -> None:
    upc_maint_map = get_upcoming_maintenance(lkp)  # map job_name -> (node_name, time)
    log.debug(f"upcoming-maintenance-vms: {upc_maint_map}")

    curr_jobs = get_slurm_maintenance_job(lkp)  # map job_name -> node.
    log.debug(f"curr-maintenance-job-map: {curr_jobs}")

    del_jobs = set(curr_jobs.keys() - upc_maint_map.keys())
    create_jobs = {}

    for job_name, (node, _) in upc_maint_map.items():
      try:
          enabled = lkp.node_nodeset(node).enable_opportunistic_maintenance
      except Exception:
          enabled = False

      if not enabled:
          if job_name in curr_jobs:
              del_jobs.add(job_name)
          continue

      if job_name not in curr_jobs:
          create_jobs[job_name] = node

    log.debug(f"del-maintenance-job: {del_jobs}")
    for job_name in del_jobs:
        delete_maintenance_job(job_name)

    log.debug(f"create-maintenance-job: {create_jobs}")
    for job_name, node in create_jobs.items():
        create_maintenance_job(job_name, node)



def sync_flex_migs(lkp: util.Lookup) -> None:
    pass


def process_messages(lkp: util.Lookup) -> None:
    try:
        watch_delete_vm_op.watch_vm_delete_ops(lkp)
    except:
        log.exception("failed during watching delete VM operations")


def main():
    lkp = lookup()
    if util.should_mount_slurm_bucket() and not lkp.is_controller:
        return

    # Track if reconfigure_slurm changed cloud.conf so we can trigger
    # a deferred scontrol reconfigure inside update_topology
    config_changed = False
    try:
        config_changed = reconfigure_slurm()
        if config_changed:
            lkp = lookup()
    except Exception:
        log.exception("failed to reconfigure slurm")

    topology_changed = False
    topology_summary = None
    if lkp.is_controller:
        try:
            topology_changed, topology_summary = update_topology(lkp)
        except Exception:
            log.exception("failed to update topology")

    if lkp.is_controller and not util.is_active_controller(lkp):
        if topology_changed and topology_summary is not None:
            topology_summary.dump(lkp)
        log.info("Local controller is in STANDBY mode. Skipping slurmsync pass.")
        return
    if lkp.is_controller:
        try:
            process_messages(lkp)
        except:
            log.exception("failed to process messages")

        try:
            sync_instances()
        except Exception:
            log.exception("failed to sync instances")

        try:
            repair.poll_operations()
        except Exception:
            log.exception("failed to poll repair operations")

        try:
            sync_flex_migs(lkp)
        except Exception:
            log.exception("failed to sync DWS Flex MIGs")

        try:
            sync_placement_groups()
        except Exception:
            log.exception("failed to sync placement groups")

        # Single deferred reconfigure for both config and topology changes.
        # Placed here so it runs even if topology generation failed above.
        if config_changed or topology_changed:
            reasons = []
            if config_changed:
                reasons.append("config updated")
            if topology_changed:
                reasons.append("topology changed")
            try:
                log.info(f"Reconfiguring Slurm ({', '.join(reasons)}).")
                # Restart slurmctld only for a config change. A topology-only
                # change is picked up by `scontrol reconfigure` alone, and
                # update_topology runs on every node power-up, so restarting
                # there would flap nodes registering during the restart window.
                util.scontrol_reconfigure(lkp, restart=config_changed)
                # Only dump summary after successful reconfigure so it reflects Slurm's view
                if topology_changed and topology_summary is not None:
                    topology_summary.dump(lkp)
                util.run(f"wall '{_UPDATE_MSG}'", timeout=30, check=False)
            except Exception:
                log.exception("failed to reconfigure slurmctld")

        try:
            sync_maintenance_reservation(lkp)
        except Exception:
            log.exception("failed to sync slurm reservation for scheduled maintenance")

        try:
            sync_opportunistic_maintenance(lkp)
        except Exception:
            log.exception("failed to sync opportunistic reservation for scheduled maintenance")


    try:
        # TODO: it performs 1 to 4 GCS list requests,
        # use cached version, combine with `_list_config_blobs`
        install_custom_scripts(check_hash=True)
    except Exception:
        log.exception("failed to sync custom scripts")



if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    _ = util.init_log_and_parse(parser)

    pid_file = (Path("/tmp") / Path(__file__).name).with_suffix(".pid")
    with pid_file.open("w") as fp:
        try:
            fcntl.lockf(fp, fcntl.LOCK_EX | fcntl.LOCK_NB)
            main()
        except BlockingIOError:
            sys.exit(0)
