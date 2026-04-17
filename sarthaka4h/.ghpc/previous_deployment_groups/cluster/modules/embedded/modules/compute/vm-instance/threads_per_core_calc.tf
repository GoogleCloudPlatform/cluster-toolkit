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

# This file is meant to be reused by multiple modules.
# "description": Allows for 'threads_per_core=0: SMT will be disabled where compatible (default)'

# "inputs":
# var.machine_type: Machine type for the instance being evaluated.
# var.threads_per_core : Sets the number of threads per physical core, where 0
#                        has behavior described in description.

# "outputs":
# local.set_threads_per_core: bool that tells if threads per core should be set,
#                             to be used with a dynamic block.
# local.threads_per_core: actual threads_per_core to be used.

locals {
  machine_vals        = split("-", var.machine_type)
  machine_family      = local.machine_vals[0]
  machine_shared_core = length(local.machine_vals) <= 2
  machine_vcpus       = try(parseint(local.machine_vals[2], 10), 1)

  smt_capable_family = !contains(["t2d", "t2a"], local.machine_family)
  smt_capable_vcpu   = local.machine_vcpus >= 2

  smt_capable          = local.smt_capable_family && local.smt_capable_vcpu && !local.machine_shared_core
  set_threads_per_core = var.threads_per_core != null && (var.threads_per_core == 0 && local.smt_capable || try(var.threads_per_core >= 1, false))
  threads_per_core     = var.threads_per_core == 2 ? 2 : 1
}
