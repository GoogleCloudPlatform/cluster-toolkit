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
        
        # Add experimental enable_async_reply feature if requested
        experimental = self.lkp.cfg.get("experimental", {}) or {}
        enable_async_reply = experimental.get("enable_async_reply", False)
        
        if enable_async_reply:
            if "SlurmctldParameters" in conf_options:
                params = conf_options["SlurmctldParameters"]
                if "enable_async_reply" not in params:
                    params.append("enable_async_reply")
            else:
                conf_options["SlurmctldParameters"] = ["enable_async_reply"]
                
        return conf_options


def generate_configs_slurm_v2511(lkp: util.Lookup) -> None:
    SlurmConfigGeneratorV2511(lkp).generate_configs()
