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

# The desktop runtime, the per-user broker and the VNC installers live in
# internal/desktop-broker. This module is the public interface onto it.
module "desktop_broker" {
  source = "../../internal/desktop-broker"

  install_root = var.install_root

  network_storage = var.network_storage
  startup_script  = var.startup_script
  slurm_auth_mode = var.slurm_auth_mode

  vnc_backend                  = var.vnc_backend
  enable_gpu_acceleration      = var.enable_gpu_acceleration
  session_resolution           = var.session_resolution
  vnc_display_number           = var.vnc_display_number
  max_user_sessions            = var.max_user_sessions
  session_idle_timeout_seconds = var.session_idle_timeout_seconds

  broker_listen_port = var.novnc_listen_port
  novnc_version      = var.novnc_version
  identity_mode      = var.novnc_identity_mode

  secret_project_id    = var.secret_project_id
  proxy_secret         = var.novnc_proxy_secret
  proxy_secret_id      = var.novnc_proxy_secret_id
  proxy_secret_version = var.novnc_proxy_secret_version

  desktop_endpoint_dir  = var.desktop_endpoint_dir
  desktop_endpoint_name = var.desktop_endpoint_name
}
