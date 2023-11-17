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
  labels = merge(var.labels, { ghpc_module = "vm-instance", ghpc_role = "compute" })
}

locals {
  native_fstype = []
  startup_script = local.startup_from_network_storage != null ? (
  { startup-script = local.startup_from_network_storage }) : {}
  network_storage = var.network_storage != null ? (
  { network_storage = jsonencode(var.network_storage) }) : {}

  prefix_optional_deployment_name = var.name_prefix != null ? var.name_prefix : var.deployment_name
  prefix_always_deployment_name   = var.name_prefix != null ? "${var.deployment_name}-${var.name_prefix}" : var.deployment_name
  resource_prefix                 = var.add_deployment_name_before_prefix ? local.prefix_always_deployment_name : local.prefix_optional_deployment_name

  enable_gvnic  = var.bandwidth_tier != "not_enabled"
  enable_tier_1 = var.bandwidth_tier == "tier_1_enabled"

  # use Spot provisioning model (now GA) over older preemptible model
  provisioning_model = var.spot ? "SPOT" : null

  # compact_placement : true when placement policy is provided and collocation set; false if unset
  compact_placement = try(var.placement_policy.collocation, null) != null

  gpu_attached = contains(["a2", "g2"], local.machine_family) || length(local.guest_accelerator) > 0

  # both of these must be false if either compact placement or preemptible/spot instances are used
  # automatic restart is tolerant of GPUs while on host maintenance is not
  automatic_restart_default   = local.compact_placement || var.spot ? false : null
  on_host_maintenance_default = local.compact_placement || var.spot || local.gpu_attached ? "TERMINATE" : "MIGRATE"

  automatic_restart = (
    var.automatic_restart != null
    ? var.automatic_restart
    : local.automatic_restart_default
  )

  on_host_maintenance = (
    var.on_host_maintenance != null
    ? var.on_host_maintenance
    : local.on_host_maintenance_default
  )

  oslogin_api_values = {
    "DISABLE" = "FALSE"
    "ENABLE"  = "TRUE"
  }
  enable_oslogin = var.enable_oslogin == "INHERIT" ? {} : { enable-oslogin = lookup(local.oslogin_api_values, var.enable_oslogin, "") }

  # Network Interfaces
  # Support for `use` input and base network paramters like `network_self_link` and `subnetwork_self_link`
  empty_access_config = {
    nat_ip                 = null,
    public_ptr_domain_name = null,
    network_tier           = null
  }
  default_network_interface = {
    network            = var.network_self_link
    subnetwork         = var.subnetwork_self_link
    subnetwork_project = null # will populate from subnetwork_self_link
    network_ip         = null
    nic_type           = local.enable_gvnic ? "GVNIC" : null
    stack_type         = null
    queue_count        = null
    access_config      = var.disable_public_ips ? [] : [local.empty_access_config]
    ipv6_access_config = []
    alias_ip_range     = []
  }
  network_interfaces = coalescelist(var.network_interfaces, [local.default_network_interface])
}

data "google_compute_image" "compute_image" {
  family  = try(var.instance_image.family, null)
  name    = try(var.instance_image.name, null)
  project = try(var.instance_image.project, null)
}

resource "null_resource" "image" {
  triggers = {
    name    = try(var.instance_image.name, null),
    family  = try(var.instance_image.family, null),
    project = try(var.instance_image.project, null)
  }
}

resource "google_compute_disk" "boot_disk" {
  project = var.project_id

  count = var.instance_count

  name   = "${local.resource_prefix}-boot-disk-${count.index}"
  image  = data.google_compute_image.compute_image.self_link
  type   = var.disk_type
  size   = var.disk_size_gb
  labels = local.labels
  zone   = var.zone

  lifecycle {
    replace_triggered_by = [null_resource.image]

    ignore_changes = [
      image
    ]
  }
}

resource "google_compute_resource_policy" "placement_policy" {
  project  = var.project_id
  provider = google-beta

  count = var.placement_policy != null ? 1 : 0
  name  = "${local.resource_prefix}-vm-instance-placement"
  group_placement_policy {
    vm_count                  = try(var.placement_policy.vm_count, null)
    availability_domain_count = try(var.placement_policy.availability_domain_count, null)
    collocation               = try(var.placement_policy.collocation, null)
    max_distance              = try(var.placement_policy.max_distance, null)
  }
}

