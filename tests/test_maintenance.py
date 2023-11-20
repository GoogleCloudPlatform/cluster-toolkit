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

import io
import unittest
import unittest.mock
import subprocess
import random
from tools.maintenance import maintenance

DESCRIPTION = """
This script is a unit test script for the tools/maintenance/maintenance.py.

Usage (run from top level directory of hpc toolkit):
python -m unittest 
"""

valid_prj_list = [f"hpc-toolkit-test{x}" for x in range(10)]
sched_maint_vms = [f"vm_test{x}" for x in range(10)]
can_resched = [True, False]
type_vars = ["SCHEDULED", "UNSCHEDULED"]
up_maint_vms = [[sched_maint_vms[i], "2024-10-26T13:25:51-0700", 
                 "2024-10-26T13:26:51-0700", can_resched[random.randint(0, 1)],
                 type_vars[random.randint(0, 1)]] 
                 for i in range(int(len(sched_maint_vms)/2))]

def subprocess_replace(cmd, shell, capture_output, text, 
                       check) -> subprocess.CompletedProcess:
    res = subprocess.CompletedProcess("", 0)
    res.stdout = ""
    if "version" in cmd:
        res.stdout = "{\n\t\"alpha\": \"2023.11.10\"\n}"
    elif "maintenanceInterval:PERIODIC" in cmd:
        for vm in sched_maint_vms:
            res.stdout += f"{vm}\n"
    elif "upcomingMaintenance" in cmd:
        for vm in up_maint_vms:
            res.stdout += f"{vm[0]}\t{vm[1]}\t{vm[2]}\t{vm[3]}\t{vm[4]}\n"
    elif "projects describe" in cmd:
        found = False
        for prj in valid_prj_list:
            if prj in cmd:
                found = True
                break
        if not found:
            res.returncode = 1

    return res

@unittest.mock.patch('sys.stdout', new_callable=io.StringIO)
@unittest.mock.patch('tools.maintenance.maintenance.subprocess.run',
                     wraps=subprocess_replace)
class TestProgram(unittest.TestCase):

    per_test_str1 = "No nodes with periodic maintenance"
    per_test_str2 = "Nodes with PERIODIC maintenance"

    def test_proj_name(self, mock_subprocess, mock_stdout):
        bad_prj = "hpc-toolkit-bad"
        try:
            maintenance.main(bad_prj)
        except subprocess.SubprocessError:
            pass
        rand_idx = random.randint(1, len(valid_prj_list))
        maintenance.main(valid_prj_list[rand_idx])

    def test_periodic_print_true(self, mock_subprocess, mock_stdout):
        maintenance.main(project=valid_prj_list[0], print_periodic_vms=True)
        self.assertTrue(self.per_test_str1 in mock_stdout.getvalue() or
                        self.per_test_str2 in mock_stdout.getvalue(),
                        msg="Output incorrect for periodic print")
                        

    def test_periodic_print_false(self, mock_subprocess, mock_stdout):
        maintenance.main(project=valid_prj_list[0], print_periodic_vms=False)
        self.assertFalse(self.per_test_str1 in mock_stdout.getvalue() or
                         self.per_test_str2 in mock_stdout.getvalue(),
                         msg="Output for periodic print found when there" \
                             " should be none")
