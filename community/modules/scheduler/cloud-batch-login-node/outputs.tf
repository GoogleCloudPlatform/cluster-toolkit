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
  Use the following commands to:

  SSH into the login node:
    gcloud compute ssh --zone ${google_compute_instance_from_template.batch_login.zone} ${google_compute_instance_from_template.batch_login.name}  --project ${google_compute_instance_from_template.batch_login.project}
  
  Submit your job from login node:
    gcloud ${var.gcloud_version} batch jobs submit ${var.job_id} --config=${var.batch_job_directory}/${var.job_filename} --location=${var.region} --project=${var.project_id}
  
  Check status:
    gcloud ${var.gcloud_version} batch jobs describe ${var.job_id} --location=${var.region} --project=${var.project_id} | grep state:
  
  Delete job:
    gcloud ${var.gcloud_version} batch jobs delete ${var.job_id} --location=${var.region} --project=${var.project_id}

  List all jobs in region:
    gcloud ${var.gcloud_version} batch jobs list ${var.region} --project=${var.project_id}
  EOT
}
