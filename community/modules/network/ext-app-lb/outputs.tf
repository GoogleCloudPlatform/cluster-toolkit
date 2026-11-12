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

output "ip_address" {
  description = "The load balancer's external IPv4 address. Point each route's hostname at it."
  value       = module.frontend.external_ip
}

output "url_map" {
  description = "Self link of the load balancer's URL map."
  value       = one(module.frontend.url_map)
}
