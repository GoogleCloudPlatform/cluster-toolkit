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
from time import sleep

log = logging.getLogger()


def _duration(flex_options: NSDict, job_id: Optional[int], lkp: util.Lookup) -> int:
    dur = flex_options.max_run_duration
    if not job_id or not flex_options.use_job_duration:
        return dur
    
    job = lkp.job(job_id)
    if not job or not job.duration:
        return dur
    
    if timedelta(minutes=10) <= job.duration <= timedelta(weeks=1):
        return int(job.duration.total_seconds())
    
    log.info("Job TimeLimit cannot be less than 10 minutes or exceed one week")
    return dur


def resume_flex_chunk(nodes: List[str], job_id: Optional[int], lkp: util.Lookup) -> None:
  assert nodes
  model = nodes[0]
  nodeset = lkp.node_nodeset(model)
  assert len(nodeset.zone_policy_allow) > 0
  region = lkp.node_region(model)

  assert nodeset.dws_flex.enabled

  uid = str(uuid.uuid4())[:8]
  if job_id:
    mig_name = f"{lkp.cfg.slurm_cluster_name}-{nodeset.nodeset_name}-job-{job_id}-{uid}"
  else:
    mig_name = f"{lkp.cfg.slurm_cluster_name}-{nodeset.nodeset_name}-{uid}"

  util.update_mig_db_entries(nodes,{"MIGOwner":mig_name,"LastAction":"Resume","LastSync":util.now().strftime("%Y-%m-%d %H:%M:%S")})

  # Create MIG
  req = lkp.compute.regionInstanceGroupManagers().insert(
    project=lkp.project,
    region=region,
    body=dict(
      name=mig_name,
      versions=[dict(instanceTemplate=nodeset.instance_template)],
      targetSize=0,
      distributionPolicy=dict(
        zones=[
           dict(zone=f"zones/{z}") for z in nodeset.zone_policy_allow
        ],
        targetShape="ANY_SINGLE_ZONE" ),
      updatePolicy = dict(instanceRedistributionType = "NONE" ),
      instanceLifecyclePolicy=dict(defaultActionOnFailure= "DO_NOTHING" ), # TODO(FLEX): Not supported yet, migrate once supported
    )
  )
  try:
    util.log_api_request(req)
    op = req.execute()
    res = util.wait_for_operation(op)
    assert "error" not in res, f"{res}"
  except:
    util.reset_mig_owner_db(nodes,{"LastAction":"Resume Failed: MIG creation error","LastSync":util.now().strftime("%Y-%m-%d %H:%M:%S")})

  # Create resize request
  req = lkp.compute.regionInstanceGroupManagerResizeRequests().insert(
    project=lkp.project,
    region=region,
    instanceGroupManager=mig_name,
    body=dict(
      name="initial-resize",
      instances=[dict(name=n) for n in nodes],
      requested_run_duration=dict(
        seconds=_duration(nodeset.dws_flex, job_id, lkp)
      )
    )
  )
  try:
    util.log_api_request(req)
    op = req.execute()
    res = util.wait_for_operation(op)
    assert "error" not in res, f"{res}"
  except:
    util.reset_mig_owner_db(nodes,{"LastAction":"Resume Failed: MIG resize request error","LastSync":util.now().strftime("%Y-%m-%d %H:%M:%S")})

def _suspend_flex_mig(mig_self_link: str, nodes: List[str], lkp: util.Lookup) -> None:
  assert nodes
  model = nodes[0]
  nodeset = lkp.node_nodeset(model)
  assert len(nodeset.zone_policy_allow) > 0


  util.reset_mig_owner_db(nodes,{"LastAction":"Suspend","LastSync":util.now().strftime("%Y-%m-%d %H:%M:%S")})

  region = lkp.node_region(model)
  project=lkp.project
  instanceGroupManager=util.trim_self_link(mig_self_link)

  links = [
    f"zones/{inst.zone}/instances/{inst.name}"
    for inst in [
      lkp.instance(node) for node in nodes
    ] if inst
  ]

  target_mig=lkp.get_mig(lkp.project, region, instanceGroupManager)
  assert target_mig

  if target_mig["targetSize"] == len(nodes): #We can just delete the whole MIG in this case
    req = lkp.compute.regionInstanceGroupManagers().delete(
    project=project,
    region=region,
    instanceGroupManager=instanceGroupManager,
    )
  else:
    req = lkp.compute.regionInstanceGroupManagers().deleteInstances(
      project=project,
      region=region,
      instanceGroupManager=instanceGroupManager,
      body=dict(
        instances=links,
        skipInstancesOnValidationError=True,
      )
    )
  try:
    util.log_api_request(req)
    op = req.execute()
    res = util.wait_for_operation(op)
    assert "error" not in res, f"{res}"
  except:
    util.update_mig_db_entries(nodes,{"MIGOwner":instanceGroupManager,"LastAction":"Suspend Failed: MIG Deletion Failed","LastSync":util.now().strftime("%Y-%m-%d %H:%M:%S")})
 
def _suspend_provisioning_inst(mig_name:str, nodes:List[str], lkp: util.Lookup) -> None:
  assert nodes
  model = nodes[0]
  nodeset = lkp.node_nodeset(model)
  assert len(nodeset.zone_policy_allow) > 0
  region = lkp.node_region(model)
  util.reset_mig_owner_db(nodes,{"LastAction":"Suspend","LastSync":util.now().strftime("%Y-%m-%d %H:%M:%S")})

  mig = lkp.get_mig(lkp.project, region, mig_name)

  if not mig or not mig["currentActions"]: 
    util.reset_mig_owner_db(nodes,{"LastAction":"No MIG to delete","LastSync":util.now().strftime("%Y-%m-%d %H:%M:%S")})
    log.info("No matching MIG found to delete!")
    return

  req = lkp.compute.regionInstanceGroupManagers().delete(
    project=lkp.project,
    region=region,
    instanceGroupManager=mig_name,
  )
  try:
    util.log_api_request(req)
    op = req.execute()
    res = util.wait_for_operation(op)
    assert "error" not in res, f"{res}"
  except:
    log.info("No matching MIG deletion failed!")
  # Don't want to wipe out mig associate in case it failed (allow slurmsync to retry)
  util.update_mig_db_entries(nodes,{"MIGOwner":mig_name,"LastAction":"Suspend Failed: MIG Deletion Failed","LastSync":util.now().strftime("%Y-%m-%d %H:%M:%S")})
    

def suspend_flex_nodes(nodes: List[str], lkp: util.Lookup) -> None:
  by_mig = defaultdict(list)
  non_inst_nodes = []
  for node in nodes:
    inst = lkp.instance(node)
    if not inst:
      non_inst_nodes.append(node)
    else:
      mig = inst.metadata.get("created-by")
      if not mig:
        log.error(f"Can not suspend {node}, can not find associated MIG")
        continue
      by_mig[mig].append(node)

  for mig, nodes in by_mig.items():
    _suspend_flex_mig(mig, nodes, lkp)
  
  db_results = util.read_from_mig_db(non_inst_nodes)
  if db_results:
    mig_groups = defaultdict(list)
    for fields in db_results:
      if fields.get('MIGOwner') != "NULL":
        mig_groups[fields.get('MIGOwner')].append(fields.get("Nodename"))

    for provisioning_mig, nodes in mig_groups.items():
      _suspend_provisioning_inst(provisioning_mig, nodes, lkp)
