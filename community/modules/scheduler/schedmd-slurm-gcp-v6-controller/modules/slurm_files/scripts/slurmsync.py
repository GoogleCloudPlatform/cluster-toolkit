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
import datetime
import fcntl
import hashlib
import json
import logging
import re
import sys
from enum import Enum
from itertools import chain
from pathlib import Path
import yaml

import util
from util import (
    batch_execute,
    ensure_execute,
    execute_with_futures,
    fetch_config_yaml,
    fetch_config_yaml_md5,
    install_custom_scripts,
    load_config_file,
    run,
    save_config,
    separate,
    to_hostlist_fast,
    Lookup,
    NSDict,
    TPU,
    chunked,
)
from util import lkp, cfg, compute, CONFIG_FILE
from suspend import delete_instances
from resume import start_tpu
from conf import (
    gen_cloud_conf,
    gen_cloud_gres_conf,
    gen_topology_conf,
    install_slurm_conf,
    install_slurmdbd_conf,
    install_gres_conf,
    install_cgroup_conf,
    install_topology_conf,
)

filename = Path(__file__).name
LOGFILE = (Path(cfg.slurm_log_dir if cfg else ".") / filename).with_suffix(".log")

log = logging.getLogger(filename)

TOT_REQ_CNT = 1000


NodeStatus = Enum(
    "NodeStatus",
    (
        "orphan",
        "power_down",
        "preempted",
        "restore",
        "resume",
        "terminated",
        "unbacked",
        "unchanged",
        "unknown",
    ),
)


def start_instance_op(inst, project=None):
    project = project or lkp.project
    return compute.instances().start(
        project=project,
        zone=lkp.instance(inst).zone,
        instance=inst,
    )


def start_instances(node_list):
    log.info("{} instances to start ({})".format(len(node_list), ",".join(node_list)))

    normal, tpu_nodes = separate(lkp.node_is_tpu, node_list)
    invalid, valid = separate(lambda inst: bool(lkp.instance), normal)

    ops = {inst: start_instance_op(inst) for inst in valid}

    done, failed = batch_execute(ops)

    tpu_start_data = []
    for ns, nodes in util.groupby_unsorted(tpu_nodes, lkp.node_nodeset_name):
        tpuobj = TPU(lkp.cfg.nodeset_tpu[ns])
        for snodes in chunked(nodes, n=tpuobj.vmcount):
            tpu_start_data.append({"tpu": tpuobj, "node": snodes})
    execute_with_futures(start_tpu, tpu_start_data)


def _find_dynamic_node_status() -> NodeStatus:
    # TODO: cover more cases:
    # * delete dead dynamic nodes
    # * delete orhpaned instances
    return NodeStatus.unchanged  # don't touch dynamic nodes


def _find_tpu_node_status(nodename, state):
    ns = lkp.node_nodeset(nodename)
    tpuobj = TPU(ns)
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
            return NodeStatus.unknown
        if len(tpus_int) == 1:
            inst = tpuobj.get_node(tpus_int[0])
        # if len(tpus_int ==0) this case is not relevant as this would be the case always that a TPU group is not running
    if inst is None:
        if state.base == "DOWN" and "POWERED_DOWN" in state.flags:
            return NodeStatus.restore
        if "POWERING_DOWN" in state.flags:
            return NodeStatus.restore
        if "COMPLETING" in state.flags:
            return NodeStatus.unbacked
        if state.base != "DOWN" and not (
            set(("POWER_DOWN", "POWERING_UP", "POWERING_DOWN", "POWERED_DOWN"))
            & state.flags
        ):
            return NodeStatus.unbacked
        if lkp.is_static_node(nodename):
            return NodeStatus.resume
    elif (
        state is not None
        and "POWERED_DOWN" not in state.flags
        and "POWERING_DOWN" not in state.flags
        and inst.state == TPU.State.STOPPED
    ):
        if tpuobj.preemptible:
            return NodeStatus.preempted
        if not state.base.startswith("DOWN"):
            return NodeStatus.terminated
    elif (
        state is None or "POWERED_DOWN" in state.flags
    ) and inst.state == TPU.State.READY:
        return NodeStatus.orphan
    elif state is None:
        # if state is None here, the instance exists but it's not in Slurm
        return NodeStatus.unknown

    return NodeStatus.unchanged


