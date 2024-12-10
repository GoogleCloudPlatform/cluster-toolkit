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
  non_static_ns_with_placement = [for ns in var.nodeset : ns.nodeset_name if ns.enable_placement && ns.node_count_static == 0]
  use_static                   = [for ns in concat(var.nodeset, var.nodeset_tpu) : ns.nodeset_name if ns.node_count_static > 0]
  uses_job_duration            = length([for ns in var.nodeset : ns.dws_flex.use_job_duration if ns.dws_flex.use_job_duration]) > 0 ? true : false

  has_node = length(var.nodeset) > 0
  has_dyn  = length(var.nodeset_dyn) > 0
  has_tpu  = length(var.nodeset_tpu) > 0
}

locals {
  partition_conf = merge({
    "Default"        = var.is_default ? "YES" : null
    "ResumeTimeout"  = var.resume_timeout != null ? var.resume_timeout : (local.has_tpu ? 600 : 300)
    "SuspendTime"    = var.suspend_time < 0 ? "INFINITE" : var.suspend_time
    "SuspendTimeout" = var.suspend_timeout != null ? var.suspend_timeout : (local.has_tpu ? 240 : 120)
  }, var.partition_conf)

  partition = {
    partition_name = var.partition_name
    partition_conf = local.partition_conf

    partition_nodeset     = [for ns in var.nodeset : ns.nodeset_name]
    partition_nodeset_tpu = [for ns in var.nodeset_tpu : ns.nodeset_name]
    partition_nodeset_dyn = [for ns in var.nodeset_dyn : ns.nodeset_name]
    # Options
    enable_job_exclusive = var.exclusive
  }
}
