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

locals {
  provided_instance_tpl_msg  = "The Batch job template uses the existing VM instance template:"
  generated_instance_tpl_msg = "The Batch job template uses a new VM instance template created matching the provided settings:"
}

output "instructions" {
  description = "Instructions for submitting the Batch job."
  value       = <<-EOT

  A Batch job template file has been created locally at:
    ${abspath(local.job_template_output_path)}

  ${var.instance_template == null ? local.generated_instance_tpl_msg : local.provided_instance_tpl_msg}
    ${local.instance_template}

  Use the following commands to:
  Submit your job:
    gcloud ${var.gcloud_version} batch jobs submit ${local.job_id} --config=${abspath(local.job_template_output_path)} --location=${var.region} --project=${var.project_id}
  
  Check status:
    gcloud ${var.gcloud_version} batch jobs describe ${local.job_id} --location=${var.region} --project=${var.project_id} | grep state:
  
  Delete job:
    gcloud ${var.gcloud_version} batch jobs delete ${local.job_id} --location=${var.region} --project=${var.project_id}

  List all jobs:
    gcloud ${var.gcloud_version} batch jobs list --project=${var.project_id}
  EOT
}

output "instance_template" {
  description = "Instance template used by the Batch job."
  value       = local.instance_template
}

output "network_storage" {
  description = "An array of network attached storage mounts used by the Batch job."
  value       = var.network_storage
}

output "startup_script" {
  description = "Startup script run before Google Cloud Batch job starts."
  value       = var.startup_script
}

output "job_template_contents" {
  description = "The generated Batch job template."
  value       = local.job_template_contents
}

output "job_filename" {
  description = "The filename of the generated Batch job template."
  value       = local.job_filename
}

output "job_id" {
  description = "The Batch job id."
  value       = local.job_id
}

output "gcloud_version" {
  description = "The version of gcloud to be used."
  value       = var.gcloud_version
}
