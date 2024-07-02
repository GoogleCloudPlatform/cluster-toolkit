# Copyright 2024 "Google LLC"
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

import subprocess
import logging

# Various plugin utility functions

# Plugin helper function to get plugin settings in the following order:
#
# 1. from job features with
# 2. from slurm-gcp config
# 3. If provided, the default
# 4. None


def get_plugin_setting(plugin, setting, lkp, job, default=None):
    features = get_job_features(job)
    if f"{plugin}.{setting}" in features:
        return features[f"{plugin}.{setting}"]

    if "enable_slurm_gcp_plugins" in lkp.cfg:
        if plugin in lkp.cfg.enable_slurm_gcp_plugins:
            try:
                iter(lkp.cfg.enable_slurm_gcp_plugins[plugin])
            except TypeError:
                # not iterable
                1
            else:
                if setting in lkp.cfg.enable_slurm_gcp_plugins[plugin]:
                    return lkp.cfg.enable_slurm_gcp_plugins[plugin][setting]

    return default


# Plugin helper function to get job features
def get_job_features(job):
    if job is None:
        return {}

    features = {}
    res, output = subprocess.getstatusoutput(f"squeue -h -o %f -j {job}")
    if res == 0:
        for feature in output.split("&"):
            kv = feature.split("=", 1)
            v = None
            if len(kv) == 2:
                v = kv[1]
            features[kv[0]] = v
    else:
        logging.error("Unable to retrieve features of job:{job}")

    return features


__all__ = [
    "get_plugin_setting",
    "get_job_features",
]
