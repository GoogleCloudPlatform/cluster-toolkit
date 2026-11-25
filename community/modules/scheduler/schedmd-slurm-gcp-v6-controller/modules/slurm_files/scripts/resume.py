#!/slurm/python/venv/bin/python3.13

# Copyright (C) SchedMD LLC.
# Copyright 2026 Google Inc. All rights reserved.
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

from typing import List, Optional, Dict, Any, Tuple
import argparse
import random
import time
from datetime import datetime, timedelta
import shlex
import json
import logging
import error_handler
import os
import re
import yaml
import collections
from pathlib import Path
from dataclasses import dataclass
from util import NSDict

import util
from util import (
    chunked,
    ensure_execute,
    execute_with_futures,
    log_api_request,
    map_with_futures,
    run,
    separate,
    to_hostlist,
    trim_self_link,
    wait_for_operation,
)
from util import lookup, ReservationDetails
import mig_flex

log = logging.getLogger()

PLACEMENT_MAX_CNT = 1500
# Placement group needs to be the same for an entire bulk_insert hence
# if placement is used the actual BULK_INSERT_LIMIT will be
# max([1000, PLACEMENT_MAX_CNT])
BULK_INSERT_LIMIT = 5000

# https://cloud.google.com/compute/docs/instance-groups#types_of_managed_instance_groups
ZONAL_MIG_SIZE_LIMIT = 1000


@dataclass(frozen=True)
class ResumeJobData:
    job_id: int
    partition: str
    nodes_alloc: List[str]
    accelerator_topology: Optional[str] = None

@dataclass(frozen=True)
class ResumeData:
    jobs: List[ResumeJobData]


def get_resume_file_data() -> Optional[ResumeData]:
    if not (path := os.getenv("SLURM_RESUME_FILE")):
        log.error("SLURM_RESUME_FILE was not in environment. Cannot get detailed job, node, partition allocation data.")
        return None
    blob = Path(path).read_text()
    log.debug(f"Resume data: {blob}")
    data = json.loads(blob)

    jobs = []
    for jo in data.get("jobs", []):
        job = ResumeJobData(
            job_id = jo.get("job_id"),
            partition = jo.get("partition"),
            nodes_alloc = util.to_hostnames(jo.get("nodes_alloc")),
            # Parse topology from job layout (e.g., tpu7x=2x2x2 -> 2x2x2)
            accelerator_topology = jo["layout"].split("=")[-1] if jo.get("layout") else None,
        )
        jobs.append(job)
    return ResumeData(jobs=jobs)

def instance_properties(nodeset: NSDict, model:str, placement_group:Optional[str], labels:Optional[dict], job_id:Optional[int]):
    props = NSDict()

    if labels: # merge in extra labels on instance and disks
        template_link = lookup().node_template(model)
        template_info = lookup().template_info(template_link)

        props.labels = {**template_info.labels, **labels}
        
        for disk in template_info.disks:
            if disk.initializeParams.get("diskType", "local-ssd") == "local-ssd":
                continue # do not label local ssd
            disk.initializeParams.labels.update(labels)
        props.disks = template_info.disks

    if placement_group:
        props.resourcePolicies = [placement_group]

    if reservation := lookup().nodeset_reservation(nodeset):
        update_reservation_props(reservation, props, placement_group, reservation.calendar)

    if (fr := lookup().future_reservation(nodeset)) and fr.specific:
        assert fr.active_reservation
        update_reservation_props(fr.active_reservation, props, placement_group, fr.calendar)

    if props.resourcePolicies:
       props.scheduling.onHostMaintenance = "TERMINATE"

    if nodeset.maintenance_interval:
        props.scheduling.maintenanceInterval = nodeset.maintenance_interval

    if nodeset.dws_flex.enabled and nodeset.dws_flex.use_bulk_insert:
        update_props_dws(props, nodeset.dws_flex, job_id)

    # Override with properties explicit specified in the nodeset
    props.update(nodeset.get("instance_properties") or {})
    return props

def update_reservation_props(reservation:ReservationDetails, props:NSDict, placement_group:Optional[str], calendar_mode:bool) -> None:
    props.reservationAffinity = {
        "consumeReservationType": "SPECIFIC_RESERVATION",
        "key": f"compute.{util.universe_domain()}/reservation-name",
        "values": [reservation.bulk_insert_name],
    }

    if reservation.dense or calendar_mode:
        props.scheduling.provisioningModel = "RESERVATION_BOUND"

    # Figure out `resourcePolicies`
    if reservation.policies: # use ones already attached to reservations
        props.resourcePolicies = reservation.policies
    elif reservation.dense and placement_group: # use once created by Slurm
        props.resourcePolicies = [placement_group]
    else: # vanilla reservations don't support external policies
        props.resourcePolicies = []
    log.info(
        f"reservation {reservation.bulk_insert_name} is being used with resourcePolicies: {props.resourcePolicies}")

def update_props_dws(props: NSDict, dws_flex: NSDict, job_id: Optional[int]) -> None:
    props.scheduling.onHostMaintenance = "TERMINATE"
    props.scheduling.instanceTerminationAction = "DELETE"
    props.reservationAffinity['consumeReservationType'] = "NO_RESERVATION"
    props.scheduling.maxRunDuration['seconds'] = dws_flex_duration(dws_flex, job_id)

def dws_flex_duration(dws_flex: NSDict, job_id: Optional[int]) -> int:
    max_duration = dws_flex.max_run_duration
    if dws_flex.use_job_duration and job_id is not None and (job := lookup().job(job_id)) and job.duration:
        if timedelta(seconds=30) <= job.duration <= timedelta(weeks=1):
            max_duration = int(job.duration.total_seconds())
        else:
            log.info("Job TimeLimit cannot be less than 30 seconds or exceed one week")
    return max_duration

def create_instances_request(nodes: List[str], placement_group: Optional[str], excl_job_id: Optional[int], is_job_request: bool):
    """Call regionInstances.bulkInsert to create instances"""
    assert 0 < len(nodes) <= BULK_INSERT_LIMIT

    # model here indicates any node that can be used to describe the rest
    model = next(iter(nodes))
    log.debug(f"create_instances_request: {model} placement: {placement_group}")

    nodeset = lookup().node_nodeset(model)
    template = lookup().node_template(model)
    labels = {"slurm_job_id": excl_job_id} if excl_job_id else None

    body = dict(
        count = len(nodes),
        sourceInstanceTemplate = template,
        # key is instance name, value overwrites properties (no overwrites)
        perInstanceProperties = {k: {} for k in nodes},
        instanceProperties = instance_properties(
            nodeset, model, placement_group, labels, excl_job_id
        ),
    )

    if is_job_request:
        # By omitting minCount, GCP will default minCount to equal count.
        # This guarantees ATOMIC PROVISIONING. If GCP cannot provide all N nodes, it fails instantly
        # rather than partially provisioning M nodes which waste budget while waiting for the rest.
        pass
    else:
        # Static pools and non-job allocations don't require all nodes to boot simultaneously.
        # minCount=1 allows partial fulfillment so available static nodes can still service smaller jobs.
        body["minCount"] = 1

    zone_allow = nodeset.zone_policy_allow or []

    if len(zone_allow) == 1: # if only one zone is used, use zonal BulkInsert API, as less prone to errors
        api_method = lookup().compute.instances().bulkInsert
        method_args = {"zone": zone_allow[0]}
    else:
        api_method = lookup().compute.regionInstances().bulkInsert
        method_args = {"region": lookup().node_region(model)}

        # The 'zones' parameter acts as an allow-list, implicitly denying any zones not included.
        body["locationPolicy"] = dict(
            zones = [ { "zone": f"zones/{z}" } for z in zone_allow ],
            targetShape = nodeset.zone_target_shape,
        )
    
    req = api_method(
        project=lookup().project, 
        body=body, 
        **method_args)
    log.debug(f"new request: endpoint={req.methodId} nodes={to_hostlist(nodes)}")
    log_api_request(req)
    return req

@dataclass()
class PlacementAndNodes:
    placement: Optional[str]
    nodes: List[str]

@dataclass(frozen=True)
class BulkChunk:
    nodes: List[str]
    prefix: str # <cluster_name>-<nodeset_name>
    chunk_idx: int
    excl_job_id: Optional[int]
    placement_group: Optional[str] = None
    is_job_request: bool = False

    @property
    def name(self):
        if self.placement_group is not None:
            return f"{self.prefix}:job{self.excl_job_id}:{self.placement_group}:{self.chunk_idx}"
        if self.excl_job_id is not None:
            return f"{self.prefix}:job{self.excl_job_id}:{self.chunk_idx}"
        return f"{self.prefix}:{self.chunk_idx}"
    

