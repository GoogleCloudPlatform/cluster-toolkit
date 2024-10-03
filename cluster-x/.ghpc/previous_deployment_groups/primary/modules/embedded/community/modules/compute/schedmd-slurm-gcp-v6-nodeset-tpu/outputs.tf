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

output "nodeset_tpu" {
  description = "Details of the nodeset tpu. Typically used as input to `schedmd-slurm-gcp-v6-partition`."
  value       = local.nodeset_tpu

  precondition {
    condition     = (var.node_type == null) != (var.accelerator_config == { topology : "", version : "" })
    error_message = "Either a node_type or an accelerator_config must be provided."
  }
}
