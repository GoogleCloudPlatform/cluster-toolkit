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

locals {
  partition = {
    default               = var.is_default
    enable_job_exclusive  = var.exclusive
    network_storage       = var.network_storage
    partition_conf        = var.partition_conf
    partition_name        = var.name
    partition_nodeset     = [for x in var.nodeset : x.nodeset_name]
    partition_nodeset_dyn = [] # TODO: add
    partition_nodeset_tpu = [] # TODO: add
    resume_timeout        = var.resume_timeout
    suspend_time          = var.suspend_time
    suspend_timeout       = var.suspend_timeout
  }
}

