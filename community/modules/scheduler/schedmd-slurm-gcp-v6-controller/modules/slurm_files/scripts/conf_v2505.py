#!/slurm/python/venv/bin/python3.13

# Copyright (C) SchedMD LLC.
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

from pathlib import Path
import logging
import conf
import util
from util import dirs

log = logging.getLogger()

def conflines(lkp: util.Lookup) -> str:
    params = lkp.cfg.cloud_parameters
    def get(key, default):
        """
        Returns the value of the key in params if it exists and is not None,
        otherwise returns supplied default.
        We can't rely on the `dict.get` method because the value could be `None` as
        well as empty NSDict, depending on type of the `cfg.cloud_parameters`.
        TODO: Simplify once NSDict is removed from the codebase.
        """
        if key not in params or params[key] is None:
            return default
        return params[key]

    no_comma_params = get("no_comma_params", False)

    any_gpus = any(
        lkp.template_info(nodeset.instance_template).gpu
        for nodeset in lkp.cfg.nodeset.values()
    )

    any_tpu = any(
        tpu_nodeset is not None
        for part in lkp.cfg.partitions.values()
        for tpu_nodeset in part.partition_nodeset_tpu
    )

    any_gke = any(
        lkp.nodeset_is_gke(nodeset)
        for nodeset in lkp.cfg.nodeset.values()
    )

    any_dynamic = any(bool(p.partition_feature) for p in lkp.cfg.partitions.values())
    comma_params = {
        "LaunchParameters": [
            "enable_nss_slurm",
            "use_interactive_step",
        ],
        "SlurmctldParameters": [
            "cloud_dns" if not(any_dynamic or any_tpu or any_gke) else None,
            "enable_configless",
            "idle_on_node_suspend",
        ],
        "GresTypes": [
            "gpu" if any_gpus else None,
        ],
    }

    scripts_dir = lkp.cfg.install_dir or dirs.scripts
    prolog_path = Path(dirs.custom_scripts / "prolog.d")
    epilog_path = Path(dirs.custom_scripts / "epilog.d")
    task_prolog_path = Path(dirs.custom_scripts / "task_prolog.d")
    task_epilog_path = Path(dirs.custom_scripts / "task_epilog.d")
    default_tree_width = 65533 if any_dynamic else 128

    conf_options = {
        **(comma_params if not no_comma_params else {}),
        "Prolog": f"{prolog_path}/*" if lkp.cfg.prolog_scripts else None,
        "Epilog": f"{epilog_path}/*" if lkp.cfg.epilog_scripts else None,
        "TaskProlog": f"{task_prolog_path}/task-prolog" if lkp.cfg.task_prolog_scripts else None,
        "TaskEpilog": f"{task_epilog_path}/task-epilog" if lkp.cfg.task_epilog_scripts else None,
        "PrologFlags": get("prolog_flags", None),
        "SwitchType": get("switch_type", None),
        "PrivateData": get("private_data", []),
        "SchedulerParameters": get("scheduler_parameters", [
            "bf_continue",
            "salloc_wait_nodes",
            "ignore_prefer_validation",
        ]),
        "ResumeProgram": f"{scripts_dir}/resume_wrapper.sh",
        "ResumeFailProgram": f"{scripts_dir}/suspend_wrapper.sh",
        "ResumeRate": get("resume_rate", 0),
        "ResumeTimeout": get("resume_timeout", 300),
        "SuspendProgram": f"{scripts_dir}/suspend_wrapper.sh",
        "SuspendRate": get("suspend_rate", 0),
        "SuspendTimeout": get("suspend_timeout", 300),
        "SlurmdTimeout": get("slurmd_timeout", 300),
        "UnkillableStepTimeout": get("unkillable_step_timeout", 300),
        "TreeWidth": get("tree_width", default_tree_width),
        "JobSubmitPlugins": "lua" if any_tpu else None,
        "TopologyPlugin": conf.topology_plugin(lkp) if lkp.cfg.nodeset else None,
        "TopologyParam": get("topology_param", "SwitchAsNodeRank"),
    }
    return conf.dict_to_conf(conf_options, delim="\n")

def make_cloud_conf(lkp: util.Lookup) -> str:
    """generate cloud.conf snippet"""
    lines = [
        conf.FILE_PREAMBLE,
        conflines(lkp),
        *(conf.nodeset_lines(n, lkp) for n in lkp.cfg.nodeset.values()),
        *(conf.nodeset_dyn_lines(n) for n in lkp.cfg.nodeset_dyn.values()),
        *(conf.nodeset_tpu_lines(n, lkp) for n in lkp.cfg.nodeset_tpu.values()),
        *(conf.partitionlines(p, lkp) for p in lkp.cfg.partitions.values()),
        *(conf.suspend_exc_lines(lkp)),
    ]
    return "\n\n".join(filter(None, lines))

def gen_cloud_conf(lkp: util.Lookup) -> None:
    content = make_cloud_conf(lkp)

    conf_file = lkp.etc_dir / "cloud.conf"
    conf_file.write_text(content)
    util.chown_slurm(conf_file, mode=0o644)

def generate_configs_slurm_v2505(lkp: util.Lookup) -> None:
    conf.install_slurm_conf(lkp)
    conf.install_slurmdbd_conf(lkp)
    gen_cloud_conf(lkp)
    conf.gen_cloud_gres_conf(lkp)
    conf.install_gres_conf(lkp)
    conf.install_cgroup_conf(lkp)
    conf.install_jobsubmit_lua(lkp)
    if lkp.cfg.nodeset:
        _, summary = conf.gen_topology_yaml(lkp)
        summary.dump(lkp)
        conf.install_topology_yaml(lkp)
