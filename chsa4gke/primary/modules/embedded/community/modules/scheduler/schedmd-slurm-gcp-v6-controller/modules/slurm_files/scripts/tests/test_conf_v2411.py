# Copyright 2025 Google LLC
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

import pytest
from mock import Mock
from common import TstNodeset, TstCfg, TstMachineConf, TstTemplateInfo, Placeholder

import addict # type: ignore
from conf import conflines
import util


@pytest.mark.parametrize(
    "cfg,want",
    [
        (TstCfg(
            install_dir="ukulele",
        ),
         """LaunchParameters=enable_nss_slurm,use_interactive_step
SlurmctldParameters=cloud_dns,enable_configless,idle_on_node_suspend
SchedulerParameters=bf_continue,salloc_wait_nodes,ignore_prefer_validation
ResumeProgram=ukulele/resume_wrapper.sh
ResumeFailProgram=ukulele/suspend_wrapper.sh
ResumeRate=0
ResumeTimeout=300
SuspendProgram=ukulele/suspend_wrapper.sh
SuspendRate=0
SuspendTimeout=300
SlurmdTimeout=300
UnkillableStepTimeout=300
TreeWidth=128
TopologyParam=SwitchAsNodeRank"""),
        (TstCfg(
            install_dir="ukulele",
            cloud_parameters={
                "no_comma_params": True,
                "private_data": None,
                "scheduler_parameters": None,
                "resume_rate": None,
                "resume_timeout": None,
                "suspend_rate": None,
                "suspend_timeout": None,
                "unkillable_step_timeout": None,
                "slurmd_timeout": None,
                "topology_plugin": None,
                "topology_param": None,
                "tree_width": None,
            },
        ),
         """SchedulerParameters=bf_continue,salloc_wait_nodes,ignore_prefer_validation
ResumeProgram=ukulele/resume_wrapper.sh
ResumeFailProgram=ukulele/suspend_wrapper.sh
ResumeRate=0
ResumeTimeout=300
SuspendProgram=ukulele/suspend_wrapper.sh
SuspendRate=0
SuspendTimeout=300
SlurmdTimeout=300
UnkillableStepTimeout=300
TreeWidth=128
TopologyParam=SwitchAsNodeRank"""),
        (TstCfg(
            install_dir="ukulele",
            cloud_parameters={
                "no_comma_params": True,
                "private_data": [
                    "events",
                    "jobs",
                ],
                "scheduler_parameters": [
                    "bf_busy_nodes",
                    "bf_continue",
                    "ignore_prefer_validation",
                    "nohold_on_prolog_fail",
                ],
                "resume_rate": 1,
                "resume_timeout": 2,
                "suspend_rate": 3,
                "suspend_timeout": 4,
                "slurmd_timeout": 5,
                "unkillable_step_timeout": 6,
                "tree_width": 7,
                "topology_plugin": "guess",
                "topology_param": "yellow",
            },
            nodeset={"a": TstNodeset()},
        ),
         """PrivateData=events,jobs
SchedulerParameters=bf_busy_nodes,bf_continue,ignore_prefer_validation,nohold_on_prolog_fail
ResumeProgram=ukulele/resume_wrapper.sh
ResumeFailProgram=ukulele/suspend_wrapper.sh
ResumeRate=1
ResumeTimeout=2
SuspendProgram=ukulele/suspend_wrapper.sh
SuspendRate=3
SuspendTimeout=4
SlurmdTimeout=5
UnkillableStepTimeout=6
TreeWidth=7
TopologyPlugin=guess
TopologyParam=yellow"""),
        (TstCfg(
            install_dir="ukulele",
            task_prolog_scripts=[Placeholder()],
            task_epilog_scripts=[Placeholder()],
        ),
         """LaunchParameters=enable_nss_slurm,use_interactive_step
SlurmctldParameters=cloud_dns,enable_configless,idle_on_node_suspend
TaskProlog=/slurm/custom_scripts/task_prolog.d/task-prolog
TaskEpilog=/slurm/custom_scripts/task_epilog.d/task-epilog
SchedulerParameters=bf_continue,salloc_wait_nodes,ignore_prefer_validation
ResumeProgram=ukulele/resume_wrapper.sh
ResumeFailProgram=ukulele/suspend_wrapper.sh
ResumeRate=0
ResumeTimeout=300
SuspendProgram=ukulele/suspend_wrapper.sh
SuspendRate=0
SuspendTimeout=300
SlurmdTimeout=300
UnkillableStepTimeout=300
TreeWidth=128
TopologyParam=SwitchAsNodeRank"""),
    ])
def test_conflines(cfg, want):
    lkp = util.Lookup(cfg)
    lkp.cfg.slurm_version = "24.11"
    lkp.template_info = Mock(return_value=TstTemplateInfo(gpu=None))
    assert conflines(lkp) == want

    cfg.cloud_parameters = addict.Dict(cfg.cloud_parameters)
    lkp = util.Lookup(cfg)
    lkp.cfg.slurm_version = "24.11"
    lkp.template_info = Mock(return_value=TstTemplateInfo(gpu=None))
    assert conflines(lkp) == want
