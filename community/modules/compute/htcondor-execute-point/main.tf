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
  labels = merge(var.labels, { ghpc_module = "htcondor-execute-point", ghpc_role = "compute" })
}

locals {
  zones                    = coalescelist(var.zones, data.google_compute_zones.available.names)
  network_storage_metadata = var.network_storage == null ? {} : { network_storage = jsonencode(var.network_storage) }

  oslogin_api_values = {
    "DISABLE" = "FALSE"
    "ENABLE"  = "TRUE"
  }
  enable_oslogin = var.enable_oslogin == "INHERIT" ? {} : { enable-oslogin = lookup(local.oslogin_api_values, var.enable_oslogin, "") }

  windows_startup_ps1 = join("\n\n", flatten([var.windows_startup_ps1, local.execute_config_windows_startup_ps1]))

  is_windows_image = anytrue([for l in data.google_compute_image.htcondor.licenses : length(regexall("windows-cloud", l)) > 0])
  windows_startup_metadata = local.is_windows_image && local.windows_startup_ps1 != "" ? {
    windows-startup-script-ps1 = local.windows_startup_ps1
  } : {}

  metadata = merge(local.windows_startup_metadata, local.network_storage_metadata, local.enable_oslogin, var.metadata)

  autoscaler_runner = {
    "type"        = "ansible-local"
    "content"     = file("${path.module}/files/htcondor_configure_autoscaler.yml")
    "destination" = "htcondor_configure_autoscaler_${module.mig.instance_group_manager.name}.yml"
    "args" = join(" ", [
      "-e project_id=${var.project_id}",
      "-e region=${var.region}",
      "-e zone=${local.zones[0]}", # this value is required, but ignored by regional MIG autoscaler
      "-e mig_id=${module.mig.instance_group_manager.name}",
      "-e max_size=${var.max_size}",
      "-e min_idle=${var.min_idle}",
    ])
  }

  execute_config = templatefile("${path.module}/templates/condor_config.tftpl", {
    htcondor_role       = "get_htcondor_execute",
    central_manager_ips = var.central_manager_ips,
    guest_accelerator   = local.guest_accelerator,
  })

  execute_object = "gs://${var.htcondor_bucket_name}/${google_storage_bucket_object.execute_config.output_name}"
  execute_runner = {
    type        = "ansible-local"
    content     = file("${path.module}/files/htcondor_configure.yml")
    destination = "htcondor_configure.yml"
    args = join(" ", [
      "-e htcondor_role=get_htcondor_execute",
      "-e config_object=${local.execute_object}",
    ])
  }

  native_fstype = []
  startup_script_network_storage = [
    for ns in var.network_storage :
    ns if !contains(local.native_fstype, ns.fs_type)
  ]
  storage_client_install_runners = [
    for ns in local.startup_script_network_storage :
    ns.client_install_runner if ns.client_install_runner != null
  ]
  mount_runners = [
    for ns in local.startup_script_network_storage :
    ns.mount_runner if ns.mount_runner != null
  ]

  all_runners = concat(
    local.storage_client_install_runners,
    local.mount_runners,
    var.execute_point_runner,
    [local.execute_runner],
  )

  execute_config_windows_startup_ps1 = templatefile(
    "${path.module}/templates/download-condor-config.ps1.tftpl",
    {
      config_object = local.execute_object,
    }
  )

  name_prefix = "${var.deployment_name}-${var.name_prefix}-ep"
}

data "google_compute_image" "htcondor" {
  family  = try(var.instance_image.family, null)
  name    = try(var.instance_image.name, null)
  project = var.instance_image.project

  lifecycle {
    postcondition {
      condition     = self.disk_size_gb <= var.disk_size_gb
      error_message = "var.disk_size_gb must be set to at least the size of the image (${self.disk_size_gb})"
    }
  }
}

data "google_compute_zones" "available" {
  project = var.project_id
  region  = var.region
}

resource "google_storage_bucket_object" "execute_config" {
  name    = "${local.name_prefix}-config-${substr(md5(local.execute_config), 0, 4)}"
  content = local.execute_config
  bucket  = var.htcondor_bucket_name
}

module "startup_script" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=50644b2"

  project_id      = var.project_id
  region          = var.region
  labels          = local.labels
  deployment_name = var.deployment_name

  runners = local.all_runners
}

module "execute_point_instance_template" {
  source  = "terraform-google-modules/vm/google//modules/instance_template"
  version = "~> 8.0"

  name_prefix = local.name_prefix
  project_id  = var.project_id
  network     = var.network_self_link
  subnetwork  = var.subnetwork_self_link
  service_account = {
    email  = var.execute_point_service_account_email
    scopes = var.service_account_scopes
  }
  labels = local.labels

  machine_type   = var.machine_type
  disk_size_gb   = var.disk_size_gb
  gpu            = one(local.guest_accelerator)
  preemptible    = var.spot
  startup_script = local.is_windows_image ? null : module.startup_script.startup_script
  metadata       = local.metadata
  source_image   = data.google_compute_image.htcondor.self_link

  # secure boot
  enable_shielded_vm       = var.enable_shielded_vm
  shielded_instance_config = var.shielded_instance_config
}

module "mig" {
  source                           = "github.com/terraform-google-modules/terraform-google-vm//modules/mig?ref=aea74d1"
  project_id                       = var.project_id
  region                           = var.region
  distribution_policy_target_shape = var.distribution_policy_target_shape
  distribution_policy_zones        = local.zones
  target_size                      = var.target_size
  hostname                         = local.name_prefix
  mig_name                         = local.name_prefix
  instance_template                = module.execute_point_instance_template.self_link

  health_check_name = "health-htcondor-${local.name_prefix}"
  health_check = {
    type                = "tcp"
    initial_delay_sec   = 600
    check_interval_sec  = 20
    healthy_threshold   = 2
    timeout_sec         = 8
    unhealthy_threshold = 3
    response            = ""
    proxy_header        = "NONE"
    port                = 9618
    request             = ""
    request_path        = ""
    host                = ""
    enable_logging      = true
  }

  update_policy = [{
    instance_redistribution_type = "NONE"
    replacement_method           = "SUBSTITUTE"
    max_surge_fixed              = length(local.zones)
    max_unavailable_fixed        = length(local.zones)
    max_surge_percent            = null
    max_unavailable_percent      = null
    min_ready_sec                = 300
    minimal_action               = "REPLACE"
    type                         = "OPPORTUNISTIC"
  }]

}
