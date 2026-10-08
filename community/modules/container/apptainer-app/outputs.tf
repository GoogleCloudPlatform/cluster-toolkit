# Copyright 2026 Google LLC
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

output "install_runner" {
  description = "Shell runner that optionally installs Apptainer when install_apptainer is true."
  value       = local.install_runner
}

output "auth_runner" {
  description = "Shell runner that authenticates Apptainer to Google Artifact Registry using gcloud."
  value       = local.auth_runner
}

output "stage_runner" {
  description = "Shell runner that pulls the configured Artifact Registry image into a SIF file."
  value       = local.stage_runner
}

output "wrapper_runner" {
  description = "Shell runner that writes the generated application wrapper script."
  value       = local.wrapper_runner
}

output "modulefile_runner" {
  description = "Shell runner that writes the generated Tcl modulefile."
  value       = local.modulefile_runner
}

output "manifest_runner" {
  description = "Shell runner that writes the generated application manifest."
  value       = local.manifest_runner
}

output "startup_runners" {
  description = <<-EOT
  Ordered list of runners suitable for direct use with modules/scripts/startup-script.
  The list includes installation, registry authentication, SIF staging, wrapper creation,
  modulefile creation, and manifest creation.
  EOT
  value       = local.startup_runners
}

output "install_root_resolved" {
  description = "Resolved install root, either from install_root or the selected network_storage local_mount."
  value       = local.install_root_resolved
}

output "sif_path" {
  description = "Resolved SIF output path."
  value       = local.sif_path
}

output "wrapper_path" {
  description = "Resolved wrapper output path."
  value       = local.wrapper_path
}

output "modulefile_path" {
  description = "Resolved modulefile output path."
  value       = local.modulefile_path
}

output "manifest_path" {
  description = "Resolved manifest output path."
  value       = local.manifest_path
}
