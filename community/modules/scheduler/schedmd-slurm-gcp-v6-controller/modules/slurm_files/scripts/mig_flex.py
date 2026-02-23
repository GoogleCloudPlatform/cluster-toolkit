# Copyright 2026 "Google LLC"
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
from datetime import datetime, timedelta
from collections import defaultdict
import logging
from time import sleep

log = logging.getLogger()

DWS_EOL_RESERVATION_DURATION = 10 # minutes

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

def _create_slurm_reservation(node_name: str, boot_time: datetime, run_duration: int, lkp: util.Lookup):
    """
    Create a Slurm reservation starting at EOL - buffer time.
    """
    eol = boot_time + timedelta(seconds=run_duration)
    start_str = eol.strftime("%Y-%m-%dT%H:%M:%S")
    reservation_name = f"dws-eol-{node_name}"
    log.debug(f"creating slurm reservation for {node_name}")
    try:
        util.run(f"{lkp.scontrol} create reservation user=slurm starttime={start_str} duration={DWS_EOL_RESERVATION_DURATION} nodes={node_name} reservationname={reservation_name} flags=maint,ignore_jobs")
    except Exception as e:
        log.error(f"Failed to create reservation for {node_name}: {e}")

def _delete_slurm_reservation(node_name: str, lkp: util.Lookup):
    """
    Delete the Slurm reservation for the given node.
    """
    reservation_name = f"dws-eol-{node_name}"
    try:
        util.run(f"{lkp.scontrol} delete reservation {reservation_name}")
        log.debug(f"Deleted Slurm reservation {reservation_name} for {node_name}")
    except Exception as e:
        log.error(f"Failed to delete reservation for {node_name}: {e}")

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
  util.log_api_request(req)
  op = req.execute()
  res = util.wait_for_operation(op)
  assert "error" not in res, f"{res}"

  # Create resize request
  duration_seconds = _duration(nodeset.dws_flex, job_id, lkp)
  req = lkp.compute.regionInstanceGroupManagerResizeRequests().insert(
    project=lkp.project,
    region=region,
    instanceGroupManager=mig_name,
    body=dict(
      name="initial-resize",
      instances=[dict(name=n) for n in nodes],
      requested_run_duration=dict(
        seconds=duration_seconds
      )
    )
  )
  util.log_api_request(req)
  op = req.execute()
  res = util.wait_for_operation(op)

  # Create Slurm reservations if use_job_duration is set
  if nodeset.dws_flex.use_job_duration:
      # Get run duration (seconds)
      run_duration = duration_seconds
      for node_name in nodes:
          # Fetch instance creation time from GCP instance (via util.py)
          instance = lkp.instance(node_name)
          if(instance and instance.creation_timestamp):
            log.debug("creating with creation_timestamp")
            boot_time = instance.creation_timestamp  # Already a datetime object
          else:
            boot_time = datetime.utcnow()
            log.debug("creating with utcnow time: {boot_time}")
          _create_slurm_reservation(node_name, boot_time, run_duration, lkp)

  assert "error" not in res, f"{res}"

def _suspend_flex_mig(mig_self_link: str, nodes: List[str], lkp: util.Lookup) -> None:
  assert nodes
  model = nodes[0]
  nodeset = lkp.node_nodeset(model)
  assert len(nodeset.zone_policy_allow) > 0
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

  # TODO(FLEX): This will not work if MIG didn't obtain capacity yet.
  # The request will fail and MIG will continue provisioning.
  # Instead whole MIG should be deleted.
  # + All other instances in MIG are not provisioned also, safe to delete
  # - Need to come up will clear test to differentiate non-provisioned MIG and single VM being down;
  #   Particularly CRITICAL due to ActionOnFailure=DO_NOTHING 
  # - Need to `down_nodes_notify_jobs` for all nodes in MIG, make sure that it doesn't interfere with Slurm suspend-flow. 
  
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
  
  util.log_api_request(req)
  op = req.execute()
   
  res = util.wait_for_operation(op)

  # Delete Slurm reservations for nodes being deprovisioned
  for node_name in nodes:
      log.info("delete dws reservation")
      _delete_slurm_reservation(node_name, lkp)

  assert "error" not in res, f"{res}"

def _suspend_provisioning_inst(nodes:List[str], node_template:str, lkp: util.Lookup) -> None:
  assert nodes
  model = nodes[0]
  nodeset = lkp.node_nodeset(model)
  assert len(nodeset.zone_policy_allow) > 0
  region = lkp.node_region(model)

  mig_list=lkp.get_mig_list(lkp.project, region)

  # FLEX (#TODO): If we enter this conditional it's likely this was called so early that MIG creation hasn't started
  # Consider potentially retrying? No natural mechanism for retry currently but we could
  # perhaps use slurmsync and then try it again to ensure it wasn't a case of being too early.
  # This is important since we're now enabling long ResumeTimeout (Slurm won't call suspend on node within reasonable timeframe) 
  # so until we do this is slurmsync this is a temporary workaround.

  if not mig_list or not mig_list.get("items"):
    log.info("No matching MIG found to delete! Retrying...")
    sleep(5)
    mig_list=lkp.get_mig_list(lkp.project, region)
    if not mig_list or not mig_list.get("items"):
      return

  for mig in mig_list["items"]:
    if mig["instanceTemplate"] == node_template:
      if mig["currentActions"]["creating"] > 0 and mig["targetSize"] == mig["currentActions"]["creating"]:
        req = lkp.compute.regionInstanceGroupManagers().delete(
          project=lkp.project,
          region=region,
          instanceGroupManager=util.trim_self_link(mig["selfLink"]),
        )

        util.log_api_request(req)
        op = req.execute()
        
        res = util.wait_for_operation(op)
        assert "error" not in res, f"{res}"
        return
  
  log.info("No matching MIG found to delete!")

def suspend_flex_nodes(nodes: List[str], lkp: util.Lookup) -> None:
  by_mig = defaultdict(list)
  not_provisioned = defaultdict(list)
  for node in nodes:
    inst = lkp.instance(node)
    if not inst:
      not_provisioned[lkp.node_template(node)].append(node)
    else:
      mig = inst.metadata.get("created-by")
      if not mig:
        log.error(f"Can not suspend {node}, can not find associated MIG")
        continue
      by_mig[mig].append(node)

  for mig, nodes in by_mig.items():
    _suspend_flex_mig(mig, nodes, lkp)
  
  for node_template, nodes in not_provisioned.items():
    _suspend_provisioning_inst(nodes, node_template, lkp)
