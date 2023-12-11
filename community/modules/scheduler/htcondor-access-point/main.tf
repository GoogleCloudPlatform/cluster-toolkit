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
  labels = merge(var.labels, { ghpc_module = "htcondor-access-point", ghpc_role = "scheduler" })
}

locals {
  network_storage_metadata = var.network_storage == null ? {} : { network_storage = jsonencode(var.network_storage) }
  oslogin_api_values = {
    "DISABLE" = "FALSE"
    "ENABLE"  = "TRUE"
  }
  enable_oslogin_metadata = var.enable_oslogin == "INHERIT" ? {} : { enable-oslogin = lookup(local.oslogin_api_values, var.enable_oslogin, "") }
  metadata                = merge(local.network_storage_metadata, local.enable_oslogin_metadata, var.metadata)

  host_count  = var.enable_high_availability ? 2 : 1
  name_prefix = "${var.deployment_name}-ap"

  example_runner = {
    type        = "data"
    destination = "/var/tmp/helloworld.sub"
    content     = <<-EOT
      universe       = vanilla
      executable     = /bin/sleep
      arguments      = 1000
      output         = out.$(ClusterId).$(ProcId)
      error          = err.$(ClusterId).$(ProcId)
      log            = log.$(ClusterId).$(ProcId)
      request_cpus   = 1
      request_memory = 100MB
      queue
    EOT
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
    var.access_point_runner,
    [local.schedd_runner],
    var.autoscaler_runner,
    [local.example_runner]
  )

  ap_config = templatefile("${path.module}/templates/condor_config.tftpl", {
    htcondor_role       = "get_htcondor_submit",
    central_manager_ips = var.central_manager_ips
    spool_dir           = "${var.spool_parent_dir}/spool",
    mig_ids             = var.mig_id,
    default_mig_id      = var.default_mig_id
  })

  ap_object = "gs://${var.htcondor_bucket_name}/${google_storage_bucket_object.ap_config.output_name}"
  schedd_runner = {
    type        = "ansible-local"
    content     = file("${path.module}/files/htcondor_configure.yml")
    destination = "htcondor_configure.yml"
    args = join(" ", [
      "-e htcondor_role=get_htcondor_submit",
      "-e config_object=${local.ap_object}",
      "-e job_queue_ha=${var.enable_high_availability}",
      "-e spool_dir=${var.spool_parent_dir}/spool",
    ])
  }

  access_point_ips  = [data.google_compute_instance.ap.network_interface[0].network_ip]
  access_point_name = data.google_compute_instance.ap.name

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

data "google_compute_region_instance_group" "ap" {
  self_link = time_sleep.mig_warmup.triggers.self_link
  lifecycle {
    postcondition {
      condition     = length(self.instances) == local.host_count
      error_message = "There should be ${local.host_count} access points found"
    }
  }
}

data "google_compute_instance" "ap" {
  self_link = data.google_compute_region_instance_group.ap.instances[0].instance
}

resource "google_storage_bucket_object" "ap_config" {
  name    = "${local.name_prefix}-config-${substr(md5(local.ap_config), 0, 4)}"
  content = local.ap_config
  bucket  = var.htcondor_bucket_name

  lifecycle {
    precondition {
      condition     = var.default_mig_id == "" || contains(var.mig_id, var.default_mig_id)
      error_message = "If set, var.default_mig_id must be an element in var.mig_id"
    }
  }
}

module "startup_script" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=50644b2"

  project_id      = var.project_id
  region          = var.region
  labels          = local.labels
  deployment_name = var.deployment_name

  runners = local.all_runners
}

module "access_point_instance_template" {
  # tflint-ignore: terraform_module_pinned_source
  source = "github.com/terraform-google-modules/terraform-google-vm//modules/instance_template?ref=84d7959"

  name_prefix = local.name_prefix
  project_id  = var.project_id
  network     = var.network_self_link
  subnetwork  = var.subnetwork_self_link
  service_account = {
    email  = var.access_point_service_account_email
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

module "htcondor_ap" {
  # tflint-ignore: terraform_module_pinned_source
  source = "github.com/terraform-google-modules/terraform-google-vm//modules/mig?ref=aea74d1"

  project_id                       = var.project_id
  region                           = var.region
  distribution_policy_target_shape = var.distribution_policy_target_shape
  distribution_policy_zones        = local.zones
  target_size                      = local.host_count
  hostname                         = local.name_prefix
  instance_template                = module.access_point_instance_template.self_link

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
    is_external    = var.enable_public_ips
  }]
}

resource "time_sleep" "mig_warmup" {
  create_duration = "120s"

  triggers = {
    self_link = module.htcondor_ap.self_link
  }
}
