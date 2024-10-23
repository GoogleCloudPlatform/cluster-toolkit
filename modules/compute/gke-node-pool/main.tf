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
  labels = merge(var.labels, { ghpc_module = "gke-node-pool", ghpc_role = "compute" })
}

locals {
  sa_email = var.service_account_email != null ? var.service_account_email : data.google_compute_default_service_account.default_sa.email

  preattached_gpu_machine_family = contains(["a2", "a3", "g2"], local.machine_family)
  has_gpu                        = (local.guest_accelerator != null && length(local.guest_accelerator) > 0) || local.preattached_gpu_machine_family
  gpu_taint = local.has_gpu ? [{
    key    = "nvidia.com/gpu"
    value  = "present"
    effect = "NO_SCHEDULE"
  }] : []

  autoscale_set   = var.autoscaling_total_min_nodes != 0 || var.autoscaling_total_max_nodes != 1000
  static_node_set = var.static_node_count != null  
}

data "google_compute_default_service_account" "default_sa" {
  project = var.project_id
}

resource "google_container_node_pool" "node_pool" {
  provider = google-beta

  name           = var.name == null ? var.machine_type : var.name
  cluster        = var.cluster_id
  node_locations = var.zones
  version        = var.node_version

  node_count = var.static_node_count
  dynamic "autoscaling" {
    for_each = local.static_node_set ? [] : [1]
    content {
      total_min_node_count = var.autoscaling_total_min_nodes
      total_max_node_count = var.autoscaling_total_max_nodes
      location_policy      = "ANY"
    }
  }

  management {
    auto_repair  = true
    auto_upgrade = var.auto_upgrade
  }

  upgrade_settings {
    strategy        = "SURGE"
    max_surge       = 0
    max_unavailable = 1
  }

  dynamic "placement_policy" {
    for_each = var.placement_policy.type != null ? [1] : []
    content {
      type        = var.placement_policy.type
      policy_name = var.placement_policy.name
    }
  }

  node_config {
    disk_size_gb    = var.disk_size_gb
    disk_type       = var.disk_type
    resource_labels = local.labels
    labels          = var.kubernetes_labels
    service_account = var.service_account_email
    oauth_scopes    = var.service_account_scopes
    machine_type    = var.machine_type
    spot            = var.spot
    image_type      = var.image_type

    dynamic "guest_accelerator" {
      for_each = local.guest_accelerator
      content {
        type                           = coalesce(guest_accelerator.value.type, try(local.generated_guest_accelerator[0].type, ""))
        count                          = coalesce(try(guest_accelerator.value.count, 0) > 0 ? guest_accelerator.value.count : try(local.generated_guest_accelerator[0].count, "0"))
        gpu_driver_installation_config = coalescelist(try(guest_accelerator.value.gpu_driver_installation_config, []), [{ gpu_driver_version = "DEFAULT" }])
        gpu_partition_size             = try(guest_accelerator.value.gpu_partition_size, "")
        gpu_sharing_config             = try(guest_accelerator.value.gpu_sharing_config, [])
      }
    }

    dynamic "taint" {
      for_each = concat(var.taints, local.gpu_taint)
      content {
        key    = taint.value.key
        value  = taint.value.value
        effect = taint.value.effect
      }
    }

    dynamic "ephemeral_storage_local_ssd_config" {
      for_each = local.local_ssd_config.local_ssd_count_ephemeral_storage != null ? [1] : []
      content {
        local_ssd_count = local.local_ssd_config.local_ssd_count_ephemeral_storage
      }
    }

    dynamic "local_nvme_ssd_block_config" {
      for_each = local.local_ssd_config.local_ssd_count_nvme_block != null ? [1] : []
      content {
        local_ssd_count = local.local_ssd_config.local_ssd_count_nvme_block
      }
    }

    shielded_instance_config {
      enable_secure_boot          = var.enable_secure_boot
      enable_integrity_monitoring = true
    }

    dynamic "gcfs_config" {
      for_each = var.enable_gcfs ? [1] : []
      content {
        enabled = true
      }
    }

    gvnic {
      enabled = var.image_type == "COS_CONTAINERD"
    }

    dynamic "advanced_machine_features" {
      for_each = local.set_threads_per_core ? [1] : []
      content {
        threads_per_core = local.threads_per_core # relies on threads_per_core_calc.tf
      }
    }

    # Implied by Workload Identity
    workload_metadata_config {
      mode = "GKE_METADATA"
    }
    # Implied by workload identity.
    metadata = {
      "disable-legacy-endpoints" = "true"
    }

    linux_node_config {
      sysctls = {
        "net.ipv4.tcp_rmem" = "4096 87380 16777216"
        "net.ipv4.tcp_wmem" = "4096 16384 16777216"
      }
    }

    reservation_affinity {
      consume_reservation_type = var.reservation_affinity.consume_reservation_type
      key                      = length(local.verified_specific_reservations) != 1 ? null : local.reservation_resource_api_label
      values                   = length(local.verified_specific_reservations) != 1 ? null : [for r in local.verified_specific_reservations : "projects/${r.project}/reservations/${r.name}"]
    }

    dynamic "host_maintenance_policy" {
      for_each = var.host_maintenance_interval != "" ? [1] : []
      content {
        maintenance_interval = var.host_maintenance_interval
      }
    }
  }

  network_config {
    dynamic "additional_node_network_configs" {
      for_each = var.additional_networks

      content {
        network    = additional_node_network_configs.value.network
        subnetwork = additional_node_network_configs.value.subnetwork
      }
    }
  }

  timeouts {
    create = var.timeout_create
    update = var.timeout_update
  }

  lifecycle {
    ignore_changes = [
      node_config[0].labels,
    ]
    precondition {
      condition     = !local.static_node_set || !local.autoscale_set
      error_message = "static_node_count cannot be set with either autoscaling_total_min_nodes or autoscaling_total_max_nodes."
    }
    precondition {
      condition     = !(coalesce(local.local_ssd_config.local_ssd_count_ephemeral_storage, 0) > 0 && coalesce(local.local_ssd_config.local_ssd_count_nvme_block, 0) > 0)
      error_message = "Only one of local_ssd_count_ephemeral_storage or local_ssd_count_nvme_block can be set to a non-zero value."
    }
    precondition {
      condition = (
        (var.reservation_affinity.consume_reservation_type != "SPECIFIC_RESERVATION" && local.input_specific_reservations_count == 0) ||
        (var.reservation_affinity.consume_reservation_type == "SPECIFIC_RESERVATION" && local.input_specific_reservations_count == 1)
      )
      error_message = <<-EOT
      When using NO_RESERVATION or ANY_RESERVATION as the `consume_reservation_type`, `specific_reservations` cannot be set.
      On the other hand, with SPECIFIC_RESERVATION you must set `specific_reservations`.
      EOT
    }
    precondition {
      condition = (
        (local.input_specific_reservations_count == 0) ||
        (local.input_specific_reservations_count == 1 && length(local.verified_specific_reservations) > 0 && length(local.specific_reservation_requirement_violations) == 0)
      )
      error_message = <<-EOT
      Check if your reservation is configured correctly:
      1. A reservation with the name must exist in the specified project and one of the specified zones
      2. Its consumption type must be "specific"
      3. Its VM Properties must match with those of the Node Pool; Machine type, Accelerators (GPU Type and count), Local SSD disk type and count
      EOT
    }
  }
}

