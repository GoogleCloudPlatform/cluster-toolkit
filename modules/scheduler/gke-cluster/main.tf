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
  upgrade_settings = {
    strategy        = var.upgrade_settings.strategy
    max_surge       = coalesce(var.upgrade_settings.max_surge, 0)
    max_unavailable = coalesce(var.upgrade_settings.max_unavailable, 1)
  }
}

locals {
  dash             = var.prefix_with_deployment_name && var.name_suffix != "" ? "-" : ""
  prefix           = var.prefix_with_deployment_name ? var.deployment_name : ""
  name_maybe_empty = "${local.prefix}${local.dash}${var.name_suffix}"
  name             = local.name_maybe_empty != "" ? local.name_maybe_empty : "NO-NAME-GIVEN"

  cluster_authenticator_security_group = var.authenticator_security_group == null ? [] : [{
    security_group = var.authenticator_security_group
  }]

  default_sa_email = "${data.google_project.project.number}-compute@developer.gserviceaccount.com"
  sa_email         = coalesce(var.service_account_email, local.default_sa_email)

  # additional VPCs enable multi networking 
  derived_enable_multi_networking = coalesce(var.enable_multi_networking, length(var.additional_networks) > 0)

  # multi networking needs enabled Dataplane v2
  derived_enable_dataplane_v2 = coalesce(var.enable_dataplane_v2, local.derived_enable_multi_networking)

  default_monitoring_component = [
    "SYSTEM_COMPONENTS",
    "POD",
    "DAEMONSET",
    "DEPLOYMENT",
    "STATEFULSET",
    "STORAGE",
    "HPA",
    "CADVISOR",
    "KUBELET"
  ]

  default_logging_component = [
    "SYSTEM_COMPONENTS",
    "WORKLOADS"
  ]
}

data "google_project" "project" {
  project_id = var.project_id
}

data "google_container_engine_versions" "version_prefix_filter" {
  provider       = google-beta
  location       = var.cluster_availability_type == "ZONAL" ? var.zone : var.region
  version_prefix = var.version_prefix
}

locals {
  master_version = var.min_master_version != null ? var.min_master_version : data.google_container_engine_versions.version_prefix_filter.latest_master_version
}