def group_nodes_bulk(nodes: List[str], resume_data: Optional[ResumeData], lkp: util.Lookup):
    """group nodes by nodeset, placement_group, exclusive_job_id if any"""
    if resume_data is None: # all nodes will be considered jobless
        resume_data = ResumeData(jobs=[])
        
    nodes_set = set(nodes) # turn into set to simplify intersection
    non_excl = nodes_set.copy()
    groups : Dict[Optional[int], List[PlacementAndNodes]] = {} # excl_job_id|none -> PlacementAndNodes

    # expand all job nodelists (both exclusive and non-exclusive)
    for job in resume_data.jobs:
        # Intersect the job's allocated nodes with the nodes currently being resumed.
        # This fixes a bug where partially provisioned nodes in previous resume calls were improperly included
        # in the new bulkInsert, causing duplicate provisioning and blocking minCount atomic requests.
        job_nodes_in_resume = set(job.nodes_alloc) & non_excl
        if not job_nodes_in_resume:
            continue

        groups[job.job_id] = []
        # placement group assignment requires full job.nodes_alloc to be accurate
        for pn in create_placements(job.nodes_alloc, job.job_id, lkp):
            groups[job.job_id].append(
                PlacementAndNodes(
                    placement=pn.placement,
                    # safely intersect with job_nodes_in_resume to prevent duplicate overlapping node requests
                    nodes = sorted(set(pn.nodes) & job_nodes_in_resume)
                ))
        non_excl.difference_update(job_nodes_in_resume)

    groups[None] = create_placements(sorted(non_excl), excl_job_id=None, lkp=lkp)

    def chunk_nodes(nodes: List[str]):
        if not nodes:
            return []
        
        model = nodes[0]
        
        # Group static TPU nodes by slice size so each slice gets its own MIG
        if lkp.is_tpu_node(model) and lkp.is_static_node(model):
            ns = lkp.node_nodeset(model)
            chunk_size = lkp.get_tpu_chunk_size(ns)
            chunks_dict = lkp.group_tpu_nodes_by_chunk_idx(nodes, chunk_size)
            return [chunks_dict[idx] for idx in sorted(chunks_dict.keys())]
        elif lkp.is_flex_node(model):
            chunk_size = ZONAL_MIG_SIZE_LIMIT
        elif lkp.is_node_mig(model):
            # Static MIG nodes handle multi-MIG routing and 1000-node createInstances
            # batching internally. Keep in a single chunk to preserve atomic multi-shard rollback.
            return [nodes]
        else:
            chunk_size = BULK_INSERT_LIMIT

        return chunked(nodes, n=chunk_size)
    
    chunks = [
        BulkChunk(
            nodes=nodes_chunk,
            prefix=lkp.node_prefix(nodes_chunk[0]), # <cluster_name>-<nodeset_name>
            excl_job_id = job_id,
            placement_group=pn.placement,
            chunk_idx=i,
            is_job_request=(job_id is not None))

        for job_id, placements in groups.items()
        for pn in placements if pn.nodes
        for i, nodes_chunk in enumerate(chunk_nodes(pn.nodes))
    ]
    return {chunk.name: chunk for chunk in chunks}


_MIG_ALREADY_EXISTS_CODES = frozenset(("MEMBER_ALREADY_EXISTS", "RESOURCE_ALREADY_EXISTS"))
_MIG_ALREADY_EXISTS_REASONS = frozenset((
    "ALREADYEXISTS",
    "MEMBERALREADYEXISTS",
    "RESOURCEALREADYEXISTS",
))


def _extract_op_error_codes(op_error: Any) -> set[str]:
    """Extract upper-cased GCE error codes from a regional operation ``error`` payload."""
    if not isinstance(op_error, dict):
        return set()
    errs = op_error.get("errors")
    if not isinstance(errs, list):
        return set()
    codes: set[str] = set()
    for err in errs:
        if isinstance(err, dict):
            code = str(err.get("code") or "").strip().upper()
            if code:
                codes.add(code)
    return codes


def _exc_has_already_exists(exc: Exception) -> bool:
    """Return True if an exception reports an HTTP 409 conflict or already-existing MIG/GCE resource."""
    resp = getattr(exc, "resp", None)
    if getattr(resp, "status", None) == 409:
        return True
    err_details = getattr(exc, "error_details", None)
    if isinstance(err_details, list):
        for err in err_details:
            if isinstance(err, dict):
                for key in ("reason", "code"):
                    raw = str(err.get(key) or "").strip()
                    if raw and raw.replace("_", "").upper() in _MIG_ALREADY_EXISTS_REASONS:
                        return True
    msg = str(exc).upper()
    return any(code in msg for code in _MIG_ALREADY_EXISTS_CODES) or "ALREADY EXISTS" in msg


def _mig_unsupported_reservation_reason(nodeset: Any, lkp: util.Lookup) -> Optional[str]:
    """Return an actionable error string if a static MIG nodeset targets an unsupported reservation.

    Static MIG Terraform templates configure SPECIFIC_RESERVATION with STANDARD provisioning_model
    and cannot consume DENSE/CALENDAR reservations (which require RESERVATION_BOUND) or reservations
    carrying attached resourcePolicies. On 403 (shared reservation without host-project read access)
    or lookup errors, returns None so standard shared reservations are not falsely blocked.
    """
    if not getattr(nodeset, "reservation_name", None):
        return None
    try:
        reservation = lkp.nodeset_reservation(nodeset)
    except Exception as e:
        log.warning(
            f"Could not inspect reservation '{getattr(nodeset, 'reservation_name', '')}' "
            f"for MIG nodeset '{getattr(nodeset, 'nodeset_name', '')}'; continuing: {e}"
        )
        return None
    if reservation is None:
        return None
    unsupported_traits: List[str] = []
    if reservation.dense:
        unsupported_traits.append("DENSE deployment requires RESERVATION_BOUND")
    if reservation.calendar:
        unsupported_traits.append("CALENDAR mode requires RESERVATION_BOUND")
    if reservation.policies:
        unsupported_traits.append("attached resourcePolicies are not wired on static MIG templates")
    if not unsupported_traits:
        return None
    detail = "; ".join(unsupported_traits)
    return (
        f"MIG does not support reservation {reservation.name} ({detail}); "
        "use provisioning_engine = BULK_INSERT or AUTO"
    )


