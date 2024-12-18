# Copyright 2024 "Google LLC"
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

from typing import List

import socket
import logging
from dataclasses import dataclass
from pathlib import Path
import yaml

import util
from util import create_client_options, ApiEndpoint

from google.cloud import tpu_v2 as tpu  # noqa: E402
import google.api_core.exceptions as gExceptions  # noqa: E402

log = logging.getLogger()

_tpu_cache = {}

class TPU:
    """Class for handling the TPU-vm nodes"""

    State = tpu.types.cloud_tpu.Node.State
    TPUS_PER_VM = 4
    __expected_states = {
        "create": State.READY,
        "start": State.READY,
        "stop": State.STOPPED,
    }

    __tpu_version_mapping = {
        "V2": tpu.AcceleratorConfig().Type.V2,
        "V3": tpu.AcceleratorConfig().Type.V3,
        "V4": tpu.AcceleratorConfig().Type.V4,
    }

    @classmethod
    def make(cls, nodeset_name: str, lkp: util.Lookup) -> "TPU":
        key = (id(lkp), nodeset_name)
        if key not in _tpu_cache:
          nodeset = lkp.cfg.nodeset_tpu[nodeset_name]
          _tpu_cache[key] = cls(nodeset, lkp)
        return _tpu_cache[key]
        

    def __init__(self, nodeset: object, lkp: util.Lookup):
        self._nodeset = nodeset
        self.lkp = lkp
        self._parent = f"projects/{lkp.project}/locations/{nodeset.zone}"
        co = create_client_options(ApiEndpoint.TPU)
        self._client = tpu.TpuClient(client_options=co)
        self.data_disks = []
        for data_disk in nodeset.data_disks:
            ad = tpu.AttachedDisk()
            ad.source_disk = data_disk
            ad.mode = tpu.AttachedDisk.DiskMode.DISK_MODE_UNSPECIFIED
            self.data_disks.append(ad)
        ns_ac = nodeset.accelerator_config
        if ns_ac.topology != "" and ns_ac.version != "":
            ac = tpu.AcceleratorConfig()
            ac.topology = ns_ac.topology
            ac.type_ = self.__tpu_version_mapping[ns_ac.version]
            self.ac = ac
        else:
            req = tpu.GetAcceleratorTypeRequest(
                name=f"{self._parent}/acceleratorTypes/{nodeset.node_type}"
            )
            self.ac = self._client.get_accelerator_type(req).accelerator_configs[0]
        self.vmcount = self.__calc_vm_from_topology(self.ac.topology)

    @property
    def nodeset(self):
        return self._nodeset

    @property
    def preserve_tpu(self):
        return self._nodeset.preserve_tpu

    @property
    def node_type(self):
        return self._nodeset.node_type

    @property
    def tf_version(self):
        return self._nodeset.tf_version

    @property
    def enable_public_ip(self):
        return self._nodeset.enable_public_ip

    @property
    def preemptible(self):
        return self._nodeset.preemptible

    @property
    def reserved(self):
        return self._nodeset.reserved

    @property
    def service_account(self):
        return self._nodeset.service_account

    @property
    def zone(self):
        return self._nodeset.zone

    def check_node_type(self):
        if self.node_type is None:
            return False
        try:
            request = tpu.GetAcceleratorTypeRequest(
                name=f"{self._parent}/acceleratorTypes/{self.node_type}"
            )
            return self._client.get_accelerator_type(request=request) is not None
        except Exception:
            return False

    def check_tf_version(self):
        try:
            request = tpu.GetRuntimeVersionRequest(
                name=f"{self._parent}/runtimeVersions/{self.tf_version}"
            )
            return self._client.get_runtime_version(request=request) is not None
        except Exception:
            return False

    def __calc_vm_from_topology(self, topology):
        topo = topology.split("x")
        tot = 1
        for num in topo:
            tot = tot * int(num)
        return tot // self.TPUS_PER_VM

    def __check_resp(self, response, op_name):
        des_state = self.__expected_states.get(op_name)
        # If the state is not in the table just print the response
        if des_state is None:
            return False
        if response.__class__.__name__ != "Node":  # If the response is not a node fail
            return False
        if response.state == des_state:
            return True
        return False

    def list_nodes(self):
        try:
            request = tpu.ListNodesRequest(parent=self._parent)
            res = self._client.list_nodes(request=request)
        except gExceptions.NotFound:
            res = None
        return res

    def list_node_names(self):
        return [node.name.split("/")[-1] for node in self.list_nodes()]

    def start_node(self, nodename):
        request = tpu.StartNodeRequest(name=f"{self._parent}/nodes/{nodename}")
        resp = self._client.start_node(request=request).result()
        return self.__check_resp(resp, "start")

    def stop_node(self, nodename):
        request = tpu.StopNodeRequest(name=f"{self._parent}/nodes/{nodename}")
        resp = self._client.stop_node(request=request).result()
        return self.__check_resp(resp, "stop")

    def get_node(self, nodename):
        try:
            request = tpu.GetNodeRequest(name=f"{self._parent}/nodes/{nodename}")
            res = self._client.get_node(request=request)
        except gExceptions.NotFound:
            res = None
        return res

    def _register_node(self, nodename, ip_addr):
        dns_name = socket.getnameinfo((ip_addr, 0), 0)[0]
        util.run(
            f"{self.lkp.scontrol} update nodename={nodename} nodeaddr={ip_addr} nodehostname={dns_name}"
        )

    def create_node(self, nodename):
        if self.vmcount > 1 and not isinstance(nodename, list):
            log.error(
                f"Tried to create a {self.vmcount} node TPU on nodeset {self._nodeset.nodeset_name} but only received one nodename {nodename}"
            )
            return False
        if self.vmcount > 1 and (
            isinstance(nodename, list) and len(nodename) != self.vmcount
        ):
            log.error(
                f"Expected to receive a list of {self.vmcount} nodenames for TPU node creation in nodeset {self._nodeset.nodeset_name}, but received this list {nodename}"
            )
            return False

        node = tpu.Node()
        node.accelerator_config = self.ac
        node.runtime_version = f"tpu-vm-tf-{self.tf_version}"
        startup_script = """
        #!/bin/bash
        echo "startup script not found > /var/log/startup_error.log"
        """
        with open(
            Path(self.lkp.cfg.slurm_scripts_dir or util.dirs.scripts) / "startup.sh", "r"
        ) as script:
            startup_script = script.read()
        if isinstance(nodename, list):
            node_id = nodename[0]
            slurm_names = []
            wid = 0
            for node_wid in nodename:
                slurm_names.append(f"WORKER_{wid}:{node_wid}")
                wid += 1
        else:
            node_id = nodename
            slurm_names = [f"WORKER_0:{nodename}"]
        node.metadata = {
            "slurm_docker_image": self.nodeset.docker_image,
            "startup-script": startup_script,
            "slurm_instance_role": "compute",
            "slurm_cluster_name": self.lkp.cfg.slurm_cluster_name,
            "slurm_bucket_path": self.lkp.cfg.bucket_path,
            "slurm_names": ";".join(slurm_names),
            "universe_domain": util.universe_domain(),
        }
        node.tags = [self.lkp.cfg.slurm_cluster_name]
        if self.nodeset.service_account:
            node.service_account.email = self.nodeset.service_account.email
            node.service_account.scope = self.nodeset.service_account.scopes
        node.scheduling_config.preemptible = self.preemptible
        node.scheduling_config.reserved = self.reserved
        node.network_config.subnetwork = self.nodeset.subnetwork
        node.network_config.enable_external_ips = self.enable_public_ip
        if self.data_disks:
            node.data_disks = self.data_disks

        request = tpu.CreateNodeRequest(parent=self._parent, node=node, node_id=node_id)
        resp = self._client.create_node(request=request).result()
        if not self.__check_resp(resp, "create"):
            return False
        if isinstance(nodename, list):
            for node_id, net_endpoint in zip(nodename, resp.network_endpoints):
                self._register_node(node_id, net_endpoint.ip_address)
        else:
            ip_add = resp.network_endpoints[0].ip_address
            self._register_node(nodename, ip_add)
        return True

    def delete_node(self, nodename):
        request = tpu.DeleteNodeRequest(name=f"{self._parent}/nodes/{nodename}")
        try:
            resp = self._client.delete_node(request=request).result()
            if resp:
                return self.get_node(nodename=nodename) is None
            return False
        except gExceptions.NotFound:
            # log only error if vmcount is 1 as for other tpu vm count, this could be "phantom" nodes
            if self.vmcount == 1:
                log.error(f"Tpu single node {nodename} not found")
            else:
                # for the TPU nodes that consist in more than one vm, only the first node of the TPU a.k.a. the master node will
                # exist as real TPU nodes, so the other ones are expected to not be found, check the hostname of the node that has
                # not been found, and if it ends in 0, it means that is the master node and it should have been found, and in consequence
                # log an error
                nodehostname = yaml.safe_load(
                    util.run(f"{self.lkp.scontrol} --yaml show node {nodename}").stdout.rstrip()
                )["nodes"][0]["hostname"]
                if nodehostname.split("-")[-1] == "0":
                    log.error(f"TPU master node {nodename} not found")
                else:
                    log.info(f"Deleted TPU 'phantom' node {nodename}")
            # If the node is not found it is tecnichally deleted, so return success.
            return True

