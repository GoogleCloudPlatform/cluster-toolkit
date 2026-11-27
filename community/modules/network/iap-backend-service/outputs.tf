# Copyright 2026 "Google LLC"
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

output "backend_service_name" {
  description = "Name of the backend service. Route a hostname to it in the load balancer's URL map."
  value       = var.name
  depends_on  = [module.backend]
}

output "backend_service_self_link" {
  description = "Self link of the backend service, as a URL map's path matcher takes it."
  value       = module.backend.backend_service_info[0].backend_service
}

output "network_endpoint_group" {
  description = "Endpoint group the desktop instances are attached to."
  value       = google_compute_network_endpoint_group.desktops.id
}
