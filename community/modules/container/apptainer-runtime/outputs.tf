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

output "layout_runner" {
  description = "Shell runner that creates the shared Apptainer layout directories."
  value       = local.layout_runner
}

output "module_init_runner" {
  description = "Shell runner that writes the profile snippet used to expose the shared modulefile tree."
  value       = local.module_init_runner
}

output "startup_runners" {
  description = "Ordered list of runners suitable for direct use with modules/scripts/startup-script."
  value       = local.startup_runners
}

output "install_root_resolved" {
  description = "Resolved install root, either from install_root or the selected network_storage local_mount."
  value       = local.install_root_resolved
}

output "sif_dir" {
  description = "Resolved directory where SIF images should be staged."
  value       = local.sif_dir
}

output "bin_dir" {
  description = "Resolved directory where wrapper commands should be written."
  value       = local.bin_dir
}

output "modulefile_dir" {
  description = "Resolved directory where Tcl modulefiles should be written."
  value       = local.modulefile_dir
}

output "manifest_dir" {
  description = "Resolved directory where generated app manifests should be written."
  value       = local.manifest_dir
}