def resume_mig_nodes(nodes: List[str], excl_job_id: Optional[int], lkp: util.Lookup, resume_data: Optional[ResumeData] = None) -> None:
    """Provisions nodes via MIG createInstances with Per-Instance Config and preserved state."""
    if not nodes:
        return

    # Group nodes by target MIG (to support multiple MIGs for >1000 nodes)
    nodes_by_mig: Dict[str, List[str]] = {}
    short_to_expected_mig: Dict[str, str] = {}
    for node in nodes:
        mig_name = lkp.node_mig_name(node)
        nodes_by_mig.setdefault(mig_name, []).append(node)
        short_to_expected_mig.setdefault(node.split(".")[0], mig_name)

    _live_statuses = frozenset(("RUNNING", "PROVISIONING", "STAGING", "REPAIRING"))
    successful_creates: List[str] = []
    skipped_live_standalone_nodes: set[str] = set()
    non_live_standalone_nodes: set[str] = set()
    foreign_mig_nodes: set[str] = set()
    already_member_nodes: set[str] = set()
    verified_safe_to_requeue: set[str] = set()
    check_gce_instances = True

    def _partition_abort_nodes(
        candidate_short_nodes: List[str],
    ) -> Tuple[List[str], List[str]]:
        """Partition abort targets into ``(requeue_safe, ignore_only)``.

        Live standalone VMs (``RUNNING``/``PROVISIONING``/``STAGING``/``REPAIRING``) are
        excluded from both lists so they are never powered down or marked ``DOWN``.
        Non-live standalone VMs (e.g. ``STOPPING``/``SUSPENDING``/``SUSPENDED``/
        ``TERMINATED``), foreign-MIG VMs, and unverified nodes when GCE instance lookup
        fails (fail closed) are routed to ``ignore_only`` (``Action.IGNORE``) so an abort
        never runs ``state=power_down`` against unmanaged or external VMs.
        """
        nonlocal check_gce_instances
        requeue_nodes: List[str] = []
        ignore_nodes: List[str] = []
        for sn in dict.fromkeys(candidate_short_nodes):
            if sn in skipped_live_standalone_nodes:
                continue
            if sn in non_live_standalone_nodes or sn in foreign_mig_nodes or sn in already_member_nodes:
                ignore_nodes.append(sn)
                continue
            if sn in verified_safe_to_requeue:
                requeue_nodes.append(sn)
                continue
            if not check_gce_instances:
                ignore_nodes.append(sn)
                continue
            try:
                inst = lkp.instance(sn)
            except Exception as e:
                log.warning(
                    f"Could not inspect GCE instance {sn} during abort protection; "
                    f"downgrading unverified node(s) from power_down to DOWN: {e}"
                )
                check_gce_instances = False
                ignore_nodes.append(sn)
                continue
            if inst is None:
                verified_safe_to_requeue.add(sn)
                requeue_nodes.append(sn)
                continue
            owner_mig = util.instance_created_by_mig(inst)
            expected_mig = short_to_expected_mig.get(sn)
            if owner_mig is None:
                if inst.status in _live_statuses:
                    skipped_live_standalone_nodes.add(sn)
                else:
                    non_live_standalone_nodes.add(sn)
                    ignore_nodes.append(sn)
            elif expected_mig is None or owner_mig == expected_mig:
                verified_safe_to_requeue.add(sn)
                requeue_nodes.append(sn)
            else:
                foreign_mig_nodes.add(sn)
                ignore_nodes.append(sn)
        return requeue_nodes, ignore_nodes

    # Fail fast before any MIG template patch or createInstances call if an explicit MIG nodeset
    # targets a DENSE, CALENDAR, or policy-attached reservation unsupported by static MIG templates.
    nodes_by_nodeset: Dict[str, List[str]] = {}
    for node in nodes:
        ns_name = lkp.node_nodeset_name(node)
        nodes_by_nodeset.setdefault(ns_name, []).append(node)

    blocked_nodes_set: set[str] = set()
    blocked_entries: List[Tuple[str, List[str], str]] = []
    for ns_name, ns_nodes in nodes_by_nodeset.items():
        ns = lkp.node_nodeset(ns_nodes[0])
        res_err = _mig_unsupported_reservation_reason(ns, lkp)
        if res_err:
            short_ns_nodes = list(dict.fromkeys(n.split(".")[0] for n in ns_nodes))
            blocked_nodes_set.update(ns_nodes)
            blocked_entries.append((ns_name, short_ns_nodes, res_err))

    if blocked_entries:
        combined_reason = "; ".join(dict.fromkeys(err for _, _, err in blocked_entries))
        for ns_name, short_ns_nodes, res_err in blocked_entries:
            requeue_ns, ignore_ns = _partition_abort_nodes(short_ns_nodes)
            down_ns = list(dict.fromkeys(requeue_ns + ignore_ns))
            log.error(f"Aborting MIG resume for nodeset {ns_name} ({to_hostlist(short_ns_nodes)}): {res_err}")
            if down_ns:
                handle_resume_failure(
                    down_ns,
                    res_err,
                    resume_data,
                    error_handler.Action.IGNORE,
                    combined_reason if excl_job_id is not None else res_err,
                )
        if excl_job_id is not None:
            remaining_short = list(dict.fromkeys(
                n.split(".")[0] for n in nodes if n not in blocked_nodes_set
            ))
            if remaining_short:
                requeue_rem, ignore_rem = _partition_abort_nodes(remaining_short)
                if ignore_rem:
                    handle_resume_failure(
                        ignore_rem,
                        f"{combined_reason} (skipped power_down for unverified/external VM)",
                        resume_data,
                        error_handler.Action.IGNORE,
                        combined_reason,
                    )
                if requeue_rem:
                    handle_resume_failure(
                        requeue_rem,
                        combined_reason,
                        resume_data,
                        error_handler.Action.REQUEUE,
                        combined_reason,
                    )
            return
        nodes_by_mig = {
            m: [n for n in m_nodes if n not in blocked_nodes_set]
            for m, m_nodes in nodes_by_mig.items()
        }
        nodes_by_mig = {m: m_nodes for m, m_nodes in nodes_by_mig.items() if m_nodes}
        if not nodes_by_mig:
            return

    for mig_name, mig_nodes in nodes_by_mig.items():
        nodeset = lkp.node_nodeset(mig_nodes[0])
        region = lkp.node_region(mig_nodes[0])

        log.info(f"Resuming {len(mig_nodes)} MIG nodes ({to_hostlist(mig_nodes)}) for MIG {mig_name}")

        # 1. Drift-Aware Group Template Alignment
        template_link = getattr(nodeset, "instance_template", None)
        if template_link:
            try:
                mig = lkp.get_mig(lkp.project, region, mig_name)
                if mig:
                    current_template = (
                        mig.get("instanceTemplate")
                        or (mig.get("versions", [{}])[0].get("instanceTemplate") if mig.get("versions") else None)
                    )
                    if current_template and current_template.split("/")[-1] != template_link.split("/")[-1]:
                        log.info(f"Updating MIG {mig_name} template: {current_template} -> {template_link}")
                        ver_payload: Dict[str, Any] = {"instanceTemplate": template_link}
                        if mig.get("versions") and mig["versions"][0].get("name"):
                            ver_payload["name"] = mig["versions"][0]["name"]
                        aic_req = lkp.compute.regionInstanceGroupManagers().patch(
                            project=lkp.project,
                            region=region,
                            instanceGroupManager=mig_name,
                            body={"versions": [ver_payload]}
                        )
                        aic_res = ensure_execute(aic_req)
                        if isinstance(aic_res, dict) and "selfLink" in aic_res:
                            op_res = wait_for_operation(aic_res)
                            if op_res and "error" in op_res:
                                raise RuntimeError(f"patch template operation failed: {op_res['error']}")
                        lkp.get_mig.cache_clear()
            except Exception as e:
                log.warning(f"Could not verify/update template for MIG {mig_name}: {e}")

        # 2. Per-Instance Config (PIC) - Lightweight instance name binding for static Slurm hostnames
        existing_mig_insts = set()
        deleting_mig_insts = set()
        mig_list_success = False
        try:
            mig_data = lkp.get_mig_instances(lkp.project, region, mig_name)
            for m in mig_data.get("managedInstances", []):
                name = m.get("name") or (m.get("instance") or "").split("/")[-1]
                current_action = m.get("currentAction")
                if current_action in ("DELETING", "ABANDONING") or m.get("instanceStatus") in ("STOPPING", "DELETING"):
                    if name:
                        deleting_mig_insts.add(name)
                        verified_safe_to_requeue.add(name)
                elif name:
                    existing_mig_insts.add(name)
                    # Leave CREATING* slots unverified so abort handling still checks
                    # GCE ownership via lkp.instance() in case an orphan CREATING slot
                    # collides with a live standalone VM.
                    if current_action not in ("CREATING", "CREATING_WITHOUT_RETRIES"):
                        verified_safe_to_requeue.add(name)
            mig_list_success = True
        except Exception as e:
            log.warning(f"Could not check existing instances for MIG {mig_name}: {e}")

        # If any requested nodes are currently in the middle of being deleted by GCE,
        # wait briefly for the deletion operation to finish so createInstances will not fail with 409 Conflict.
        nodes_to_wait = {n.split(".")[0] for n in mig_nodes} & deleting_mig_insts
        still_deleting = set(nodes_to_wait)
        if nodes_to_wait:
            log.info(f"Waiting for in-flight deletion of {nodes_to_wait} in MIG {mig_name} before resuming...")
            for _ in range(10):
                time.sleep(2)
                lkp.clear_mig_caches()
                try:
                    mig_data = lkp.get_mig_instances(lkp.project, region, mig_name)
                    current_insts = {
                        (m.get("name") or (m.get("instance") or "").split("/")[-1])
                        for m in mig_data.get("managedInstances", [])
                        if (m.get("name") or m.get("instance"))
                    }
                    still_deleting = nodes_to_wait & current_insts
                    if not still_deleting:
                        break
                except Exception:
                    pass
            lkp.instances.cache_clear()
            if still_deleting:
                log.error(f"Instances {still_deleting} still deleting in GCE after timeout; failing resume to trigger immediate requeue.")
                verified_safe_to_requeue.update(still_deleting)
                if excl_job_id is not None:
                    # Partition abort targets BEFORE rollback so suspend_mig_nodes()'s
                    # instances.cache_clear() does not drop the lookup cache first.
                    requeue_nodes, ignore_nodes = _partition_abort_nodes(
                        [n.split(".")[0] for n in nodes]
                    )
                    if successful_creates:
                        log.warning(
                            f"Aborting multi-slice exclusive job {excl_job_id}: cleaning up {len(successful_creates)} previously created instances: {to_hostlist(successful_creates)}"
                        )
                        try:
                            # Lazy import to avoid circular dependency between resume and suspend
                            import suspend
                            lkp.clear_mig_caches()
                            suspend.suspend_mig_nodes(successful_creates, lkp)
                        except Exception as clean_err:
                            log.error(f"Failed cleaning up instances {successful_creates}: {clean_err}")
                    # Multi-node exclusive job cannot proceed without all nodes across all shards.
                    # Reset verified MIG/absent nodes and downgrade external/unverified nodes to DOWN.
                    if ignore_nodes:
                        handle_resume_failure(
                            ignore_nodes,
                            "GCP MIG Error: Instances still in-flight deleting after timeout (skipped power_down for unverified/external VM)",
                            resume_data,
                            error_handler.Action.IGNORE,
                            "MIG in-flight deletion timeout",
                        )
                    if requeue_nodes:
                        handle_resume_failure(
                            requeue_nodes,
                            "GCP MIG Error: Instances still in-flight deleting after timeout",
                            resume_data,
                            error_handler.Action.REQUEUE,
                            "MIG in-flight deletion timeout",
                        )
                    return
                else:
                    # For non-exclusive/jobless nodes, identify all affected jobs and cancel their associated nodes
                    affected_nodes = set(still_deleting)
                    if resume_data:
                        for job in resume_data.jobs:
                            job_nodes = {n.split(".")[0] for n in job.nodes_alloc}
                            if job_nodes & still_deleting:
                                affected_nodes.update(job_nodes)
                    affected_candidates = [
                        sn for sn in dict.fromkeys(n.split(".")[0] for n in mig_nodes)
                        if sn in affected_nodes
                    ]
                    requeue_nodes, ignore_nodes = _partition_abort_nodes(affected_candidates)
                    if ignore_nodes:
                        handle_resume_failure(
                            ignore_nodes,
                            "GCP MIG Error: Instances still in-flight deleting after timeout (skipped power_down for unverified/external VM)",
                            resume_data,
                            error_handler.Action.IGNORE,
                            "MIG in-flight deletion timeout",
                        )
                    if requeue_nodes:
                        handle_resume_failure(
                            requeue_nodes,
                            "GCP MIG Error: Instances still in-flight deleting after timeout",
                            resume_data,
                            error_handler.Action.REQUEUE,
                            "MIG in-flight deletion timeout",
                        )
                    existing_mig_insts.update(affected_nodes)

        pic_instances: List[Dict[str, Any]] = []
        legacy_blocking_nodes: List[Tuple[str, str]] = []
        same_mig_blocking_nodes: List[Tuple[str, str]] = []
        mig_list_unknown_nodes: List[Tuple[str, str]] = []
        foreign_mig_blocking_nodes: List[Tuple[str, str]] = []
        skipped_live_standalone = False
        seen_names = set()
        for node in mig_nodes:
            short_name = node.split(".")[0]
            if short_name in existing_mig_insts or short_name in seen_names:
                log.info(f"Instance {short_name} already exists in MIG {mig_name} or duplicate; skipping createInstances.")
                continue
            seen_names.add(short_name)
            existing_inst = None
            inst_lookup_ok = False
            if check_gce_instances:
                try:
                    existing_inst = lkp.instance(short_name)
                    inst_lookup_ok = True
                except Exception as e:
                    log.warning(
                        f"Could not list existing GCE instances; skipping standalone pre-check for MIG {mig_name}: {e}"
                    )
                    check_gce_instances = False
            if existing_inst is not None:
                owner_mig = util.instance_created_by_mig(existing_inst)
                if owner_mig == mig_name:
                    verified_safe_to_requeue.add(short_name)
                    if existing_inst.status in _live_statuses:
                        log.info(
                            f"Instance {short_name} already belongs to MIG {mig_name} "
                            f"(status={existing_inst.status}); skipping createInstances."
                        )
                        continue
                    if not mig_list_success:
                        log.warning(
                            f"Instance {short_name} ({existing_inst.status}) carries created-by={mig_name} "
                            "while listManagedInstances() failed; leaving existing VM untouched instead of treating it as absent."
                        )
                        mig_list_unknown_nodes.append((short_name, existing_inst.status or "UNKNOWN"))
                        continue
                    action_desc = "requeuing" if existing_inst.status == "STOPPING" else "marking DOWN"
                    log.warning(
                        f"Instance {short_name} ({existing_inst.status}) carries created-by={mig_name} "
                        f"but is absent from listManagedInstances(); {action_desc} instead of skipping."
                    )
                    same_mig_blocking_nodes.append((short_name, existing_inst.status or "UNKNOWN"))
                    continue
                if owner_mig is None and existing_inst.status in _live_statuses:
                    log.warning(
                        f"Legacy standalone VM {short_name} ({existing_inst.status}) already exists "
                        f"outside MIG {mig_name}; skipping createInstances to avoid disrupting it. "
                        "Drain and run power_down to recreate it inside the MIG."
                    )
                    skipped_live_standalone = True
                    skipped_live_standalone_nodes.add(short_name)
                    continue
                if owner_mig is not None:
                    log.warning(
                        f"Instance {short_name} ({existing_inst.status}) is managed by a different "
                        f"MIG ({owner_mig}, expected {mig_name}); skipping createInstances."
                    )
                    foreign_mig_nodes.add(short_name)
                    foreign_mig_blocking_nodes.append((short_name, owner_mig))
                    continue
                log.warning(
                    f"Legacy standalone VM {short_name} ({existing_inst.status}) blocks "
                    f"MIG {mig_name} creation; marking DOWN instead of calling createInstances."
                )
                non_live_standalone_nodes.add(short_name)
                legacy_blocking_nodes.append((short_name, existing_inst.status or "UNKNOWN"))
                continue
            if inst_lookup_ok:
                verified_safe_to_requeue.add(short_name)
            inst: Dict[str, Any] = {"name": short_name}
            if excl_job_id is not None:
                inst["preservedState"] = {
                    "metadata": {
                        "slurm_job_id": str(excl_job_id)
                    }
                }
            pic_instances.append(inst)

        if (
            legacy_blocking_nodes
            or same_mig_blocking_nodes
            or mig_list_unknown_nodes
            or foreign_mig_blocking_nodes
        ):
            blocking_comments: List[str] = []
            stopping_names = [
                name for name, status in same_mig_blocking_nodes if status == "STOPPING"
            ]
            other_same_mig = [
                (name, status)
                for name, status in same_mig_blocking_nodes
                if status != "STOPPING"
            ]
            # Pre-partition unscanned abort candidates BEFORE rollback clears lkp.instances().
            requeue_other: List[str] = []
            ignore_other: List[str] = []
            if excl_job_id is not None:
                direct_blocking_set = (
                    {name for name, _ in legacy_blocking_nodes}
                    | {name for name, _ in other_same_mig}
                    | {name for name, _ in mig_list_unknown_nodes}
                    | {name for name, _ in foreign_mig_blocking_nodes}
                )
                requeue_other, ignore_other = _partition_abort_nodes([
                    sn for sn in dict.fromkeys(n.split(".")[0] for n in nodes)
                    if sn not in direct_blocking_set
                ])
                if successful_creates:
                    log.warning(
                        f"Aborting multi-slice exclusive job {excl_job_id}: cleaning up "
                        f"{len(successful_creates)} previously created instances: "
                        f"{to_hostlist(successful_creates)}"
                    )
                    try:
                        import suspend
                        lkp.clear_mig_caches()
                        suspend.suspend_mig_nodes(successful_creates, lkp)
                    except Exception as clean_err:
                        log.error(f"Failed cleaning up instances {successful_creates}: {clean_err}")
            if legacy_blocking_nodes:
                legacy_stopping = [
                    name for name, status in legacy_blocking_nodes if status == "STOPPING"
                ]
                legacy_other = [
                    (name, status)
                    for name, status in legacy_blocking_nodes
                    if status != "STOPPING"
                ]
                if legacy_stopping:
                    stop_reason = (
                        f"Legacy standalone VM outside MIG {mig_name} is stopping/deleting; "
                        "retry after deletion completes"
                    )
                    stop_comment = (
                        f"Blocked by legacy standalone VM outside MIG {mig_name} (STOPPING)"
                    )
                    handle_resume_failure(
                        legacy_stopping,
                        stop_reason,
                        resume_data,
                        error_handler.Action.IGNORE,
                        stop_comment,
                    )
                    blocking_comments.append(stop_comment)
                if legacy_other:
                    legacy_names = [name for name, _ in legacy_other]
                    statuses = sorted({status for _, status in legacy_other})
                    status_str = ",".join(statuses)
                    block_reason = (
                        f"Legacy standalone VM outside MIG {mig_name} ({status_str}); "
                        "run power_down to retire before resuming"
                    )
                    legacy_comment = (
                        f"Blocked by legacy standalone VM outside MIG {mig_name} ({status_str})"
                    )
                    handle_resume_failure(
                        legacy_names,
                        block_reason,
                        resume_data,
                        error_handler.Action.IGNORE,
                        legacy_comment,
                    )
                    blocking_comments.append(legacy_comment)
            if mig_list_unknown_nodes:
                unknown_names = [name for name, _ in mig_list_unknown_nodes]
                unknown_statuses = sorted({status for _, status in mig_list_unknown_nodes})
                unknown_status_str = ",".join(unknown_statuses)
                unknown_reason = (
                    f"MIG Error (LIST_UNAVAILABLE): could not list MIG {mig_name} managed "
                    f"instances while VM is {unknown_status_str}; retrying next sync"
                )
                unknown_comment = (
                    f"MIG {mig_name} listing unavailable ({unknown_status_str})"
                )
                handle_resume_failure(
                    unknown_names,
                    unknown_reason,
                    resume_data,
                    error_handler.Action.IGNORE,
                    unknown_comment,
                )
                blocking_comments.append(unknown_comment)
            if stopping_names:
                stopping_comment = f"Waiting on MIG {mig_name} VM deletion (STOPPING)"
                blocking_comments.append(stopping_comment)
                if excl_job_id is None:
                    stopping_reason = (
                        f"Instance from MIG {mig_name} is stopping/deleting; "
                        "retry after deletion completes"
                    )
                    handle_resume_failure(
                        stopping_names,
                        stopping_reason,
                        resume_data,
                        error_handler.Action.REQUEUE,
                        stopping_comment,
                    )
            if other_same_mig:
                other_names = [name for name, _ in other_same_mig]
                other_statuses = sorted({status for _, status in other_same_mig})
                other_status_str = ",".join(other_statuses)
                other_reason = (
                    f"Instance carrying created-by={mig_name} ({other_status_str}) is absent "
                    "from MIG; delete it directly in GCE before resuming"
                )
                other_comment = (
                    f"Blocked by VM carrying created-by={mig_name} ({other_status_str}) absent from MIG"
                )
                handle_resume_failure(
                    other_names,
                    other_reason,
                    resume_data,
                    error_handler.Action.IGNORE,
                    other_comment,
                )
                blocking_comments.append(other_comment)
            if foreign_mig_blocking_nodes:
                foreign_names = [name for name, _ in foreign_mig_blocking_nodes]
                owners = ",".join(sorted({owner for _, owner in foreign_mig_blocking_nodes}))
                foreign_reason = (
                    f"Existing VM managed by different MIG ({owners}); "
                    f"remove from that MIG before resuming into {mig_name}"
                )
                foreign_comment = f"Blocked by VM managed by different MIG ({owners})"
                handle_resume_failure(
                    foreign_names,
                    foreign_reason,
                    resume_data,
                    error_handler.Action.IGNORE,
                    foreign_comment,
                )
                blocking_comments.append(foreign_comment)
            if excl_job_id is not None:
                requeue_reason = "; ".join(blocking_comments)
                if ignore_other:
                    handle_resume_failure(
                        ignore_other,
                        f"{requeue_reason} (skipped power_down for unverified/external VM)",
                        resume_data,
                        error_handler.Action.IGNORE,
                        requeue_reason,
                    )
                if requeue_other:
                    handle_resume_failure(
                        requeue_other,
                        requeue_reason,
                        resume_data,
                        error_handler.Action.REQUEUE,
                        requeue_reason,
                    )
                return

        if not pic_instances:
            if (
                legacy_blocking_nodes
                or same_mig_blocking_nodes
                or mig_list_unknown_nodes
                or foreign_mig_blocking_nodes
                or skipped_live_standalone
            ):
                log.info(f"No new instances to create in MIG {mig_name}; requested nodes already exist outside the MIG or are blocked.")
            else:
                log.info(f"All requested nodes already exist in MIG {mig_name}; nothing to create.")
            continue

        for chunk in chunked(pic_instances, n=ZONAL_MIG_SIZE_LIMIT):
            chunk_nodes = [item["name"] for item in chunk]
            # False only while the outcome is unknown (request accepted, operation not yet
            # observed terminal). Deleting there would kill healthy VMs.
            observed_terminal_failure = True
            # True when createInstances hit MEMBER/RESOURCE_ALREADY_EXISTS so rollback cleanup
            # never deletes a concurrently created or pre-existing VM in this chunk.
            skip_chunk_cleanup = False
            try:
                pic_req = lkp.compute.regionInstanceGroupManagers().createInstances(
                    project=lkp.project,
                    region=region,
                    instanceGroupManager=mig_name,
                    body={"instances": chunk}
                )
                res = ensure_execute(pic_req)
                log.debug(f"createInstances submitted for {mig_name}: {res}")
                if isinstance(res, dict) and "selfLink" in res:
                    observed_terminal_failure = False
                    op_res = wait_for_operation(res)
                    observed_terminal_failure = True
                    if op_res and "error" in op_res:
                        op_error = op_res["error"]
                        err_codes = _extract_op_error_codes(op_error)
                        if err_codes == {"MEMBER_ALREADY_EXISTS"}:
                            lkp.clear_mig_caches()
                            try:
                                refreshed = lkp.get_mig_instances(lkp.project, region, mig_name)
                            except Exception as list_err:
                                log.warning(
                                    f"createInstances hit MEMBER_ALREADY_EXISTS for MIG {mig_name} and "
                                    f"listManagedInstances() failed ({list_err}); skipping inline cleanup "
                                    "to avoid deleting an existing MIG instance."
                                )
                                skip_chunk_cleanup = True
                                raise RuntimeError(f"createInstances operation failed: {op_error}") from list_err
                            active_now: set[str] = set()
                            for m in refreshed.get("managedInstances", []):
                                if not isinstance(m, dict):
                                    continue
                                if (
                                    m.get("currentAction") in ("DELETING", "ABANDONING")
                                    or m.get("instanceStatus") in ("STOPPING", "DELETING")
                                ):
                                    continue
                                name = m.get("name") or (m.get("instance") or "").split("/")[-1]
                                if name:
                                    active_now.add(name)
                            existing_in_chunk = [n for n in chunk_nodes if n in active_now]
                            missing_items = [item for item in chunk if item["name"] not in active_now]
                            if existing_in_chunk and not missing_items:
                                log.info(
                                    f"createInstances skipped for MIG {mig_name}: requested node(s) "
                                    f"({to_hostlist(existing_in_chunk)}) are already active in the MIG."
                                )
                                already_member_nodes.update(existing_in_chunk)
                                continue
                            if existing_in_chunk and missing_items:
                                log.info(
                                    f"createInstances hit MEMBER_ALREADY_EXISTS for MIG {mig_name}; "
                                    f"skipping already-active node(s) ({to_hostlist(existing_in_chunk)}) "
                                    f"and retrying remaining {len(missing_items)} node(s)."
                                )
                                already_member_nodes.update(existing_in_chunk)
                                chunk = missing_items
                                chunk_nodes = [item["name"] for item in chunk]
                                retry_req = lkp.compute.regionInstanceGroupManagers().createInstances(
                                    project=lkp.project,
                                    region=region,
                                    instanceGroupManager=mig_name,
                                    body={"instances": chunk}
                                )
                                retry_res = ensure_execute(retry_req)
                                log.debug(f"createInstances retry submitted for {mig_name}: {retry_res}")
                                if isinstance(retry_res, dict) and "selfLink" in retry_res:
                                    observed_terminal_failure = False
                                    retry_op = wait_for_operation(retry_res)
                                    observed_terminal_failure = True
                                    if retry_op and "error" in retry_op:
                                        if _extract_op_error_codes(retry_op["error"]) & _MIG_ALREADY_EXISTS_CODES:
                                            skip_chunk_cleanup = True
                                        raise RuntimeError(f"createInstances operation failed: {retry_op['error']}")
                                log.debug(f"createInstances retry completed for {mig_name}")
                                lkp.clear_mig_caches()
                                successful_creates.extend(chunk_nodes)
                                verified_safe_to_requeue.update(chunk_nodes)
                                continue
                            skip_chunk_cleanup = True
                        elif err_codes & _MIG_ALREADY_EXISTS_CODES:
                            skip_chunk_cleanup = True
                        raise RuntimeError(f"createInstances operation failed: {op_error}")
                log.debug(f"createInstances completed for {mig_name}")
                lkp.clear_mig_caches()
                successful_creates.extend(chunk_nodes)
                verified_safe_to_requeue.update(chunk_nodes)
            except Exception as e:
                log.error(f"Failed createInstances for MIG {mig_name} on nodes {to_hostlist(chunk_nodes)}: {e}")
                if not skip_chunk_cleanup and _exc_has_already_exists(e):
                    skip_chunk_cleanup = True
                if skip_chunk_cleanup:
                    already_member_nodes.update(chunk_nodes)
                reason = getattr(e, "_get_reason", lambda: str(e))()
                action, admin_comment = error_handler.classify_gcp_error(reason, str(e))
                if skip_chunk_cleanup:
                    nodes_to_cleanup = (
                        list(dict.fromkeys(successful_creates))
                        if excl_job_id is not None
                        else []
                    )
                else:
                    nodes_to_cleanup = (
                        list(dict.fromkeys(successful_creates + chunk_nodes))
                        if excl_job_id is not None
                        else list(chunk_nodes)
                    )
                deferred_reclaim = bool(nodes_to_cleanup) and not observed_terminal_failure
                if deferred_reclaim:
                    log.warning(
                        f"createInstances outcome unobserved for {to_hostlist(nodes_to_cleanup)}; "
                        f"skipping inline cleanup, deferring reclamation to the suspend path."
                    )
                    nodes_to_cleanup = []
                    # Only REQUEUE issues `state=power_down`, which runs SuspendProgram.
                    # `state=down` alone may never reclaim the VMs we declined to delete.
                    action = error_handler.Action.REQUEUE
                    admin_comment = "MIG createInstances outcome unobserved; requeued so suspend reclaims instances"
                raw_failed = (
                    [n.split(".")[0] for n in nodes]
                    if excl_job_id is not None
                    else chunk_nodes
                )
                # Partition abort targets BEFORE suspend_mig_nodes() clears lkp.instances().
                requeue_nodes, ignore_nodes = _partition_abort_nodes(raw_failed)
                if nodes_to_cleanup:
                    log.warning(
                        f"Cleaning up {len(nodes_to_cleanup)} failed/aborted MIG instances: {to_hostlist(nodes_to_cleanup)}"
                    )
                    try:
                        # Lazy import to avoid circular dependency between resume and suspend
                        import suspend
                        lkp.clear_mig_caches()
                        suspend.suspend_mig_nodes(nodes_to_cleanup, lkp)
                    except Exception as clean_err:
                        log.error(f"Failed cleaning up instances {nodes_to_cleanup}: {clean_err}")
                if action == error_handler.Action.REQUEUE:
                    if ignore_nodes:
                        if deferred_reclaim:
                            log.warning(
                                f"Unobserved createInstances for {to_hostlist(ignore_nodes)} could not be "
                                "reclaimed via state=power_down because GCE instance ownership could not be "
                                "verified; manually check and power_down any newly created MIG instances."
                            )
                        handle_resume_failure(
                            ignore_nodes,
                            f"GCP Error: {reason} (skipped power_down for unverified/external VM)",
                            resume_data,
                            error_handler.Action.IGNORE,
                            admin_comment,
                        )
                    if requeue_nodes:
                        handle_resume_failure(
                            requeue_nodes,
                            f"GCP Error: {reason}",
                            resume_data,
                            error_handler.Action.REQUEUE,
                            admin_comment,
                        )
                else:
                    down_nodes = list(dict.fromkeys(requeue_nodes + ignore_nodes))
                    if down_nodes:
                        handle_resume_failure(
                            down_nodes,
                            f"GCP Error: {reason}",
                            resume_data,
                            action,
                            admin_comment,
                        )
                if excl_job_id is not None:
                    return
            finally:
                lkp.clear_mig_caches()


