#!/usr/bin/env python3
# Copyright 2023 Google LLC
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
import re
import sys
import json
import subprocess
from typing import List
import argparse

DESCRIPTION = """
maintenance.py is a tool to identify print information about VMs with periodic
maintenance enabled or VMs with upcoming maintenance.

With no mode flags set, this program will print the information about VMs with
upcoming maintenance.
"""
USAGE = """
tools/maintenance/maintenance.py -p <PROJECT_ID> [-n <regex string>] [-m] [-s]
"""

UPC_MAINT_CMD = "gcloud alpha compute instances list --project={}" \
                " --filter='upcomingMaintenance:*' --format='value(name," \
                "upcomingMaintenance.startTimeWindow.earliest," \
                "upcomingMaintenance.startTimeWindow.latest," \
                "upcomingMaintenance.canReschedule,upcomingMaintenance.type)'"
PER_MAINT_CMD = "gcloud alpha compute instances list --project={}" \
                " --filter=scheduling.maintenanceInterval:PERIODIC " \
                " --format='value(name)'"
VER_CMD = "gcloud version --format='json(\"alpha\")'"
PRJ_CMD = "gcloud projects describe {}"
SLURM_CMD = "sinfo --format=%n --noheader"

def run_command(cmd: str, err_msg: str = None) -> subprocess.CompletedProcess:
    res = subprocess.run(cmd, shell=True, capture_output=True, text=True,
                         check=False)
    if res.returncode != 0:
        raise subprocess.SubprocessError(f"{err_msg}:\n{res.stderr}")

    return res

def check_gcloud_components() -> None:
    err_msg = "Error getting Google Cloud SDK versions"
    res = run_command(VER_CMD, err_msg)

    version_dict = json.loads(res.stdout)
    if "alpha" not in version_dict:
        err_msg = "Component \"alpha\" was not found in the Google Cloud SDK" \
                   " version list"
        raise LookupError(err_msg)

def get_maintenance_nodes(project: str) -> List[str]:
    err_msg = "Error getting VMs that have scheduled maintenance"
    res = run_command(PER_MAINT_CMD.format(project), err_msg)

    maint_nodes = res.stdout.split('\n')[:-1]

    return maint_nodes

def get_upcoming_maintenance(project: str) -> List[str]:
    err_msg = "Error getting upcoming maintenance list"
    res = run_command(UPC_MAINT_CMD.format(project), err_msg)

    upc_maint = [x.split() for x in res.stdout.split("\n")[:-1]]

    return upc_maint

class NodeMaintenance:
    '''
    Class to keep track of project, zones, and search terms for nodes, as well
    as results from gcloud queries
    '''
    def __init__(self, project: str, regex: re = None,
                 slurm_nodes: List[str] = None) -> None:
        self.project = project
        self.regex = regex
        self.slurm_nodes = slurm_nodes
        self.per_maint_vms = None
        self.upc_maint_vms = None

    def update_maintenance_nodes(self) -> None:
        per_maint_vms = get_maintenance_nodes(self.project)
        if self.regex:
            per_maint_vms = list(filter(self.regex.search, per_maint_vms))

        if self.slurm_nodes:
            per_maint_vms = list(set(per_maint_vms) & set(self.slurm_nodes))
        
        self.per_maint_vms = per_maint_vms

    def update_upcoming_maintenance(self) -> None:
        upc_maint_vms = get_upcoming_maintenance(self.project)
        if self.regex:
            upc_maint_vms = list(filter(lambda x: self.regex.match(x[0]),
                                  upc_maint_vms))

        if self.slurm_nodes:
            upc_maint_vms = [u for u in upc_maint_vms if u[0] in
                              self.slurm_nodes]

        self.upc_maint_vms = upc_maint_vms

    def print_maintenance_nodes(self) -> None:
        if self.per_maint_vms is None:
            self.update_maintenance_nodes()

        if not self.per_maint_vms:
            print("No nodes with periodic maintenance\n")
            return

        print("Nodes with PERIODIC maintenance")
        for x in self.per_maint_vms:
            print(x)
        print()

    def print_upcoming_maintenance(self) -> None:
        if self.upc_maint_vms is None:
            self.update_upcoming_maintenance()

        if not self.upc_maint_vms:
            print("No upcoming maintenance\n")
            return

        print("Upcoming maintenance:")
        row_format ="{:30}" * (len(self.upc_maint_vms[0]))
        print(row_format.format(*["Name", "Earliest Start", "Latest Start",
                                  "Can Reschedule", "Maintenance Type"]))
        for row in self.upc_maint_vms:
            print(row_format.format(*row))
        print()

def node_maintenace_factory(project: str, regex: str = None,
                            check_maint: bool = False,
                            slurm: bool = False) -> NodeMaintenance:
    err_msg = f"{project} does not exist or you may not have permission to" \
              " access it"
    res = run_command(PRJ_CMD.format(project), err_msg)

    compiled_regex = None
    if regex:
        try:
            compiled_regex = re.compile(regex)
        except re.error as e:
            print(f"Invalid regular expression: {e}")
            sys.exit()

    slurm_nodes = None
    if slurm:
        err_msg = "sinfo command failed, are you on a Slurm cluster?"
        res = run_command(SLURM_CMD, err_msg)
        slurm_nodes = res.stdout.split()

    maint = NodeMaintenance(project, compiled_regex, slurm_nodes)
    if check_maint:
        maint.update_maintenance_nodes()
    maint.update_upcoming_maintenance()

    return maint

def main(project: str, vm_regex: str = None, print_periodic_vms: bool = False,
         slurm: bool = False) -> None:
    check_gcloud_components()

    maint = node_maintenace_factory(project, vm_regex, print_periodic_vms, 
                                    slurm)

    if print_periodic_vms:
        maint.print_maintenance_nodes()

    maint.print_upcoming_maintenance()

if __name__ == "__main__":
    parser = argparse.ArgumentParser(prog='maintenance.py',
                                     description=DESCRIPTION,
                                     formatter_class=argparse.RawTextHelpFormatter)
    parser.add_argument("-p", "--project", required=True,
                        help="GCP project id")
    parser.add_argument("-v", "--vm_regex",
                        help="Regular expression search for nodes")
    parser.add_argument("-m", "--print_periodic_vms", action="store_true",
                        help="Display nodes that have periodic" \
                             " maintenance setup")
    parser.add_argument("-s", "--slurm", action="store_true",
                        help="Filter results based on local slurm cluster")

    if len(sys.argv)==1:
        parser.print_help(sys.stderr)
        sys.exit(1)

    args = parser.parse_args()

    main(args.project, args.vm_regex, args.print_periodic_vms, args.slurm)
