# Copyright 2026 Google LLC
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
import mock
from common import TstNodeset, TstCfg, TstMachineConf, TstTemplateInfo, Placeholder

from util import NSDict
import conf
import util


def test_nodeset_tpu_lines():
    nodeset = TstNodeset(
        "turbo",
        node_count_static=2,
        node_count_dynamic_max=3,
        node_conf={"red": "velvet"},
    )
    assert conf.nodeset_tpu_lines(nodeset, util.Lookup(TstCfg())) == "\n".join(
        [
            "NodeName=m22-turbo-[0-4] State=CLOUD red=velvet",
            "NodeSet=turbo Nodes=m22-turbo-[0-4]",
        ]
    )


def test_nodeset_lines():
    nodeset = TstNodeset(
        "turbo",
        node_count_static=2,
        node_count_dynamic_max=3,
        node_conf={"red": "velvet", "CPUs": 55},
    )
    lkp = util.Lookup(TstCfg(nodeset={'turbo': nodeset}))
    lkp.template_info = mock.Mock(return_value=TstTemplateInfo(
        gpu=util.AcceleratorInfo(type="Popov", count=33)
    ))
    mc = TstMachineConf(
        cpus=5,
        memory=6,
        sockets=7,
        sockets_per_board=8,
        boards=9,
        threads_per_core=10,
        cores_per_socket=11,
    )
    lkp.template_machine_conf = mock.Mock(return_value=mc) # type: ignore[method-assign]
    assert conf.nodeset_lines(nodeset, lkp) == "\n".join(
        [
            "NodeName=m22-turbo-[0-4] State=CLOUD RealMemory=6 Boards=9 SocketsPerBoard=8 CoresPerSocket=11 ThreadsPerCore=10 CPUs=55 Gres=gpu:33 red=velvet",
            "NodeSet=turbo Nodes=m22-turbo-[0-4]",
        ]
    )


@pytest.mark.parametrize(
    "value,want",
    [
        ({"a": 1}, "a=1"),
        ({"a": "two"}, "a=two"),
        ({"a": [3, 4]}, "a=3,4"),
        ({"a": ["five", "six"]}, "a=five,six"),
        ({"a": None}, ""),
        ({"a": ["seven", None, 8]}, "a=seven,8"),
        ({"a": 1, "b": "two"}, "a=1 b=two"),
        ({"a": 1, "b": None, "c": "three"}, "a=1 c=three"),
        ({"a": 0, "b": None, "c": 0.0, "e": ""}, "a=0 c=0.0"),
        ({"a": [0, 0.0, None, "X", "", "Y"]}, "a=0,0.0,X,,Y"),
    ])
def test_dict_to_conf(value: dict, want: str):
    assert conf.dict_to_conf(value) == want



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
@pytest.mark.parametrize(
    "version",
    ["24.11", "25.05", "25.11"]
)
@mock.patch('util.Lookup.slurm_version', new_callable=mock.PropertyMock)
def test_conflines(mock_slurm_version, version, cfg, want):
    mock_slurm_version.return_value = version
    lkp = util.Lookup(cfg)
    lkp.template_info = mock.Mock(return_value=TstTemplateInfo(gpu=None))
    assert conf.conflines(lkp) == want

    cfg.cloud_parameters = NSDict(cfg.cloud_parameters)
    lkp = util.Lookup(cfg)
    lkp.template_info = mock.Mock(return_value=TstTemplateInfo(gpu=None))
    assert conf.conflines(lkp) == want


@pytest.mark.parametrize(
    "version,expect_async",
    [
        ("24.11", False),
        ("25.05", False),
        ("25.11", True),
    ]
)
@mock.patch('util.Lookup.slurm_version', new_callable=mock.PropertyMock)
def test_conflines_async_reply(mock_slurm_version, version, expect_async):
    cfg = TstCfg(
        install_dir="ukulele",
        experimental={
            "enable_async_reply": True,
        },
    )
    mock_slurm_version.return_value = version
    lkp = util.Lookup(cfg)
    lkp.template_info = mock.Mock(return_value=TstTemplateInfo(gpu=None))
    
    res = conf.conflines(lkp)
    if expect_async:
        assert "enable_async_reply" in res
        # Validate exact expected output structure for 25.11 with async reply
        assert "SlurmctldParameters=cloud_dns,enable_configless,idle_on_node_suspend,enable_async_reply" in res
    else:
        assert "enable_async_reply" not in res
        # Validate exact expected output structure for older versions without async reply
        assert "SlurmctldParameters=cloud_dns,enable_configless,idle_on_node_suspend" in res