def resume_nodes(nodes: List[str], resume_data: Optional[ResumeData]):
    """resume nodes in nodelist"""
    lkp = lookup()
    # Prevent dormant nodes associated with a reservation from being resumed, except static
    # MIG nodesets targeting an unsupported reservation (e.g. an upcoming CALENDAR reservation),
    # which should fail fast immediately in resume_mig_nodes with an actionable error.
    nodes, dormant_res_nodes = util.separate(
        lambda n: lkp.is_dormant_res_node(n)
        and not (
            lkp.is_node_mig(n)
            and not lkp.is_tpu_node(n)
            and _mig_unsupported_reservation_reason(lkp.node_nodeset(n), lkp) is not None
        ),
        nodes,
    )
    
    if dormant_res_nodes:
        log.warning(f"Resume was unable to resume reservation nodes={dormant_res_nodes}")
        down_nodes_notify_jobs(dormant_res_nodes, "Reservation is not active, nodes cannot be resumed", resume_data)

    nodes, flex_managed = util.separate(lkp.is_provisioning_flex_node, nodes)
    if flex_managed:
        log.warning(f"Resume was unable to resume nodes={flex_managed} already managed by MIGs")
        down_nodes_notify_jobs(flex_managed, "VM is managed MIG, can not be resumed", resume_data)

    if not nodes:
        log.info("No nodes to resume")
        return

    nodes = sorted(nodes, key=lkp.node_prefix)
    grouped_nodes = group_nodes_bulk(nodes, resume_data, lkp)

    if log.isEnabledFor(logging.DEBUG):
        grouped_nodelists = {
            group: to_hostlist(chunk.nodes) for group, chunk in grouped_nodes.items()
        }
        log.debug(
            "node bulk groups: \n{}".format(yaml.safe_dump(grouped_nodelists).rstrip())
        )

    gce_tpu_chunks, flex_chunks, mig_chunks = [], [], []
    bi_inserts = {}

    for group, chunk in grouped_nodes.items():
        model = chunk.nodes[0]

        if lkp.is_tpu_node(model):
            gce_tpu_chunks.append(chunk)
        elif lkp.is_flex_node(model):
            flex_chunks.append(chunk)
        elif lkp.is_node_mig(model):
            mig_chunks.append(chunk)
        else:
            bi_inserts[group] = create_instances_request(
                chunk.nodes, chunk.placement_group, chunk.excl_job_id, chunk.is_job_request
            )

    if gce_tpu_chunks:
        for chunk, tpu_exc in mig_flex.resume_tpu_chunks(gce_tpu_chunks, resume_data, lkp):
            err_msg = str(tpu_exc)
            reason = getattr(tpu_exc, "_get_reason", lambda: err_msg)()
            _, admin_comment = error_handler.classify_gcp_error(reason, err_msg)
            # Notify the job before marking nodes down so srun prints the error
            if chunk.excl_job_id is not None:
                run(f"{lkp.scontrol} update jobid={chunk.excl_job_id} admincomment={shlex.quote(admin_comment)}", check=False)
                run(f"{lkp.scontrol} notify {chunk.excl_job_id} {shlex.quote(admin_comment)}", check=False)
                time.sleep(1.0)
            # Reset nodes back to idle~ and cancel the job if the config/layout was invalid
            is_invalid = "INVALID_FIELD_VALUE" in err_msg
            handle_resume_failure(
                chunk.nodes,
                f"GCP TPU Error: {err_msg}",
                None,
                error_handler.Action.REQUEUE,
                admin_comment,
            )
            if chunk.excl_job_id is not None and is_invalid:
                scancel_bin = str(Path(lkp.scontrol).with_name("scancel"))
                run(f"{scancel_bin} {chunk.excl_job_id}", check=False)

    for chunk in flex_chunks:
        try:
            mig_flex.resume_flex_chunk(chunk.nodes, chunk.excl_job_id, lkp, chunk.placement_group)
        except Exception:
            log.exception(f"failed to resume flex chunk {chunk.nodes}")

    for chunk in mig_chunks:
        try:
            resume_mig_nodes(chunk.nodes, chunk.excl_job_id, lkp, resume_data)
        except Exception:
            log.exception(f"failed to resume MIG chunk {chunk.nodes}")


    # execute all bulkInsert requests  with batch
    bulk_ops = dict(
        zip(bi_inserts.keys(), map_with_futures(ensure_execute, bi_inserts.values()))
    )
    log.debug(f"bulk_ops={yaml.safe_dump(bulk_ops)}")
    started = {
        group: op for group, op in bulk_ops.items() if not isinstance(op, Exception)
    }
    failed = {
        group: err for group, err in bulk_ops.items() if isinstance(err, Exception)
    }
    if failed:
        failed_reqs = [str(e) for e in failed.items()]
        log.error("bulkInsert API failures: {}".format("; ".join(failed_reqs)))
        for ident, exc in failed.items():
            reason = exc._get_reason() if hasattr(exc, "_get_reason") else str(exc) # type: ignore
            details = reason
            err_details = getattr(exc, "error_details", None)
            if err_details:
                if isinstance(err_details, list):
                    details = "; ".join(
                        err.get("message", str(err)) if isinstance(err, dict) else str(err)
                        for err in err_details
                    )
                else:
                    details = str(err_details)
            
            action, admin_comment = error_handler.classify_gcp_error(reason, details)
            handle_resume_failure(grouped_nodes[ident].nodes, f"GCP Error: {reason}", resume_data, action, admin_comment)

    if log.isEnabledFor(logging.DEBUG):
        for group, op in started.items():
            group_nodes = grouped_nodelists[group]
            name = op["name"]
            gid = op["operationGroupId"]
            log.debug(
                f"new bulkInsert operation started: group={group} nodes={group_nodes} name={name} operationGroupId={gid}"
            )
    # wait for all bulkInserts to complete and log any errors
    bulk_operations = {group: wait_for_operation(op) for group, op in started.items()}

    for group, op in bulk_operations.items():
        _handle_bulk_insert_op(op, grouped_nodes[group].nodes, resume_data)
        

