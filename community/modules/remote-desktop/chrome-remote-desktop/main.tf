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

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "chrome-remote-desktop", ghpc_role = "remote-desktop" })
}

locals {

  user_startup_script_runners = var.startup_script == null ? [] : [
    {
      type        = "shell"
      content     = var.startup_script
      destination = "user_startup_script.sh"
    }
  ]

  configure_nvidia_driver_runners = var.install_nvidia_driver == false ? [] : [
    {
      type        = "ansible-local"
      content     = file("${path.module}/scripts/configure-grid-drivers.yml")
      destination = "/usr/local/ghpc/configure-grid-drivers.yml"
    }
  ]

  configure_chrome_remote_desktop_runners = [
    {
      type        = "ansible-local"
      content     = file("${path.module}/scripts/configure-chrome-desktop.yml")
      destination = "/usr/local/ghpc/configure-chrome-desktop.yml"
    }
  ]

  disable_sleep = [
    {
      type        = "ansible-local"
      content     = file("${path.module}/scripts/disable-sleep.yml")
      destination = "/usr/local/ghpc/disable-sleep.yml"
    }
  ]
}

module "client_startup_script" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=50644b2"

  deployment_name = var.deployment_name
  project_id      = var.project_id
  region          = var.region
  labels          = local.labels

  runners = flatten([
    local.user_startup_script_runners,
    local.configure_nvidia_driver_runners,
    local.configure_chrome_remote_desktop_runners,
    local.disable_sleep
  ])
}

module "instances" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/compute/vm-instance?ref=bb47067"

  instance_count                    = var.instance_count
  name_prefix                       = var.name_prefix
  add_deployment_name_before_prefix = var.add_deployment_name_before_prefix
  spot                              = var.spot

  deployment_name = var.deployment_name
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
  labels          = local.labels

  machine_type    = var.machine_type
  service_account = var.service_account
  metadata        = var.metadata
  startup_script  = module.client_startup_script.startup_script
  enable_oslogin  = var.enable_oslogin

  instance_image        = var.instance_image
  disk_size_gb          = var.disk_size_gb
  disk_type             = var.disk_type
  auto_delete_boot_disk = var.auto_delete_boot_disk

  disable_public_ips   = !var.enable_public_ips
  network_self_link    = var.network_self_link
  subnetwork_self_link = var.subnetwork_self_link
  network_interfaces   = var.network_interfaces
  bandwidth_tier       = var.bandwidth_tier
  tags                 = var.tags

  threads_per_core    = var.threads_per_core
  guest_accelerator   = var.guest_accelerator
  on_host_maintenance = var.on_host_maintenance

  network_storage = var.network_storage

}