def _stop_tpu(node: str) -> None:
    lkp = util.lookup()
    tpuobj = TPU.make(lkp.node_nodeset_name(node), lkp)
    if tpuobj.nodeset.preserve_tpu and tpuobj.vmcount == 1:
        log.info(f"stopping node {node}")
        if tpuobj.stop_node(node):
            return
        log.error("Error stopping node {node} will delete instead")
    log.info(f"deleting node {node}")
    if not tpuobj.delete_node(node):
        log.error("Error deleting node {node}")


def delete_tpu_instances(instances: List[str]) -> None:
    util.execute_with_futures(_stop_tpu, instances)


def start_tpu(node: List[str]):
    lkp = util.lookup()
    tpuobj = TPU.make(lkp.node_nodeset_name(node[0]), lkp)

    if len(node) == 1:
        node = node[0]
        log.debug(
            f"Will create a TPU of type {tpuobj.node_type} tf_version {tpuobj.tf_version} in zone {tpuobj.zone} with name {node}"
        )
        tpunode = tpuobj.get_node(node)
        if tpunode is None:
            if not tpuobj.create_node(nodename=node):
                log.error("Error creating tpu node {node}")
        else:
            if tpuobj.preserve_tpu:
                if not tpuobj.start_node(nodename=node):
                    log.error("Error starting tpu node {node}")
            else:
                log.info(
                    f"Tpu node {node} is already created, but will not start it because nodeset does not have preserve_tpu option active."
                )
    else:
        log.debug(
            f"Will create a multi-vm TPU of type {tpuobj.node_type} tf_version {tpuobj.tf_version} in zone {tpuobj.zone} with name {node[0]}"
        )
        if not tpuobj.create_node(nodename=node):
            log.error("Error creating tpu node {node}")
