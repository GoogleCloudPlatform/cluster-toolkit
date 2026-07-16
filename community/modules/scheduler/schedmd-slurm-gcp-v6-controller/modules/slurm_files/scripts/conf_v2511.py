#!/slurm/python/venv/bin/python3.13

# Copyright (C) SchedMD LLC.
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

from conf import SlurmConfigGenerator
import util

class SlurmConfigGeneratorV2511(SlurmConfigGenerator):
    """Slurm 25.11 configuration generator (experimental & new features)."""

    def get_conf_options(self) -> dict:
        conf_options = super().get_conf_options()
        
        slurmctld_params = []
        
        # Add experimental enable_async_reply feature if requested
        experimental = self.lkp.cfg.experimental or {}
        enable_async_reply = experimental.get("enable_async_reply", False)
        if enable_async_reply:
            slurmctld_params.append("enable_async_reply")
            
        # Enable expedited requeue for high priority workload recovery
        enable_expedited_requeue = self.lkp.cfg.get("enable_expedited_requeue", False)
        if enable_expedited_requeue:
            slurmctld_params.append("enable_expedited_requeue")

        if slurmctld_params and "SlurmctldParameters" in conf_options:
            params = conf_options["SlurmctldParameters"]
            if isinstance(params, list):
                for new_param in slurmctld_params:
                    if new_param not in params:
                        params.append(new_param)

        # Configure Node health checks to execute only at node startup
        enable_health_check_start_only = self.lkp.cfg.get("enable_health_check_start_only", False)
        if enable_health_check_start_only:
            conf_options["HealthCheckNodeState"] = "START_ONLY"

        return conf_options


def generate_configs_slurm_v2511(lkp: util.Lookup) -> None:
    SlurmConfigGeneratorV2511(lkp).generate_configs()
