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

output "subnetwork_self_link_a4high-slurm-net-0" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.a4high-slurm-net-0.subnetwork_self_link
  sensitive   = true
}

output "subnetwork_self_link_a4high-slurm-net-1" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.a4high-slurm-net-1.subnetwork_self_link
  sensitive   = true
}

output "subnetwork_interfaces_a4high-slurm-rdma-net" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.a4high-slurm-rdma-net.subnetwork_interfaces
  sensitive   = true
}

output "network_storage_homefs" {
  description = "Generated output from module 'homefs'"
  value       = module.homefs.network_storage
}

output "network_storage_gcs_bucket" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.gcs_bucket.network_storage
  sensitive   = true
}

output "client_install_runner_gcs_checkpoints" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.gcs_checkpoints.client_install_runner
  sensitive   = true
}

output "mount_runner_gcs_checkpoints" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.gcs_checkpoints.mount_runner
  sensitive   = true
}

output "client_install_runner_gcs_training_data" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.gcs_training_data.client_install_runner
  sensitive   = true
}

output "mount_runner_gcs_training_data" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.gcs_training_data.mount_runner
  sensitive   = true
}

output "client_install_runner_gcs_model_serving" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.gcs_model_serving.client_install_runner
  sensitive   = true
}

output "mount_runner_gcs_model_serving" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.gcs_model_serving.mount_runner
  sensitive   = true
}
