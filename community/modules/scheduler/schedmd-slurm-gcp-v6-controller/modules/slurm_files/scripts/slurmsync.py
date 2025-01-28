#!/usr/bin/env python3

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
from datetime import datetime, timedelta
from itertools import chain
from pathlib import Path
from dataclasses import dataclass
from typing import Dict, Tuple, List, Optional, Protocol
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
from suspend import delete_instances
import tpu
import conf

log = logging.getLogger()

TOT_REQ_CNT = 1000
_MAINTENANCE_SBATCH_SCRIPT_PATH = dirs.custom_scripts / "perform_maintenance.sh"

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
    def apply(self, nodes:List[str]) -> None:
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} instances to power down ({hostlist})")
        run(f"{lookup().scontrol} update nodename={hostlist} state=power_down")

@dataclass(frozen=True)
class NodeActionDelete():
    def apply(self, nodes:List[str]) -> None:
        hostlist = util.to_hostlist(nodes)
        log.info(f"{len(nodes)} instances to delete ({hostlist})")
        delete_instances(nodes)

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
class NodeActionUnknown():
    slurm_state: Optional[NodeState]
    instance_state: Optional[str]

    def apply(self, nodes:List[str]) -> None:
        hostlist = util.to_hostlist(nodes)    
        log.error(f"{len(nodes)} nodes have unexpected {self.slurm_state} and instance state:{self.instance_state}, ({hostlist})")

def start_instance_op(inst):
    return lookup().compute.instances().start(
        project=lookup().project,
        zone=lookup().instance(inst).zone,
        instance=inst,
    )


def start_instances(node_list):
    log.info("{} instances to start ({})".format(len(node_list), ",".join(node_list)))
    lkp = lookup()
    # TODO: use code from resume.py to assign proper placement
    normal, tpu_nodes = separate(lkp.node_is_tpu, node_list)
    ops = {inst: start_instance_op(inst) for inst in normal}

    done, failed = batch_execute(ops)

    tpu_start_data = []
    for ns, nodes in util.groupby_unsorted(tpu_nodes, lkp.node_nodeset_name):
        tpuobj = tpu.TPU.make(ns, lkp)
        for snodes in chunked(nodes, n=tpuobj.vmcount):
            tpu_start_data.append({"tpu": tpuobj, "node": snodes})
    execute_with_futures(tpu.start_tpu, tpu_start_data)


def _find_dynamic_node_status() -> NodeAction:
    # TODO: cover more cases:
    # * delete dead dynamic nodes
    # * delete orhpaned instances
    return NodeActionUnchanged()  # don't touch dynamic nodes

def get_fr_action(fr: FutureReservation, nodename:str, state:NodeState) -> Optional[NodeAction]:
    now = datetime.utcnow()
    if fr.start_time < now < fr.end_time:
        return None # handle like any other node
    if state.base == "DOWN":
        return NodeActionUnchanged()
    if fr.start_time >= now:
        msg = f"Waiting for reservation:{fr.name} to start at {fr.start_time}" 
    else:
        msg = f"Reservation:{fr.name} is after its end-time"
    return NodeActionDown(reason=msg)

def _find_tpu_node_action(nodename, state) -> NodeAction:
    lkp = lookup()
    tpuobj = tpu.TPU.make(lkp.node_nodeset_name(nodename), lkp)
    inst = tpuobj.get_node(nodename)
    # If we do not find the node but it is from a Tpu that has multiple vms look for the master node
    if inst is None and tpuobj.vmcount > 1:
        # Get the tpu slurm nodelist of the nodes in the same tpu group as nodename
        nodelist = run(
            f"{lkp.scontrol} show topo {nodename}"
            + " | awk -F'=' '/Level=0/ { print $NF }'",
            shell=True,
        ).stdout
        l_nodelist = util.to_hostnames(nodelist)
        group_names = set(l_nodelist)
        # get the list of all the existing tpus in the nodeset
        tpus_list = set(tpuobj.list_node_names())
        # In the intersection there must be only one node that is the master
        tpus_int = list(group_names.intersection(tpus_list))
        if len(tpus_int) > 1:
            log.error(
                f"More than one cloud tpu node for tpu group {nodelist}, there should be only one that should be {l_nodelist[0]}, but we have found {tpus_int}"
            )
            return NodeActionUnknown(slurm_state=state, instance_state=None)
        if len(tpus_int) == 1:
            inst = tpuobj.get_node(tpus_int[0])
        # if len(tpus_int ==0) this case is not relevant as this would be the case always that a TPU group is not running
    if inst is None:
        if state.base == "DOWN" and "POWERED_DOWN" in state.flags:
            return NodeActionIdle()
        if "POWERING_DOWN" in state.flags:
            return NodeActionIdle()
        if "COMPLETING" in state.flags:
            return NodeActionDown(reason="Unbacked instance")
        if state.base != "DOWN" and not (
            set(("POWER_DOWN", "POWERING_UP", "POWERING_DOWN", "POWERED_DOWN"))
            & state.flags
        ):
            return NodeActionDown(reason="Unbacked instance")
        if lkp.is_static_node(nodename):
            return NodeActionPowerUp()
    elif (
        state is not None
        and "POWERED_DOWN" not in state.flags
        and "POWERING_DOWN" not in state.flags
        and inst.state == tpu.TPU.State.STOPPED
    ):
        if tpuobj.preemptible:
            return NodeActionPrempt()
        if state.base != "DOWN":
            return NodeActionDown(reason="Instance terminated")
    elif (
        state is None or "POWERED_DOWN" in state.flags
    ) and inst.state == tpu.TPU.State.READY:
        return NodeActionDelete()
    elif state is None:
        # if state is None here, the instance exists but it's not in Slurm
        return NodeActionUnknown(slurm_state=state, instance_state=inst.status)

    return NodeActionUnchanged()

