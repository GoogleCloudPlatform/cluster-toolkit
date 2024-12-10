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
  enable_oslogin_metadata            = var.enable_oslogin == "INHERIT" ? {} : { enable-oslogin = lookup(local.oslogin_api_values, var.enable_oslogin, "") }
  disable_automatic_updates_metadata = var.allow_automatic_updates ? {} : { google_disable_automatic_updates = "TRUE" }
  metadata = merge(
    local.network_storage_metadata,
    local.enable_oslogin_metadata,
    local.disable_automatic_updates_metadata,
    var.metadata
  )

  host_count  = 1
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
      "-e spool_dir=${var.spool_parent_dir}/spool",
      "-e htcondor_spool_disk_device=/dev/disk/by-id/google-${local.spool_disk_device_name}",
    ])
  }

  access_point_ips  = [data.google_compute_instance.ap.network_interface[0].network_ip]
  access_point_name = data.google_compute_instance.ap.name

  spool_disk_resource_name = "${var.deployment_name}-spool-disk"
  spool_disk_device_name   = "htcondor-spool-disk"
  spool_disk_source        = try(google_compute_disk.spool[0].name, google_compute_region_disk.spool[0].self_link)

  zones = coalescelist(var.zones, random_shuffle.zones.result)

  vm_family            = split("-", var.machine_type)[0]
  regional_pd_families = ["e2", "n1", "n2", "n2d"]
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
    postcondition {
      # Condition needs to check the suffix of the license, as prefix contains an API version which can change.
      # Example license value: https://www.googleapis.com/compute/v1/projects/cloud-hpc-image-public/global/licenses/hpc-vm-image-feature-disable-auto-updates
      condition     = var.allow_automatic_updates || anytrue([for license in self.licenses : endswith(license, "/projects/cloud-hpc-image-public/global/licenses/hpc-vm-image-feature-disable-auto-updates")])
      error_message = "Disabling automatic updates is not supported with the selected VM image.  More information: https://cloud.google.com/compute/docs/instances/create-hpc-vm#disable_automatic_updates"
    }
  }
}

data "google_compute_zones" "available" {
  project = var.project_id
  region  = var.region

  lifecycle {
    postcondition {
      condition = alltrue([
        for z in var.zones : contains(self.names, z)
      ])
      error_message = "Each entry in var.zones must be a zone in var.region: ${var.region}"
    }
  }
}

resource "random_shuffle" "zones" {
  input        = data.google_compute_zones.available.names
  result_count = var.enable_high_availability ? 2 : 1
}

data "google_compute_region_instance_group" "ap" {
  self_link = module.htcondor_ap.self_link
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

    # by construction, this precondition only fails when the user has set
    # var.zones to a non-empty list of length not equal to 2
    precondition {
      condition     = !var.enable_high_availability || length(local.zones) == 2
      error_message = "When using HTCondor access point high availability, var.zones must be of length 2."
    }
  }
}

module "startup_script" {
  source = "../../../../modules/scripts/startup-script"

  project_id      = var.project_id
  region          = var.region
  labels          = local.labels
  deployment_name = var.deployment_name

  runners = local.all_runners
}

resource "google_compute_region_disk" "spool" {
  count  = var.enable_high_availability ? 1 : 0
  name   = local.spool_disk_resource_name
  labels = local.labels
  type   = var.spool_disk_type
  region = var.region
  size   = var.spool_disk_size_gb

  replica_zones = local.zones

  lifecycle {
    precondition {
      condition     = var.spool_disk_size_gb >= 200
      error_message = "When using HTCondor access point high availability, var.spool_disk_size_gb must be set to 200 or greater."
    }

    precondition {
      condition     = contains(local.regional_pd_families, local.vm_family)
      error_message = "When using HTCondor access point high availability, var.machine_type must be one of ${jsonencode(local.regional_pd_families)}."
    }
  }
}

resource "google_compute_disk" "spool" {
  count  = var.enable_high_availability ? 0 : 1
  name   = local.spool_disk_resource_name
  labels = local.labels
  type   = var.spool_disk_type
  zone   = local.zones[0]
  size   = var.spool_disk_size_gb
}

module "access_point_instance_template" {
  source  = "terraform-google-modules/vm/google//modules/instance_template"
  version = "~> 12.1"

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
  disk_type      = var.disk_type
  preemptible    = false
  startup_script = module.startup_script.startup_script
  metadata       = local.metadata
  source_image   = data.google_compute_image.htcondor.self_link

  # secure boot
  enable_shielded_vm       = var.enable_shielded_vm
  shielded_instance_config = var.shielded_instance_config

  # spool disk
  additional_disks = [
    {
      source      = local.spool_disk_source
      device_name = local.spool_disk_device_name
    }
  ]
}

module "htcondor_ap" {
  source  = "terraform-google-modules/vm/google//modules/mig"
  version = "~> 12.1"

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
    replacement_method           = "RECREATE" # preserves hostnames (necessary for PROACTIVE replacement)
    max_surge_fixed              = 0          # must be 0 to preserve hostnames
    max_unavailable_fixed        = length(local.zones)
    max_surge_percent            = null
    max_unavailable_percent      = null
    min_ready_sec                = 300
    minimal_action               = "REPLACE"
    type                         = var.update_policy
  }]

  stateful_disks = [{
    device_name = local.spool_disk_device_name
    delete_rule = "ON_PERMANENT_INSTANCE_DELETION"
  }]

  stateful_ips = [{
    interface_name = "nic0"
    delete_rule    = "ON_PERMANENT_INSTANCE_DELETION"
    is_external    = var.enable_public_ips
  }]

  # the timeouts below are default for resource
  wait_for_instances = true
  mig_timeouts = {
    create = "15m"
    delete = "15m"
    update = "15m"
  }
}
