# Copyright 2026 Google LLC
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
  labels = merge(var.labels, { ghpc_module = "schedmd-slurm-gcp-v6-nodeset", ghpc_role = "compute" })
}

module "instance_validation" {
  source = "../../../../modules/internal/instance_validations"

  machine_type = var.machine_type
  disk_type    = var.disk_type
}

module "gpu" {
  source = "../../../../modules/internal/gpu-definition"

  machine_type      = var.machine_type
  guest_accelerator = var.guest_accelerator
  machine_configs   = var.machine_configs
}

locals {
  has_flex_policy = var.instance_flexibility_policy != null && length(try(var.instance_flexibility_policy.instance_selections, [])) > 0

  flex_machine_types = !local.has_flex_policy ? toset([]) : toset(flatten([
    for s in var.instance_flexibility_policy.instance_selections : tolist(s.machine_types)
  ]))

  instance_flexibility_policy = !local.has_flex_policy ? null : {
    instance_selections = concat(
      contains(local.flex_machine_types, var.machine_type) ? [] : [{
        name          = "primary-default"
        rank          = 0
        machine_types = toset([var.machine_type])
      }],
      [
        for idx, s in var.instance_flexibility_policy.instance_selections : {
          name          = (s.name != null && trimspace(s.name) != "") ? trimspace(s.name) : "selection-${idx + 1}"
          rank          = s.rank
          machine_types = s.machine_types
        }
      ]
    )
  }
}

module "fallback_instance_validation" {
  source   = "../../../../modules/internal/instance_validations"
  for_each = local.flex_machine_types

  machine_type = each.value
  disk_type    = var.disk_type
}

module "fallback_gpu" {
  source   = "../../../../modules/internal/gpu-definition"
  for_each = local.flex_machine_types

  machine_type      = each.value
  guest_accelerator = startswith(each.value, "n1-") ? var.guest_accelerator : []
  machine_configs   = var.machine_configs
}

