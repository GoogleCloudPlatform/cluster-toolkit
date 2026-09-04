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

from typing import List, Any, Dict
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
import tpu
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


def delete_instance_request(name: str) -> Any:
    inst = lookup().instance(name)
    assert inst

    request = lookup().compute.instances().delete(
        project=lookup().project,
        zone=inst.zone,
        instance=name,
    )
    log_api_request(request)
    return request


def delete_instances(instances):
    """delete instances individually"""
    invalid, valid = separate(lambda inst: bool(lookup().instance(inst)), instances)
    if len(invalid) > 0:
        log.debug("instances do not exist: {}".format(",".join(invalid)))
    if len(valid) == 0:
        log.debug("No instances to delete")
        return

    requests = {inst: delete_instance_request(inst) for inst in valid}

    log.info(f"to delete {len(valid)} instances ({to_hostlist(valid)})")
    ops, failed = batch_execute(requests)
    for node, (_, err) in failed.items():
        log.error(f"instance {node} failed to delete: {err}")
    
    log.info(f"deleting {len(ops)} instances {to_hostlist(ops.keys())}")

    topic = watch_delete_vm_op.watch_delete_vm_op_topic()
    for node, op in ops.items():
        topic.publish(op, node)




def suspend_mig_nodes(nodes: List[str], lkp: util.Lookup) -> None:
    """Deletes specific instance names from MIG and reduces target size atomically."""
    if not nodes:
        return

    # Group nodes by target MIG (to support multiple MIGs for >1000 nodes)
    nodes_by_mig: Dict[str, List[str]] = {}
    for node in nodes:
        mig_name = lkp.node_mig_name(node)
        nodes_by_mig.setdefault(mig_name, []).append(node)

    for mig_name, mig_nodes in nodes_by_mig.items():
        region = lkp.node_region(mig_nodes[0])

        # Resolve instance URLs from MIG or local lookup
        mig_inst_map = {}
        mig_list_success = False
        try:
            mig_data = lkp.get_mig_instances(lkp.project, region, mig_name)
            for item in mig_data.get("managedInstances", []):
                if item.get("instance"):
                    mig_inst_map[item["instance"].split("/")[-1]] = item["instance"]
                    if item.get("name"):
                        mig_inst_map[item["name"]] = item["instance"]
            mig_list_success = True
        except Exception as e:
            log.warning(f"Could not list managed instances for MIG {mig_name}: {e}")
            mig_inst_map = {}
            mig_list_success = False

        links = []
        seen_links = set()
        for node in mig_nodes:
            short_name = node.split(".")[0]
            if short_name in mig_inst_map:
                link = mig_inst_map[short_name]
                if link not in seen_links:
                    links.append(link)
                    seen_links.add(link)
            elif not mig_list_success:
                # Only guess fallback URLs if listManagedInstances failed completely
                inst = lkp.instance(short_name)
                zone = getattr(inst, "zone", None)
                if zone:
                    zone_name = zone.split("/")[-1]
                    link = f"zones/{zone_name}/instances/{short_name}"
                    if link not in seen_links:
                        links.append(link)
                        seen_links.add(link)
                else:
                    # If the instance does not exist in GCE, skip it to prevent batch rejection
                    log.debug(f"Instance {short_name} does not exist in GCE; skipping deleteInstances.")
            else:
                log.debug(f"Node {node} is not present in MIG {mig_name}; skipping deleteInstances.")

        if links:
            active_nodes = [l.split("/")[-1] for l in links]
            log.info(f"Deleting {len(links)} MIG instances ({to_hostlist(active_nodes)}) from MIG {mig_name}")
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
                    lkp.get_mig_instances.cache_clear()
                    lkp.get_mig_repairing_instances.cache_clear()
        else:
            log.info(f"No active instances found to delete in MIG {mig_name}")

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
    other_nodes, tpu_nodes = util.separate(lkp.node_is_tpu, nodes)
    bulk_nodes, flex_nodes = util.separate(lkp.is_flex_node, other_nodes)

    if flex_nodes:
        try:
            mig_flex.suspend_flex_nodes(flex_nodes, lkp)
        except Exception:
            log.exception(f"Failed to suspend flex nodes {flex_nodes}")

    non_mig_nodes, mig_nodes = util.separate(lkp.is_node_mig, bulk_nodes)
    if mig_nodes:
        try:
            suspend_mig_nodes(mig_nodes, lkp)
        except Exception:
            log.exception(f"Failed to suspend MIG nodes {mig_nodes}")

    if non_mig_nodes:
        try:
            delete_instances(non_mig_nodes)
        except Exception:
            log.exception(f"Failed to suspend bulk nodes {non_mig_nodes}")

    if tpu_nodes:
        try:
            tpu.delete_tpu_instances(tpu_nodes)
        except Exception:
            log.exception(f"Failed to delete TPU instances {tpu_nodes}")


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
