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

from typing import List, Any, Dict, Optional
import argparse
import logging

import util
from util import (
    log_api_request,
    batch_execute,
    to_hostlist,
    separate,
)
from util import lookup
import mig_flex
import watch_delete_vm_op

log = logging.getLogger()

TOT_REQ_CNT = 1000


def truncate_iter(iterable, max_count):
    end = "..."
    _iter = iter(iterable)
    for i, el in enumerate(_iter, start=1):
        if i >= max_count:
            yield end
            break
        yield el


def delete_instance_request(name: str, lkp: Optional[util.Lookup] = None) -> Any:
    active_lkp = lkp if lkp is not None else lookup()
    inst = active_lkp.instance(name)
    assert inst

    request = active_lkp.compute.instances().delete(
        project=active_lkp.project,
        zone=inst.zone,
        instance=name,
    )
    log_api_request(request)
    return request


def delete_instances(instances, lkp: Optional[util.Lookup] = None):
    """delete instances individually"""
    active_lkp = lkp if lkp is not None else lookup()
    invalid, valid = separate(lambda inst: bool(active_lkp.instance(inst)), instances)
    if len(invalid) > 0:
        log.debug("instances do not exist: {}".format(",".join(invalid)))
    if len(valid) == 0:
        log.debug("No instances to delete")
        return

    requests = {inst: delete_instance_request(inst, lkp=active_lkp) for inst in valid}

    log.info(f"to delete {len(valid)} instances ({to_hostlist(valid)})")
    ops, failed = batch_execute(requests)
    for node, (_, err) in failed.items():
        log.error(f"instance {node} failed to delete: {err}")
    
    log.info(f"deleting {len(ops)} instances {to_hostlist(ops.keys())}")

    topic = watch_delete_vm_op.watch_delete_vm_op_topic()
    for node, op in ops.items():
        topic.publish(op, node)




def suspend_mig_nodes(
    nodes: List[str],
    lkp: util.Lookup,
    retire_standalone: bool = False,
) -> None:
    """Deletes specific instance names from MIG and reduces target size atomically.

    When ``retire_standalone`` is True (used by explicit suspend / orphan cleanup paths,
    never by ``resume.py`` rollback), any requested node absent from the MIG that still
    exists in GCE as an unmanaged standalone VM (no ``created-by`` MIG metadata) is
    deleted via ``instances().delete()`` so upgraded static clusters can roll legacy
    ``BULK_INSERT`` VMs into the MIG cleanly without leaking them.
    """
    if not nodes:
        return

    # Invalidate cached managed instances to ensure fresh live state from GCE
    lkp.clear_mig_caches()
    lkp.instances.cache_clear()

    # Group nodes by target MIG (to support multiple MIGs for >1000 nodes)
    nodes_by_mig: Dict[str, List[str]] = {}
    for node in nodes:
        mig_name = lkp.node_mig_name(node)
        nodes_by_mig.setdefault(mig_name, []).append(node)

    for mig_name, mig_nodes in nodes_by_mig.items():
        region = lkp.node_region(mig_nodes[0])

        # Resolve instance URLs from MIG or local lookup
        mig_inst_map: Dict[str, str] = {}
        mig_unplaced_names: set[str] = set()
        mig_list_success = False
        try:
            mig_data = lkp.get_mig_instances(lkp.project, region, mig_name)
            for item in mig_data.get("managedInstances", []):
                if item.get("instance"):
                    mig_inst_map[item["instance"].split("/")[-1]] = item["instance"]
                    if item.get("name"):
                        mig_inst_map[item["name"]] = item["instance"]
                elif item.get("name"):
                    mig_unplaced_names.add(item["name"])
            mig_list_success = True
        except Exception as e:
            log.warning(f"Could not list managed instances for MIG {mig_name}: {e}")
            mig_inst_map = {}
            mig_unplaced_names = set()
            mig_list_success = False

        links: List[str] = []
        seen_links: set[str] = set()
        standalone_nodes: List[str] = []
        seen_standalone: set[str] = set()
        fallback_zones: Optional[List[str]] = None
        check_gce_instances = True
        for node in mig_nodes:
            short_name = node.split(".")[0]
            if short_name in mig_inst_map:
                link = mig_inst_map[short_name]
                if link not in seen_links:
                    links.append(link)
                    seen_links.add(link)
            elif short_name in mig_unplaced_names or not mig_list_success:
                inst = lkp.instance(short_name)
                zone = getattr(inst, "zone", None)
                if zone:
                    zone_name = zone.split("/")[-1]
                    link = f"zones/{zone_name}/instances/{short_name}"
                    if link not in seen_links:
                        links.append(link)
                        seen_links.add(link)
                elif short_name in mig_unplaced_names:
                    if fallback_zones is None:
                        nodeset = lkp.node_nodeset(mig_nodes[0])
                        resolved_zones = list(getattr(nodeset, "zone_policy_allow", None) or [])
                        if not resolved_zones:
                            try:
                                reg_info = util.ensure_execute(
                                    lkp.compute.regions().get(project=lkp.project, region=region)
                                )
                                resolved_zones = [z.split("/")[-1] for z in reg_info.get("zones", []) if z]
                            except Exception:
                                pass
                        # Only memoise a genuinely resolved list. Leaving the cache unset on
                        # failure keeps the per-node retry of the pre-hoist behaviour, so one
                        # transient error cannot pin the whole shard to the static zone guess.
                        if resolved_zones:
                            fallback_zones = resolved_zones
                    zones_for_node = fallback_zones or [f"{region}-{z}" for z in ("a", "b", "c", "f")]
                    for z in zones_for_node:
                        z_name = z.split("/")[-1]
                        link = f"zones/{z_name}/instances/{short_name}"
                        if link not in seen_links:
                            links.append(link)
                            seen_links.add(link)
                else:
                    # If the instance does not exist in GCE, skip it to prevent batch rejection
                    log.debug(f"Instance {short_name} does not exist in GCE; skipping deleteInstances.")
            elif retire_standalone:
                inst = None
                if check_gce_instances:
                    try:
                        inst = lkp.instance(short_name)
                    except Exception as e:
                        log.warning(
                            f"Could not list existing GCE instances; skipping standalone retirement check for MIG {mig_name}: {e}"
                        )
                        check_gce_instances = False
                if inst is not None:
                    owner_mig = util.instance_created_by_mig(inst)
                    if owner_mig == mig_name:
                        zone = getattr(inst, "zone", None)
                        if zone:
                            zone_name = zone.split("/")[-1]
                            link = f"zones/{zone_name}/instances/{short_name}"
                            if link not in seen_links:
                                links.append(link)
                                seen_links.add(link)
                    elif owner_mig is None:
                        if short_name not in seen_standalone:
                            standalone_nodes.append(short_name)
                            seen_standalone.add(short_name)
                    else:
                        log.warning(
                            f"Node {short_name} is managed by a different MIG ({owner_mig}, "
                            f"expected {mig_name}); skipping deletion."
                        )
                else:
                    log.debug(f"Node {node} is not present in MIG {mig_name}; skipping deleteInstances.")
            else:
                log.debug(f"Node {node} is not present in MIG {mig_name}; skipping deleteInstances.")

        if links:
            active_nodes = list(dict.fromkeys(l.split("/")[-1] for l in links))
            log.info(f"Deleting {len(active_nodes)} MIG instances ({to_hostlist(active_nodes)}) from MIG {mig_name}")
            for chunk_links in util.chunked(links, n=1000):
                req = lkp.compute.regionInstanceGroupManagers().deleteInstances(
                    project=lkp.project,
                    region=region,
                    instanceGroupManager=mig_name,
                    body={
                        "instances": chunk_links,
                        "skipInstancesOnValidationError": True,
                    }
                )
                try:
                    res = util.ensure_execute(req)
                    log.debug(f"deleteInstances response for {mig_name}: {res}")
                except Exception as e:
                    log.error(f"Failed deleteInstances for MIG {mig_name}: {e}")
                finally:
                    lkp.clear_mig_caches()
        else:
            log.info(f"No active instances found to delete in MIG {mig_name}")

        if standalone_nodes:
            log.info(
                f"Retiring {len(standalone_nodes)} legacy standalone VM(s) outside MIG "
                f"{mig_name}: {to_hostlist(standalone_nodes)}"
            )
            try:
                delete_instances(standalone_nodes, lkp=lkp)
            except Exception as e:
                log.error(
                    f"Failed retiring standalone VM(s) for MIG {mig_name} "
                    f"({to_hostlist(standalone_nodes)}): {e}"
                )

        # Clean up Per-Instance Configurations (PICs) for all requested nodes after instance deletion
        all_short_names = list({n.split(".")[0] for n in mig_nodes})
        for chunk_names in util.chunked(all_short_names, n=1000):
            try:
                pic_del_req = lkp.compute.regionInstanceGroupManagers().deletePerInstanceConfigs(
                    project=lkp.project,
                    region=region,
                    instanceGroupManager=mig_name,
                    body={"names": chunk_names},
                )
                util.ensure_execute(pic_del_req)
                log.debug(f"deletePerInstanceConfigs submitted for {mig_name}: {chunk_names}")
            except Exception as e:
                log.debug(f"deletePerInstanceConfigs note for {mig_name} on {chunk_names}: {e}", exc_info=log.isEnabledFor(logging.DEBUG))


