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

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "novnc-desktop", ghpc_role = "remote-desktop" })
}

locals {
  remote_desktop_tag = "ghpc-novnc-desktop"
}

module "novnc_runtime" {
  source = "../novnc-runtime"

  network_storage              = var.network_storage
  enable_gpu_acceleration      = var.enable_gpu_acceleration
  desktop_endpoint_dir         = var.desktop_endpoint_dir
  desktop_endpoint_name        = var.desktop_endpoint_name
  install_root                 = var.install_root
  max_user_sessions            = var.max_user_sessions
  novnc_identity_mode          = var.novnc_identity_mode
  novnc_listen_port            = var.novnc_listen_port
  secret_project_id            = var.secret_project_id
  novnc_proxy_secret           = var.novnc_proxy_secret
  novnc_proxy_secret_id        = var.novnc_proxy_secret_id
  novnc_proxy_secret_version   = var.novnc_proxy_secret_version
  novnc_version                = var.novnc_version
  session_idle_timeout_seconds = var.session_idle_timeout_seconds
  session_resolution           = var.session_resolution
  slurm_auth_mode              = var.slurm_auth_mode
  startup_script               = var.startup_script
  vnc_backend                  = var.vnc_backend
  vnc_display_number           = var.vnc_display_number
}

module "instances" {
  source = "../../../../modules/compute/vm-instance"

  instance_count                    = var.instance_count
  name_prefix                       = var.name_prefix
  add_deployment_name_before_prefix = var.add_deployment_name_before_prefix
  provisioning_model                = var.spot ? "SPOT" : null

  deployment_name = var.deployment_name
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
  labels          = local.labels

  machine_type           = var.machine_type
  service_account_email  = var.service_account_email
  service_account_scopes = var.service_account_scopes
  metadata               = var.metadata
  startup_script         = module.client_startup_script.startup_script
  enable_oslogin         = var.enable_oslogin

  instance_image        = var.instance_image
  disk_size_gb          = var.disk_size_gb
  disk_type             = var.disk_type
  auto_delete_boot_disk = var.auto_delete_boot_disk

  disable_public_ips   = !var.enable_public_ips
  network_self_link    = var.network_self_link
  subnetwork_self_link = var.subnetwork_self_link
  network_interfaces   = var.network_interfaces
  bandwidth_tier       = var.bandwidth_tier
  tags                 = distinct(concat(var.tags, [local.remote_desktop_tag]))

  threads_per_core    = var.threads_per_core
  guest_accelerator   = var.guest_accelerator
  on_host_maintenance = var.on_host_maintenance

  network_storage = []
}

module "client_startup_script" {
  source = "../../../../modules/scripts/startup-script"

  deployment_name = var.deployment_name
  project_id      = var.project_id
  region          = var.region
  labels          = local.labels

  runners = module.novnc_runtime.startup_runners
}

resource "google_compute_firewall" "novnc_ingress" {
  count = length(var.allowed_ingress_cidrs) > 0 && length(var.network_interfaces) == 0 ? 1 : 0

  project = var.project_id
  name    = "${substr(var.deployment_name, 0, 40)}-novnc"
  network = var.network_self_link

  direction     = "INGRESS"
  source_ranges = var.allowed_ingress_cidrs
  target_tags   = [local.remote_desktop_tag]

  allow {
    protocol = "tcp"
    ports    = [tostring(var.novnc_listen_port)]
  }
}