# For container logs to show up under Cloud Logging and GKE metrics to show up
# on Cloud Monitoring console, some project level roles are needed for the
# node_service_account
resource "google_project_iam_member" "node_service_account_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${local.sa_email}"
}

resource "google_project_iam_member" "node_service_account_metric_writer" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${local.sa_email}"
}

resource "google_project_iam_member" "node_service_account_monitoring_viewer" {
  project = var.project_id
  role    = "roles/monitoring.viewer"
  member  = "serviceAccount:${local.sa_email}"
}

resource "google_project_iam_member" "node_service_account_resource_metadata_writer" {
  project = var.project_id
  role    = "roles/stackdriver.resourceMetadata.writer"
  member  = "serviceAccount:${local.sa_email}"
}

resource "google_project_iam_member" "node_service_account_gcr" {
  project = var.project_id
  role    = "roles/storage.objectViewer"
  member  = "serviceAccount:${local.sa_email}"
}

resource "google_project_iam_member" "node_service_account_artifact_registry" {
  project = var.project_id
  role    = "roles/artifactregistry.reader"
  member  = "serviceAccount:${local.sa_email}"
}

resource "null_resource" "install_dependencies" {
  provisioner "local-exec" {
    command = "pip3 install --break-system-packages pyyaml argparse"
  }
}

locals {
  gpu_direct_setting = lookup(local.gpu_direct_settings, var.machine_type, { gpu_direct_manifests = [], updated_workload_path = "", rxdm_version = "" })
}

# execute script to inject rxdm sidecar into workload to enable tcpx for a3-highgpu-8g VM workload
resource "null_resource" "enable_tcpx_in_workload" {
  count = var.machine_type == "a3-highgpu-8g" ? 1 : 0
  triggers = {
    always_run = timestamp()
  }
  provisioner "local-exec" {
    command = "python3 ${path.module}/gpu-direct-workload/scripts/enable-tcpx-in-workload.py --file ${local.workload_path_tcpx} --rxdm ${local.gpu_direct_setting.rxdm_version}"
  }

  depends_on = [null_resource.install_dependencies]
}

# execute script to inject rxdm sidecar into workload to enable tcpxo for a3-megagpu-8g VM workload
resource "null_resource" "enable_tcpxo_in_workload" {
  count = var.machine_type == "a3-megagpu-8g" ? 1 : 0
  triggers = {
    always_run = timestamp()
  }
  provisioner "local-exec" {
    command = "python3 ${path.module}/gpu-direct-workload/scripts/enable-tcpxo-in-workload.py --file ${local.workload_path_tcpxo} --rxdm ${local.gpu_direct_setting.rxdm_version}"
  }

  depends_on = [null_resource.install_dependencies]
}

# apply manifest to enable tcpx
module "kubectl_apply" {
  source = "../../management/kubectl-apply"

  cluster_id = var.cluster_id
  project_id = var.project_id

  apply_manifests = flatten([
    for manifest in local.gpu_direct_setting.gpu_direct_manifests : [
      {
        source = manifest
      }
    ]
  ])
}
