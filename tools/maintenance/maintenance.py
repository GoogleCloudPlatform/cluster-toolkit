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
tools/maintenance/maintenance.py -p <PROJECT_ID> [-n <regex string>] [-m]
"""

def check_gcloud_components() -> None:
    cmd = "gcloud version --format='json(\"alpha\")'"
    res = subprocess.run(cmd, shell=True, capture_output=True, text=True,
                         check=False)
    if res.returncode != 0:
        err_msg = f"Error getting Google Cloud SDK versions:\n{res.stderr}"
        raise subprocess.SubprocessError(err_msg)

    version_dict = json.loads(res.stdout)
    if "alpha" not in version_dict:
        err_msg = "Component \"alpha\" was not found in the Google Cloud SDK" \
                   " version list"
        raise LookupError(err_msg)

def get_maintenance_nodes(project: str, regex: re = None) -> List[str]:
    cmd = f"gcloud alpha compute instances list --project={project}" \
            " --filter=scheduling.maintenanceInterval:PERIODIC " \
            " --format='value(name)'"

    res = subprocess.run(cmd, shell=True, capture_output=True, text=True,
                         check=False)
    if res.returncode != 0:
        err_msg = "Error getting VMs that have scheduled " \
                  f"maintenance:\n{res.stderr}"
        raise subprocess.SubprocessError(err_msg)

    maint_nodes = res.stdout.split('\n')[:-1]
    if regex:
        return list(filter(regex.match, maint_nodes))

    return maint_nodes

def get_upcoming_maintenance(project: str, regex: re = None) -> List[str]:
    cmd = f"gcloud alpha compute instances list --project={project}" \
            " --filter='upcomingMaintenance:*' --format='value(name," \
            "upcomingMaintenance.startTimeWindow.earliest," \
            "upcomingMaintenance.startTimeWindow.latest," \
            "upcomingMaintenance.canReschedule,upcomingMaintenance.type)'"

    res = subprocess.run(cmd, shell=True, capture_output=True, text=True, check=False)
    if res.returncode != 0:
        err_msg = f"Error getting upcoming maintenance list:\n{res.stderr}"
        raise subprocess.SubprocessError(err_msg)

    upc_maint = [x.split() for x in res.stdout.split("\n")[:-1]]
    if regex:
        return list(filter(lambda x: regex.match(x[0]), upc_maint))

    return upc_maint

class NodeMaintenance:
    '''
    Class to keep track of project, zones, and search terms for nodes, as well
    as results from gcloud queries
    '''
    def __init__(self, project: str, regex: re = None) -> None:
        self.project = project
        self.regex = regex
        self.maint_nodes = None
        self.upc_maint = None

    def update_maintenance_nodes(self) -> None:
        self.maint_nodes = get_maintenance_nodes(self.project, self.regex)

    def update_upcoming_maintenance(self) -> None:
        self.upc_maint = get_upcoming_maintenance(self.project, self.regex)

    def print_maintenance_nodes(self) -> None:
        if self.maint_nodes is None:
            self.update_maintenance_nodes()

        if not self.maint_nodes:
            print("No nodes with periodic maintenance\n")
            return

        print("Nodes with PERIODIC maintenance")
        for x in self.maint_nodes:
            print(x)
        print()

    def print_upcoming_maintenance(self) -> None:
        if self.upc_maint is None:
            self.update_upcoming_maintenance()

        if not self.upc_maint:
            print("No upcoming maintenance\n")
            return

        print("Upcoming maintenance:")
        row_format ="{:30}" * (len(self.upc_maint[0]))
        print(row_format.format(*["Name", "Earliest Start", "Latest Start",
                                  "Can Reschedule", "Maintenance Type"]))
        for row in self.upc_maint:
            print(row_format.format(*row))
        print()

def node_maintenace_factory(project: str, regex: str = None,
                            run_maintenance: bool = False) -> NodeMaintenance:
    res = subprocess.run(f"gcloud projects describe {project}", shell=True,
                         capture_output=True, text=True, check=False)

    if res.returncode != 0:
        err_msg = f"{project} does not exist or you may not have permission" \
                   " to access it"
        raise subprocess.SubprocessError(err_msg)

    compiled_regex = None
    if regex:
        compiled_regex = re.compile(regex)

    maint = NodeMaintenance(project, compiled_regex)
    if run_maintenance:
        maint.update_maintenance_nodes()
    maint.update_upcoming_maintenance()

    return maint

def main(project: str, vm_regex: str = None, print_periodic_vms: bool = False) -> None:
    check_gcloud_components()

    maint = node_maintenace_factory(project, vm_regex, print_periodic_vms)

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
                        help="Disply nodes that have periodic" \
                             " maintenance setup")

    if len(sys.argv)==1:
        parser.print_help(sys.stderr)
        sys.exit(1)

    args = parser.parse_args()

    main(args.project, args.vm_regex, args.print_periodic_vms)
