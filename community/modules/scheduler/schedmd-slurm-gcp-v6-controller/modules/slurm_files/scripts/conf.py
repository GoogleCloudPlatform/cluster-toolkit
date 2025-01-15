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

from typing import List, Optional, Iterable, Dict, Set, Tuple
from itertools import chain
from collections import defaultdict
import json
from pathlib import Path
import util
from util import dirs, slurmdirs
import tpu

FILE_PREAMBLE = """
# Warning:
# This file is managed by a script. Manual modifications will be overwritten.
"""



def dict_to_conf(conf, delim=" ") -> str:
    """convert dict to delimited slurm-style key-value pairs"""

    def filter_conf(pair):
        k, v = pair
        if isinstance(v, list):
            v = ",".join(str(el) for el in v if el is not None)
        return k, (v if bool(v) or v == 0 else None)

    return delim.join(
        f"{k}={v}" for k, v in map(filter_conf, conf.items()) if v is not None
    )


TOPOLOGY_PLUGIN_TREE = "topology/tree"

def topology_plugin(lkp: util.Lookup) -> str:
    """
    Returns configured topology plugin, defaults to `topology/tree`.
    """
    cp, key = lkp.cfg.cloud_parameters, "topology_plugin"
    if key not in cp or cp[key] is None:
        return TOPOLOGY_PLUGIN_TREE
    return cp[key]

def conflines(lkp: util.Lookup) -> str:
    params = lkp.cfg.cloud_parameters
    def get(key, default):
        """
        Returns the value of the key in params if it exists and is not None, 
        otherwise returns supplied default.
        We can't rely on the `dict.get` method because the value could be `None` as 
        well as empty NSDict, depending on type of the `cfg.cloud_parameters`.
        TODO: Simplify once NSDict is removed from the codebase.
        """
        if key not in params or params[key] is None:
            return default
        return params[key]
    
    no_comma_params = get("no_comma_params", False)

    any_gpus = any(
        lkp.template_info(nodeset.instance_template).gpu
        for nodeset in lkp.cfg.nodeset.values()
    )

    any_tpu = any(
        tpu_nodeset is not None
        for part in lkp.cfg.partitions.values()
        for tpu_nodeset in part.partition_nodeset_tpu
    )

    any_dynamic = any(bool(p.partition_feature) for p in lkp.cfg.partitions.values())
    comma_params = {
        "LaunchParameters": [
            "enable_nss_slurm",
            "use_interactive_step",
        ],
        "SlurmctldParameters": [
            "cloud_reg_addrs" if any_dynamic or any_tpu else "cloud_dns",
            "enable_configless",
            "idle_on_node_suspend",
        ],
        "GresTypes": [
            "gpu" if any_gpus else None,
        ],
    }

    scripts_dir = lkp.cfg.install_dir or dirs.scripts
    prolog_path = Path(dirs.custom_scripts / "prolog.d")
    epilog_path = Path(dirs.custom_scripts / "epilog.d")
    default_tree_width = 65533 if any_dynamic else 128

    conf_options = {
        **(comma_params if not no_comma_params else {}),
        "Prolog": f"{prolog_path}/*" if lkp.cfg.prolog_scripts else None,
        "Epilog": f"{epilog_path}/*" if lkp.cfg.epilog_scripts else None,
        "PrivateData": get("private_data", []),
        "SchedulerParameters": get("scheduler_parameters", [
            "bf_continue",
            "salloc_wait_nodes",
            "ignore_prefer_validation",
        ]),
        "ResumeProgram": f"{scripts_dir}/resume.py",
        "ResumeFailProgram": f"{scripts_dir}/suspend.py",
        "ResumeRate": get("resume_rate", 0),
        "ResumeTimeout": get("resume_timeout", 300),
        "SuspendProgram": f"{scripts_dir}/suspend.py",
        "SuspendRate": get("suspend_rate", 0),
        "SuspendTimeout": get("suspend_timeout", 300),
        "TreeWidth": get("tree_width", default_tree_width),
        "JobSubmitPlugins": "lua" if any_tpu else None,
        "TopologyPlugin": topology_plugin(lkp),
        "TopologyParam": get("topology_param", "SwitchAsNodeRank"),
    }
    return dict_to_conf(conf_options, delim="\n")