def allow_power_down(state):
    config = run(f"{lkp.scontrol} show config").stdout.rstrip()
    m = re.search(r"SuspendExcStates\s+=\s+(?P<states>[\w\(\)]+)", config)
    if not m:
        log.warning("SuspendExcStates not found in Slurm config")
        return True
    states = set(m.group("states").split(","))
    if "(null)" in states or bool(state & state.flags.union(state.base)):
        return False
    return True


def find_node_status(nodename):
    """Determine node/instance status that requires action"""
    state = lkp.slurm_node(nodename)

    if lkp.node_is_dyn(nodename):
        return _find_dynamic_node_status()

    if lkp.node_is_tpu(nodename):
        return _find_tpu_node_status(nodename, state)

    # split below is workaround for VMs whose hostname is FQDN
    inst = lkp.instance(nodename.split(".")[0])
    power_flags = frozenset(
        ("POWER_DOWN", "POWERING_UP", "POWERING_DOWN", "POWERED_DOWN")
    ) & (state.flags if state is not None else set())

    if inst is None:
        if "POWERING_UP" in state.flags:
            return NodeStatus.unchanged
        if state.base == "DOWN" and "POWERED_DOWN" in state.flags:
            return NodeStatus.restore
        if "POWERING_DOWN" in state.flags:
            return NodeStatus.restore
        if "COMPLETING" in state.flags:
            return NodeStatus.unbacked
        if state.base != "DOWN" and not power_flags:
            return NodeStatus.unbacked
        if state.base == "DOWN" and not power_flags and allow_power_down(state):
            return NodeStatus.power_down
        if "POWERED_DOWN" in state.flags and lkp.is_static_node(nodename):
            return NodeStatus.resume
    elif (
        state is not None
        and "POWERED_DOWN" not in state.flags
        and "POWERING_DOWN" not in state.flags
        and inst.status == "TERMINATED"
    ):
        if inst.scheduling.preemptible:
            return NodeStatus.preempted
        if not state.base.startswith("DOWN"):
            return NodeStatus.terminated
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
            return NodeStatus.unchanged
        return NodeStatus.orphan
    elif state is None:
        # if state is None here, the instance exists but it's not in Slurm
        return NodeStatus.unknown

    return NodeStatus.unchanged


def _seconds_since_timestamp(timestamp):
    """Returns duration in seconds since a timestamp
    Args:
        timestamp: A formatted timestamp string (%Y-%m-%dT%H:%M:%S.%f%z)
    Returns:
        number of seconds that have past since the timestamp (float)
    """
    if timestamp[-3] == ":":  # python 36 datetime does not support the colon
        timestamp = timestamp[:-3] + timestamp[-2:]
    creation_dt = datetime.datetime.strptime(timestamp, "%Y-%m-%dT%H:%M:%S.%f%z")
    return datetime.datetime.now().timestamp() - creation_dt.timestamp()


