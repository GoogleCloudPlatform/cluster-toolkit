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

output "runners" {
  description = "Runner to install HTCondor using startup-scripts"
  value       = local.runners
}

output "windows_startup_ps1" {
  description = "Windows PowerShell script to install HTCondor"
  value       = local.install_htcondor_ps1
}

output "gcp_service_list" {
  description = "Google Cloud APIs required by HTCondor"
  value       = local.required_apis
}
