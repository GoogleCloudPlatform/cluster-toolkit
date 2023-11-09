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

  use_placement = [for ns in var.partition_conf : ns.nodeset_name if ns.enable_placement]

  partition = {
    default               = var.is_default
    enable_job_exclusive  = var.exclusive
    network_storage       = var.network_storage
    partition_conf        = var.partition_conf
    partition_name        = var.partition_name
    partition_nodeset     = [for ns in var.nodeset : ns.nodeset_name]
    partition_nodeset_tpu = [for ns in var.nodeset_tpu : ns.nodeset_name]
  }
}