locals {
  guest_accelerator = module.gpu.guest_accelerator
  inferred_gpu_signature = {
    for mt in setunion([var.machine_type], local.flex_machine_types) : mt => (
      mt == var.machine_type && length(local.guest_accelerator) > 0 ? "${replace(local.guest_accelerator[0].type, "/^.*\\//", "")}:${local.guest_accelerator[0].count}" :
      contains(keys(module.fallback_gpu), mt) && length(module.fallback_gpu[mt].guest_accelerator) > 0 ? "${replace(module.fallback_gpu[mt].guest_accelerator[0].type, "/^.*\\//", "")}:${module.fallback_gpu[mt].guest_accelerator[0].count}" :
      can(regex("^a2-(highgpu|megagpu)-", mt)) ? "nvidia-tesla-a100:${try(tonumber(regex("-([0-9]+)g$", mt)[0]), 1)}" :
      can(regex("^a2-ultragpu-", mt)) ? "nvidia-a100-80gb:${try(tonumber(regex("-([0-9]+)g$", mt)[0]), 1)}" :
      can(regex("^a3-(highgpu|edgegpu)-", mt)) ? "nvidia-h100-80gb:${try(tonumber(regex("-([0-9]+)g$", mt)[0]), 1)}" :
      can(regex("^a3-megagpu-", mt)) ? "nvidia-h100-mega-80gb:${try(tonumber(regex("-([0-9]+)g$", mt)[0]), 1)}" :
      can(regex("^(a3-ultragpu|a4|a4x)-", mt)) ? "${join("-", slice(split("-", mt), 0, 2))}:${try(tonumber(regex("-([0-9]+)g$", mt)[0]), 1)}" :
      can(regex("^g2-standard-", mt)) ? "nvidia-l4:${lookup({ "24" = 2, "48" = 4, "96" = 8 }, split("-", mt)[2], 1)}" :
      can(regex("^g4-standard-", mt)) ? "nvidia-rtx-pro-6000:${lookup({ "96" = 2, "192" = 4, "384" = 8 }, split("-", mt)[2], 1)}" :
      "none:0"
    )
  }
  # GPUs per VM: attached accelerators, else the "-Ng" suffix in the machine type name.
  # The literal fallback never sizes a slice MIG; outputs.tf requires a determinable count there.
  gpu_count = coalesce(
    try(local.guest_accelerator[0].count, null),
    try(tonumber(regex("-([0-9]+)g", var.machine_type)[0]), null),
    4
  )

  disable_automatic_updates_metadata = var.allow_automatic_updates ? {} : { google_disable_automatic_updates = "TRUE" }

  metadata = merge(
    local.disable_automatic_updates_metadata,
    var.metadata
  )

  name = substr(replace(var.name, "/[^a-z0-9]/", ""), 0, 14)

  additional_disks = [
    for ad in var.additional_disks : {
      disk_name                           = ad.disk_name
      device_name                         = ad.device_name
      disk_type                           = ad.disk_type
      disk_storage_pool                   = ad.disk_storage_pool
      disk_size_gb                        = ad.disk_size_gb
      disk_labels                         = merge(ad.disk_labels, local.labels)
      auto_delete                         = ad.auto_delete
      boot                                = ad.boot
      disk_resource_manager_tags          = ad.disk_resource_manager_tags
      disk_encryption_key                 = ad.disk_encryption_key
      disk_encryption_key_service_account = ad.disk_encryption_key_service_account
    }
  ]

  public_access_config = var.enable_public_ips ? [{ nat_ip = null, network_tier = null }] : []
  access_config        = length(var.access_config) == 0 ? local.public_access_config : var.access_config

  service_account = {
    email  = var.service_account_email
    scopes = var.service_account_scopes
  }

  ghpc_startup_script = [{
    filename = "ghpc_nodeset_startup.sh"
    content  = var.startup_script
  }]

  termination_action = (var.dws_flex.enabled && !var.dws_flex.use_bulk_insert) ? "DELETE" : try(var.spot_instance_config.termination_action, null)

  nodeset = {
    node_count_static      = var.node_count_static
    node_count_dynamic_max = var.node_count_dynamic_max
    node_conf              = var.node_conf
    nodeset_name           = local.name
    dws_flex               = var.dws_flex
    provisioning_engine    = var.provisioning_engine

    disk_auto_delete           = var.disk_auto_delete
    disk_labels                = merge(local.labels, var.disk_labels)
    disk_size_gb               = var.disk_size_gb
    disk_type                  = var.disk_type
    disk_storage_pool          = var.disk_storage_pool
    disk_resource_manager_tags = var.disk_resource_manager_tags
    additional_disks           = local.additional_disks

    disk_encryption_key                 = var.disk_encryption_key
    disk_encryption_key_service_account = var.disk_encryption_key_service_account

    bandwidth_tier = var.bandwidth_tier
    can_ip_forward = var.can_ip_forward

    enable_confidential_vm     = var.enable_confidential_vm
    confidential_instance_type = var.confidential_instance_type
    enable_placement           = var.enable_placement
    placement_max_distance     = var.placement_max_distance
    enable_oslogin             = var.enable_oslogin
    enable_shielded_vm         = var.enable_shielded_vm
    gpu                        = one(local.guest_accelerator)
    gpu_count                  = local.gpu_count
    # Normalize once: util.py has_block_topology() compares against "1x72" exactly.
    accelerator_topology = var.accelerator_topology == null ? null : lower(trimspace(var.accelerator_topology))

    labels                    = local.labels
    machine_type              = var.machine_type
    advanced_machine_features = var.advanced_machine_features
    metadata                  = local.metadata
    min_cpu_platform          = var.min_cpu_platform

    on_host_maintenance      = var.on_host_maintenance
    preemptible              = var.preemptible
    region                   = var.region
    resource_manager_tags    = var.resource_manager_tags
    service_account          = local.service_account
    shielded_instance_config = var.shielded_instance_config
    source_image_family      = local.source_image_family             # requires source_image_logic.tf
    source_image_project     = local.source_image_project_normalized # requires source_image_logic.tf
    source_image             = local.source_image                    # requires source_image_logic.tf
    subnetwork_self_link     = var.subnetwork_self_link
    additional_networks      = var.additional_networks
    access_config            = local.access_config
    tags                     = var.tags
    spot                     = var.enable_spot_vm
    termination_action       = local.termination_action
    reservation_name         = local.reservation_name
    future_reservation       = local.future_reservation
    maintenance_interval     = var.maintenance_interval
    instance_properties_json = jsonencode(var.instance_properties)

    zone_target_shape = local.zone_target_shape
    zone_policy_allow = local.zones_allow
    zone_policy_deny  = local.zones_deny

    instance_flexibility_policy = local.instance_flexibility_policy

    startup_script  = local.ghpc_startup_script
    network_storage = var.network_storage

    enable_maintenance_reservation   = var.enable_maintenance_reservation
    enable_opportunistic_maintenance = var.enable_opportunistic_maintenance
  }
}