@pytest.mark.parametrize(
    "version,expect_feature",
    [
        ("24.11", False),
        ("25.05", False),
        ("25.11", True),
    ]
)
@mock.patch('util.Lookup.slurm_version', new_callable=mock.PropertyMock)
def test_conflines_expedited_requeue(mock_slurm_version, version, expect_feature):
    cfg = TstCfg(
        install_dir="ukulele",
        enable_expedited_requeue=True,
    )
    mock_slurm_version.return_value = version
    lkp = util.Lookup(cfg)
    lkp.template_info = mock.Mock(return_value=TstTemplateInfo(gpu=None))
    
    res = conf.conflines(lkp)
    if expect_feature:
        assert "enable_expedited_requeue" in res
        assert "SlurmctldParameters=cloud_dns,enable_configless,idle_on_node_suspend,enable_expedited_requeue" in res
    else:
        assert "enable_expedited_requeue" not in res
        assert "SlurmctldParameters=cloud_dns,enable_configless,idle_on_node_suspend" in res

@pytest.mark.parametrize(
    "version,expect_feature",
    [
        ("24.11", False),
        ("25.05", False),
        ("25.11", True),
    ]
)
@mock.patch('util.Lookup.slurm_version', new_callable=mock.PropertyMock)
def test_conflines_health_check_start_only(mock_slurm_version, version, expect_feature):
    cfg = TstCfg(
        install_dir="ukulele",
        enable_health_check_start_only=True,
    )
    mock_slurm_version.return_value = version
    lkp = util.Lookup(cfg)
    lkp.template_info = mock.Mock(return_value=TstTemplateInfo(gpu=None))
    
    res = conf.conflines(lkp)
    if expect_feature:
        assert "HealthCheckNodeState=START_ONLY" in res
    else:
        assert "HealthCheckNodeState=START_ONLY" not in res


@pytest.mark.parametrize(
    "version,expect_feature",
    [
        ("24.11", False),
        ("25.05", False),
        ("25.11", True),
    ]
)
@mock.patch('util.Lookup.slurm_version', new_callable=mock.PropertyMock)
def test_conflines_enable_openmetrics(mock_slurm_version, version, expect_feature):
    cfg = TstCfg(
        install_dir="ukulele",
        enable_openmetrics=True,
    )
    mock_slurm_version.return_value = version
    lkp = util.Lookup(cfg)
    lkp.template_info = mock.Mock(return_value=TstTemplateInfo(gpu=None))
    
    res = conf.conflines(lkp)
    if expect_feature:
        assert "MetricsType=metrics/openmetrics" in res
    else:
        assert "MetricsType=metrics/openmetrics" not in res


@pytest.mark.parametrize(
    "version",
    ["25.05", "25.11"]
)
@pytest.mark.parametrize(
    "cfg,gputype,gpucount,want",
    [
        (TstCfg(),
        "",
        0,
         "\n"),
        (TstCfg(
            nodeset={"turbo": TstNodeset("turbo")}
        ), 
        "Popov",
        8,
         "NodeName=m22-turbo-[0-4] Name=gpu Type=Popov File=/dev/nvidia[0-7]\n\n"),
    ])
@mock.patch('util.Lookup.slurm_version', new_callable=mock.PropertyMock)
def test_gen_cloud_gres_conf_lines(mock_slurm_version, version, cfg, gputype, gpucount, want):
    mock_slurm_version.return_value = version
    lkp = util.Lookup(cfg)
    lkp.template_info = mock.Mock(return_value=TstTemplateInfo(
        gpu=util.AcceleratorInfo(type=gputype, count=gpucount)
    ))
    # mock nodelist to be smaller
    lkp.nodelist = mock.Mock(return_value="m22-turbo-[0-4]")  # type: ignore[method-assign]
    assert conf.get_generator(lkp).gen_cloud_gres_conf_lines() == want