def _get_failed_zonal_instance_inserts(bulk_op: Any, zone: str, lkp: util.Lookup) -> list[Any]:
    group_id = bulk_op["operationGroupId"]
    user = bulk_op["user"]
    started = bulk_op["startTime"]
    ended = bulk_op["endTime"]
   
    fltr = f'(user eq "{user}") AND (operationType eq "insert") AND (creationTimestamp > "{started}") AND (creationTimestamp < "{ended}")'
    act = lkp.compute.zoneOperations()
    req = act.list(project=lkp.project, zone=zone, filter=fltr)
    ops = []
    while req is not None:
        result = util.ensure_execute(req)
        for op in result.get("items", []):
            if op.get("operationGroupId") == group_id and "error" in op:
                ops.append(op)
        req = act.list_next(req, result)
    return ops


def _get_failed_instance_inserts(bulk_op: Any, lkp: util.Lookup) -> list[Any]:
    zones = set() # gather zones that had failed inserts
    for loc, stat in bulk_op.get("instancesBulkInsertOperationMetadata", {}).get("perLocationStatus", {}).items():
        pref, zone = loc.split("/", 1)
        if not pref == "zones":
            log.error(f"Unexpected location: {loc} in operation {bulk_op['name']}")
            continue
        if stat.get("targetVmCount", 0) !=  stat.get("createdVmCount", 0):
            zones.add(zone)
    
    res = []
    for zone in zones:
        res.extend(_get_failed_zonal_instance_inserts(bulk_op, zone, lkp))
    return res

