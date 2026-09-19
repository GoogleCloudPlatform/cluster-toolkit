#!/slurm/python/venv/bin/python3.13

# Copyright (C) SchedMD LLC.
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

from collections import defaultdict
import logging
import os
import re
from typing import Any, List, Tuple
import yaml

import conf
from conf_v2511 import SlurmConfigGeneratorV2511
import util
from util import NSDict

log = logging.getLogger()
TOPOLOGY_TPU_BLOCK = "tpu-block-topo"

class SlurmConfigGeneratorV2605(SlurmConfigGeneratorV2511):
    """Slurm 26.05 configuration generator."""

    def _add_tpu_conflines(self, conf_options: dict[str, Any]) -> None:
        """Adds TPU conf lines (GresTypes, JobSubmitPlugins, CliFilterPlugins) to slurm.conf options."""
        for param in ("GresTypes", "JobSubmitPlugins", "CliFilterPlugins"):
            val = conf_options.get(param, None)
            if not val:
                conf_options[param] = ["tpu"]
            elif isinstance(val, str):
                conf_options[param] = [val]
            if "tpu" not in conf_options[param]:
                conf_options[param].append("tpu")

    def _append_feature(self, node_conf: dict[str, Any], feature: str) -> None:
        """Appends a feature string to the node configuration's Feature list."""
        existing = node_conf.get("Feature")
        if existing:
            feature_list = existing.split(",") if isinstance(existing, str) else list(existing)
        else:
            feature_list = []
        if feature not in feature_list:
            feature_list.append(feature)
        node_conf["Feature"] = feature_list

    def _install_tpu_conf(self) -> None:
        """Installs the tpu.conf file by renaming tpu.conf.example in the image."""
        if not (self.lkp.etc_dir / "tpu.conf.example").exists() or (self.lkp.etc_dir / "tpu.conf").exists():
          return
        os.rename(self.lkp.etc_dir / "tpu.conf.example", self.lkp.etc_dir / "tpu.conf")
        util.chown_slurm(self.lkp.etc_dir / "tpu.conf", mode=0o644)

    def topology_type(self, partition: NSDict) -> str:
        if self.lkp.is_tpu_static_partition(partition):
            log.info(f"Partition {partition.partition_name} is using TPU block topology")
            return TOPOLOGY_TPU_BLOCK
        return super().topology_type(partition)

    def get_conf_options(self) -> dict:
        conf_options = super().get_conf_options()
        if self.lkp.has_tpu_nodesets():
            self._add_tpu_conflines(conf_options)
        return conf_options

    def make_cloud_conf(self) -> str:
        for nodeset in self.lkp.cfg.nodeset.values():
            if tpu := self.lkp.node_tpu_info(nodeset):
                nodeset.node_conf["Gres"] = f"tpu:{tpu.type}:{tpu.tpus_per_node}"
                if self.lkp.is_tpu_static_nodeset(nodeset.nodeset_name):
                    if topo := self.lkp.nodeset_accelerator_topology(nodeset.nodeset_name):
                        self._append_feature(nodeset.node_conf, f"{tpu.type}_{topo}")
                elif self.lkp.is_tpu_dynamic_nodeset(nodeset.nodeset_name):
                    self._append_feature(nodeset.node_conf, "tpu_dynamic")
        return super().make_cloud_conf()

    def partitionlines(self, partition) -> str:
        if self.lkp.is_tpu_partition(partition):
            partition.partition_conf.setdefault("Oversubscribe", "Exclusive")
            if self.lkp.is_tpu_static_partition(partition):
                partition.partition_conf.setdefault("PowerDownOnIdle", "NO")
            elif self.lkp.is_tpu_dynamic_partition(partition):
                partition.partition_conf.setdefault("PowerDownOnIdle", "YES")
        return super().partitionlines(partition)

    def gen_cloud_gres_conf_lines(self) -> str:
        base_lines = super().gen_cloud_gres_conf_lines()
        if not self.lkp.has_tpu_nodesets():
            return base_lines

        tpu_nodes: defaultdict[Tuple[int, str], List[str]] = defaultdict(list)
        for nodeset in self.lkp.cfg.nodeset.values():
            nodelist = self.lkp.nodelist(nodeset)
            if nodelist and (tpu_info := self.lkp.node_tpu_info(nodeset)):
                tpu_nodes[(tpu_info.tpus_per_node, tpu_info.type)].append(nodelist)

        tpu_lines = [
            conf.dict_to_conf({
                "NodeName": names,
                "Name": "tpu",
                "Type": tpu_type,
                "File": f"/dev/vfio/devices/vfio[0-{i-1}]" if i > 1 else "/dev/vfio/devices/vfio0",
            })
            for (i, tpu_type), names in tpu_nodes.items()
        ]
        combined = [line for line in base_lines.splitlines() if line.strip()] + tpu_lines
        combined.append("\n")
        return "\n".join(combined)

    def install_cgroup_conf(self) -> None:
        if not self.lkp.remove_device_constrain():
            return super().install_cgroup_conf()

        cgroup_conf = re.sub(
            r"^ConstrainDevices=yes$",
            "ConstrainDevices=no",
            self.lkp.cfg.cgroup_conf_tpl,
            flags=re.MULTILINE,
        )
        conf_file = self.lkp.etc_dir / "cgroup.conf"
        conf_file.write_text(cgroup_conf)
        util.chown_slurm(conf_file, mode=0o600)

    def generate_topology_data(self) -> Tuple[bool, Any]:
        if not self.lkp.has_tpu_nodesets():
            return super().generate_topology_data()

        topo = conf.gen_topology(self.lkp).compress()
        block_is_default = any(self.lkp.has_block_topology(p) for p in self.lkp.cfg.partitions.values())
        sections = topo.render_yaml(block_is_default=block_is_default)

        tpu_blocks = []
        for ns in self.lkp.cfg.nodeset.values():
            if self.lkp.is_tpu_static_nodeset(ns.nodeset_name):
                chunk_size = self.lkp.get_tpu_chunk_size(ns)
                static_nodes = list(self.lkp.nodenames(ns)[0])
                chunks_dict = self.lkp.group_tpu_nodes_by_chunk_idx(static_nodes, chunk_size)
                for idx in sorted(chunks_dict.keys()):
                    blk = conf.Block(name=ns.nodeset_name, nodes=list(chunks_dict[idx]), block_uuid=str(idx)).render_yaml_block()
                    if blk:
                        tpu_blocks.append(blk)

        if tpu_blocks:
            sections.append({
                "topology": TOPOLOGY_TPU_BLOCK,
                "cluster_default": False,
                "block": {
                    "block_sizes": [1],
                    "blocks": tpu_blocks,
                },
            })

        yaml_file = self.lkp.etc_dir / "cloud_topology.yaml"
        with open(yaml_file, "w") as f:
            f.write(conf.FILE_PREAMBLE)
            f.write("---\n\n")
            yaml.dump(sections, f, sort_keys=False)

        if yaml_file.stat().st_size % 4096 == 0:
            with yaml_file.open("a") as f:
                f.write("\n")

        prev_summary = conf.TopologySummary.load(self.lkp)
        return topo.summary.requires_reconfigure(prev_summary), topo.summary

    def generate_configs(self) -> None:
        super().generate_configs()
        if self.lkp.has_tpu_nodesets():
            self._install_tpu_conf()
