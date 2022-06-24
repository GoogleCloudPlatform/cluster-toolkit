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

output "instructions" {
  description = "Instructions for submitting Cloud Batch job."
  value       = <<-EOT
  Use the following commands to:
  
  Submit your job:
    gcloud ${local.gcloud_version}batch jobs submit ${var.job_id} --location=${var.region} --config=${abspath(local.job_template_output_path)}
  
  Check status:
    gcloud ${local.gcloud_version}batch jobs describe ${var.job_id} --location=${var.region} | grep state:
  
  Delete job:
    gcloud ${local.gcloud_version}batch jobs delete ${var.job_id} --location=${var.region}

  List all jobs in region:
    gcloud ${local.gcloud_version}batch jobs list ${var.region} | grep ^name:
  EOT
}
