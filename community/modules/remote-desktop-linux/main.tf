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
  resource_prefix = var.name_prefix != null ? var.name_prefix : "${var.deployment_name}-chrome-remote-desktop"

  /*
  #
  # if a machine type is a2-*-?g it will automatically fill in the guest_accelerator structure.
  #
  is_a2_vm = length(regexall("a2-[a-z]+-\\d+g", var.machine_type)) > 0
  accelerator_types = {
    "highgpu"  = "nvidia-tesla-a100"
    "megagpu"  = "nvidia-tesla-a100"
    "ultragpu" = "nvidia-a100-80gb"
  }
  guest_accelerator = var.guest_accelerator == null && local.is_a2_vm ? [{
    type  = lookup(local.accelerator_types, regex("a2-([A-Za-z]+)-", var.machine_type)[0], ""),
    count = one(regex("a2-[A-Za-z]+-(\\d+)", var.machine_type)),
  }] : var.guest_accelerator

  gpu_count = length(local.guest_accelerator) > 0 ? 0 : local.guest_accelerator[0].count

*/
  user_startup_script_runners = var.startup_script == null ? [] : [
    {
      type        = "shell"
      content     = var.startup_script
      destination = "user_startup_script.sh"
    }
  ]

  ssh_args = join("", [
    "-e host_name_prefix=${local.resource_prefix}"
  ])

  configure_ssh_runners = [
    {
      type        = "data"
      source      = "${path.module}/scripts/setup-ssh-keys.sh"
      destination = "/usr/local/ghpc/setup-ssh-keys.sh"
    },
    {
      type        = "data"
      source      = "${path.module}/scripts/setup-ssh-keys.yml"
      destination = "/usr/local/ghpc/setup-ssh-keys.yml"
    },
    {
      type        = "ansible-local"
      content     = file("${path.module}/scripts/configure-ssh.yml")
      destination = "configure-ssh.yml"
      args        = local.ssh_args
    }
  ]
  # todo change this to driver install script
  configure_nvidia_driver_runners = var.install_nvidia_driver == false ? [] : [
    {
      type        = "shell"
      content     = file("${path.module}/scripts/configure-grid-drivers.sh")
      destination = "/usr/local/ghpc/configure-grid-drivers.yml"
    }
  ]
  # todo change this to chrome install script & merge with xfce install script
  configure_chrome_remote_desktop_runners = var.configure_chrome_remote_desktop == false ? [] : [
    {
      type        = "shell"
      content     = file("${path.module}/scripts/configure-chrome-desktop.sh")
      destination = "/usr/local/ghpc/configure-chrome-desktop.yml"
    }
  ]

  driver     = { install-nvidia-driver = var.install_nvidia_driver }
  logging    = var.enable_google_logging ? { google-logging-enable = 1 } : { google-logging-enable = 0 }
  monitoring = var.enable_google_monitoring ? { google-monitoring-enable = 1 } : { google-monitoring-enable = 0 }
  shutdown   = { shutdown-script = "/opt/deeplearning/bin/shutdown_script.sh" }
  metadata   = merge(local.driver, local.logging, local.monitoring, local.shutdown, var.metadata)
}

module "client_startup_script" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=e889ede"

  deployment_name = var.deployment_name
  project_id      = var.project_id
  region          = var.region
  labels          = var.labels

  runners = flatten([
    local.user_startup_script_runners, local.configure_ssh_runners, local.configure_nvidia_driver_runners, local.configure_chrome_remote_desktop_runners
  ])
}

module "instances" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/compute/vm-instance?ref=264e99c"

  instance_count = var.instance_count
  spot           = var.spot

  deployment_name = var.deployment_name
  name_prefix     = local.resource_prefix
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
  labels          = var.labels

  machine_type    = var.machine_type
  service_account = var.service_account
  metadata        = local.metadata
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

  guest_accelerator   = var.guest_accelerator
  on_host_maintenance = var.on_host_maintenance

  network_storage = var.network_storage

}
