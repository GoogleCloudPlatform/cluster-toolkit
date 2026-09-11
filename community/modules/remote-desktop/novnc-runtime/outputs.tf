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

output "startup_runners" {
  description = "Startup-script runners required to install and launch the noVNC desktop runtime."
  value       = module.desktop_broker.startup_runners
}

output "novnc_listen_port" {
  description = "Port exposed by the desktop broker for noVNC browser access."
  value       = module.desktop_broker.broker_listen_port
}

output "vnc_backend" {
  description = "VNC backend used by the noVNC desktop runtime."
  value       = module.desktop_broker.vnc_backend
}

output "healthcheck_path" {
  description = "Unauthenticated broker path that responds once the broker is running."
  value       = module.desktop_broker.healthcheck_path
}
