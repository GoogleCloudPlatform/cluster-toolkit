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

output "startup_script" {
  description = "script to load and run all runners, as a string value."
  value       = module.client_startup_script.startup_script
}

output "name" {
  description = "Name of any instance created"
  value       = module.instances[*].name
}


/*
output "compute_startup_script" {
  description = "script to load and run all runners, as a string value. Targets the inputs for the slurm controller."
  value       = module.client_startup_script.compute_startup_script
}

output "controller_startup_script" {
  description = "script to load and run all runners, as a string value. Targets the inputs for the slurm controller."
  value       = module.client_startup_script.controller_startup_script
}
*/
