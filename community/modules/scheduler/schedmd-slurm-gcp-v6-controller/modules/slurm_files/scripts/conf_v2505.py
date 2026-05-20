#!/slurm/python/venv/bin/python3.13

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

import conf
import util

def conflines(lkp: util.Lookup) -> str:
    return conf.conflines(lkp)

def make_cloud_conf(lkp: util.Lookup) -> str:
    return conf.make_cloud_conf(lkp)

def gen_cloud_conf(lkp: util.Lookup) -> None:
    conf.gen_cloud_conf(lkp)

def generate_configs_slurm_v2505(lkp: util.Lookup) -> None:
    conf.get_generator(lkp).generate_configs()
