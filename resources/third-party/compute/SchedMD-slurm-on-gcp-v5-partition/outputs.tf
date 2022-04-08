#
# Copyright 2019 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# output "partition" {
#   description = "The partition structure containing all the set variables"
#   value = {
#     name : var.partition_name
#     machine_type : var.machine_type
#     static_node_count : var.static_node_count
#     max_node_count : var.max_node_count
#     zone : var.zone
#     image : var.image
#     image_hyperthreads : var.image_hyperthreads
#     compute_disk_type : var.compute_disk_type
#     compute_disk_size_gb : var.compute_disk_size_gb
#     compute_labels : var.labels
#     cpu_platform : var.cpu_platform
#     gpu_count : var.gpu_count
#     gpu_type : var.gpu_type
#     network_storage : var.network_storage
#     preemptible_bursting : var.preemptible_bursting
#     vpc_subnet : var.subnetwork_name
#     exclusive : var.exclusive
#     enable_placement : var.enable_placement
#     regional_capacity : var.regional_capacity
#     regional_policy : var.regional_policy
#     instance_template : var.instance_template
#   }
# }

output "partition" {
  description = "Details of a slurm partition"
  value = {
    compute_list = module.slurm_partition.compute_list
    partition    = module.slurm_partition.partition
  }
}