def do_node_update(status, nodes):
    """update node/instance based on node status"""
    if status == NodeStatus.unchanged:
        return
    count = len(nodes)
    hostlist = util.to_hostlist(nodes)

    def nodes_down():
        """down nodes"""
        log.info(
            f"{count} nodes set down due to node status '{status.name}' ({hostlist})"
        )
        run(
            f"{lkp.scontrol} update nodename={hostlist} state=down reason='Instance stopped/deleted'"
        )

    def nodes_restart():
        """start instances for nodes"""
        log.info(f"{count} instances restarted ({hostlist})")
        start_instances(nodes)

    def nodes_idle():
        """idle nodes"""
        log.info(f"{count} nodes to idle ({hostlist})")
        run(f"{lkp.scontrol} update nodename={hostlist} state=resume")

    def nodes_resume():
        """resume nodes via scontrol"""
        log.info(f"{count} instances to resume ({hostlist})")
        run(f"{lkp.scontrol} update nodename={hostlist} state=power_up")

    def nodes_delete():
        """delete instances for nodes"""
        log.info(f"{count} instances to delete ({hostlist})")
        delete_instances(nodes)

    def nodes_power_down():
        """power_down node in slurm"""
        log.info(f"{count} instances to power down ({hostlist})")
        run(f"{lkp.scontrol} update nodename={hostlist} state=power_down")

    def nodes_unknown():
        """Error status, nodes shouldn't get in this status"""
        log.error(f"{count} nodes have unexpected status: ({hostlist})")
        first = next(iter(nodes))
        state = lkp.slurm_node(first)
        state = "{}+{}".format(state.base, "+".join(state.flags)) if state else "None"
        inst = lkp.instance(first)
        log.error(f"{first} state: {state}, instance status:{inst.status}")

    update = dict.get(
        {
            NodeStatus.orphan: nodes_delete,
            NodeStatus.power_down: nodes_power_down,
            NodeStatus.preempted: lambda: (nodes_down(), nodes_restart()),
            NodeStatus.restore: nodes_idle,
            NodeStatus.resume: nodes_resume,
            NodeStatus.terminated: nodes_down,
            NodeStatus.unbacked: nodes_down,
            NodeStatus.unchanged: lambda: None,
            NodeStatus.unknown: nodes_unknown,
        },
        status,
    )
    update()