resource "google_container_cluster" "gke_cluster" {
  provider = google-beta

  project         = var.project_id
  name            = local.name
  location        = var.cluster_availability_type == "ZONAL" ? var.zone : var.region
  resource_labels = local.labels
  networking_mode = var.networking_mode
  # decouple node pool lifecycle from cluster life cycle
  remove_default_node_pool = true
  initial_node_count       = 1 # must be set when remove_default_node_pool is set
  node_locations           = var.system_node_pool_zones

  deletion_protection = var.deletion_protection

  dynamic "enable_k8s_beta_apis" {
    for_each = var.enable_k8s_beta_apis != null ? [1] : []
    content {
      enabled_apis = var.enable_k8s_beta_apis
    }
  }

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
    gcp_public_cidrs_access_enabled = var.gcp_public_cidrs_access_enabled
  }

  private_ipv6_google_access = var.enable_private_ipv6_google_access ? "PRIVATE_IPV6_GOOGLE_ACCESS_TO_GOOGLE" : null
  default_max_pods_per_node  = var.default_max_pods_per_node
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

  datapath_provider = local.derived_enable_dataplane_v2 ? "ADVANCED_DATAPATH" : "LEGACY_DATAPATH"

  enable_multi_networking = local.derived_enable_multi_networking

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

  dynamic "gateway_api_config" {
    for_each = var.enable_inference_gateway ? [1] : []
    content {
      channel = "CHANNEL_STANDARD"
    }
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
  min_master_version = local.master_version

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

  dynamic "dns_config" {
    for_each = var.cloud_dns_config != null ? [1] : []
    content {
      additive_vpc_scope_dns_domain = var.cloud_dns_config.additive_vpc_scope_dns_domain
      cluster_dns                   = var.cloud_dns_config.cluster_dns
      cluster_dns_scope             = var.cloud_dns_config.cluster_dns_scope
      cluster_dns_domain            = var.cloud_dns_config.cluster_dns_domain
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
    dns_cache_config {
      enabled = var.enable_node_local_dns_cache
    }
    parallelstore_csi_driver_config {
      enabled = var.enable_parallelstore_csi
    }
    ray_operator_config {
      enabled = var.enable_ray_operator
    }
    lustre_csi_driver_config {
      enabled = var.enable_managed_lustre_csi
    }
    dynamic "http_load_balancing" {
      for_each = var.enable_inference_gateway ? [1] : []
      content {
        disabled = false
      }
    }
  }

  timeouts {
    create = var.timeout_create
    update = var.timeout_update
  }

  node_config {
    shielded_instance_config {
      enable_secure_boot          = var.system_node_pool_enable_secure_boot
      enable_integrity_monitoring = true
    }
  }

  control_plane_endpoints_config {
    dns_endpoint_config {
      allow_external_traffic = var.enable_external_dns_endpoint
    }
  }

  lifecycle {
    # Ignore all changes to the default node pool. It's being removed after creation.
    ignore_changes = [
      node_config,
      min_master_version,
    ]
    precondition {
      condition     = var.default_max_pods_per_node == null || var.networking_mode == "VPC_NATIVE"
      error_message = "default_max_pods_per_node does not work on `routes-based` clusters, that don't have IP Aliasing enabled."
    }
    precondition {
      condition     = coalesce(var.enable_dataplane_v2, true) || !local.derived_enable_multi_networking
      error_message = "'enable_dataplane_v2' cannot be false when enabling multi networking."
    }
    precondition {
      condition     = coalesce(var.enable_multi_networking, true) || length(var.additional_networks) == 0
      error_message = "'enable_multi_networking' cannot be false when using multivpc module, which passes additional_networks."
    }
  }

  monitoring_config {
    enable_components = var.enable_dcgm_monitoring ? concat(local.default_monitoring_component, ["DCGM"]) : local.default_monitoring_component
    managed_prometheus {
      enabled = true
    }
  }

  logging_config {
    enable_components = local.default_logging_component
  }
}

# We define explicit node pools, so that it can be modified without
# having to destroy the entire cluster.
resource "google_container_node_pool" "system_node_pools" {
  provider = google-beta
  count    = var.system_node_pool_enabled ? 1 : 0

  project        = var.project_id
  name           = var.system_node_pool_name
  cluster        = var.cluster_reference_type == "NAME" ? google_container_cluster.gke_cluster.name : google_container_cluster.gke_cluster.self_link
  location       = var.cluster_availability_type == "ZONAL" ? var.zone : var.region
  node_locations = var.system_node_pool_zones
  version        = local.master_version

  autoscaling {
    total_min_node_count = var.system_node_pool_node_count.total_min_nodes
    total_max_node_count = var.system_node_pool_node_count.total_max_nodes
  }

  upgrade_settings {
    strategy        = local.upgrade_settings.strategy
    max_surge       = local.upgrade_settings.max_surge
    max_unavailable = local.upgrade_settings.max_unavailable
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }

  node_config {
    labels          = var.system_node_pool_kubernetes_labels
    resource_labels = local.labels
    service_account = var.service_account_email
    oauth_scopes    = var.service_account_scopes
    machine_type    = var.system_node_pool_machine_type
    disk_size_gb    = var.system_node_pool_disk_size_gb
    disk_type       = var.system_node_pool_disk_type

    dynamic "taint" {
      for_each = var.system_node_pool_taints
      content {
        key    = taint.value.key
        value  = taint.value.value
        effect = taint.value.effect
      }
    }

    # Forcing the use of the Container-optimized image, as it is the only
    # image with the proper logging daemon installed.
    #
    # cos images use Shielded VMs since v1.13.6-gke.0.
    # https://cloud.google.com/kubernetes-engine/docs/how-to/node-images
    #
    # We use COS_CONTAINERD to be compatible with (optional) gVisor.
    # https://cloud.google.com/kubernetes-engine/docs/how-to/sandbox-pods
    image_type = var.system_node_pool_image_type

    shielded_instance_config {
      enable_secure_boot          = var.system_node_pool_enable_secure_boot
      enable_integrity_monitoring = true
    }

    gvnic {
      enabled = var.system_node_pool_image_type == "COS_CONTAINERD"
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
      version,
    ]
    precondition {
      condition     = contains(["SURGE"], local.upgrade_settings.strategy)
      error_message = "Only SURGE strategy is supported"
    }
    precondition {
      condition     = local.upgrade_settings.max_unavailable >= 0
      error_message = "max_unavailable should be set to 0 or greater"
    }
    precondition {
      condition     = local.upgrade_settings.max_surge >= 0
      error_message = "max_surge should be set to 0 or greater"
    }
    precondition {
      condition     = local.upgrade_settings.max_unavailable > 0 || local.upgrade_settings.max_surge > 0
      error_message = "At least one of max_unavailable or max_surge must greater than 0"
    }
  }
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
  version = ">= 40.0"

  use_existing_gcp_sa = true
  name                = var.k8s_service_account_name
  gcp_sa_name         = local.sa_email
  project_id          = var.project_id

  # https://github.com/terraform-google-modules/terraform-google-kubernetes-engine/issues/1059
  depends_on = [
    data.google_project.project,
    google_container_cluster.gke_cluster
  ]
}

locals {
  k8s_service_account_name = one(module.workload_identity[*].k8s_service_account_name)
}

locals {
  # Separate gvnic and rdma networks and assign indexes
  gvnic_networks = [for idx, net in [for n in var.additional_networks : n if strcontains(upper(n.nic_type), "GVNIC")] :
    merge(net, { name = "${var.k8s_network_names.gvnic_prefix}${idx + var.k8s_network_names.gvnic_start_index}${var.k8s_network_names.gvnic_postfix}" })
  ]

  rdma_networks = [for idx, net in [for n in var.additional_networks : n if strcontains(upper(n.nic_type), "RDMA")] :
    merge(net, { name = "${var.k8s_network_names.rdma_prefix}${idx + var.k8s_network_names.rdma_start_index}${var.k8s_network_names.rdma_postfix}" })
  ]

  all_networks = concat(local.gvnic_networks, local.rdma_networks)
}

module "kubectl_apply" {
  source = "../../management/kubectl-apply"

  cluster_id = google_container_cluster.gke_cluster.id
  project_id = var.project_id

  apply_manifests = concat(flatten([
    for idx, network_info in local.all_networks : [
      {
        source = "${path.module}/templates/gke-network-paramset.yaml.tftpl",
        template_vars = {
          name            = network_info.name,
          network_name    = network_info.network
          subnetwork_name = network_info.subnetwork,
          device_mode     = strcontains(upper(network_info.nic_type), "RDMA") ? "RDMA" : "NetDevice"
        }
      },
      {
        source        = "${path.module}/templates/network-object.yaml.tftpl",
        template_vars = { name = network_info.name }
      }
    ]
    ]),
    var.enable_inference_gateway ? [
      {
        source        = "https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/download/v1.0.0/manifests.yaml",
        template_vars = {}
      }
    ] : []
  )
}
