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

output "name" {
  description = "Names of instances created"
  value       = google_compute_instance.compute_vm[*].name
}

output "self_link" {
  description = "The tuple URIs of the created instances"
  value       = google_compute_instance.compute_vm[*].self_link
}

output "external_ip" {
  description = "External IP of the instances (if enabled)"
  value       = try(google_compute_instance.compute_vm[*].network_interface[0].access_config[0].nat_ip, [])
}

output "internal_ip" {
  description = "Internal IP of the instances"
  value       = google_compute_instance.compute_vm[*].network_interface[0].network_ip
}

locals {
  first_instance_link = try(google_compute_instance.compute_vm[0].self_link, "no-instance")
  ssh_instructions    = <<-EOT
    Use the following commands to SSH into the first VM created:
      gcloud compute ssh ${local.first_instance_link} --project ${var.project_id}
    If not accessible from the public internet, use an SSH tunnel through IAP:
      gcloud compute ssh ${local.first_instance_link} --tunnel-through-iap --project ${var.project_id}
  EOT
}

output "instructions" {
  description = "Instructions on how to SSH into the created VM. Commands may fail depending on VM configuration and IAM permissions."
  value       = var.instance_count > 0 ? local.ssh_instructions : "No instances were created."
}
