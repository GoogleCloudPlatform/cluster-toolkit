#!/slurm/python/venv/bin/python3.13

# Copyright 2025 Google Inc. All rights reserved.
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

# This script cleans up any VMs that are in the Slurm DOWN* state and are not
# in the TERMINATED state in GCE.  This should run periodically in the
# slurmcleanup service.  It assumes that the slurmcmd service is running to
# periodically restart the nodes that have been cleaned up.

import argparse
import logging
import fcntl
import pathlib
import sys
from typing import List

import util 
import suspend

log = logging.getLogger()

def _get_dead_nodes(lkp: util.Lookup) -> List[str]:
    """_get_dead_nodes checks sinfo for any nodes that are in the *DOWN state
    and correlates them with nodes that are still active in GCE.

    Args:
        lkp (util.Lookup): information on the current state of the slurm
        cluster

    Returns:
        List[str]: list of nodes to suspend
    """

    # Get a list of nodes that fit the state DOWN*, should be a list of all
    # nodes on their own lines with no header
    result = util.run('sinfo -t "DOWN" -o "%N" --dead --noheader -N', timeout=30)
    down_nodes = result.stdout.splitlines()

    all_instances = lkp.instances()
    dead_nodes = [
        node for node in down_nodes 
        if node not in all_instances and all_instances[node].status != "TERMINATED"]

    return dead_nodes


def cleanup_nodes(lkp: util.Lookup) -> None:
    """cleanup_nodes suspends any nodes that are unresponsive and in a bad
    state (as defined by _get_dead_nodes).

    Args:
        lkp (util.Lookup): information on the current state of the slurm
        cluster
    """
    nodes_to_delete = _get_dead_nodes(lkp)
    if nodes_to_delete:
        log.info(f"Cleaning up the following nodes: {nodes_to_delete}")
        suspend.suspend_nodes(nodes_to_delete)


def main():
    lkp = util.lookup()
    if lkp.is_controller:
        try:
            cleanup_nodes(lkp)
        except Exception as e:
            log.exception(f"failed to cleanup slurm nodes: {e}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    _ = util.init_log_and_parse(parser)

    pid_file = (pathlib.Path("/tmp") / pathlib.Path(__file__).name).with_suffix(".pid")
    with pid_file.open("w") as fp:
        try:
            fcntl.lockf(fp, fcntl.LOCK_EX | fcntl.LOCK_NB)
            main()
        except BlockingIOError:
            sys.exit(0)