def nodeset_lines(nodeset, lkp: util.Lookup) -> str:
    template_info = lkp.template_info(nodeset.instance_template)
    machine_conf = lkp.template_machine_conf(nodeset.instance_template)

    # follow https://slurm.schedmd.com/slurm.conf.html#OPT_Boards
    # by setting Boards, SocketsPerBoard, CoresPerSocket, and ThreadsPerCore
    gres = f"gpu:{template_info.gpu.count}" if template_info.gpu else None
    node_conf = {
        "RealMemory": machine_conf.memory,
        "Boards": machine_conf.boards,
        "SocketsPerBoard": machine_conf.sockets_per_board,
        "CoresPerSocket": machine_conf.cores_per_socket,
        "ThreadsPerCore": machine_conf.threads_per_core,
        "CPUs": machine_conf.cpus,
        "Gres": gres,
        **nodeset.node_conf,
    }
    nodelist = lkp.nodelist(nodeset)

    return "\n".join(
        map(
            dict_to_conf,
            [
                {"NodeName": nodelist, "State": "CLOUD", **node_conf},
                {"NodeSet": nodeset.nodeset_name, "Nodes": nodelist},
            ],
        )
    )


def nodeset_tpu_lines(nodeset, lkp: util.Lookup) -> str:
    nodelist = lkp.nodelist(nodeset)
    return "\n".join(
        map(
            dict_to_conf,
            [
                {"NodeName": nodelist, "State": "CLOUD", **nodeset.node_conf},
                {"NodeSet": nodeset.nodeset_name, "Nodes": nodelist},
            ],
        )
    )


def nodeset_dyn_lines(nodeset):
    """generate slurm NodeSet definition for dynamic nodeset"""
    return dict_to_conf(
        {"NodeSet": nodeset.nodeset_name, "Feature": nodeset.nodeset_feature}
    )


