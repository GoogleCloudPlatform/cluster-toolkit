/**
 * Copyright 2026 Google LLC
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

output "id" {
  description = "A composite ID ensuring all staged files and directories have finished uploading."
  value = join(",", concat(
    [for d in terraform_data.stage_gcs_directory : d.id],
    [for f in terraform_data.stage_gcs_file : f.id]
  ))
}

output "bucket_name" {
  description = "Name of the target Google Cloud Storage bucket."
  value       = var.bucket_name
}

output "staged_directory_uris" {
  description = "List of destination GCS URIs (gs://...) for staged directories."
  value       = [for d in local.staged_directories : d.destination_uri]
}

output "staged_file_uris" {
  description = "List of destination GCS URIs (gs://...) for staged files."
  value       = [for f in local.staged_files : f.destination_uri]
}

output "all_staged_uris" {
  description = "Combined list of all destination GCS URIs managed by this module."
  value       = distinct(concat([for d in local.staged_directories : d.destination_uri], [for f in local.staged_files : f.destination_uri]))
}