def _handle_bulk_insert_op(op: Dict, nodes: List[str], resume_data: Optional[ResumeData]) -> None:
    """
    Handles **DONE** BulkInsert operations
    """
    assert op["operationType"] == "bulkInsert" and op["status"] == "DONE", f"unexpected op: {op}"

    group_id = op["operationGroupId"]
    if "error" in op:
        error = op["error"]["errors"][0]
        log.error(
            f"bulkInsert operation error: {error['code']} name={op['name']} operationGroupId={group_id} nodes={to_hostlist(nodes)}"
        )
    
    created = 0
    for status in op["instancesBulkInsertOperationMetadata"]["perLocationStatus"].values():
        created += status.get("createdVmCount", 0)
    if created == len(nodes):
        log.info(f"created {len(nodes)} instances: nodes={to_hostlist(nodes)}")
        return # no need to gather status of insert-operations.

    # TODO: don't gather insert-operations per bulkInsert request, instead aggregate it
    #  across all bulkInserts (goes one level above this function) 
    failed = _get_failed_instance_inserts(op, util.lookup())
    
    # Multiple errors are possible, group by all of them (joined string codes)
    by_error_inserts = util.groupby_unsorted(
        failed,
        lambda op: "+".join(err["code"] for err in op["error"]["errors"]),
    )
    for code, failed_ops in by_error_inserts:
        failed_ops = list(failed_ops)
        failed_nodes = [trim_self_link(op["targetLink"]) for op in failed_ops]
        hostlist = util.to_hostlist(failed_nodes)
        log.error(
            f"{len(failed_nodes)} instances failed to start: {code} ({hostlist}) operationGroupId={group_id}"
        )

        msg = "; ".join(
            f"{err['code']}: {err['message'] if 'message' in err else 'no message'}"
            for err in failed_ops[0]["error"]["errors"]
        )
        if code != "RESOURCE_ALREADY_EXISTS":
            action, admin_comment = error_handler.classify_gcp_error(code, msg)
            handle_resume_failure(failed_nodes, f"GCP Error: {msg}", resume_data, action, admin_comment)
        log.error(
            f"errors from insert for node '{failed_nodes[0]}' ({failed_ops[0]['name']}): {msg}"
        )


