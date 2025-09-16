# Copyright 2024 Google LLC
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


from typing import Optional, List, Dict, Any

from dataclasses import dataclass
from functools import lru_cache
from collections import defaultdict
import googleapiclient.discovery # type: ignore 
import logging
import subprocess
import json

import util
import resume

log = logging.getLogger()

@dataclass(frozen=True)
class MIG:
  name: str
  target_size: int
  versions: List[str]
  zone: str 

  @classmethod
  def from_json(cls, jo: object) -> "MIG":
    return cls(
      name=jo["name"], # type: ignore
      target_size=jo["targetSize"], # type: ignore
      versions=[v["instanceTemplate"] for v in jo.get("versions", [])], # type: ignore
      zone=util.trim_self_link(jo["zone"]), # type: ignore
    )

@lru_cache
def migs(lkp: util.Lookup, zone: str) -> Dict[str, MIG]:
  resp = lkp.compute.instanceGroupManagers().list(project=lkp.project, zone=zone).execute()
  return {m.name: m for m in [MIG.from_json(o) for o in resp.get('items', [])]}


@lru_cache
def get_mig(lkp: util.Lookup, zone: str, mig_name: str) -> Optional[MIG]:
    try:
        resp = lkp.compute.instanceGroupManagers().get(
            project=lkp.project, zone=zone, instanceGroupManager=mig_name
        ).execute()
        return MIG.from_json(resp)
    except googleapiclient.errors.HttpError as e:
        if e.resp.status == 404:
            return None
        else:
            raise

def create_workload_policy_request(lkp: util.Lookup, nodeset: Dict, topology: str):
    name = f"{lkp.cfg.slurm_cluster_name}-{nodeset['nodeset_name']}"
    zone = nodeset["zone_policy_allow"][0]
    region = '-'.join(zone.split('-')[:2])
    body = {
        "name": name,
        "region": region,
        "workloadPolicy": {
            "type": "HIGH_THROUGHPUT",
            "accelerator_topology": topology,
        },
    }

    workload_req = lkp.compute.resourcePolicies().insert(
          project=lkp.project, region=region, body=body
    )

    return workload_req

def create_mig_request(lkp: util.Lookup, mig: MIG):
  assert len(mig.versions) == 1
  region = '-'.join(mig.zone.split('-')[:2])
  workload_policy_name = f"{'-'.join(mig.name.split('-')[:2])}"

  mig_req = lkp.compute.instanceGroupManagers().insert(
    project=lkp.project,
    zone=mig.zone,
    body = dict(
      name=mig.name,
      versions=[dict(
        instanceTemplate=mig.versions[0])],
      targetSize=mig.target_size,
      # Sensible defaults, allow for changes when needed
      instanceLifecyclePolicy= { "defaultActionOnFailure": "DO_NOTHING" },
      resourcePolicies = {
       "workloadPolicy": f"projects/{lkp.project}/regions/{region}/resourcePolicies/{workload_policy_name}"
      },
    )
  )

  return mig_req


def _allocate_node_to_mig(lkp: util.Lookup, nodes: List[str]) -> Dict[str, List[str]]:
  def slice_id(node: str) -> int:
    accelerator_topology = lkp.node_accelerator_topology(node)
    topo = int(accelerator_topology.split("x")[1]) // lkp.node_template_info(node).gpu.count
    return lkp.node_index(node) // topo

  res : Dict[str, List[str]] = defaultdict(list)
  for _, nodes in util.groupby_unsorted(nodes, lkp.node_nodeset_name):
    nodes = list(nodes)
    ns = lkp.node_nodeset(nodes[0])
    for sid, nodes in util.groupby_unsorted(nodes, slice_id):
      mig_name = f"{lkp.cfg.slurm_cluster_name}-{ns.nodeset_name}-{sid}"
      res[mig_name] = list(nodes)
  return res

def submit_batch_request(requests, resume_data):  
  done, failed = util.batch_execute(requests, log_err=util.swallow_err)

  def ignore_err(e) -> bool:
    return "resourceNotReady" in str(e) or "alreadyExists" in str(e)

  failed = [(n, _, e) for n, (_, e) in failed.items() if not ignore_err(e)]
  if failed:
    for request_id, request, error in failed:
      log.warn(f"Error raised when attempting: {request_id}. Error: {error}")
      request_body_dict = json.loads(request.body)
      failed_nodes_in_mig = [instance['name'] for instance in request_body_dict.get('instances', [])]
      resume.down_nodes_notify_jobs(failed_nodes_in_mig, f"{error}", resume_data)

  for operation_id, operation in done.items():
    try:
      done[operation_id] = util.wait_for_operation(operation)
    except Exception as e:
      log.error(f"Unexpected error waiting for operation {operation_id}: {e}")
      failed[operation_id] = (operation, e)

