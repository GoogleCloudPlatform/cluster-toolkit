/**
  * Copyright 2023 Google LLC
  *
  * Licensed under the Apache License, Version 2.0 (the "License");
  * you may not use this file except in compliance with the License.
  * You may obtain a copy of the License at
  *
  *      http://www.apache.org/licenses/LICENSE-2.0
  *
  * Unless required by applicable law or agreed to in writing, software
  * distributed under the License is distributed on an "AS IS" BASIS,
  * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  * See the License for the specific language governing permissions and
  * limitations under the License.
  */

output "node_pool_name" {
  description = "Name of the node pool."
  value       = google_container_node_pool.node_pool.name
}

locals {
  is_single_shared_core = contains(["g1", "f1"], local.machine_family) # note GKE does not support f1 machines
  is_double_shared_core = local.machine_family == "e2" && !local.machine_not_shared_core
  is_a_series           = local.machine_family == "a2"
  last_digit            = try(local.machine_vals[2], 0)

  vcpu        = local.is_single_shared_core ? 1 : local.is_double_shared_core ? 2 : local.is_a_series ? local.last_digit * 12 : local.last_digit
  useable_cpu = local.set_threads_per_core ? local.threads_per_core * local.vcpu / 2 : local.vcpu
}

output "cpu_per_node" {
  description = "Number of CPUs available"
  value       = local.useable_cpu
}

locals {
  translate_toleration = {
    PREFER_NO_SCHEDULE = "PreferNoSchedule"
    NO_SCHEDULE        = "NoSchedule"
    NO_EXECUTE         = "NoExecute"
  }
  taints = google_container_node_pool.node_pool.node_config[0].taint
  tolerations = [for taint in local.taints : {
    key      = taint.key
    operator = "Equal"
    value    = taint.value
    effect   = lookup(local.translate_toleration, taint.effect, null)
  }]
}

output "tolerations" {
  description = "value"
  value       = local.tolerations
}