def down_nodes_notify_jobs(nodes: List[str], reason: str, resume_data: Optional[ResumeData]) -> None:
    """set nodes down with reason"""
    nodes_set = set(nodes) # turn into set to speed up intersection
    jobs = resume_data.jobs if resume_data else []
    reason_quoted = shlex.quote(reason)

    for job in jobs:
        if not (set(job.nodes_alloc) & nodes_set):
            continue
        run(f"{lookup().scontrol} update jobid={job.job_id} admincomment={reason_quoted}", check=False)
        run(f"{lookup().scontrol} notify {job.job_id} {reason_quoted}", check=False)

    nodelist = util.to_hostlist(nodes)
    log.error(f"Marking nodes {nodelist} as DOWN, reason: {reason}")
    run(f"{lookup().scontrol} update nodename={nodelist} state=down reason={reason_quoted}", check=False)


def handle_resume_failure(nodes: List[str], reason: str, resume_data: Optional[ResumeData], action: error_handler.Action, admin_comment: str) -> None:
    """Handle resume failures via node and job management based on error action."""
    nodes_set = set(nodes)
    jobs = resume_data.jobs if resume_data else []
    
    admin_reason_quoted = shlex.quote(admin_comment)
    fallback_reason_quoted = shlex.quote(reason)

    nodelist = util.to_hostlist(nodes)
    if action == error_handler.Action.REQUEUE:
        log.error(f"Resetting nodes {nodelist} to power_down. Reason: {reason}")
        # 1. Force DOWN to instantly strip the POWERING_UP (#) flag and bypass ResumeTimeout
        run(f"{lookup().scontrol} update nodename={nodelist} state=down reason='Force clear booting state'", check=False)
        # 2. Return to power_down so Slurm transitions it safely to idle~
        run(f"{lookup().scontrol} update nodename={nodelist} state=power_down", check=False)
    else:
        log.error(f"Marking nodes {nodelist} as DOWN. Reason: {reason}")
        run(f"{lookup().scontrol} update nodename={nodelist} state=down reason={fallback_reason_quoted}", check=False)

    jobs_to_requeue = []
    for job in jobs:
        if not (set(job.nodes_alloc) & nodes_set):
            continue
            
        run(f"{lookup().scontrol} update jobid={job.job_id} admincomment={admin_reason_quoted}", check=False)

        if action == error_handler.Action.REQUEUE:
            jobs_to_requeue.append(job)

    if jobs_to_requeue:
        # Safely extract ID from Slurm JSON dicts or legacy primitives
        def extract_json_id(val) -> str:
            if isinstance(val, dict):
                return str(val.get("number", "0"))
            return str(val) if val else "0"

        pending_jobs = set()
        parent_job_ids = {}
        
        # Wait up to 10 seconds for jobs to natively requeue to PENDING
        for _ in range(10):
            remaining_ids = [str(job.job_id) for job in jobs_to_requeue if str(job.job_id) not in pending_jobs]
            if not remaining_ids:
                break

            for ids_chunk in util.chunked(remaining_ids, 1000):
                try:
                    res = run(f"{lookup().scontrol} show job {','.join(ids_chunk)} --json", check=False)
                    if not (res and res.stdout): 
                        raise RuntimeError(f"scontrol show job returned empty output. (code={getattr(res, 'returncode', 'None')}, stderr={getattr(res, 'stderr', 'None')})")
                        
                    data = json.loads(res.stdout)
                    for job_dict in data.get("jobs", []):
                        state = job_dict.get("job_state", "")
                        # Unlike sbatch, interactive srun jobs fail immediately and won't be in PENDING state
                        if "PENDING" in state:
                            job_id = extract_json_id(job_dict.get("job_id"))
                            pending_jobs.add(job_id)
                            parent_id = job_id
                            arr_id = extract_json_id(job_dict.get("array_job_id"))
                            het_id = extract_json_id(job_dict.get("het_job_id") or job_dict.get("pack_job_id"))
                            # Ignore "0" and "0_0" for unset array/het IDs
                            if arr_id not in ("0", "0_0"):
                                parent_id = arr_id
                            elif het_id not in ("0", "0_0"):
                                parent_id = het_id
                            parent_job_ids[job_id] = parent_id
                except Exception as e:
                    log.debug(f"Failed to query job states: {e}")

            if len(pending_jobs) == len(jobs_to_requeue):
                break
            time.sleep(1)

        jobs_by_backoff_time = collections.defaultdict(list)
        # Define globally so grouped jobs share the exact same backoff time
        base_timestamp = datetime.now().replace(microsecond=0)

        for job in jobs_to_requeue:
            job_id = str(job.job_id)
            if job_id not in pending_jobs:
                continue

            # Use parent job ID (array/het) as seed value so grouped jobs generate identical random backoffs
            seed_val = parent_job_ids.get(job_id, job_id)
            random_generator = random.Random(seed_val)

            # To prevent the requeued jobs from immediately retrying and hammering
            # the GCP API during a stockout, we calculate a randomized backoff (e.g. 1-2 mins) and
            # explicitly delay the job's next evaluation via the StartTime parameter.
            delay_seconds = random_generator.randint(60, 120)

            backoff_time = (base_timestamp + timedelta(seconds=delay_seconds)).strftime("%Y-%m-%dT%H:%M:%S")
            jobs_by_backoff_time[backoff_time].append(job_id)
            log.info(f"Setting job {job_id} backoff: {delay_seconds}s (until {backoff_time})")

        # Batch execute the StartTime updates to prevent massive subprocess generation
        for backoff_time, job_ids in jobs_by_backoff_time.items():
            for ids_chunk in util.chunked(job_ids, 1000):
                run(f"{lookup().scontrol} update JobId={','.join(ids_chunk)} StartTime={backoff_time}", check=False)


def create_placement_request(pg_name: str, region: str, max_distance: Optional[int], accelerator_topology: Optional[str], is_flex: bool = False):
    if is_flex:
        distance_map = {
            1: "SUBBLOCK",
            2: "BLOCK",
            3: "CLUSTER",
        }
        topo_distance = "CLUSTER"
        if max_distance in distance_map:
            topo_distance = distance_map[max_distance]
        

        config = {
            "name": pg_name,
            "region": region,
            "workloadPolicy": {
                "type": "HIGH_THROUGHPUT",
                "maxTopologyDistance": topo_distance,
            },
        }

    else:
        config = {
            "name": pg_name,
            "region": region,
            "groupPlacementPolicy": {
                "collocation": "COLLOCATED",
                "maxDistance": max_distance,
                "gpuTopology": accelerator_topology,
            },
        }
    
    request = lookup().compute.resourcePolicies().insert(
        project=lookup().project, region=region, body=config
    )

    log_api_request(request)
    return request


def create_placements(nodes: List[str], excl_job_id:Optional[int], lkp: util.Lookup) -> List[PlacementAndNodes]:
    nodeset_map = collections.defaultdict(list)
    for node in nodes: # split nodes on nodesets
        nodeset_map[lkp.node_nodeset_name(node)].append(node)

    placements = []
    for _, ns_nodes in nodeset_map.items():
        placements.extend(create_nodeset_placements(ns_nodes, excl_job_id, lkp))
    return placements


