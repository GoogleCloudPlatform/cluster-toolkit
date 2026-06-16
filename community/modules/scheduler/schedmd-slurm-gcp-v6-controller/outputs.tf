# Copyright 2026 "Google LLC"
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

output "slurm_cluster_name" {
  description = "Slurm cluster name."
  value       = local.slurm_cluster_name
}

output "slurm_controller_instance" {
  description = "Compute instance of controller node"
  value       = var.enable_backup_controller ? null : google_compute_instance_from_template.controller[0]
}

output "slurm_login_instances" {
  description = "Compute instances of login nodes"
  value       = flatten([for k, v in module.login : v.instances])
}

output "slurm_bucket_path" {
  description = "Bucket path used by cluster."
  value       = module.slurm_files.slurm_bucket_path
}

output "slurm_bucket_name" {
  description = "GCS Bucket name of Slurm cluster file storage."
  value       = module.slurm_files.bucket_name
}

output "slurm_bucket" {
  description = "GCS Bucket of Slurm cluster file storage."
  value       = module.bucket
}

output "slurm_bucket_dir" {
  description = "Path directory within `bucket_name` for Slurm cluster file storage."
  value       = module.slurm_files.bucket_dir
}

output "munge_deprecation_warning" {
  description = "Deprecation warning for legacy MUNGE authentication."
  value       = var.enable_slurm_auth ? null : "WARNING: Support for MUNGE-based authentication is DEPRECATING and scheduled for complete removal on July 31, 2026. Please plan to migrate to Slurm Native Authentication by setting enable_slurm_auth: true. See docs/slurm-native-auth-migration-guide.md for the destroy-and-recreate migration steps."
}

output "instructions" {
  description = "Post deployment instructions."
  value       = <<-EOT
    ${var.enable_slurm_auth ? "" : "DEPRECATION NOTICE: Support for MUNGE-based authentication is DEPRECATING and scheduled for complete removal on July 31, 2026. Please migrate to Slurm Native Authentication by setting enable_slurm_auth: true.\n"}
    To SSH to the controller (may need to add '--tunnel-through-iap'):
      gcloud compute ssh ${var.enable_backup_controller ? "projects/${local.controller_project_id}/zones/${var.zone}/instances/${local.slurm_cluster_name}-controller-0" : google_compute_instance_from_template.controller[0].self_link}
    
    If you are using cloud ops agent with this deployment,
    you can use the following command to see the logs for the entire cluster or any particular VM host:
      gcloud logging read labels.cluster_name=${local.slurm_cluster_name}
      gcloud logging read labels.hostname=${local.slurm_cluster_name}-controller
  EOT
}
