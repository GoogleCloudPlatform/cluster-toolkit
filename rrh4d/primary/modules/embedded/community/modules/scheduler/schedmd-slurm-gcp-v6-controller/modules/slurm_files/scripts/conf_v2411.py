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

from typing import List, Optional, Iterable, Dict, Set, Tuple
from itertools import chain
from collections import defaultdict
import logging
import json
import conf
from pathlib import Path
import util
from util import dirs, slurmdirs
from addict import Dict as NSDict # type: ignore

FILE_PREAMBLE = """
# Warning:
# This file is managed by a script. Manual modifications will be overwritten.
"""
log = logging.getLogger()
_SLURM_TOPO_ROOT = "slurm-root"

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
        return conf.dict_to_conf(d)

    def render_conf_lines(self) -> Iterable[str]:
        yield self.conf_line()
        for s in sorted(self.switches.values(), key=lambda s: s.name):
            yield from s.render_conf_lines()

class TopologyBuilder:
    def __init__(self) -> None:
        self._r = Switch("")  # fake root, not part of the tree
        self.summary = conf.TopologySummary()

    def add(self, path: List[str], nodes: Iterable[str]) -> None:
        n = self._r
        assert path
        for p in path:
            n = n.switches.setdefault(p, Switch(p))
        n.nodes = [*n.nodes, *nodes]

    def render_conf_lines(self) -> Iterable[str]:
        if not self._r.switches:
            return [] # type: ignore
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

def gen_topology(lkp: util.Lookup) -> TopologyBuilder:
    bldr = TopologyBuilder()
    for ns in lkp.cfg.nodeset_tpu.values():
        conf.add_tpu_nodeset_topology(ns, bldr, lkp)
    for ns in lkp.cfg.nodeset.values():
        conf.add_nodeset_topology(ns, bldr, lkp)
    return bldr

def gen_topology_conf(lkp: util.Lookup) -> Tuple[bool, conf.TopologySummary]:
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

    prev_summary = conf.TopologySummary.load(lkp)
    return topo.summary.requires_reconfigure(prev_summary), topo.summary

def install_topology_conf(lkp: util.Lookup) -> None:
    conf_file = lkp.etc_dir / "cloud_topology.conf"
    summary_file = lkp.etc_dir / "cloud_topology.summary.json"
    topo_conf = lkp.etc_dir / "topology.conf"

    if not topo_conf.exists():
        topo_conf.symlink_to(conf_file)

    util.chown_slurm(conf_file, mode=0o600)
    util.chown_slurm(summary_file, mode=0o600)

def generate_configs_slurm_v2411(lkp: util.Lookup) -> None:
    conf.install_slurm_conf(lkp)
    conf.install_slurmdbd_conf(lkp)
    conf.gen_cloud_conf(lkp)
    conf.gen_cloud_gres_conf(lkp)
    conf.install_gres_conf(lkp)
    conf.install_cgroup_conf(lkp)
    # do not generate topology until nodesets are present
    if lkp.cfg.nodeset:
        _, summary = gen_topology_conf(lkp)
        summary.dump(lkp)
        install_topology_conf(lkp)