def test_block_size_is_power_of_two():
    """Verify that BLOCK_SIZE is a power of two."""
    block_size = conf.BLOCK_SIZE
    assert block_size > 0 and (block_size & (block_size - 1) == 0)


@mock.patch('conf.socket.gethostbyname')
@mock.patch('conf.util.chown_slurm')
def test_install_slurm_conf_load_balancer(mock_chown, mock_gethostbyname, tmp_path):
    # Test with enable_controller_load_balancer = True
    cfg = TstCfg(
        output_dir=str(tmp_path),
    )
    cfg.enable_controller_load_balancer = True
    cfg.slurm_control_host = "controller-0"
    cfg.slurm_control_addr = "1.2.3.4"
    cfg.slurm_conf_tpl = "{slurmctld_hosts}"
    cfg.ompi_version = None
    cfg.controller_network_attachment = False
    cfg.enable_slurm_auth = False
    cfg.accounting_storage_backup_host = None
    cfg.slurm_backup_controller_name = None
    cfg.slurm_backup_controller_ip = None
    cfg.slurm_control_host_port = "6820-6830"
    lkp = util.Lookup(cfg)
    
    conf.install_slurm_conf(lkp)
    
    # Since enable_controller_load_balancer is True, socket.gethostbyname should NOT be called for control_host
    mock_gethostbyname.assert_not_called()
    
    conf_file = tmp_path / "slurm.conf"
    assert conf_file.exists()
    content = conf_file.read_text()
    assert "SlurmctldHost=controller-0(controller-0)" in content


@mock.patch('conf.socket.gethostbyname')
@mock.patch('conf.util.chown_slurm')
def test_install_slurm_conf_no_load_balancer(mock_chown, mock_gethostbyname, tmp_path):
    # Test with enable_controller_load_balancer = False
    mock_gethostbyname.return_value = "5.6.7.8"
    cfg = TstCfg(
        output_dir=str(tmp_path),
    )
    cfg.enable_controller_load_balancer = False
    cfg.slurm_control_host = "controller-0"
    cfg.slurm_control_addr = "1.2.3.4"
    cfg.slurm_conf_tpl = "{slurmctld_hosts}"
    cfg.ompi_version = None
    cfg.controller_network_attachment = False
    cfg.enable_slurm_auth = False
    cfg.accounting_storage_backup_host = None
    cfg.slurm_backup_controller_name = None
    cfg.slurm_backup_controller_ip = None
    cfg.slurm_control_host_port = "6820-6830"
    lkp = util.Lookup(cfg)
    
    conf.install_slurm_conf(lkp)
    
    mock_gethostbyname.assert_called_once_with("controller-0")
    
    conf_file = tmp_path / "slurm.conf"
    assert conf_file.exists()
    content = conf_file.read_text()
    assert "SlurmctldHost=controller-0(5.6.7.8)" in content

