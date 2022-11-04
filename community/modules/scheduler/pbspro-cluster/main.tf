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
  server_hostname_prefix    = "${var.deployment_name}-server"
  client_hostname_prefix    = "${var.deployment_name}-client"
  execution_hostname_prefix = "${var.deployment_name}-execution"

  server_hosts = {
    instance_count = tonumber(lookup(var.server_host_settings, "instance_count", 1))
    spot           = tobool(lookup(var.server_host_settings, "spot", false))

    machine_type = tostring(lookup(var.server_host_settings, "machine_type", "c2-standard-8"))
    service_account = lookup(var.server_host_settings, "service_account", {
      email = null
      scopes = [
        "https://www.googleapis.com/auth/devstorage.read_write",
        "https://www.googleapis.com/auth/logging.write",
        "https://www.googleapis.com/auth/monitoring.write",
        "https://www.googleapis.com/auth/servicecontrol",
        "https://www.googleapis.com/auth/service.management.readonly",
        "https://www.googleapis.com/auth/trace.append"
      ]
    })
    metadata       = lookup(var.server_host_settings, "metadata", {})
    startup_script = lookup(var.server_host_settings, "startup_script", null)
    enable_oslogin = lookup(var.server_host_settings, "enable_oslogin", "ENABLE")

    instance_image = lookup(var.server_host_settings, "instance_image", {
      family  = "hpc-centos-7"
      project = "cloud-hpc-image-public"
    })
    disk_size_gb          = tonumber(lookup(var.server_host_settings, "disk_size_gb", 200))
    disk_type             = tostring(lookup(var.server_host_settings, "disk_type", "pd-standard"))
    auto_delete_boot_disk = tobool(lookup(var.server_host_settings, "auto_delete_boot_disk", true))
    local_ssd_count       = tonumber(lookup(var.server_host_settings, "local_ssd_count", 0))
    local_ssd_interface   = tostring(lookup(var.server_host_settings, "local_ssd_interface", "NVME"))

    enable_public_ips    = tobool(lookup(var.server_host_settings, "enable_public_ips", true))
    network_self_link    = tostring(lookup(var.server_host_settings, "network_self_link", var.network_self_link))
    subnetwork_self_link = tostring(lookup(var.server_host_settings, "subnetwork_self_link", var.subnetwork_self_link))
    network_interfaces   = tolist(lookup(var.server_host_settings, "network_interfaces", []))
    bandwidth_tier       = tostring(lookup(var.server_host_settings, "bandwidth_tier", "not_enabled"))
    placement_policy     = lookup(var.server_host_settings, "placement_policy", null)
    tags                 = tolist(lookup(var.server_host_settings, "tags", []))

    guest_accelerator   = lookup(var.server_host_settings, "guest_accelerator", null)
    on_host_maintenance = tostring(lookup(var.server_host_settings, "on_host_maintenance", null))
    threads_per_core    = tonumber(lookup(var.server_host_settings, "threads_per_core", 0))
  }

  client_hosts = {
    instance_count = tonumber(lookup(var.client_host_settings, "instance_count", 1))
    spot           = tobool(lookup(var.client_host_settings, "spot", false))

    machine_type = tostring(lookup(var.client_host_settings, "machine_type", "c2-standard-8"))
    service_account = lookup(var.client_host_settings, "service_account", {
      email = null
      scopes = [
        "https://www.googleapis.com/auth/devstorage.read_write",
        "https://www.googleapis.com/auth/logging.write",
        "https://www.googleapis.com/auth/monitoring.write",
        "https://www.googleapis.com/auth/servicecontrol",
        "https://www.googleapis.com/auth/service.management.readonly",
        "https://www.googleapis.com/auth/trace.append"
      ]
    })
    metadata       = lookup(var.client_host_settings, "metadata", {})
    startup_script = lookup(var.client_host_settings, "startup_script", null)
    enable_oslogin = lookup(var.client_host_settings, "enable_oslogin", "ENABLE")

    instance_image = lookup(var.client_host_settings, "instance_image", {
      family  = "hpc-centos-7"
      project = "cloud-hpc-image-public"
    })
    disk_size_gb          = tonumber(lookup(var.client_host_settings, "disk_size_gb", 200))
    disk_type             = tostring(lookup(var.client_host_settings, "disk_type", "pd-standard"))
    auto_delete_boot_disk = tobool(lookup(var.client_host_settings, "auto_delete_boot_disk", true))
    local_ssd_count       = tonumber(lookup(var.client_host_settings, "local_ssd_count", 0))
    local_ssd_interface   = tostring(lookup(var.client_host_settings, "local_ssd_interface", "NVME"))

    enable_public_ips    = tobool(lookup(var.client_host_settings, "enable_public_ips", true))
    network_self_link    = tostring(lookup(var.client_host_settings, "network_self_link", var.network_self_link))
    subnetwork_self_link = tostring(lookup(var.client_host_settings, "subnetwork_self_link", var.subnetwork_self_link))
    network_interfaces   = tolist(lookup(var.client_host_settings, "network_interfaces", []))
    bandwidth_tier       = tostring(lookup(var.client_host_settings, "bandwidth_tier", "not_enabled"))
    placement_policy     = lookup(var.client_host_settings, "placement_policy", null)
    tags                 = tolist(lookup(var.client_host_settings, "tags", []))

    guest_accelerator   = lookup(var.client_host_settings, "guest_accelerator", null)
    on_host_maintenance = tostring(lookup(var.client_host_settings, "on_host_maintenance", null))
    threads_per_core    = tonumber(lookup(var.client_host_settings, "threads_per_core", 0))
  }

  execution_hosts = {
    instance_count = tonumber(lookup(var.execution_host_settings, "instance_count", 1))
    spot           = tobool(lookup(var.execution_host_settings, "spot", false))

    machine_type = tostring(lookup(var.execution_host_settings, "machine_type", "c2-standard-60"))
    service_account = lookup(var.execution_host_settings, "service_account", {
      email = null
      scopes = [
        "https://www.googleapis.com/auth/devstorage.read_write",
        "https://www.googleapis.com/auth/logging.write",
        "https://www.googleapis.com/auth/monitoring.write",
        "https://www.googleapis.com/auth/servicecontrol",
        "https://www.googleapis.com/auth/service.management.readonly",
        "https://www.googleapis.com/auth/trace.append"
      ]
    })
    metadata       = lookup(var.execution_host_settings, "metadata", {})
    startup_script = lookup(var.execution_host_settings, "startup_script", null)
    enable_oslogin = lookup(var.execution_host_settings, "enable_oslogin", "ENABLE")

    instance_image = lookup(var.execution_host_settings, "instance_image", {
      family  = "hpc-centos-7"
      project = "cloud-hpc-image-public"
    })
    disk_size_gb          = tonumber(lookup(var.execution_host_settings, "disk_size_gb", 200))
    disk_type             = tostring(lookup(var.execution_host_settings, "disk_type", "pd-standard"))
    auto_delete_boot_disk = tobool(lookup(var.execution_host_settings, "auto_delete_boot_disk", true))
    local_ssd_count       = tonumber(lookup(var.execution_host_settings, "local_ssd_count", 0))
    local_ssd_interface   = tostring(lookup(var.execution_host_settings, "local_ssd_interface", "NVME"))

    enable_public_ips    = tobool(lookup(var.execution_host_settings, "enable_public_ips", true))
    network_self_link    = tostring(lookup(var.execution_host_settings, "network_self_link", var.network_self_link))
    subnetwork_self_link = tostring(lookup(var.execution_host_settings, "subnetwork_self_link", var.subnetwork_self_link))
    network_interfaces   = tolist(lookup(var.execution_host_settings, "network_interfaces", []))
    bandwidth_tier       = tostring(lookup(var.execution_host_settings, "bandwidth_tier", "not_enabled"))
    placement_policy     = lookup(var.execution_host_settings, "placement_policy", null)
    tags                 = tolist(lookup(var.execution_host_settings, "tags", []))

    guest_accelerator   = lookup(var.execution_host_settings, "guest_accelerator", null)
    on_host_maintenance = tostring(lookup(var.execution_host_settings, "on_host_maintenance", null))
    threads_per_core    = tonumber(lookup(var.execution_host_settings, "threads_per_core", 0))
  }
}

