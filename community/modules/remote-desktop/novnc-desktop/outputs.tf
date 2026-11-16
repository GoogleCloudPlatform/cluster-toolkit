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

output "startup_script" {
  description = "Script to load and run all desktop runtime runners."
  value       = module.client_startup_script.startup_script
}

output "instance_name" {
  description = "Name of the first instance created, if any."
  value       = var.instance_count > 0 ? module.instances.name[0] : null
}

output "internal_ip" {
  description = "Internal IP addresses of created desktop instances."
  value       = module.instances.internal_ip
}

output "external_ip" {
  description = "External IP addresses of created desktop instances, if enabled."
  value       = module.instances.external_ip
}

output "broker_socket_path" {
  description = "Unix socket the desktop broker serves on. Forward a local port to it over SSH to reach a desktop."
  value       = module.novnc_runtime.broker_socket_path
}

output "vnc_backend" {
  description = "VNC backend used by the desktop host."
  value       = module.novnc_runtime.vnc_backend
}

output "iap_tunnel_command" {
  description = "SSH command that forwards localhost:6080 to the desktop broker through IAP. Run it, then browse to http://localhost:6080/vnc.html."
  value = var.instance_count > 0 ? join(" ", [
    "gcloud", "compute", "ssh",
    module.instances.name[0],
    "--project", var.project_id,
    "--zone", var.zone,
    "--tunnel-through-iap",
    "--", "-N",
    "-L", "6080:${module.novnc_runtime.broker_socket_path}",
  ]) : null
}
