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

output "nat_ips_network0" {
  description = "Generated output from module 'network0'"
  value       = module.network0.nat_ips
}

output "subnetwork_name_network0" {
  description = "Generated output from module 'network0'"
  value       = module.network0.subnetwork_name
}

output "network_id_network0" {
  description = "Automatically-generated output exported for use by later deployment groups"
  value       = module.network0.network_id
  sensitive   = true
}