def suspend_nodes(nodes: List[str]) -> None:
    lkp = lookup()
    other_nodes, gce_tpu_nodes = util.separate(lkp.is_tpu_node, nodes)
    bulk_nodes, flex_nodes = util.separate(lkp.is_flex_node, other_nodes)

    if gce_tpu_nodes:
        try:
            mig_flex.suspend_tpu_nodes(gce_tpu_nodes, lkp)
        except Exception:
            log.exception(f"Failed to suspend GCE TPU nodes {gce_tpu_nodes}")

    if flex_nodes:
        try:
            mig_flex.suspend_flex_nodes(flex_nodes, lkp)
        except Exception:
            log.exception(f"Failed to suspend flex nodes {flex_nodes}")

    non_mig_nodes, mig_nodes = util.separate(lkp.is_node_mig, bulk_nodes)
    if mig_nodes:
        try:
            suspend_mig_nodes(mig_nodes, lkp, retire_standalone=True)
        except Exception:
            log.exception(f"Failed to suspend MIG nodes {mig_nodes}")

    if non_mig_nodes:
        try:
            delete_instances(non_mig_nodes)
        except Exception:
            log.exception(f"Failed to suspend bulk nodes {non_mig_nodes}")


def main(nodelist):
    """main called when run as script"""
    log.debug(f"SuspendProgram {nodelist}")

    # Filter out nodes not in config.yaml
    other_nodes, pm_nodes = separate(
        lookup().is_power_managed_node, util.to_hostnames(nodelist)
    )
    if other_nodes:
        log.debug(
            f"Ignoring non-power-managed nodes '{to_hostlist(other_nodes)}' from '{nodelist}'"
        )
    if pm_nodes:
        log.debug(f"Suspending nodes '{to_hostlist(pm_nodes)}' from '{nodelist}'")
    else:
        log.debug("No cloud nodes to suspend")
        return

    log.info(f"suspend {nodelist}")
    suspend_nodes(pm_nodes)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("nodelist", help="list of nodes to suspend")
    args = util.init_log_and_parse(parser)

    main(args.nodelist)