module "pbs_server" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//community/modules/scheduler/pbspro-server?ref=7206f3b1"

  instance_count = local.server_hosts.instance_count
  spot           = local.server_hosts.spot

  deployment_name = var.deployment_name
  name_prefix     = local.server_hostname_prefix
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
  labels          = var.labels

  machine_type    = local.server_hosts.machine_type
  service_account = local.server_hosts.service_account
  metadata        = local.server_hosts.metadata
  startup_script  = local.server_hosts.startup_script
  enable_oslogin  = local.server_hosts.enable_oslogin

  instance_image        = local.server_hosts.instance_image
  disk_size_gb          = local.server_hosts.disk_size_gb
  disk_type             = local.server_hosts.disk_type
  auto_delete_boot_disk = local.server_hosts.auto_delete_boot_disk
  local_ssd_count       = local.server_hosts.local_ssd_count
  local_ssd_interface   = local.server_hosts.local_ssd_interface

  enable_public_ips    = local.server_hosts.enable_public_ips
  network_self_link    = local.server_hosts.network_self_link
  subnetwork_self_link = local.server_hosts.subnetwork_self_link
  network_interfaces   = local.server_hosts.network_interfaces
  bandwidth_tier       = local.server_hosts.bandwidth_tier
  placement_policy     = local.server_hosts.placement_policy
  tags                 = local.server_hosts.tags

  guest_accelerator   = local.server_hosts.guest_accelerator
  on_host_maintenance = local.server_hosts.on_host_maintenance
  threads_per_core    = local.server_hosts.threads_per_core

  network_storage = var.network_storage

  pbs_data_service_user     = var.pbs_data_service_user
  pbs_exec                  = var.pbs_exec
  pbs_home                  = var.pbs_home
  pbs_license_server        = var.pbs_license_server
  pbs_license_server_port   = var.pbs_license_server_port
  pbs_server_rpm_url        = var.pbs_server_rpm_url
  client_hostname_prefix    = local.client_hostname_prefix
  client_host_count         = local.client_hosts.instance_count
  execution_hostname_prefix = local.execution_hostname_prefix
  execution_host_count      = local.execution_hosts.instance_count
  server_conf               = var.server_conf
}

