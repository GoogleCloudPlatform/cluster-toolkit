/**
 * Copyright (C) SchedMD LLC.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

output "slurm_bucket_path" {
  description = "GCS Bucket URI of Slurm cluster file storage."
  value       = local.bucket_path
}

output "bucket_name" {
  description = "GCS Bucket name of Slurm cluster file storage."
  value       = data.google_storage_bucket.this.name
}

output "bucket_dir" {
  description = "Path directory within `bucket_name` for Slurm cluster file storage."
  value       = local.bucket_dir
}

output "config" {
  description = "Cluster configuration."
  value       = local.config

  precondition {
    condition     = var.enable_hybrid ? can(coalesce(var.hybrid_conf.slurm_control_host)) : true
    error_message = "Input slurm_control_host is required in hybrid mode."
  }

  precondition {
    condition     = var.enable_hybrid ? can(coalesce(var.slurm_cluster_name)) : true
    error_message = "Input slurm_cluster_name is required in hybrid mode."
  }

  precondition {
    condition     = length(local.x_nodeset_overlap) == 0
    error_message = "All nodeset names must be unique among all nodeset types."
  }
}
