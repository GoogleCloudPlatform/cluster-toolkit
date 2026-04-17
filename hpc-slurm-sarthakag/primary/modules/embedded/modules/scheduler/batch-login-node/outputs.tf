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

output "login_node_name" {
  description = "Name of the created VM"
  value       = google_compute_instance_from_template.batch_login.name
}

output "instructions" {
  description = "Instructions for accessing the login node and submitting Google Cloud Batch jobs"
  value       = <<-EOT

  Batch job template files will be placed on the Batch login node in the following directory:
    ${var.batch_job_directory}

  Use the following commands to:
  SSH into the login node:
    gcloud compute ssh --zone ${google_compute_instance_from_template.batch_login.zone} ${google_compute_instance_from_template.batch_login.name}  --project ${google_compute_instance_from_template.batch_login.project}

  ${local.list_all_jobs}

  ${local.batch_command_instructions}
  EOT
}