locals {
  zones             = setunion(var.zones, [var.zone])
  zone_target_shape = coalesce(var.zone_target_shape, "ANY_SINGLE_ZONE")
  # Static MIG only: DWS Flex MIGs are created by mig_flex.py with ANY_SINGLE_ZONE.
  mig_provisioned = !var.dws_flex.enabled && var.provisioning_engine == "MIG"

  # Expand to all regional zones when a multi-zone target shape is set on a MIG without explicit zones or zonal reservations.
  expand_zones = (
    length(var.zones) == 0 &&
    local.zone_target_shape != "ANY_SINGLE_ZONE" &&
    local.mig_provisioned &&
    var.reservation_name == "" &&
    var.future_reservation == ""
  )

  zones_allow = local.expand_zones ? toset(data.google_compute_zones.available.names) : local.zones
  zones_deny  = setsubtract(data.google_compute_zones.available.names, local.zones_allow)
}

data "google_compute_zones" "available" {
  project = var.project_id
  region  = var.region

  lifecycle {
    postcondition {
      condition     = length(setsubtract(local.zones, self.names)) == 0
      error_message = <<-EOD
      Invalid zones=${jsonencode(setsubtract(local.zones, self.names))}
      Available zones=${jsonencode(self.names)}
      EOD
    }
  }
}

locals {
  res_match = regex("^(?P<whole>(?P<prefix>projects/(?P<project>[a-z0-9-]+)/reservations/)?(?P<name>[a-z0-9-]+)(?P<suffix>/reservationBlocks/[a-z0-9-]+(?:/reservationSubBlocks/[a-z0-9-]+)?)?)?$", var.reservation_name)

  res_short_name = local.res_match.name
  res_project    = coalesce(local.res_match.project, var.project_id)
  res_prefix     = coalesce(local.res_match.prefix, "projects/${local.res_project}/reservations/")
  res_suffix     = local.res_match.suffix == null ? "" : local.res_match.suffix

  reservation_name = local.res_match.whole == null ? "" : "${local.res_prefix}${local.res_short_name}${local.res_suffix}"
}

locals {
  fr_match = regex("^(?P<whole>projects/(?P<project>[a-z0-9-]+)/zones/(?P<zone>[a-z0-9-]+)/futureReservations/)?(?P<name>[a-z0-9-]+)?$", var.future_reservation)

  fr_name    = local.fr_match.name
  fr_project = coalesce(local.fr_match.project, var.project_id)
  fr_zone    = coalesce(local.fr_match.zone, var.zone)

  future_reservation = var.future_reservation == "" ? "" : "projects/${local.fr_project}/zones/${local.fr_zone}/futureReservations/${local.fr_name}"
}


# tflint-ignore: terraform_unused_declarations
data "google_compute_reservation" "reservation" {
  count = length(local.reservation_name) > 0 ? 1 : 0

  name    = local.res_short_name
  project = local.res_project
  zone    = var.zone

  lifecycle {
    postcondition {
      condition     = self.self_link != null
      error_message = "Couldn't find the reservation ${var.reservation_name}"
    }

    postcondition {
      condition     = coalesce(self.specific_reservation_required, true)
      error_message = <<EOT
      your reservation has to be specific,
      see https://cloud.google.com/compute/docs/instances/reservations-overview#how-reservations-work
      for more information. if it's intentionally automatic, don't specify
      it in the blueprint.
      EOT
    }

    # TODO: wait for https://github.com/hashicorp/terraform-provider-google/issues/18248
    # Add a validation that if reservation.project != var.project_id it should be a shared reservation
  }
}
