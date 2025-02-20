# Copyright 2025 "Google LLC"
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

from typing import List, Optional

import util
import uuid
from addict import Dict as NSDict # type: ignore
from datetime import timedelta
from collections import defaultdict
import logging

log = logging.getLogger()


def _duration(flex_options: NSDict, job_id: Optional[int], lkp: util.Lookup) -> int:
    dur = flex_options.max_run_duration
    if not job_id or not flex_options.use_job_duration:
        return dur
    
    job = lkp.job(job_id)
    if not job or not job.duration:
        return dur
    
    if timedelta(seconds=30) <= job.duration <= timedelta(weeks=1):
        return int(job.duration.total_seconds())
    
    log.info("Job TimeLimit cannot be less than 30 seconds or exceed one week")
    return dur


def resume_flex_chunk(nodes: List[str], job_id: Optional[int], lkp: util.Lookup) -> None:
  assert nodes
  model = nodes[0]
  nodeset = lkp.node_nodeset(model)
  zones = nodeset.zone_policy_allow
  assert len(zones) == 1
  zone = zones[0]

  assert nodeset.dws_flex.enabled

  uid = str(uuid.uuid4())[:8]
  if job_id:
    mig_name = f"{lkp.cfg.slurm_cluster_name}-{nodeset.nodeset_name}-job-{job_id}-{uid}"
  else:
    mig_name = f"{lkp.cfg.slurm_cluster_name}-{nodeset.nodeset_name}-{uid}"
  
  # Create MIG
  req = lkp.compute.instanceGroupManagers().insert(
    project=lkp.project,
    zone=zone,
    body=dict(
      name=mig_name,
      versions=[dict(
        instanceTemplate=nodeset.instance_template)],
      targetSize=0,
      # TODO(FLEX): uncomment once moved to RMIG
      # distributionPolicy=dict(
      #   zones=[
      #      dict(zone=f"zones/{z}") for z in nodeset.zone_policy_allow
      #   ],
      #   targetShape="ANY_SINGLE_ZONE" ),
      #updatePolicy = dict(
      #  instanceRedistributionType = "NONE" ),
      instanceLifecyclePolicy=dict(
          defaultActionOnFailure= "DO_NOTHING" ), # TODO(FLEX): Not supported yet, migrate once supported
    )
  )
  util.log_api_request(req)
  op = req.execute()
  res = util.wait_for_operation(op)
  assert "error" not in res, f"{res}"
  
  # Create resize request
  req = lkp.compute.instanceGroupManagerResizeRequests().insert(
    project=lkp.project,
    zone=zone,
    instanceGroupManager=mig_name,
    body=dict(
      name="initial-resize",
      instances=[dict(name=n) for n in nodes],
      requested_run_duration=dict(
        seconds=_duration(nodeset.dws_flex, job_id, lkp)
      )
    )
  )
  util.log_api_request(req)
  op = req.execute()
  res = util.wait_for_operation(op)
  assert "error" not in res, f"{res}"


def _suspend_flex_mig(mig_self_link: str, nodes: List[str], lkp: util.Lookup) -> None:
  assert nodes

  links = [
    f"zones/{inst.zone}/instances/{inst.name}"
    for inst in [
      lkp.instance(node) for node in nodes
    ] if inst
  ]
   
  # TODO(FLEX): This will not work if MIG didn't obtain capacity yet.
  # The request will fail and MIG will continue provisioning.
  # Instead whole MIG should be deleted.
  # + All other instances in MIG are not provisioned also, safe to delete
  # - Need to come up wil clear test to differentiate non-provisioned MIG and single VM being down;
  #   Particularly CRITICAL due to ActionOnFailure=DO_NOTHING 
  # - Need to `down_nodes_notify_jobs` for all nodes in MIG, make sure that it doesn't interfere with Slurm suspend-flow. 
  req = lkp.compute.instanceGroupManagers().deleteInstances(
    project=lkp.project,
    zone=util.parse_self_link(mig_self_link).zone,
    instanceGroupManager=util.trim_self_link(mig_self_link),
    body=dict(
      instances=links,
      skipInstancesOnValidationError=True,
    )
  )
  util.log_api_request(req)
  op = req.execute()
   
  res = util.wait_for_operation(op)
  assert "error" not in res, f"{res}"


def suspend_flex_nodes(nodes: List[str], lkp: util.Lookup) -> None:
  by_mig = defaultdict(list)
  for node in nodes:
    inst = lkp.instance(node)
    if not inst:
      log.error(f"Can not suspend {node}, instance not found")
      continue
    mig = inst.metadata.get("created-by")
    if not mig:
        log.error(f"Can not suspend {node}, can not find associated MIG")
        continue
    by_mig[mig].append(node)
  
  for mig, nodes in by_mig.items():
    _suspend_flex_mig(mig, nodes, lkp)


  
