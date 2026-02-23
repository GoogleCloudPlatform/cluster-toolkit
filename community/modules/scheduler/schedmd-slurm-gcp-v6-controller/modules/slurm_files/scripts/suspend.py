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

from typing import List, Any
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




def suspend_nodes(nodes: List[str]) -> None:
    lkp = lookup()
    other_nodes, tpu_nodes = util.separate(lkp.node_is_tpu, nodes)
    bulk_nodes, flex_nodes = util.separate(lkp.is_flex_node, other_nodes)

    mig_flex.suspend_flex_nodes(flex_nodes, lkp)
    delete_instances(bulk_nodes)
    tpu.delete_tpu_instances(tpu_nodes)


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
