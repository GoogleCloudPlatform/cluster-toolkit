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

output "nodeset_dyn" {
  description = "Details of the nodeset. Typically used as input to `schedmd-slurm-gcp-v6-partition`."
  value       = local.nodeset
}

output "instance_template_self_link" {
  description = "The URI of the template."
  value       = module.slurm_nodeset_template.self_link
}

output "node_name_prefix" {
  description = <<-EOD
  The prefix to be used for the node names. 
  
  Make sure that nodes are named `<node_name_prefix>-<any_suffix>`
  This temporary required for proper functioning of the nodes.
  While Slurm scheduler uses "features" to bind node and nodeset,
  the SlurmGCP relies on node names for this (to be switched to features as well).
  EOD
  value       = "${var.slurm_cluster_name}-${local.nodeset_name}"

}
