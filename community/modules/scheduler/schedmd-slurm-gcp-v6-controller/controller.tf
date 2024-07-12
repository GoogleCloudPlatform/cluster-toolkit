# Copyright 2024 Google LLC
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
  controller_labels = merge({
    slurm_cluster_name  = local.slurm_cluster_name
    slurm_instance_role = "controller"
  }, local.labels)

  on_host_maintenance = (
    var.preemptible || var.enable_confidential_vm || length(local.guest_accelerator) > 0
    ? "TERMINATE"
    : var.on_host_maintenance
  )

  nic_type_map = {
    virtio_enabled = "VIRTIO_NET"
    gvnic_enabled  = "GVNIC"
    tier_1_enabled = "GVNIC"
  }
  nic_type                    = lookup(local.nic_type_map, var.bandwidth_tier, null)
  total_egress_bandwidth_tier = var.bandwidth_tier == "tier_1_enabled" ? "TIER_1" : "DEFAULT"

  scratch_disks  = [for d in var.additional_disks : d if d.disk_type == "local-ssd"]
  attached_disks = { for d in var.additional_disks : d.disk_name => d if d.disk_type != "local-ssd" }
}

resource "google_compute_disk" "attached_disk" {
  for_each = local.attached_disks

  project = var.project_id
  name    = each.value.disk_name
  size    = each.value.disk_size_gb
  type    = each.value.disk_type
  zone    = var.zone
  labels  = merge(local.controller_labels, each.value.disk_labels)
}

resource "google_compute_instance" "controller" {
  project          = var.project_id
  zone             = var.zone
  name             = "${local.slurm_cluster_name}-controller"
  machine_type     = var.machine_type
  min_cpu_platform = var.min_cpu_platform

  labels = merge(local.files_cs_labels, local.controller_labels)

  metadata = merge(
    var.metadata,
    local.universe_domain,
    {
      enable-oslogin      = var.enable_oslogin ? "TRUE" : "FALSE"
      slurm_bucket_path   = module.slurm_files.slurm_bucket_path
      slurm_cluster_name  = local.slurm_cluster_name
      slurm_instance_role = "controller"
      VmDnsSetting        = "GlobalOnly"
      startup-script      = file("${path.module}/scripts/startup.sh")
  })


  boot_disk {
    auto_delete = var.disk_auto_delete

    initialize_params {
      size   = var.disk_size_gb
      type   = var.disk_type
      image  = "${local.source_image_project_normalized}/${coalesce(local.source_image, local.source_image_family)}"
      labels = merge(local.controller_labels, var.disk_labels)
    }
  }

  dynamic "scratch_disk" {
    for_each = local.scratch_disks
    content {
      interface = "NVME"
    }
  }

  dynamic "attached_disk" {
    for_each = local.attached_disks
    content {
      source      = google_compute_disk.attached_disk[attached_disk.key].self_link
      device_name = attached_disk.value.device_name
    }
  }

  network_interface {
    subnetwork = var.subnetwork_self_link
    network_ip = try(var.static_ips[0], null)
    nic_type   = local.nic_type

    dynamic "access_config" {
      for_each = var.enable_controller_public_ips ? [1] : []
      content {
        nat_ip       = null
        network_tier = null
      }
    }
  }
  network_performance_config {
    total_egress_bandwidth_tier = local.total_egress_bandwidth_tier
  }
  can_ip_forward = var.can_ip_forward

  service_account {
    email  = local.service_account.email
    scopes = local.service_account.scopes
  }

  scheduling {
    preemptible                 = var.preemptible
    provisioning_model          = var.preemptible ? "SPOT" : "STANDARD"
    automatic_restart           = !var.preemptible # yes, unless preemptible
    on_host_maintenance         = local.on_host_maintenance
    instance_termination_action = var.preemptible ? "STOP" : null
  }

  advanced_machine_features {
    enable_nested_virtualization = false
    threads_per_core             = var.enable_smt ? null : 1
  }

  # NOTE: Even if all the shielded_instance_config values are false, 
  # if the config block exists and an unsupported image is chosen,
  # the apply will fail so we use a single-value array with the default value to
  # initialize the block only if it is enabled.
  dynamic "shielded_instance_config" {
    for_each = var.enable_shielded_vm ? [1] : []
    content {
      enable_secure_boot          = var.shielded_instance_config.enable_secure_boot
      enable_vtpm                 = var.shielded_instance_config.enable_vtpm
      enable_integrity_monitoring = var.shielded_instance_config.enable_integrity_monitoring
    }
  }

  dynamic "confidential_instance_config" {
    for_each = var.enable_confidential_vm ? [1] : []
    content {
      enable_confidential_compute = true
    }
  }

  dynamic "guest_accelerator" {
    for_each = local.guest_accelerator
    content {
      type  = guest_accelerator.value.type
      count = guest_accelerator.value.count
    }
  }

  tags = concat([local.slurm_cluster_name], var.tags)

  lifecycle {
    create_before_destroy = "true"
  }

  depends_on = [
    null_resource.cleanup_compute[0], # Ensure that controller is destroyed BEFORE doing cleanup
  ]
}