def _allocate_nodes_to_placements(nodes: List[str], excl_job_id:Optional[int], lkp: util.Lookup) -> List[PlacementAndNodes]:
    # canned result for no placement policies created
    no_pp = [PlacementAndNodes(placement=None, nodes=nodes)]

    model = nodes[0]
    nodeset = lkp.node_nodeset(model)

    # Static MIG NodeSets already have their Workload Policies provisioned by Terraform
    # and bound to each slice MIG. Skip runtime GCE placement policy allocation.
    # GCE TPU nodesets get their workload policy from mig_flex at resume time, so skip them too.
    # (Note: DWS Flex NodeSets must continue to runtime placement policy allocation).
    if lkp.is_tpu_node(model) or (lkp.is_node_mig(model) and not lkp.is_flex_node(model)):
        return no_pp

    topo = nodeset.get("accelerator_topology") if isinstance(nodeset, dict) else getattr(nodeset, "accelerator_topology", None)
    is_slice = bool(topo)

    excl_job_placement = (excl_job_id is not None) and (not is_slice)
    
    if excl_job_placement and len(nodes) < 2:
        return no_pp # don't create placement_policy for just one node

    if not (nodeset.enable_placement and valid_placement_node(model)):
        return no_pp
    
    max_count = calculate_chunk_size(nodeset, lkp)

    name_prefix = f"{lkp.cfg.slurm_cluster_name}-slurmgcp-managed-{nodeset.nodeset_name}"
   
    if excl_job_placement: # simply chunk given nodes by max size of placement
        return [
            PlacementAndNodes(placement=f"{name_prefix}-{excl_job_id}-{i}", nodes=chunk)
            for i, chunk in enumerate(chunked(nodes, n=max_count))
        ]

    # split whole nodeset (not only nodes to resume) into chunks of max size of placement
    # create placements (most likely already exists) placements for requested nodes
    chunks = collections.defaultdict(list) # chunk_id -> nodes
    invalid = []

    for node in nodes:
        try:
            chunk = lkp.node_index(node) // max_count
            chunks[chunk].append(node)
        except:
            invalid.append(node)
    
    placements = [
        # NOTE: use 0 instead of job_id for consistency with previous SlurmGCP behavior
        PlacementAndNodes(placement=f"{name_prefix}-0-{c_id}", nodes=c_nodes) 
        for c_id, c_nodes in chunks.items() 
    ]

    if invalid:
        placements.append(PlacementAndNodes(placement=None, nodes=invalid))
        log.error(f"Could not find placement for nodes with unexpected names: {to_hostlist(invalid)}")

    return placements

def calculate_hosts_per_topo(accelerator_topology: str, machine_type: NSDict) -> int:
    # Calculate total number of hosts per topology (Assumes format: '1x72')
    try:
        top_split = [int(x) for x in accelerator_topology.lower().strip().split("x")]
    except Exception as e:
        log.error(f"Accelerator topology {accelerator_topology} is formatted incorrectly.")
        raise e

    if len(machine_type.accelerators) > 0:
        gpus_per_machine = machine_type.accelerators[0].count
    else:
        m = re.search(r"-([0-9]+)g\b", str(getattr(machine_type, "name", "")))
        gpus_per_machine = int(m.group(1)) if m else 0

    if len(top_split) != 2 or top_split[0] <= 0 or top_split[1] <= 0:
        log.error(f"Accelerator topology {accelerator_topology} is formatted incorrectly.")
        raise ValueError(f"Accelerator topology {accelerator_topology} is formatted incorrectly; expected '<dim1>x<dim2>'.")
    elif gpus_per_machine <= 0:
        log.error(f"The machine type has no accelerators. Cannot use accelerator topology {accelerator_topology}.")
        raise ValueError(f"Machine type has no accelerators; cannot compute hosts for topology {accelerator_topology}")
    elif top_split[1] % gpus_per_machine:
        log.error(f"The GPU count {gpus_per_machine} per node is not a factor of the accelerator topology {accelerator_topology}")
        raise ValueError(f"The GPU count {gpus_per_machine} per node is not a factor of the accelerator topology {accelerator_topology}")
    
    return (top_split[0] * top_split[1]) // gpus_per_machine
 
def calculate_chunk_size(nodeset: NSDict, lkp: util.Lookup) -> int:
    # Calculates the chunk size based on max distance value received or accelerator topology
    # Assuming nodeset is not tpu
    accelerator_topology = nodeset.get("accelerator_topology") if isinstance(nodeset, dict) else getattr(nodeset, "accelerator_topology", None)
    if accelerator_topology:
        slice_val = nodeset.get("slice_size") if isinstance(nodeset, dict) else getattr(nodeset, "slice_size", None)
        if slice_val:  # static MIG only; same value partition.tf sharded on
            try:
                return max(1, int(slice_val))
            except (ValueError, TypeError):
                pass
        # Bulk Insert / DWS Flex: derive from the live machine type, as before. Not
        # lkp.nodeset_slice_size() -- it prefers config gpu_count, moving the source of truth.
        template_link = nodeset.get("instance_template") if isinstance(nodeset, dict) else getattr(nodeset, "instance_template", None)
        machine_type = lkp.template_info(template_link).machine_type
        return calculate_hosts_per_topo(accelerator_topology, machine_type)

    machine_type = lkp.template_info(nodeset.instance_template).machine_type
    max_distance = nodeset.placement_max_distance

    if max_distance == 1:
        return 22
    elif max_distance == 2:
        if machine_type.family.startswith("a3"):
            return 256
        else:
            return 150
    elif max_distance == 3:
        return 1500
    else:
        return PLACEMENT_MAX_CNT

def create_nodeset_placements(nodes: List[str], excl_job_id:Optional[int], lkp: util.Lookup) -> List[PlacementAndNodes]:    
    placements = _allocate_nodes_to_placements(nodes, excl_job_id, lkp)
    # Static MIG NodeSets already have their Workload Policies provisioned by Terraform
    # and bound to each slice MIG. Skip runtime GCE compute.resourcePolicies.insert API requests.
    if lkp.is_node_mig(nodes[0]) and not lkp.is_flex_node(nodes[0]):
        return placements

    region = lkp.node_region(nodes[0])
    ns = lkp.node_nodeset(nodes[0])
    max_distance = getattr(ns, 'placement_max_distance', None) if not isinstance(ns, dict) else ns.get('placement_max_distance')
    accelerator_topology = lkp.nodeset_accelerator_topology(lkp.node_nodeset_name(nodes[0]))
    is_flex = lkp.is_flex_node(nodes[0])

    if log.isEnabledFor(logging.DEBUG):
        debug_p = {p.placement: to_hostlist(p.nodes) for p in placements}
        log.debug(
            f"creating {len(placements)} placement groups: \n{yaml.safe_dump(debug_p).rstrip()}"
        )

    requests = {
        p.placement: create_placement_request(p.placement, region, max_distance, accelerator_topology, is_flex) for p in placements if p.placement
    }

    if not requests:
        return placements
    # TODO: aggregate all requests for whole resume and execute them at once (don't limit to nodeset/job)
    ops = dict(
        zip(requests.keys(), map_with_futures(ensure_execute, requests.values()))
    )

    def classify_result(item):
        op = item[1]
        if not isinstance(op, Exception):
            return "submitted"
        if all(e.get("reason") == "alreadyExists" for e in op.error_details): # type: ignore
            return "redundant"
        return "failed"

    grouped_ops = dict(util.groupby_unsorted(list(ops.items()), classify_result))
    submitted, redundant, failed = (
        dict(grouped_ops.get(key, {})) for key in ("submitted", "redundant", "failed")
    )
    if redundant:
        log.warning(
            "placement policies already exist: {}".format(",".join(redundant.keys()))
        )
    if failed:
        reqs = [f"{e}" for e in failed.values()]
        log.fatal("failed to create placement policies: {}".format("; ".join(reqs)))
    operations = {group: wait_for_operation(op) for group, op in submitted.items()}
    for group, op in operations.items():
        if "error" in op:
            msg = "; ".join(
                f"{err['code']}: {err['message'] if 'message' in err else 'no message'}"
                for err in op["error"]["errors"]
            )
            log.error(
                f"placement group failed to create: '{group}' ({op['name']}): {msg}"
            )

    log.info(
        f"created {len(operations)} placement groups ({to_hostlist(operations.keys())})"
    )
    return placements


def valid_placement_node(node: str) -> bool:
    invalid_types = frozenset(["e2", "t2d", "n1", "t2a", "m1", "m2", "m3"])
    mt = lookup().node_template_info(node).machineType
    if mt.split("-")[0] in invalid_types:
        log.warn(f"Unsupported machine type for placement policy: {mt}.")
        log.warn(
            f"Please do not use any the following machine types with placement policy: ({','.join(invalid_types)})"
        )
        return False
    return True


def main(nodelist: str) -> None:
    """main called when run as script"""
    log.debug(f"ResumeProgram {nodelist}")
    # Filter out nodes not in config.yaml
    other_nodes, nodes = separate(
        lookup().is_power_managed_node, util.to_hostnames(nodelist)
    )
    if other_nodes:
        log.error(
            f"Ignoring non-power-managed nodes '{to_hostlist(other_nodes)}' from '{nodelist}'"
        )

    if not nodes:
        log.info("No nodes to resume")
        return
    resume_data = get_resume_file_data()
    log.info(f"resume {util.to_hostlist(nodes)}")
    resume_nodes(nodes, resume_data)

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("nodelist", help="list of nodes to resume")
    args = util.init_log_and_parse(parser)
    main(args.nodelist)
