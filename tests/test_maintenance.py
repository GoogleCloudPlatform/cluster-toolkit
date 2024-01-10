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

import io
import fnmatch
import unittest
import unittest.mock
import subprocess
from tools.maintenance import maintenance

DESCRIPTION = """
This is a unit test script for the tools/maintenance/maintenance.py.

Usage (run from top level directory of hpc toolkit):
python -m unittest 
"""

# hpc-toolkit-test0 will have nodes that have maintenance
# hpc-toolkit-test1 will not have nodes that have maintenance
VALID_PRJS = {"scheduled": "hpc-toolkit-test0", "unscheduled": "hpc-toolkit-test1"}
VM_TYPE_CNT = 10
# Perioidc maintenance vm list
PER_MAINT_VMS = [f"vm_test{x}" for x in range(VM_TYPE_CNT)]
# Add slurm specific nodes to list of periodic maintenance VMS
PER_MAINT_VMS += [f"slurm_test{x}" for x in range(VM_TYPE_CNT)]
T1 = "2024-10-26T13:25:51-0700"
T2 = "2024-10-26T13:26:51-0700"
# Columns that represent the upcoming maintenance given the command in
# maintenance.py
# VM Name, Earliest Sched Maint Start Time, Latest Sched Maint Start Time,
# Reschedulable, Sched type
# Last 4 are slurm nodes
UPC_MAINT_VMS = [[PER_MAINT_VMS[0], T1, T2, "TRUE", "SCHEDULED"],
                [PER_MAINT_VMS[1], T1, T2, "FALSE", "SCHEDULED"],
                [PER_MAINT_VMS[2], T1, T2, "TRUE", "UNSCHEDULED"],
                [PER_MAINT_VMS[3], T1, T2, "FALSE", "UNSCHEDULED"],
                [PER_MAINT_VMS[10], T1, T2, "TRUE", "SCHEDULED"],
                [PER_MAINT_VMS[11], T1, T2, "FALSE", "SCHEDULED"],
                [PER_MAINT_VMS[12], T1, T2, "TRUE", "UNSCHEDULED"],
                [PER_MAINT_VMS[13], T1, T2, "FALSE", "UNSCHEDULED"]]
VERSION_RES = """
{
    "alpha": "2023.11.10"
}
"""

def subprocess_replace(cmd, shell, capture_output, text,
                       check) -> subprocess.CompletedProcess:
    res = subprocess.CompletedProcess("", 0)
    res.stdout = ""
    if cmd == maintenance.VER_CMD:
        res.stdout = VERSION_RES
        return res
    if cmd == maintenance.SLURM_CMD:
        for vm in PER_MAINT_VMS[10:]:
            res.stdout += f"{vm}\n"
        return res
    if fnmatch.fnmatch(cmd, maintenance.PRJ_CMD.format("*")):
        if not any(prj in cmd for prj in list(VALID_PRJS.values())):
            res.returncode = 1
        return res
    if fnmatch.fnmatch(cmd, maintenance.PER_MAINT_CMD.format("*")):
        if VALID_PRJS["scheduled"] in cmd:
            for vm in PER_MAINT_VMS:
                res.stdout += f"{vm}\n"
        return res
    if fnmatch.fnmatch(cmd, maintenance.UPC_MAINT_CMD.format("*")):
        if VALID_PRJS["scheduled"] in cmd:
            for vm in UPC_MAINT_VMS:
                res.stdout += f"{vm[0]}\t{vm[1]}\t{vm[2]}\t{vm[3]}\t{vm[4]}\n"
        return res

    return res

@unittest.mock.patch('sys.stdout', new_callable=io.StringIO)
@unittest.mock.patch('tools.maintenance.maintenance.subprocess.run',
                     wraps=subprocess_replace)
class TestProgram(unittest.TestCase):

    no_per_str = "No nodes with periodic maintenance"
    has_per_str = "Nodes with PERIODIC maintenance"

    def test_valid_proj_name(self, mock_subprocess, mock_stdout):
        maintenance.main(VALID_PRJS["scheduled"])

    def test_invalid_proj_name(self, mock_subprocess, mock_stdout):
        invalid_prj = "hpc-toolkit-bad"
        self.assertRaises(subprocess.SubprocessError, maintenance.main,
                          project=invalid_prj)

    def test_periodic_print_true(self, mock_subprocess, mock_stdout):
        maintenance.main(project=VALID_PRJS["scheduled"],
                         print_periodic_vms=True)
        self.assertIn(self.has_per_str, mock_stdout.getvalue(),
                        msg="No nodes found when there should be some")

        maintenance.main(project=VALID_PRJS["unscheduled"],
                         print_periodic_vms=True)
        self.assertIn(self.has_per_str, mock_stdout.getvalue(),
                        msg="Nodes found when there should be none")

    def test_periodic_print_false(self, mock_subprocess, mock_stdout):
        maintenance.main(project=VALID_PRJS["scheduled"],
                         print_periodic_vms=False)
        err_msg = "Output for periodic print found when there should be none"
        self.assertNotIn(self.no_per_str, mock_stdout.getvalue(), err_msg)
        self.assertNotIn(self.has_per_str, mock_stdout.getvalue(), err_msg)

        maintenance.main(project=VALID_PRJS["unscheduled"],
                         print_periodic_vms=False)
        self.assertNotIn(self.no_per_str, mock_stdout.getvalue(), err_msg)
        self.assertNotIn(self.has_per_str, mock_stdout.getvalue(), err_msg)

    def test_slurm_filter(self, mock_subprocess, mock_stdout):
        maint = maintenance.node_maintenace_factory(VALID_PRJS["scheduled"],
                                                    check_maint = True,
                                                    slurm = True)
        err_msg = "Correct number of slurm nodes not found"
        self.assertEqual(len(maint.slurm_nodes), VM_TYPE_CNT, err_msg)
        self.assertEqual(len(maint.per_maint_vms), VM_TYPE_CNT, err_msg)
        self.assertEqual(len(maint.upc_maint_vms), 4, err_msg)
