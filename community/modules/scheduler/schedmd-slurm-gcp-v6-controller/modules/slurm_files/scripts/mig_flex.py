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
from util import NSDict
from datetime import datetime, timedelta
from collections import defaultdict
import logging
from time import sleep
import dataclasses
import re
import shlex
import googleapiclient.errors  # type: ignore

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

def resume_flex_chunk(nodes: List[str], job_id: Optional[int], lkp: util.Lookup, placement_group: Optional[str] = None) -> None:
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
  body = dict(
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
  if placement_group:
    body["resourcePolicies"] = {
        "workloadPolicy": f"regions/{region}/resourcePolicies/{placement_group}"
    }



  req = lkp.compute.regionInstanceGroupManagers().insert(
    project=lkp.project,
    region=region,
    body=body
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

  # NOTE: If the MIG hasn't obtained capacity yet, instances are not provisioned,
  # and the suspend flow routes to `_suspend_provisioning_inst` where unprovisioned/queued
  # MIGs are fully deleted. This path handles partially or fully provisioned MIGs.
  
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
      actions = mig.get("currentActions", {})
      # If targetSize > 0 but no instances are running normally (none == 0),
      # the MIG is either actively creating instances or queued waiting for compact capacity.
      # Fully deleting it upon ResumeTimeout prevents orphaned MIGs.
      if mig.get("targetSize", 0) > 0 and actions.get("none", 0) == 0:
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

@dataclasses.dataclass
class WorkloadPolicy:
    """Represents a Workload Policy."""
    acceleratorTopology: str
    type: str

@dataclasses.dataclass
class ResourcePolicy:
    """Represents a Resource Policy containing a Workload Policy."""
    name: str
    region: str
    workloadPolicy: WorkloadPolicy

def get_instance_template_copy(nodeset: NSDict, lkp: util.Lookup) -> str:
    """Gets or creates a copy of the instance template with TPU/reservation overrides."""
    source_template_name = util.trim_self_link(nodeset.instance_template)
    new_template_name = source_template_name.replace("-compute-", "-copy-")
    new_self_link = nodeset.instance_template.replace(source_template_name, new_template_name)
    try:
        lkp.compute.instanceTemplates().get(
            project=lkp.project, instanceTemplate=new_template_name
        ).execute()
        return new_self_link
    except googleapiclient.errors.HttpError as e:
        if e.resp.status != 404:
            raise
    except Exception:
        pass
    source_template = (
        lkp.compute.instanceTemplates()
        .get(project=lkp.project, instanceTemplate=source_template_name)
        .execute()
    )
    properties = dict(source_template.get("properties") or {})
    properties["labels"] = {
        **(properties.get("labels") or {}),
        "slurm_template_role": "copy",
    }
    # TPU reservations require RESERVATION_BOUND and DELETE on termination in the template
    if getattr(nodeset, "reservation_name", None):
        local_prefix = f"projects/{lkp.project}/reservations/"
        res_name = (
            nodeset.reservation_name[len(local_prefix):]
            if nodeset.reservation_name.startswith(local_prefix)
            else nodeset.reservation_name
        )
        properties["reservationAffinity"] = {
            "consumeReservationType": "SPECIFIC_RESERVATION",
            "key": f"compute.{util.universe_domain()}/reservation-name",
            "values": [res_name],
        }
        scheduling = properties.get("scheduling") or {}
        properties["scheduling"] = scheduling
        scheduling["provisioningModel"] = "RESERVATION_BOUND"
        scheduling["instanceTerminationAction"] = "DELETE"
    # Remove threadsPerCore=1 default since TPU machine types reject custom SMT settings
    if lkp.is_tpu_nodeset(nodeset.nodeset_name):
        if isinstance(properties.get("advancedMachineFeatures"), dict):
            properties["advancedMachineFeatures"].pop("threadsPerCore", None)
        scheduling = properties.get("scheduling") or {}
        if scheduling.get("provisioningModel") == "SPOT" or scheduling.get("preemptible"):
            scheduling["instanceTerminationAction"] = "DELETE"
    req = lkp.compute.instanceTemplates().insert(
        project=lkp.project,
        body=dict(
            name=new_template_name,
            properties=properties,
            description=f"Cluster Toolkit copy of {source_template_name}",
        ),
    )
    util.log_api_request(req)
    try:
        op = req.execute()
    except googleapiclient.errors.HttpError as insert_err:
        if insert_err.resp.status != 409:
            raise
        log.info("Instance template %s already exists, skipping creation.", new_template_name)
    else:
        res = util.wait_for_operation(op)
        if "error" in res:
            raise RuntimeError(f"Failed to copy instance template: {res}")
    return new_self_link

def get_mig_for_node(node: str, lkp: util.Lookup) -> tuple[Optional[str], List[str], bool]:
    """Returns (mig_self_link, all_nodes_in_mig, is_creating) for the MIG managing the given node."""
    # Check VM metadata first; if VM hasn't booted yet, fall back to matching MIG description (slurm_nodes:...)
    inst = lkp.instance(node)
    mig_link = inst.metadata.get("created-by") if inst else None
    # Query MIGs in zone (if single zone) or region to find description & peer nodes
    nodeset = lkp.node_nodeset(node)
    zones = getattr(nodeset, "zone_policy_allow", None) or []
    mig_items = []
    try:
        mig_list = (
            lkp.get_mig_list(lkp.project, zone=zones[0])
            if len(zones) == 1
            else lkp.get_mig_list(lkp.project, lkp.node_region(node))
        )
        if mig_list and "items" in mig_list:
            mig_items.extend(mig_list["items"])
    except Exception as e:
        log.error("Failed to list MIGs for node %s: %s", node, e)
    for mig_item in mig_items:
        desc = mig_item.get("description") or ""
        peers = util.to_hostnames(desc.split(":", 1)[1]) if desc.startswith("slurm_nodes:") else []
        if (mig_link and util.trim_self_link(mig_item.get("selfLink", "")) == util.trim_self_link(mig_link)) or (node in peers):
            actions = mig_item.get("currentActions") or {}
            is_creating = (actions.get("creating", 0) + actions.get("creatingWithoutRetries", 0)) > 0
            return mig_item.get("selfLink") or mig_link, peers, is_creating
    return (mig_link, [node], False) if mig_link else (None, [], False)

def _get_tpu_full_chunk(model: str, expected_size: int, lkp: util.Lookup) -> List[str]:
    """Returns the full static slice chunk of TPU nodes that 'model' belongs to."""
    node_idx = lkp.node_index(model)
    chunk_start = (node_idx // expected_size) * expected_size
    prefix = lkp.node_prefix(model)
    return [f"{prefix}-{i}" for i in range(chunk_start, chunk_start + expected_size)]

def resume_tpu_chunk(
    chunk: List[str],
    job_id: Optional[int],
    lkp: util.Lookup,
    topology: Optional[str] = None,
) -> None:
    """Creates zonal/regional TPU MIG to satisfy GCE topology bounds for the chunk of nodes."""
    if not chunk:
        return
    log.info("Scaling TPU nodeset: resuming nodes %s", chunk)
    model = chunk[0]
    nodeset = lkp.node_nodeset(model)
    if lkp.is_static_node(model):
        topology = lkp.nodeset_accelerator_topology(nodeset.nodeset_name)
    else:
        topology = topology or lkp.nodeset_accelerator_topology(nodeset.nodeset_name)
    if not topology:
        raise RuntimeError(
            f"INVALID_FIELD_VALUE: Missing --layout for dynamic TPU nodes {util.to_hostlist(chunk)}. "
            "Jobs on dynamic TPU partitions must specify --layout=<tpu_type>=<topology> (e.g., --layout=tpu7x=2x2x2)."
        )
    tot_chips = 1
    for dim in topology.lower().split("x"):  # type: ignore[union-attr]
        tot_chips *= int(dim)
    tpus_per_node = 4
    template = lkp.template_info(nodeset.instance_template)
    match = re.search(r"-(\d+)t$", template.machine_type.name.lower())
    if match:
        tpus_per_node = int(match.group(1))
    vmcount = max(1, tot_chips // tpus_per_node)
    log.info(
        "TPU slice size derived: %s VMs per slice (%s chips, %s/node)",
        vmcount,
        tot_chips,
        tpus_per_node,
    )
    # Static TPU slice protection: prevent booting a partial slice
    if lkp.is_static_node(model) and len(chunk) != vmcount:
        log.warning(
            "Only %d of %d nodes in static TPU slice (%s) are ready to be resumed, skipping resume",
            len(chunk),
            vmcount,
            nodeset.nodeset_name,
        )
        full_chunk = _get_tpu_full_chunk(model, vmcount, lkp)
        nodelist = util.to_hostlist(full_chunk)
        reason = shlex.quote("Node cannot be resumed, not all nodes in the target TPU slice are ready to be resumed")
        util.run(f"{lkp.scontrol} update nodename={nodelist} state=POWER_DOWN_FORCE reason={reason}", check=False)
        return
    for slice_chunk in util.chunked(chunk, n=vmcount):
        _resume_single_tpu_node(slice_chunk, job_id, lkp, topology=topology)

def _resume_single_tpu_node(
    chunk: List[str],
    job_id: Optional[int],
    lkp: util.Lookup,
    topology: Optional[str] = None,
) -> None:
    """Resumes a single TPU slice by creating its own dedicated zonal/regional MIG."""
    first_node = chunk[0]
    if lkp.is_static_node(first_node):
        existing_mig, existing_peers, is_creating = get_mig_for_node(first_node, lkp)
        if existing_mig:
            if not is_creating and any(
                (inst := lkp.instance(n)) is None or inst.status == "TERMINATED"
                for n in (existing_peers or chunk)
            ):
                log.warning(
                    "Deleting dead static TPU MIG %s (no VMs and not creating) before re-provisioning.",
                    existing_mig,
                )
                _delete_tpu_mig(existing_mig, existing_peers or chunk, lkp)
            else:
                # Reuse existing static MIG to avoid hitting GCE API limits and losing queue position
                log.info(
                    "TPU node %s already managed by MIG %s. Skipping.", first_node, existing_mig
                )
                return
    else:
        # Delete leftover dynamic MIG from any previous cancelled job across all slice nodes
        seen_migs: set[str] = set()
        for n in chunk:
            existing_mig, existing_peers, _ = get_mig_for_node(n, lkp)
            if existing_mig and existing_mig not in seen_migs:
                seen_migs.add(existing_mig)
                log.warning(
                    "Deleting stale dynamic TPU MIG %s for nodes %s before resuming new slice.",
                    existing_mig,
                    existing_peers or chunk,
                )
                _delete_tpu_mig(existing_mig, existing_peers or chunk, lkp)
    model = chunk[0]
    nodeset = lkp.node_nodeset(model)
    region = lkp.node_region(model)
    # Static nodes get topology from blueprint; dynamic nodes get it from --layout
    if lkp.is_static_node(model):
        topology = lkp.nodeset_accelerator_topology(nodeset.nodeset_name)
    else:
        topology = topology or lkp.nodeset_accelerator_topology(nodeset.nodeset_name)
    if not topology:
        raise RuntimeError(
            f"INVALID_FIELD_VALUE: Missing --layout for dynamic TPU nodes {util.to_hostlist(chunk)}. "
            "Jobs on dynamic TPU partitions must specify --layout=<tpu_type>=<topology> (e.g., --layout=tpu7x=2x2x2)."
        )
    wp_name = (
        f"{lkp.cfg.slurm_cluster_name}-slurmgcp-{nodeset.nodeset_name}-wp-"
        f"{topology.replace('x', '-')}"  # type: ignore[union-attr]
    )
    workload_policy = WorkloadPolicy(
        acceleratorTopology=topology,  # type: ignore[arg-type]
        type="HIGH_THROUGHPUT",
    )
    policy = ResourcePolicy(
        name=wp_name,
        region=region,
        workloadPolicy=workload_policy,
    )
    wp_resource_name = _ensure_tpu_workload_policy(policy, lkp)
    mig_name = f"{lkp.cfg.slurm_cluster_name}-{nodeset.nodeset_name}-{uuid.uuid4().hex[:8]}"
    zones = nodeset.zone_policy_allow or []
    is_zonal = len(zones) == 1
    # Create MIG with targetSize=0 and workload policy, then boot VMs via createInstances
    mig_req_body = dict(
        name=mig_name,
        versions=[dict(instanceTemplate=get_instance_template_copy(nodeset, lkp))],
        targetSize=0,
        targetSizePolicy=dict(mode="BULK"),
        description=f"slurm_nodes:{util.to_hostlist(chunk)}",
        instanceLifecyclePolicy=dict(defaultActionOnFailure="DO_NOTHING"),
        resourcePolicies=dict(workloadPolicy=wp_resource_name),
    )
    if not is_zonal:
        mig_req_body["distributionPolicy"] = dict(
            zones=[dict(zone=f"zones/{z}") for z in zones],
            targetShape="ANY_SINGLE_ZONE",
        )
        mig_req_body["updatePolicy"] = dict(instanceRedistributionType="NONE")
    if is_zonal:
        log.info("Creating Zonal TPU MIG %s in zone %s", mig_name, zones[0])
        req = lkp.compute.instanceGroupManagers().insert(
            project=lkp.project,
            zone=zones[0],
            body=mig_req_body,
        )
    else:
        log.info("Creating Regional TPU MIG %s", mig_name)
        req = lkp.compute.regionInstanceGroupManagers().insert(
            project=lkp.project,
            region=region,
            body=mig_req_body,
        )
    util.log_api_request(req)
    op = req.execute()
    res = util.wait_for_operation(op)
    if "error" in res:
        errs = res.get("error", {}).get("errors", [])
        err_str = "; ".join(f"{e.get('code', 'ERROR')}: {e.get('message', '')}" for e in errs) or str(res)
        raise RuntimeError(f"TPU MIG creation operation failed: {err_str}")
    log.info("TPU MIG creation of %s succeeded.", mig_name)
    log.info("Resizing TPU MIG %s to %s instances", mig_name, len(chunk))
    if is_zonal:
        req = lkp.compute.instanceGroupManagers().createInstances(
            project=lkp.project,
            zone=zones[0],
            instanceGroupManager=mig_name,
            body=dict(instances=[dict(name=n) for n in chunk]),
        )
    else:
        req = lkp.compute.regionInstanceGroupManagers().createInstances(
            project=lkp.project,
            region=region,
            instanceGroupManager=mig_name,
            body=dict(instances=[dict(name=n) for n in chunk]),
        )
    try:
        util.log_api_request(req)
        op = req.execute()
        res = util.wait_for_operation(op)
        if "error" in res:
            errs = res.get("error", {}).get("errors", [])
            err_str = "; ".join(f"{e.get('code', 'ERROR')}: {e.get('message', '')}" for e in errs) or str(res)
            raise RuntimeError(f"TPU MIG resize operation failed: {err_str}")
    except Exception:
        mig_self_link = (
            f"zones/{zones[0]}/instanceGroupManagers/{mig_name}"
            if is_zonal
            else f"regions/{region}/instanceGroupManagers/{mig_name}"
        )
        _delete_tpu_mig(mig_self_link, chunk, lkp)
        raise
    log.info("TPU MIG and Resize Request for %s succeeded.", mig_name)

def _ensure_tpu_workload_policy(policy: ResourcePolicy, lkp: util.Lookup) -> str:
    """Gets or creates a regional Resource Policy containing a Workload Policy for TPUs."""
    policy_resource_name = (
        f"projects/{lkp.project}/regions/{policy.region}/resourcePolicies/{policy.name}"
    )
    try:
        lkp.compute.resourcePolicies().get(
            project=lkp.project, region=policy.region, resourcePolicy=policy.name
        ).execute()
        return policy_resource_name
    except googleapiclient.errors.HttpError as e:
        if e.resp.status != 404:
            raise
        log.info("Creating TPU Workload Policy: %s", policy.name)
        req = lkp.compute.resourcePolicies().insert(
            project=lkp.project,
            region=policy.region,
            body=dataclasses.asdict(policy),
        )
        util.log_api_request(req)
        try:
            op = req.execute()
        except googleapiclient.errors.HttpError as insert_err:
            if insert_err.resp.status != 409:
                raise
            log.info("TPU Workload Policy %s already exists, skipping creation.", policy.name)
            return policy_resource_name
        res = util.wait_for_operation(op)
        if "error" in res:
            raise RuntimeError(f"Failed to create TPU Workload Policy: {res}")
        return policy_resource_name

def suspend_tpu_nodes(nodes: List[str], lkp: util.Lookup) -> None:
    """Suspends TPU nodes by identifying and deleting their ephemeral MIGs and cleaning up phantom peers."""
    if not nodes:
        return
    log.info("Suspending TPU nodes: %s", nodes)
    by_mig = defaultdict(list)
    mig_all_peers: dict[str, List[str]] = {}
    mig_creating: dict[str, bool] = {}
    for node in nodes:
        if any(node in peers for peers in mig_all_peers.values()):
            continue
        mig_link, peers, is_creating = get_mig_for_node(node, lkp)
        if mig_link:
            by_mig[mig_link].append(node)
            mig_creating[mig_link] = is_creating
            if peers:
                mig_all_peers[mig_link] = peers
        else:
            log.error("Cannot find MIG for TPU node %s", node)
    for mig_link, mig_nodes in by_mig.items():
        all_mig_nodes = mig_all_peers.get(mig_link, mig_nodes)
        # Keep static MIG if VMs haven't booted yet so we reuse it instead of hitting GCE API limits
        if (
            lkp.is_static_node(mig_nodes[0])
            and all(lkp.instance(n) is None for n in all_mig_nodes)
            and mig_creating.get(mig_link, False)
        ):
            log.info(
                "Static TPU MIG %s has no backing VMs yet (waiting for GCE capacity); keeping MIG instead of deleting.",
                mig_link,
            )
            continue
        # Delete the whole slice MIG and power down remaining peer nodes
        _delete_tpu_mig(mig_link, mig_nodes, lkp)
        phantom_nodes = set(mig_all_peers.get(mig_link, [])) - set(nodes)
        if phantom_nodes:
            nodelist = util.to_hostlist(list(phantom_nodes))
            reason = shlex.quote("The associated TPU slice has been deleted")
            log.info("Powering down phantom peer nodes in deleted TPU MIG %s: %s", mig_link, nodelist)
            util.run(
                f"{lkp.scontrol} update nodename={nodelist} state=POWER_DOWN_FORCE reason={reason}",
                check=False,
            )

def _delete_tpu_mig(mig_self_link: str, nodes: List[str], lkp: util.Lookup) -> None:
    """Deletes an entire TPU slice by removing its MIG."""
    project = lkp.project
    mig_name = util.trim_self_link(mig_self_link)
    is_zonal = "/zones/" in mig_self_link
    log.info("Deleting entire TPU MIG %s (nodes: %s)", mig_self_link, nodes)
    if is_zonal:
        zone = util.get_self_link_component(mig_self_link, "zones")
        req = lkp.compute.instanceGroupManagers().delete(
            project=project,
            zone=zone,
            instanceGroupManager=mig_name,
        )
    else:
        region = lkp.node_region(nodes[0])
        req = lkp.compute.regionInstanceGroupManagers().delete(
            project=project,
            region=region,
            instanceGroupManager=mig_name,
        )
    try:
        util.log_api_request(req)
        op = req.execute()
        res = util.wait_for_operation(op)
        if "error" in res:
            raise RuntimeError(f"Failed to suspend TPU slice: {res}")
        log.info("Successfully suspended TPU slice %s", mig_self_link)
    except googleapiclient.errors.HttpError as e:
        if e.resp.status == 404:
            log.info("TPU MIG %s has already been deleted.", mig_name)
        elif e.resp.status == 400:
            log.info("TPU MIG %s is currently not in ready state.", mig_name)
        else:
            raise
    if hasattr(lkp.get_mig_list, "cache_clear"):
        lkp.get_mig_list.cache_clear()