def partitionlines(partition, lkp: util.Lookup) -> str:
    """Make a partition line for the slurm.conf"""
    MIN_MEM_PER_CPU = 100

    def defmempercpu(nodeset_name: str) -> int:
        nodeset = lkp.cfg.nodeset.get(nodeset_name)
        template = nodeset.instance_template
        machine = lkp.template_machine_conf(template)
        mem_spec_limit = int(nodeset.node_conf.get("MemSpecLimit", 0))
        return max(MIN_MEM_PER_CPU, (machine.memory - mem_spec_limit) // machine.cpus)

    defmem = min(
        map(defmempercpu, partition.partition_nodeset), default=MIN_MEM_PER_CPU
    )

    nodesets = list(
        chain(
            partition.partition_nodeset,
            partition.partition_nodeset_dyn,
            partition.partition_nodeset_tpu,
        )
    )

    is_tpu = len(partition.partition_nodeset_tpu) > 0
    is_dyn = len(partition.partition_nodeset_dyn) > 0

    oversub_exlusive = partition.enable_job_exclusive or is_tpu
    power_down_on_idle = partition.enable_job_exclusive and not is_dyn

    line_elements = {
        "PartitionName": partition.partition_name,
        "Nodes": ",".join(nodesets),
        "State": "UP",
        "DefMemPerCPU": defmem,
        "SuspendTime": 300,
        "Oversubscribe": "Exclusive" if oversub_exlusive else None,
        "PowerDownOnIdle": "YES" if power_down_on_idle else None,
        **partition.partition_conf,
    }

    return dict_to_conf(line_elements)


def suspend_exc_lines(lkp: util.Lookup) -> Iterable[str]:
    static_nodelists = []
    for ns in lkp.power_managed_nodesets():
        if ns.node_count_static:
            nodelist = lkp.nodelist_range(ns.nodeset_name, 0, ns.node_count_static)
            static_nodelists.append(nodelist)
    suspend_exc_nodes = {"SuspendExcNodes": static_nodelists}

    dyn_parts = [
        p.partition_name
        for p in lkp.cfg.partitions.values()
        if len(p.partition_nodeset_dyn) > 0
    ]
    suspend_exc_parts = {"SuspendExcParts": [*dyn_parts]}

    return filter(
        None,
        [
            dict_to_conf(suspend_exc_nodes) if static_nodelists else None,
            dict_to_conf(suspend_exc_parts),
        ],
    )


def make_cloud_conf(lkp: util.Lookup) -> str:
    """generate cloud.conf snippet"""
    lines = [
        FILE_PREAMBLE,
        conflines(lkp),
        *(nodeset_lines(n, lkp) for n in lkp.cfg.nodeset.values()),
        *(nodeset_dyn_lines(n) for n in lkp.cfg.nodeset_dyn.values()),
        *(nodeset_tpu_lines(n, lkp) for n in lkp.cfg.nodeset_tpu.values()),
        *(partitionlines(p, lkp) for p in lkp.cfg.partitions.values()),
        *(suspend_exc_lines(lkp)),
    ]
    return "\n\n".join(filter(None, lines))


def gen_cloud_conf(lkp: util.Lookup) -> None:
    content = make_cloud_conf(lkp)

    conf_file = lkp.etc_dir / "cloud.conf"
    conf_file.write_text(content)
    util.chown_slurm(conf_file, mode=0o644)


def install_slurm_conf(lkp: util.Lookup) -> None:
    """install slurm.conf"""
    if lkp.cfg.ompi_version:
        mpi_default = "pmi2"
    else:
        mpi_default = "none"

    conf_options = {
        "name": lkp.cfg.slurm_cluster_name,
        "control_addr": lkp.control_addr if lkp.control_addr else lkp.hostname_fqdn,
        "control_host": lkp.control_host,
        "control_host_port": lkp.control_host_port,
        "scripts": dirs.scripts,
        "slurmlog": dirs.log,
        "state_save": slurmdirs.state,
        "mpi_default": mpi_default,
    }

    conf = lkp.cfg.slurm_conf_tpl.format(**conf_options)

    conf_file = lkp.etc_dir / "slurm.conf"
    conf_file.write_text(conf)
    util.chown_slurm(conf_file, mode=0o644)


def install_slurmdbd_conf(lkp: util.Lookup) -> None:
    """install slurmdbd.conf"""
    conf_options = {
        "control_host": lkp.control_host,
        "slurmlog": dirs.log,
        "state_save": slurmdirs.state,
        "db_name": "slurm_acct_db",
        "db_user": "slurm",
        "db_pass": '""',
        "db_host": "localhost",
        "db_port": "3306",
    }

    if lkp.cfg.cloudsql_secret:
        secret_name = f"{lkp.cfg.slurm_cluster_name}-slurm-secret-cloudsql"
        payload = json.loads(util.access_secret_version(lkp.project, secret_name))

        if payload["db_name"] and payload["db_name"] != "":
            conf_options["db_name"] = payload["db_name"]
        if payload["user"] and payload["user"] != "":
            conf_options["db_user"] = payload["user"]
        if payload["password"] and payload["password"] != "":
            conf_options["db_pass"] = payload["password"]

        db_host_str = payload["server_ip"].split(":")
        if db_host_str[0]:
            conf_options["db_host"] = db_host_str[0]
            conf_options["db_port"] = (
                db_host_str[1] if len(db_host_str) >= 2 else "3306"
            )

    conf = lkp.cfg.slurmdbd_conf_tpl.format(**conf_options)

    conf_file = lkp.etc_dir / "slurmdbd.conf"
    conf_file.write_text(conf)
    util.chown_slurm(conf_file, 0o600)


def install_cgroup_conf(lkp: util.Lookup) -> None:
    """install cgroup.conf"""
    conf_file = lkp.etc_dir / "cgroup.conf"
    conf_file.write_text(lkp.cfg.cgroup_conf_tpl)
    util.chown_slurm(conf_file, mode=0o600)


def install_jobsubmit_lua(lkp: util.Lookup) -> None:
    """install job_submit.lua if there are tpu nodes in the cluster"""
    if not any(
        tpu_nodeset is not None
        for part in lkp.cfg.partitions.values()
        for tpu_nodeset in part.partition_nodeset_tpu
    ):
        return # No TPU partitions, no need for job_submit.lua
    
    scripts_dir = lkp.cfg.slurm_scripts_dir or dirs.scripts
    tpl = (scripts_dir / "job_submit.lua.tpl").read_text()
    conf = tpl.format(scripts_dir=scripts_dir)

    conf_file = lkp.etc_dir / "job_submit.lua"
    conf_file.write_text(conf)
    util.chown_slurm(conf_file, 0o600)


def gen_cloud_gres_conf(lkp: util.Lookup) -> None:
    """generate cloud_gres.conf"""

    gpu_nodes = defaultdict(list)
    for nodeset in lkp.cfg.nodeset.values():
        ti = lkp.template_info(nodeset.instance_template)
        gpu_count = ti.gpu.count if ti.gpu  else 0
        if gpu_count:
            gpu_nodes[gpu_count].append(lkp.nodelist(nodeset))

    lines = [
        dict_to_conf(
            {
                "NodeName": names,
                "Name": "gpu",
                "File": "/dev/nvidia{}".format(f"[0-{i-1}]" if i > 1 else "0"),
            }
        )
        for i, names in gpu_nodes.items()
    ]
    lines.append("\n")
    content = FILE_PREAMBLE + "\n".join(lines)

    conf_file = lkp.etc_dir / "cloud_gres.conf"
    conf_file.write_text(content)
    util.chown_slurm(conf_file, mode=0o600)


def install_gres_conf(lkp: util.Lookup) -> None:
    conf_file = lkp.etc_dir / "cloud_gres.conf"
    gres_conf = lkp.etc_dir / "gres.conf"
    if not gres_conf.exists():
        gres_conf.symlink_to(conf_file)
    util.chown_slurm(gres_conf, mode=0o600)


class Switch:
    """
    Represents a switch in the topology.conf file.
    NOTE: It's class user job to make sure that there is no leaf-less Switches in the tree
    """

    def __init__(
        self,
        name: str,
        nodes: Optional[Iterable[str]] = None,
        switches: Optional[Dict[str, "Switch"]] = None,
    ):
        self.name = name
        self.nodes = nodes or []
        self.switches = switches or {}

    def conf_line(self) -> str:
        d = {"SwitchName": self.name}
        if self.nodes:
            d["Nodes"] = util.to_hostlist(self.nodes)
        if self.switches:
            d["Switches"] = util.to_hostlist(self.switches.keys())
        return dict_to_conf(d)

    def render_conf_lines(self) -> Iterable[str]:
        yield self.conf_line()
        for s in sorted(self.switches.values(), key=lambda s: s.name):
            yield from s.render_conf_lines()

class TopologySummary:
    """
    Represents a summary of the topology, to make judgements about changes.
    To be stored in JSON file along side of topology.conf to simplify parsing.
    """
    def __init__(
            self,
            physical_host: Optional[Dict[str, str]] = None,
            down_nodes: Optional[Iterable[str]] = None,
            tpu_nodes: Optional[Iterable[str]] = None,
        ) -> None:
        self.physical_host = physical_host or {}
        self.down_nodes = set(down_nodes or [])
        self.tpu_nodes = set(tpu_nodes or [])


    @classmethod
    def path(cls, lkp: util.Lookup) -> Path:
        return lkp.etc_dir / "cloud_topology.summary.json"

    @classmethod
    def loads(cls, s: str) -> "TopologySummary":
        d = json.loads(s)
        return cls(
            physical_host=d.get("physical_host"),
            down_nodes=d.get("down_nodes"),
            tpu_nodes=d.get("tpu_nodes"),
        )
    
    @classmethod
    def load(cls, lkp: util.Lookup) -> "TopologySummary":
        p = cls.path(lkp)
        if not p.exists():
            return cls() # Return empty instance
        return cls.loads(p.read_text())
    
    def dumps(self) -> str:
        return json.dumps(
            {
                "physical_host": self.physical_host,
                "down_nodes": list(self.down_nodes),
                "tpu_nodes": list(self.tpu_nodes),
            },
            indent=2)
    
    def dump(self, lkp: util.Lookup) -> None:
        TopologySummary.path(lkp).write_text(self.dumps())
    
    def _nodenames(self) -> Set[str]:
        return set(self.physical_host) | self.down_nodes | self.tpu_nodes
        
    def requires_reconfigure(self, prev: "TopologySummary") -> bool:
        """
        Reconfigure IFF one of the following occurs:
        * A node is added
        * A node get a non-empty physicalHost
        """
        if len(self._nodenames() - prev._nodenames()) > 0:
            return True
        for n, ph in self.physical_host.items():
            if ph and ph != prev.physical_host.get(n):
                return True
        return False

class TopologyBuilder:
    def __init__(self) -> None:
        self._r = Switch("")  # fake root, not part of the tree
        self.summary = TopologySummary()

    def add(self, path: List[str], nodes: Iterable[str]) -> None:
        n = self._r
        assert path
        for p in path:
            n = n.switches.setdefault(p, Switch(p))
        n.nodes = [*n.nodes, *nodes]

    def render_conf_lines(self) -> Iterable[str]:
        if not self._r.switches:
            return []
        for s in sorted(self._r.switches.values(), key=lambda s: s.name):
            yield from s.render_conf_lines()

    def compress(self) -> "TopologyBuilder":
        compressed = TopologyBuilder()
        compressed.summary = self.summary
        def _walk(
            u: Switch, c: Switch
        ):  # u: uncompressed node, c: its counterpart in compressed tree
            pref = f"{c.name}_" if c != compressed._r else "s"
            for i, us in enumerate(sorted(u.switches.values(), key=lambda s: s.name)):
                cs = Switch(f"{pref}{i}", nodes=us.nodes)
                c.switches[cs.name] = cs
                _walk(us, cs)

        _walk(self._r, compressed._r)
        return compressed


def add_tpu_nodeset_topology(nodeset: object, bldr: TopologyBuilder, lkp: util.Lookup):
    tpuobj = tpu.TPU.make(nodeset.nodeset_name, lkp)
    static, dynamic = lkp.nodenames(nodeset)

    pref = ["tpu-root",  f"ns_{nodeset.nodeset_name}"]
    if tpuobj.vmcount == 1:  # Put all nodes in one switch
        all_nodes = list(chain(static, dynamic))
        bldr.add(pref, all_nodes)
        bldr.summary.tpu_nodes.update(all_nodes)
        return

    # Chunk nodes into sub-switches of size `vmcount`
    chunk_num = 0
    for nodenames in (static, dynamic):
        for nodeschunk in util.chunked(nodenames, n=tpuobj.vmcount):
            chunk_name = f"{nodeset.nodeset_name}-{chunk_num}"
            chunk_num += 1
            bldr.add([*pref, chunk_name], nodeschunk)
            bldr.summary.tpu_nodes.update(nodeschunk)

_SLURM_TOPO_ROOT = "slurm-root"

def _make_physical_path(physical_host: str) -> List[str]:
    assert physical_host.startswith("/"), f"Unexpected physicalHost: {physical_host}"
    parts = physical_host[1:].split("/")
    # Due to issues with Slurm's topology plugin, we can not use all components of `physicalHost`,
    # trim it down to `cluster/rack`.
    short_path = parts[:2]
    return [_SLURM_TOPO_ROOT, *short_path]

def add_nodeset_topology(
    nodeset: object, bldr: TopologyBuilder, lkp: util.Lookup
) -> None:
    up_nodes = set()
    default_path = [_SLURM_TOPO_ROOT,  f"ns_{nodeset.nodeset_name}"]

    for inst in lkp.instances().values():
        try:
            if lkp.node_nodeset_name(inst.name) != nodeset.nodeset_name:
                continue
        except Exception:
            continue
    
        phys_host = inst.resourceStatus.get("physicalHost", "")
        bldr.summary.physical_host[inst.name] = phys_host
        up_nodes.add(inst.name)

        if phys_host:
            bldr.add(_make_physical_path(phys_host), [inst.name])
        else:
            bldr.add(default_path, [inst.name])
        
    down_nodes = []
    for node in chain(*lkp.nodenames(nodeset)):
        if node not in up_nodes:
            down_nodes.append(node)
    if down_nodes:
        bldr.add(default_path, down_nodes)
        bldr.summary.down_nodes.update(down_nodes)

def gen_topology(lkp: util.Lookup) -> TopologyBuilder:
    bldr = TopologyBuilder()
    for ns in lkp.cfg.nodeset_tpu.values():
        add_tpu_nodeset_topology(ns, bldr, lkp)
    for ns in lkp.cfg.nodeset.values():
        add_nodeset_topology(ns, bldr, lkp)
    return bldr

def gen_topology_conf(lkp: util.Lookup) -> Tuple[bool, TopologySummary]:
    """
    Generates slurm topology.conf.
    Returns whether the topology.conf got updated.
    """
    topo = gen_topology(lkp).compress()
    conf_file = lkp.etc_dir / "cloud_topology.conf"

    with open(conf_file, "w") as f:
        f.writelines(FILE_PREAMBLE + "\n")
        for line in topo.render_conf_lines():
            f.write(line)
            f.write("\n")
        f.write("\n")

    prev_summary = TopologySummary.load(lkp)
    return topo.summary.requires_reconfigure(prev_summary), topo.summary

def install_topology_conf(lkp: util.Lookup) -> None:
    conf_file = lkp.etc_dir / "cloud_topology.conf"
    summary_file = lkp.etc_dir / "cloud_topology.summary.json"
    topo_conf = lkp.etc_dir / "topology.conf"
    
    if not topo_conf.exists():
        topo_conf.symlink_to(conf_file)

    util.chown_slurm(conf_file, mode=0o600)
    util.chown_slurm(summary_file, mode=0o600)


def gen_controller_configs(lkp: util.Lookup) -> None:
    install_slurm_conf(lkp)
    install_slurmdbd_conf(lkp)
    gen_cloud_conf(lkp)
    gen_cloud_gres_conf(lkp)
    install_gres_conf(lkp)
    install_cgroup_conf(lkp) 
    install_jobsubmit_lua(lkp)

    if topology_plugin(lkp) == TOPOLOGY_PLUGIN_TREE:
        _, summary = gen_topology_conf(lkp)
        summary.dump(lkp)
        install_topology_conf(lkp)
