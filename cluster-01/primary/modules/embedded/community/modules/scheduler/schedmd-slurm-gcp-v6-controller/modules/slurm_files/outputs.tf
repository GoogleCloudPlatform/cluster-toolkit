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

output "config" {
  description = "Cluster configuration."
  value       = local.config

  precondition {
    condition     = var.enable_hybrid ? can(coalesce(var.slurm_control_host)) : true
    error_message = "Input slurm_control_host is required."
  }

  precondition {
    condition     = length(local.x_nodeset_overlap) == 0
    error_message = "All nodeset names must be unique among all nodeset types."
  }
}

output "partitions" {
  description = "Cluster partitions."
  value       = lookup(local.config, "partitions", null)
}

output "nodeset" {
  description = "Cluster nodesets."
  value       = lookup(local.config, "nodeset", null)
}

output "nodeset_dyn" {
  description = "Cluster nodesets (dynamic)."
  value       = lookup(local.config, "nodeset_dyn", null)
}

output "nodeset_tpu" {
  description = "Cluster nodesets (TPU)."
  value       = lookup(local.config, "nodeset_tpu", null)
}