@mock.patch("util.Lookup.slurm_version", new_callable=mock.PropertyMock)
def test_v2605_generator_tpu_vs_cpu(mock_slurm_version, tmp_path):
    import yaml
    import conf_v2605
    from common import TstPartition
    from util import MachineType

    mock_slurm_version.return_value = "26.05"

    def _mk_tpl(machine_name: str):
        return mock.Mock(
            machine_type=MachineType(
                name=machine_name, guest_cpus=0, memory_mb=0, accelerators=[]
            ),
            gpu=None,
        )

    def _mk_part(name: str, nodesets: list[str]) -> TstPartition:
        p = TstPartition(name, partition_nodeset=nodesets)
        p.partition_nodeset_dyn = []  # type: ignore[attr-defined]
        p.partition_feature = None  # type: ignore[attr-defined]
        p.partition_conf = {}  # type: ignore[attr-defined]
        return p

    # 1. CPU-only cluster on 26.05 -> No TPU plugins or GresTypes=tpu
    cfg_cpu = TstCfg(
        install_dir="ukulele",
        nodeset={
            "cpu": TstNodeset("cpu", instance_template="tpl-cpu", node_count_static=2)
        },
    )
    lkp_cpu = util.Lookup(cfg_cpu)
    lkp_cpu.template_info = mock.Mock(return_value=_mk_tpl("c2-standard-60"))
    gen_cpu = conf.get_generator(lkp_cpu)
    assert isinstance(gen_cpu, conf_v2605.SlurmConfigGeneratorV2605)
    cpu_conflines = gen_cpu.conflines()
    assert "GresTypes=tpu" not in cpu_conflines
    assert "JobSubmitPlugins=tpu" not in cpu_conflines
    assert "CliFilterPlugins=tpu" not in cpu_conflines

    # 2. TPU cluster on 26.05 -> Includes GresTypes=tpu, JobSubmitPlugins=tpu, CliFilterPlugins=tpu, gres, and topology
    ns_v6e_static = TstNodeset(
        "v6es",
        instance_template="tpl-v6e",
        node_count_static=4,
        node_count_dynamic_max=0,
        accelerator_topology="2x4",
    )
    ns_7x_dyn = TstNodeset(
        "tpu7xd",
        instance_template="tpl-7x",
        node_count_static=0,
        node_count_dynamic_max=4,
        accelerator_topology=None,
    )
    cfg_tpu = TstCfg(
        install_dir="ukulele",
        output_dir=str(tmp_path),
        nodeset={"v6es": ns_v6e_static, "tpu7xd": ns_7x_dyn},
        partitions={
            "p_static": _mk_part("p_static", ["v6es"]),
            "p_dyn": _mk_part("p_dyn", ["tpu7xd"]),
        },
    )
    lkp_tpu = util.Lookup(cfg_tpu)
    lkp_tpu.template_info = mock.Mock(
        side_effect=lambda tpl: _mk_tpl(
            "ct6e-standard-4t" if tpl == "tpl-v6e" else "tpu7x-standard-4t"
        )
    )
    lkp_tpu.template_machine_conf = mock.Mock(  # type: ignore[method-assign]
        return_value=TstMachineConf(
            cpus=4,
            memory=1024,
            sockets=1,
            sockets_per_board=1,
            boards=1,
            threads_per_core=1,
            cores_per_socket=4,
        )
    )
    lkp_tpu.instances = mock.Mock(return_value={})

    gen_tpu = conf.get_generator(lkp_tpu)
    tpu_conflines = gen_tpu.conflines()
    assert "GresTypes=tpu" in tpu_conflines
    assert "JobSubmitPlugins=tpu" in tpu_conflines
    assert "CliFilterPlugins=tpu" in tpu_conflines

    cloud_conf = gen_tpu.make_cloud_conf()
    assert "Gres=tpu:v6e:4" in cloud_conf
    assert "Feature=v6e_2x4" in cloud_conf
    assert "Gres=tpu:tpu7x:4" in cloud_conf
    assert "Feature=tpu_dynamic" in cloud_conf
    assert "Oversubscribe=Exclusive" in cloud_conf
    assert f"Topology={conf_v2605.TOPOLOGY_TPU_BLOCK}" in cloud_conf
    assert "Topology=topology/tree" in cloud_conf
    assert "PowerDownOnIdle=YES" in cloud_conf

    # Gres conf lines
    gres_lines = gen_tpu.gen_cloud_gres_conf_lines()
    assert "Name=tpu Type=v6e File=/dev/vfio/devices/vfio[0-3]" in gres_lines
    assert "Name=tpu Type=tpu7x File=/dev/vfio/devices/vfio[0-3]" in gres_lines

    # Topology YAML v2605: TOPOLOGY_TPU_BLOCK for static, topology/tree for dynamic
    gen_tpu.generate_topology_data()
    topo_sections = yaml.safe_load((tmp_path / "cloud_topology.yaml").read_text())
    topo_by_name = {entry["topology"]: entry for entry in topo_sections}
    assert conf_v2605.TOPOLOGY_TPU_BLOCK in topo_by_name
    assert topo_by_name[conf_v2605.TOPOLOGY_TPU_BLOCK]["block"]["block_sizes"] == [1]
