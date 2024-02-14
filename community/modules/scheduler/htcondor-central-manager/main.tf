/**
 * Copyright 2023 Google LLC
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
  labels = merge(var.labels, { ghpc_module = "htcondor-central-manager", ghpc_role = "scheduler" })
}

locals {
  network_storage_metadata = var.network_storage == null ? {} : { network_storage = jsonencode(var.network_storage) }
  oslogin_api_values = {
    "DISABLE" = "FALSE"
    "ENABLE"  = "TRUE"
  }
  enable_oslogin_metadata = var.enable_oslogin == "INHERIT" ? {} : { enable-oslogin = lookup(local.oslogin_api_values, var.enable_oslogin, "") }
  metadata                = merge(local.network_storage_metadata, local.enable_oslogin_metadata, var.metadata)

  name_prefix = "${var.deployment_name}-cm"

  cm_config = templatefile("${path.module}/templates/condor_config.tftpl", {})

  cm_object = "gs://${var.htcondor_bucket_name}/${google_storage_bucket_object.cm_config.output_name}"
  schedd_runner = {
    type        = "ansible-local"
    content     = file("${path.module}/files/htcondor_configure.yml")
    destination = "htcondor_configure.yml"
    args = join(" ", [
      "-e config_object=${local.cm_object}",
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
    var.central_manager_runner,
    [local.schedd_runner]
  )

  central_manager_ips  = [data.google_compute_instance.cm.network_interface[0].network_ip]
  central_manager_name = data.google_compute_instance.cm.name

  list_instances_command = "gcloud compute instance-groups list-instances ${data.google_compute_region_instance_group.cm.name} --region ${var.region} --project ${var.project_id}"

  zones = coalescelist(var.zones, data.google_compute_zones.available.names)
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

data "google_compute_region_instance_group" "cm" {
  self_link = time_sleep.mig_warmup.triggers.self_link
  lifecycle {
    postcondition {
      condition     = length(self.instances) == 1
      error_message = "There should only be 1 central manager found"
    }
  }
}

data "google_compute_instance" "cm" {
  self_link = data.google_compute_region_instance_group.cm.instances[0].instance
}

resource "google_storage_bucket_object" "cm_config" {
  name    = "${local.name_prefix}-config-${substr(md5(local.cm_config), 0, 4)}"
  content = local.cm_config
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

module "central_manager_instance_template" {
  # tflint-ignore: terraform_module_pinned_source
  source = "github.com/terraform-google-modules/terraform-google-vm//modules/instance_template?ref=84d7959"

  name_prefix = local.name_prefix
  project_id  = var.project_id
  network     = var.network_self_link
  subnetwork  = var.subnetwork_self_link
  service_account = {
    email  = var.central_manager_service_account_email
    scopes = var.service_account_scopes
  }
  labels = local.labels

  machine_type   = var.machine_type
  disk_size_gb   = var.disk_size_gb
  preemptible    = false
  startup_script = module.startup_script.startup_script
  metadata       = local.metadata
  source_image   = data.google_compute_image.htcondor.self_link

  # secure boot
  enable_shielded_vm       = var.enable_shielded_vm
  shielded_instance_config = var.shielded_instance_config
}

module "htcondor_cm" {
  # tflint-ignore: terraform_module_pinned_source
  source = "github.com/terraform-google-modules/terraform-google-vm//modules/mig?ref=aea74d1"

  project_id                       = var.project_id
  region                           = var.region
  distribution_policy_target_shape = var.distribution_policy_target_shape
  distribution_policy_zones        = local.zones
  target_size                      = 1
  hostname                         = local.name_prefix
  instance_template                = module.central_manager_instance_template.self_link

  health_check_name = "health-${local.name_prefix}"
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

  stateful_ips = [{
    interface_name = "nic0"
    delete_rule    = "ON_PERMANENT_INSTANCE_DELETION"
    is_external    = false
  }]
}

resource "time_sleep" "mig_warmup" {
  create_duration = "120s"

  triggers = {
    self_link = module.htcondor_cm.self_link
  }
}