resource "null_resource" "replace_vm_trigger_from_placement" {
  triggers = {
    vm_count                  = try(tostring(var.placement_policy.vm_count), "")
    availability_domain_count = try(tostring(var.placement_policy.availability_domain_count), "")
    max_distance              = try(tostring(var.placement_policy.max_distance), "")
    collocation               = try(var.placement_policy.collocation, "")
  }
}

resource "google_compute_instance" "compute_vm" {
  project  = var.project_id
  provider = google-beta

  count = var.instance_count

  depends_on = [var.network_self_link, var.network_storage]

  name         = "${local.resource_prefix}-${count.index}"
  machine_type = var.machine_type
  zone         = var.zone

  resource_policies = google_compute_resource_policy.placement_policy[*].self_link

  tags   = var.tags
  labels = local.labels

  boot_disk {
    source      = google_compute_disk.boot_disk[count.index].self_link
    device_name = google_compute_disk.boot_disk[count.index].name
    auto_delete = var.auto_delete_boot_disk
  }

  dynamic "scratch_disk" {
    for_each = range(var.local_ssd_count)
    content {
      interface = var.local_ssd_interface
    }
  }

  dynamic "network_interface" {
    for_each = local.network_interfaces

    content {
      network            = network_interface.value.network
      subnetwork         = network_interface.value.subnetwork
      subnetwork_project = network_interface.value.subnetwork_project
      network_ip         = network_interface.value.network_ip
      nic_type           = network_interface.value.nic_type
      stack_type         = network_interface.value.stack_type
      queue_count        = network_interface.value.queue_count
      dynamic "access_config" {
        for_each = network_interface.value.access_config
        content {
          nat_ip                 = access_config.value.nat_ip
          public_ptr_domain_name = access_config.value.public_ptr_domain_name
          network_tier           = access_config.value.network_tier
        }
      }
      dynamic "ipv6_access_config" {
        for_each = network_interface.value.ipv6_access_config
        content {
          public_ptr_domain_name = ipv6_access_config.value.public_ptr_domain_name
          network_tier           = ipv6_access_config.value.network_tier
        }
      }
      dynamic "alias_ip_range" {
        for_each = network_interface.value.alias_ip_range
        content {
          ip_cidr_range         = alias_ip_range.value.ip_cidr_range
          subnetwork_range_name = alias_ip_range.value.subnetwork_range_name
        }
      }
    }
  }

  network_performance_config {
    total_egress_bandwidth_tier = local.enable_tier_1 ? "TIER_1" : "DEFAULT"
  }

  dynamic "service_account" {
    for_each = var.service_account == null ? [] : [var.service_account]
    content {
      email  = lookup(service_account.value, "email", null)
      scopes = lookup(service_account.value, "scopes", null)
    }
  }

  guest_accelerator = local.guest_accelerator
  scheduling {
    on_host_maintenance = local.on_host_maintenance
    automatic_restart   = local.automatic_restart
    preemptible         = var.spot
    provisioning_model  = local.provisioning_model
  }

  dynamic "advanced_machine_features" {
    for_each = local.set_threads_per_core ? [1] : []
    content {
      threads_per_core = local.threads_per_core # relies on threads_per_core_calc.tf
    }
  }

  metadata = merge(local.network_storage, local.startup_script, local.enable_oslogin, var.metadata)

  lifecycle {
    ignore_changes = [
      metadata["ssh-keys"],
    ]

    replace_triggered_by = [
      null_resource.replace_vm_trigger_from_placement
    ]

    precondition {
      condition     = (length(var.network_interfaces) == 0) != (var.network_self_link == null && var.subnetwork_self_link == null)
      error_message = "Exactly one of network_interfaces or network_self_link/subnetwork_self_link must be specified."
    }
    precondition {
      condition     = alltrue([for interface in var.network_interfaces : interface.network_ip == null]) || var.instance_count == 1
      error_message = <<-EOT
                        The network_ip cannot be statically set on vm-instance when the VM instance_count is greater than 1.
                        Either set the network_ip to null to allow it to be set dynamically for all instances, or create modules for each VM instance with its own network interface.
                        EOT
    }
    precondition {
      condition = !contains([
        "c3-:pd-standard",
        "h3-:pd-standard",
        "h3-:pd-ssd",
      ], "${substr(var.machine_type, 0, 3)}:${var.disk_type}")
      error_message = "A disk_type=${var.disk_type} cannot be used with machine_type=${var.machine_type}."
    }
  }
}
