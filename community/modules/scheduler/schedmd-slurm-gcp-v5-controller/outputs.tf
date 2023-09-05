/**
 * Copyright 2022 Google LLC
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

output "controller_instance_id" {
  description = "The server-assigned unique identifier of the controller compute instance."
  value       = one(module.slurm_controller_instance.slurm_controller_instance.instances_details[*].id)
}

output "cloud_logging_filter" {
  description = "Cloud Logging filter to cluster errors."
  value       = module.slurm_controller_instance.cloud_logging_filter
}

output "pubsub_topic" {
  description = "Cluster Pub/Sub topic."
  value       = module.slurm_controller_instance.pubsub_topic
}