module "pbs_client" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//community/modules/scheduler/pbspro-client?ref=7206f3b1"

  instance_count = local.client_hosts.instance_count
  spot           = local.client_hosts.spot

  deployment_name = var.deployment_name
  name_prefix     = local.client_hostname_prefix
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
  labels          = var.labels

  machine_type    = local.client_hosts.machine_type
  service_account = local.client_hosts.service_account
  metadata        = local.client_hosts.metadata
  startup_script  = local.client_hosts.startup_script
  enable_oslogin  = local.client_hosts.enable_oslogin

  instance_image        = local.client_hosts.instance_image
  disk_size_gb          = local.client_hosts.disk_size_gb
  disk_type             = local.client_hosts.disk_type
  auto_delete_boot_disk = local.client_hosts.auto_delete_boot_disk
  local_ssd_count       = local.client_hosts.local_ssd_count
  local_ssd_interface   = local.client_hosts.local_ssd_interface

  enable_public_ips    = local.client_hosts.enable_public_ips
  network_self_link    = local.client_hosts.network_self_link
  subnetwork_self_link = local.client_hosts.subnetwork_self_link
  network_interfaces   = local.client_hosts.network_interfaces
  bandwidth_tier       = local.client_hosts.bandwidth_tier
  placement_policy     = local.client_hosts.placement_policy
  tags                 = local.client_hosts.tags

  guest_accelerator   = local.client_hosts.guest_accelerator
  on_host_maintenance = local.client_hosts.on_host_maintenance
  threads_per_core    = local.client_hosts.threads_per_core

  network_storage = var.network_storage

  pbs_exec           = var.pbs_exec
  pbs_home           = var.pbs_home
  pbs_server         = module.pbs_server.pbs_server
  pbs_client_rpm_url = var.pbs_client_rpm_url
}

module "pbs_execution" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//community/modules/compute/pbspro-execution?ref=7206f3b1"

  instance_count = local.execution_hosts.instance_count
  spot           = local.execution_hosts.spot

  deployment_name = var.deployment_name
  name_prefix     = local.execution_hostname_prefix
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
  labels          = var.labels

  machine_type    = local.execution_hosts.machine_type
  service_account = local.execution_hosts.service_account
  metadata        = local.execution_hosts.metadata
  startup_script  = local.execution_hosts.startup_script
  enable_oslogin  = local.execution_hosts.enable_oslogin

  instance_image        = local.execution_hosts.instance_image
  disk_size_gb          = local.execution_hosts.disk_size_gb
  disk_type             = local.execution_hosts.disk_type
  auto_delete_boot_disk = local.execution_hosts.auto_delete_boot_disk
  local_ssd_count       = local.execution_hosts.local_ssd_count
  local_ssd_interface   = local.execution_hosts.local_ssd_interface

  enable_public_ips    = local.execution_hosts.enable_public_ips
  network_self_link    = local.execution_hosts.network_self_link
  subnetwork_self_link = local.execution_hosts.subnetwork_self_link
  network_interfaces   = local.execution_hosts.network_interfaces
  bandwidth_tier       = local.execution_hosts.bandwidth_tier
  placement_policy     = local.execution_hosts.placement_policy
  tags                 = local.execution_hosts.tags

  guest_accelerator   = local.execution_hosts.guest_accelerator
  on_host_maintenance = local.execution_hosts.on_host_maintenance
  threads_per_core    = local.execution_hosts.threads_per_core

  network_storage = var.network_storage

  pbs_exec              = var.pbs_exec
  pbs_home              = var.pbs_home
  pbs_server            = module.pbs_server.pbs_server
  pbs_execution_rpm_url = var.pbs_execution_rpm_url
}