def get_node_action(nodename: str) -> NodeAction:
    """Determine node/instance status that requires action"""
    state = lookup().node_state(nodename)

    if lookup().node_is_fr(nodename):
        fr = lookup().future_reservation(lookup().node_nodeset(nodename))
        if action := get_fr_action(fr, nodename, state):
            return action

    if lookup().node_is_dyn(nodename):
        return _find_dynamic_node_status()

    if lookup().node_is_tpu(nodename):
        return _find_tpu_node_action(nodename, state)

    # split below is workaround for VMs whose hostname is FQDN
    inst = lookup().instance(nodename.split(".")[0])
    power_flags = frozenset(
        ("POWER_DOWN", "POWERING_UP", "POWERING_DOWN", "POWERED_DOWN")
    ) & (state.flags if state is not None else set())

    if inst is None:
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
        if "POWERED_DOWN" in state.flags and lookup().is_static_node(nodename):
            return NodeActionPowerUp()
    elif (
        state is not None
        and "POWERED_DOWN" not in state.flags
        and "POWERING_DOWN" not in state.flags
        and inst.status == "TERMINATED"
    ):
        if inst.scheduling.preemptible:
            return NodeActionPrempt()
        if state.base != "DOWN":
            return NodeActionDown(reason="Instance terminated")
    elif (state is None or "POWERED_DOWN" in state.flags) and inst.status == "RUNNING":
        log.info("%s is potential orphan node", nodename)
        age_threshold_seconds = 90
        inst_seconds_old = _seconds_since_timestamp(inst.creationTimestamp)
        log.info("%s state: %s, age: %0.1fs", nodename, state, inst_seconds_old)
        if inst_seconds_old < age_threshold_seconds:
            log.info(
                "%s not marked as orphan, it started less than %ds ago (%0.1fs)",
                nodename,
                age_threshold_seconds,
                inst_seconds_old,
            )
            return NodeActionUnchanged()
        return NodeActionDelete()
    elif state is None:
        # if state is None here, the instance exists but it's not in Slurm
        return NodeActionUnknown(slurm_state=state, instance_state=inst.status)

    return NodeActionUnchanged()


def _seconds_since_timestamp(timestamp):
    """Returns duration in seconds since a timestamp
    Args:
        timestamp: A formatted timestamp string (%Y-%m-%dT%H:%M:%S.%f%z)
    Returns:
        number of seconds that have past since the timestamp (float)
    """
    if timestamp[-3] == ":":  # python 36 datetime does not support the colon
        timestamp = timestamp[:-3] + timestamp[-2:]
    creation_dt = datetime.strptime(timestamp, "%Y-%m-%dT%H:%M:%S.%f%z")
    return datetime.now().timestamp() - creation_dt.timestamp()


