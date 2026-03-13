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

output "subnetwork_name_network0" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.network0.subnetwork_name
  sensitive   = true
}

output "startup_script_script" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.script.startup_script
  sensitive   = true
}

output "windows_startup_ps1_windows_startup" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.windows_startup.windows_startup_ps1
  sensitive   = true
}