def resume_slice_nodes(lkp: util.Lookup, nodes: List[str], resume_data):
  mig_requests = {}
  workload_requests = {} # type: ignore
  
  for mig_name, nodes in _allocate_node_to_mig(lkp, nodes).items():
    mig_req, workload_req = _resume_slice_nodes_requests(lkp, mig_name, nodes)

    if mig_req:
      mig_requests[mig_name] = mig_req
      if workload_req not in workload_requests.values(): # type: ignore
        workload_requests[mig_name] = workload_req

  if workload_requests:
    submit_batch_request(workload_requests, resume_data)
  
  if mig_requests:
    submit_batch_request(mig_requests, resume_data)
   
def _resume_slice_nodes_requests(lkp: util.Lookup, mig_name: str, nodes: List[str]):
  assert nodes
  model = nodes[0]
  ns = lkp.node_nodeset(model)
  zone = ns["zone_policy_allow"][0]
  mig = migs(lkp, zone).get(mig_name)
  mig_req = None
  workload_req = None

  if not mig:
    mig = MIG(
      name=mig_name,
      target_size=len(nodes),
      zone=zone,
      versions=[ns.instance_template])
    mig_req = create_mig_request(lkp, mig)
    workload_req = create_workload_policy_request(lkp, ns, ns["accelerator_topology"])

  return mig_req, workload_req


def suspend_slice_nodes(lkp: util.Lookup, nodes: List[str]):
  requests = {}
  for mig_name, nodes in _allocate_node_to_mig(lkp, nodes).items():
    request = _suspend_slice_nodes_request(lkp, mig_name, nodes)
    if request:
      requests[mig_name] = request

  done, failed = util.batch_execute(requests, log_err=util.swallow_err)
  if failed:
    failures = [f"{n}: {e}" for n, (_, e) in failed.items()]
    if failures:
      log.error(f"some mig nodes failed to delete: {failures}")

def _suspend_slice_nodes_request(lkp: util.Lookup, mig_name: str, nodes: List[str]):
  assert nodes
  model = nodes[0]
  ns = lkp.node_nodeset(model)
  zone = ns["zone_policy_allow"][0]

  migs_in_zone = migs(lkp, zone)
  mig_obj = migs_in_zone.get(mig_name)

  if mig_obj is None:
    log.info(f"MIG {mig_name} not found (likely already deleted). Skipping suspend.")
    return None

  links = []
  instance_names = list_instances_in_mig(lkp.project, mig_obj.zone, mig_obj.name)
  for node in nodes:
    if node in instance_names:
      links.append(f"zones/{mig_obj.zone}/instances/{node}")
    else:
      log.info(f"Instance {node} is not part of MIG {mig_name}. Skipping.")

  op = lkp.compute.instanceGroupManagers().deleteInstances(
    project=lkp.project, zone=mig_obj.zone, instanceGroupManager=mig_obj.name,
    body=dict(
      instances=links,
      skipInstancesOnValidationError=True,
    )
  )

  return op


def is_slice_node(node: str) -> bool:
  return util.lookup().node_accelerator_topology(node) is not None

def delete_workload_policies(lkp: util.Lookup, migs: List[MIG]):
    requests = {
        f"{mig.name}": lkp.compute.resourcePolicies().delete(
            project=lkp.project,
            region='-'.join(mig.zone.split('-')[:2]),
            resourcePolicy=f"{'-'.join(mig.name.split('-')[:2])}")
        for mig in migs
    }

    done, failed = util.batch_execute(requests, log_err=util.swallow_err)
    if failed:
        def ignore_err(e) -> bool:
            return "resourceInUseByAnotherResource" in str(e)

        failures = [f"{n}: {e}" for n, (_, e) in failed.items() if not ignore_err(e)]
        if failures:
            log.error(f"some workload policies failed to delete: {failures}")
    log.info(
        f"deleted {len(done)} of {len(migs)} workload policies ({util.to_hostlist(done.keys())})"
    )

def delete_migs(lkp: util.Lookup, migs: List[MIG]):
    requests = {
        mig.name: lkp.compute.instanceGroupManagers().delete(
            project=lkp.project,
            zone=mig.zone,
            instanceGroupManager=mig.name)
        for mig in migs
    }

    done, failed = util.batch_execute(requests, log_err=util.swallow_err)
    if failed:
        def ignore_err(e) -> bool:
            return "resourceInUseByAnotherResource" in str(e)

        failures = [f"{n}: {e}" for n, (_, e) in failed.items() if not ignore_err(e)]
        if failures:
            log.error(f"some mig groups failed to delete: {failures}")
    log.info(
        f"deleted {len(done)} of {len(migs)} mig groups ({util.to_hostlist(done.keys())})"
    )

def mig_details(lkp: util.Lookup, mig: MIG):
    result = lkp.compute.instanceGroupManagers().get(
        project=lkp.project,
        zone=mig.zone,
        instanceGroupManager=mig.name
    ).execute()

    return result

def list_instances_in_mig(project_id: str, zone: str, mig_name: str) -> List[str]:
    instance_names = []
    result = util.lookup().compute.instanceGroupManagers().listManagedInstances(
        project=project_id,
        zone=zone,
        instanceGroupManager=mig_name
    ).execute()

    for item in result.get('managedInstances', []):
      instance_names.append(item['instance'].split('/')[-1])
    return instance_names