def delete_placement_groups(placement_groups):
    def delete_placement_request(pg_name, region):
        return compute.resourcePolicies().delete(
            project=lkp.project, region=region, resourcePolicy=pg_name
        )

    requests = {
        pg.name: delete_placement_request(pg["name"], util.trim_self_link(pg["region"]))
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
        f"deleted {len(done)} of {len(placement_groups)} placement groups ({to_hostlist_fast(done.keys())})"
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
        ]
    )

    if lkp.instance_role_safe != "controller":
        return

    keep_jobs = {
        str(job["job_id"])
        for job in json.loads(run(f"{lkp.scontrol} show jobs --json").stdout)["jobs"]
        if "job_state" in job and set(job["job_state"]) & keep_states
    }
    keep_jobs.add("0")  # Job 0 is a placeholder for static node placement

    fields = "items.regions.resourcePolicies,nextPageToken"
    flt = f"name={lkp.cfg.slurm_cluster_name}-*"
    act = compute.resourcePolicies()
    op = act.aggregatedList(project=lkp.project, fields=fields, filter=flt)
    placement_groups = {}
    pg_regex = re.compile(
        rf"{lkp.cfg.slurm_cluster_name}-(?P<partition>[^\s\-]+)-(?P<job_id>\d+)-(?P<index>\d+)"
    )
    while op is not None:
        result = ensure_execute(op)
        # merge placement group info from API and job_id,partition,index parsed from the name
        pgs = (
            NSDict({**pg, **pg_regex.match(pg["name"]).groupdict()})
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
    if lkp.instance_role_safe != "controller":
        return

    compute_instances = [
        name for name, inst in lkp.instances().items() if inst.role == "compute"
    ]
    slurm_nodes = list(lkp.slurm_nodes().keys())

    all_nodes = list(
        set(
            chain(
                compute_instances,
                slurm_nodes,
            )
        )
    )
    log.debug(
        f"reconciling {len(compute_instances)} ({len(all_nodes)-len(compute_instances)}) GCP instances and {len(slurm_nodes)} Slurm nodes ({len(all_nodes)-len(slurm_nodes)})."
    )
    node_statuses = {
        k: list(v) for k, v in util.groupby_unsorted(all_nodes, find_node_status)
    }
    if log.isEnabledFor(logging.DEBUG):
        status_nodelist = {
            status.name: to_hostlist_fast(nodes)
            for status, nodes in node_statuses.items()
        }
        log.debug(f"node statuses: \n{yaml.safe_dump(status_nodelist).rstrip()}")

    for status, nodes in node_statuses.items():
        do_node_update(status, nodes)


def read_hash(filename):
    filename = Path(filename)
    if not filename.exists():
        return None
    with open(filename, "r", encoding="utf-8") as file:
        return file.readline()


def save_hash(filename, hash):
    with open(filename, "w+", encoding="utf-8") as file:
        file.write(hash)


def reconfigure_slurm():
    CONFIG_HASH = Path("/slurm/scripts/.config.hash")
    update_msg = "*** slurm configuration was updated ***"
    cfg_old = load_config_file(CONFIG_FILE)

    if cfg_old.hybrid:
        # terraform handles generating the config.yaml, don't do it here
        return

    hash_new: hashlib.md5 = fetch_config_yaml_md5()
    hash_old: str = read_hash(CONFIG_HASH)

    if hash_new.hexdigest() != hash_old:
        log.debug("Delta detected. Reconfiguring Slurm now.")
        cfg_new = fetch_config_yaml()
        save_hash(CONFIG_HASH, hash_new.hexdigest())
        save_config(cfg_new, CONFIG_FILE)
        cfg_new = load_config_file(CONFIG_FILE)
        lkp = Lookup(cfg_new)
        util.lkp = lkp
        if lkp.instance_role_safe == "controller":
            install_slurm_conf(lkp)
            install_slurmdbd_conf(lkp)
            gen_cloud_conf(lkp)
            gen_cloud_gres_conf(lkp)
            gen_topology_conf(lkp)
            install_gres_conf(lkp)
            install_cgroup_conf(lkp)
            install_topology_conf(lkp)
            log.info("Restarting slurmctld to make changes take effect.")
            try:
                run("sudo systemctl restart slurmctld.service", check=False)
                run(f"{lkp.scontrol} reconfigure", timeout=30)
            except Exception as e:
                log.error(e)
            util.run(f"wall '{update_msg}'", timeout=30)
            log.debug("Done.")
        elif lkp.instance_role_safe in ["compute", "login"]:
            log.info("Restarting slurmd to make changes take effect.")
            run("systemctl restart slurmd")
            util.run(f"wall '{update_msg}'", timeout=30)
            log.debug("Done.")


def main():
    try:
        reconfigure_slurm()
    except Exception:
        log.exception("failed to reconfigure slurm")

    try:
        sync_slurm()
    except Exception:
        log.exception("failed to sync instances")

    try:
        sync_placement_groups()
    except Exception:
        log.exception("failed to sync placement groups")

    try:
        install_custom_scripts(check_hash=True)
    except Exception:
        log.exception("failed to sync custom scripts")


parser = argparse.ArgumentParser(
    description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
)
parser.add_argument(
    "--debug",
    "-d",
    dest="loglevel",
    action="store_const",
    const=logging.DEBUG,
    default=logging.INFO,
    help="Enable debugging output",
)
parser.add_argument(
    "--trace-api",
    "-t",
    action="store_true",
    help="Enable detailed api request output",
)
parser.add_argument(
    "--force",
    "-f",
    action="store_true",
    help="Force tasks to run, regardless of lock.",
)

if __name__ == "__main__":
    args = parser.parse_args()
    util.chown_slurm(LOGFILE, mode=0o600)

    if cfg.enable_debug_logging:
        args.loglevel = logging.DEBUG
    if args.trace_api:
        cfg.extra_logging_flags = list(cfg.extra_logging_flags)
        cfg.extra_logging_flags.append("trace_api")
    util.config_root_logger(filename, level=args.loglevel, logfile=LOGFILE)

    sys.excepthook = util.handle_exception

    # only run one instance at a time unless --force
    if args.force:
        main()
    else:
        pid_file = (Path("/tmp") / Path(__file__).name).with_suffix(".pid")
        with pid_file.open("w") as fp:
            try:
                fcntl.lockf(fp, fcntl.LOCK_EX | fcntl.LOCK_NB)
                main()
            except BlockingIOError:
                sys.exit(0)
