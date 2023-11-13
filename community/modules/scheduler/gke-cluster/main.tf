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
  labels = merge(var.labels, { ghpc_module = "gke-cluster", ghpc_role = "scheduler" })
}

locals {
  dash             = var.prefix_with_deployment_name && var.name_suffix != "" ? "-" : ""
  prefix           = var.prefix_with_deployment_name ? var.deployment_name : ""
  name_maybe_empty = "${local.prefix}${local.dash}${var.name_suffix}"
  name             = local.name_maybe_empty != "" ? local.name_maybe_empty : "NO-NAME-GIVEN"

  cluster_authenticator_security_group = var.authenticator_security_group == null ? [] : [{
    security_group = var.authenticator_security_group
  }]

  sa_email = var.service_account_email != null ? var.service_account_email : data.google_compute_default_service_account.default_sa.email
}

data "google_compute_default_service_account" "default_sa" {
  project = var.project_id
}

resource "google_container_cluster" "gke_cluster" {
  provider = google-beta

  project         = var.project_id
  name            = local.name
  location        = var.region
  resource_labels = local.labels

  # decouple node pool lifecyle from cluster life cycle
  remove_default_node_pool = true
  initial_node_count       = 1 # must be set when remove_default_node_pool is set

  network    = var.network_id
  subnetwork = var.subnetwork_self_link

  # Note: the existence of the "master_authorized_networks_config" block enables
  # the master authorized networks even if it's empty.
  master_authorized_networks_config {
    dynamic "cidr_blocks" {
      for_each = var.master_authorized_networks
      content {
        cidr_block   = cidr_blocks.value.cidr_block
        display_name = cidr_blocks.value.display_name
      }
    }
  }

  private_ipv6_google_access = var.enable_private_ipv6_google_access ? "PRIVATE_IPV6_GOOGLE_ACCESS_TO_GOOGLE" : null

  master_auth {
    client_certificate_config {
      issue_client_certificate = false
    }
  }

  enable_shielded_nodes = true

  cluster_autoscaling {
    # Controls auto provisioning of node-pools
    enabled = false

    # Controls autoscaling algorithm of node-pools
    autoscaling_profile = var.autoscaling_profile
  }

  datapath_provider = var.enable_dataplane_v2 ? "ADVANCED_DATAPATH" : "LEGACY_DATAPATH"

  network_policy {
    # Enabling NetworkPolicy for clusters with DatapathProvider=ADVANCED_DATAPATH
    # is not allowed. Dataplane V2 will take care of network policy enforcement
    # instead.
    enabled = false
    # GKE Dataplane V2 support. This must be set to PROVIDER_UNSPECIFIED in
    # order to let the datapath_provider take effect.
    # https://github.com/terraform-google-modules/terraform-google-kubernetes-engine/issues/656#issuecomment-720398658
    provider = "PROVIDER_UNSPECIFIED"
  }

  private_cluster_config {
    enable_private_nodes    = var.enable_private_nodes
    enable_private_endpoint = var.enable_private_endpoint
    master_ipv4_cidr_block  = var.master_ipv4_cidr_block
    master_global_access_config {
      enabled = var.enable_master_global_access
    }
  }

  ip_allocation_policy {
    cluster_secondary_range_name  = var.pods_ip_range_name
    services_secondary_range_name = var.services_ip_range_name
  }

  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  dynamic "authenticator_groups_config" {
    for_each = local.cluster_authenticator_security_group
    content {
      security_group = authenticator_groups_config.value.security_group
    }
  }

  release_channel {
    channel = var.release_channel
  }
  min_master_version = var.min_master_version

  maintenance_policy {
    daily_maintenance_window {
      start_time = var.maintenance_start_time
    }

    dynamic "maintenance_exclusion" {
      for_each = var.maintenance_exclusions
      content {
        exclusion_name = maintenance_exclusion.value.name
        start_time     = maintenance_exclusion.value.start_time
        end_time       = maintenance_exclusion.value.end_time
        exclusion_options {
          scope = maintenance_exclusion.value.exclusion_scope
        }
      }
    }
  }

  addons_config {
    gcp_filestore_csi_driver_config {
      enabled = var.enable_filestore_csi
    }
    gcs_fuse_csi_driver_config {
      enabled = var.enable_gcsfuse_csi
    }
    gce_persistent_disk_csi_driver_config {
      enabled = var.enable_persistent_disk_csi
    }
  }

  timeouts {
    create = var.timeout_create
    update = var.timeout_update
  }

  lifecycle {
    # Ignore all changes to the default node pool. It's being removed after creation.
    ignore_changes = [
      node_config
    ]
  }

  logging_service    = "logging.googleapis.com/kubernetes"
  monitoring_service = "monitoring.googleapis.com/kubernetes"
}

# We define explict node pools, so that it can be modified without
# having to destroy the entire cluster.
resource "google_container_node_pool" "system_node_pools" {
  provider = google-beta
  count    = var.system_node_pool_enabled ? 1 : 0

  project = var.project_id
  name    = var.system_node_pool_name
  cluster = google_container_cluster.gke_cluster.self_link
  autoscaling {
    total_min_node_count = var.system_node_pool_node_count.total_min_nodes
    total_max_node_count = var.system_node_pool_node_count.total_max_nodes
  }

  upgrade_settings {
    max_surge       = 1
    max_unavailable = 0
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }

  node_config {
    resource_labels = local.labels
    service_account = var.service_account_email
    oauth_scopes    = var.service_account_scopes
    machine_type    = var.system_node_pool_machine_type
    taint           = var.system_node_pool_taints

    # Forcing the use of the Container-optimized image, as it is the only
    # image with the proper logging deamon installed.
    #
    # cos images use Shielded VMs since v1.13.6-gke.0.
    # https://cloud.google.com/kubernetes-engine/docs/how-to/node-images
    #
    # We use COS_CONTAINERD to be compatible with (optional) gVisor.
    # https://cloud.google.com/kubernetes-engine/docs/how-to/sandbox-pods
    image_type = "COS_CONTAINERD"

    shielded_instance_config {
      enable_secure_boot          = true
      enable_integrity_monitoring = true
    }

    gvnic {
      enabled = true
    }

    # Implied by Workload Identity
    workload_metadata_config {
      mode = "GKE_METADATA"
    }
    # Implied by workload identity.
    metadata = {
      "disable-legacy-endpoints" = "true"
    }
  }

  lifecycle {
    ignore_changes = [
      node_config[0].labels,
      node_config[0].taint,
    ]
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

data "google_client_config" "default" {}

provider "kubernetes" {
  host                   = "https://${google_container_cluster.gke_cluster.endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.gke_cluster.master_auth[0].cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
}

module "workload_identity" {
  count   = var.configure_workload_identity_sa ? 1 : 0
  source  = "terraform-google-modules/kubernetes-engine/google//modules/workload-identity"
  version = "29.0.0"

  use_existing_gcp_sa = true
  name                = "workload-identity-k8-sa"
  gcp_sa_name         = local.sa_email
  project_id          = var.project_id
  roles               = var.enable_gcsfuse_csi ? ["roles/storage.admin"] : []

  # https://github.com/terraform-google-modules/terraform-google-kubernetes-engine/issues/1059
  depends_on = [
    data.google_compute_default_service_account.default_sa,
    google_container_cluster.gke_cluster
  ]
}