def delete_placement_groups(placement_groups):
    requests = {
        pg["name"]: lookup().compute.resourcePolicies().delete(
            project=lookup().project,
            region=util.trim_self_link(pg["region"]),
            resourcePolicy=pg["name"])
        for pg in placement_groups
    }

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
        f"deleted {len(done)} of {len(placement_groups)} placement groups ({to_hostlist(done.keys())})"
    )


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

    keep_jobs = {
        str(job.id)
        for job in lookup().get_jobs()
        if job.job_state in keep_states
    }
    keep_jobs.add("0")  # Job 0 is a placeholder for static node placement

    fields = "items.regions.resourcePolicies,nextPageToken"
    flt = f"name={lookup().cfg.slurm_cluster_name}-*"
    act = lookup().compute.resourcePolicies()
    op = act.aggregatedList(project=lookup().project, fields=fields, filter=flt)
    placement_groups = {}
    pg_regex = re.compile(
        rf"{lookup().cfg.slurm_cluster_name}-slurmgcp-managed-(?P<partition>[^\s\-]+)-(?P<job_id>\d+)-(?P<index>\d+)"
    )
    while op is not None:
        result = ensure_execute(op)
        # merge placement group info from API and job_id,partition,index parsed from the name
        pgs = (
            {**pg, **pg_regex.match(pg["name"]).groupdict()}
            for pg in chain.from_iterable(
                item["resourcePolicies"]
                for item in result.get("items", {}).values()
                if item
            )
            if pg_regex.match(pg["name"]) is not None
        )
        placement_groups.update(
            {pg["name"]: pg for pg in pgs if pg.get("job_id") not in keep_jobs}
        )
        op = act.aggregatedList_next(op, result)

    if len(placement_groups) > 0:
        delete_placement_groups(list(placement_groups.values()))


def sync_slurm():
    compute_instances = {
        name for name, inst in lookup().instances().items() if inst.role == "compute"
    }
    slurm_nodes = set(lookup().slurm_nodes().keys())
    log.debug(f"reconciling {len(compute_instances)} GCP instances and {len(slurm_nodes)} Slurm nodes.")

    for action, nodes in util.groupby_unsorted(list(compute_instances | slurm_nodes), get_node_action):
        action.apply(list(nodes))


def reconfigure_slurm():
    update_msg = "*** slurm configuration was updated ***"
    if lookup().cfg.hybrid:
        # terraform handles generating the config.yaml, don't do it here
        return

    upd, cfg_new = util.fetch_config()
    if not upd:
        log.debug("No changes in config detected.")
        return
    log.debug("Changes in config detected. Reconfiguring Slurm now.")
    util.update_config(cfg_new)

    if lookup().is_controller:
        conf.gen_controller_configs(lookup())
        log.info("Restarting slurmctld to make changes take effect.")
        try:
            # TODO: consider removing "restart" since "reconfigure" should restart slurmctld as well
            run("sudo systemctl restart slurmctld.service", check=False)
            util.scontrol_reconfigure(lookup())
        except Exception:
            log.exception("failed to reconfigure slurmctld")
        util.run(f"wall '{update_msg}'", timeout=30)
        log.debug("Done.")
    elif lookup().instance_role_safe == "compute":
        log.info("Restarting slurmd to make changes take effect.")
        run("systemctl restart slurmd")
        util.run(f"wall '{update_msg}'", timeout=30)
        log.debug("Done.")
    elif lookup().instance_role_safe == "login":
        log.info("Restarting sackd to make changes take effect.")
        run("systemctl restart sackd")
        util.run(f"wall '{update_msg}'", timeout=30)
        log.debug("Done.")


def update_topology(lkp: util.Lookup) -> None:
    if conf.topology_plugin(lkp) != conf.TOPOLOGY_PLUGIN_TREE:
        return
    updated, summary = conf.gen_topology_conf(lkp)
    if updated:
        log.info("Topology configuration updated. Reconfiguring Slurm.")
        util.scontrol_reconfigure(lkp)
        # Safe summary only after Slurm got reconfigured, so summary reflects Slurm POV
        summary.dump(lkp)


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

    for node, properties in lkp.instances().items():
        if 'upcomingMaintenance' in properties:
          start_time = datetime.strptime(properties['upcomingMaintenance']['startTimeWindow']['earliest'], '%Y-%m-%dT%H:%M:%S%z')
          upc_maint_map[node + "_maintenance"] = (node, start_time)

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


def main():
    try:
        reconfigure_slurm()
    except Exception:
        log.exception("failed to reconfigure slurm")

    if lookup().is_controller:
        try:
            sync_slurm()
        except Exception:
            log.exception("failed to sync instances")

        try:
            sync_placement_groups()
        except Exception:
            log.exception("failed to sync placement groups")

        try:
            update_topology(lookup())
        except Exception:
            log.exception("failed to update topology")

        try:
            sync_maintenance_reservation(lookup())
        except Exception:
            log.exception("failed to sync slurm reservation for scheduled maintenance")

        try:
            sync_opportunistic_maintenance(lookup())
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
