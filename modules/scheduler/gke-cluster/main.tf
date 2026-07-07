/**
  * Copyright 2026 Google LLC
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

resource "time_static" "exclusion_start" {}

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



  default_logging_component = [
    "SYSTEM_COMPONENTS",
    "WORKLOADS"
  ]
}

# GKE Node Auto-Provisioning (NAP) locals
locals {
  autoscaling_enabled = var.cluster_autoscaling != null
  autoscaling_config = local.autoscaling_enabled ? var.cluster_autoscaling : {
    limits                        = []
    service_account_email         = ""
    oauth_scopes                  = []
    autoprovisioning_disk_size_gb = null
    autoprovisioning_disk_type    = null
    autoprovisioning_auto_upgrade = null
    autoprovisioning_auto_repair  = null
    autoprovisioning_cpu_max      = null
    autoprovisioning_memory_max   = null
  }

  has_autoscaling_limits = local.autoscaling_enabled && length(local.autoscaling_config.limits) > 0
  nap_service_account    = local.autoscaling_enabled ? (local.autoscaling_config.service_account_email != "" ? local.autoscaling_config.service_account_email : local.sa_email) : null

  # These maximum values represent massive upper bounds for the GKE Node Auto-Provisioning 
  # and Cluster Autoscaler to allow essentially unlimited CPU and memory scaling for the cluster.
  nap_cpu_max    = local.autoscaling_enabled ? local.autoscaling_config.autoprovisioning_cpu_max : null
  nap_memory_max = local.autoscaling_enabled ? local.autoscaling_config.autoprovisioning_memory_max : null

  user_provided_resource_types = local.has_autoscaling_limits ? [for limit in local.autoscaling_config.limits : limit.autoprovisioning_resource_type] : []

  add_default_cpu    = local.autoscaling_enabled && !contains(local.user_provided_resource_types, "cpu")
  add_default_memory = local.autoscaling_enabled && !contains(local.user_provided_resource_types, "memory")

  machine_mappings = jsondecode(var.machine_mappings_json)
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
  latest_master_version  = data.google_container_engine_versions.version_prefix_filter.latest_master_version
  latest_channel_version = lookup(data.google_container_engine_versions.version_prefix_filter.release_channel_latest_version, var.release_channel, local.latest_master_version)
  master_version = var.min_master_version != null ? var.min_master_version : (
    var.release_channel != "UNSPECIFIED" ? local.latest_channel_version : local.latest_master_version
  )

  mldiagnostics_minimum_version = "1.35.0-gke.3065000"
}


module "slice_controller_version_check" {
  source          = "../../internal/semver_compare"
  current_version = local.master_version
  minimum_version = "1.35.0-gke.274500"
}

module "mldiagnostics_version_check" {
  source          = "../../internal/semver_compare"
  current_version = local.master_version
  minimum_version = local.mldiagnostics_minimum_version
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

  enable_shielded_nodes = var.enable_shielded_nodes

  dynamic "cluster_autoscaling" {
    for_each = local.autoscaling_enabled ? [1] : []
    content {
      enabled = true

      # Controls autoscaling algorithm of node-pools
      autoscaling_profile = var.autoscaling_profile

      dynamic "resource_limits" {
        for_each = concat(
          local.add_default_cpu ? [{ type = "cpu", min = 1, max = local.nap_cpu_max }] : [],
          local.add_default_memory ? [{ type = "memory", min = 1, max = local.nap_memory_max }] : [],
          local.has_autoscaling_limits ? [
            for limit in local.autoscaling_config.limits : {
              type = lookup(
                local.machine_mappings.machine_family_to_label_map,
                length(split("-", limit.autoprovisioning_resource_type)) > 1 ? join("-", slice(split("-", limit.autoprovisioning_resource_type), 0, length(split("-", limit.autoprovisioning_resource_type)) - 1)) : limit.autoprovisioning_resource_type,
                limit.autoprovisioning_resource_type
              )
              min = 0
              max = limit.autoprovisioning_max_count
            }
          ] : []
        )
        content {
          resource_type = resource_limits.value.type
          minimum       = resource_limits.value.min
          maximum       = resource_limits.value.max
        }
      }

      auto_provisioning_defaults {
        service_account = local.nap_service_account
        oauth_scopes    = local.autoscaling_config.oauth_scopes

        management {
          auto_upgrade = local.autoscaling_config.autoprovisioning_auto_upgrade
          auto_repair  = local.autoscaling_config.autoprovisioning_auto_repair
        }

        disk_size = local.autoscaling_config.autoprovisioning_disk_size_gb
        disk_type = local.autoscaling_config.autoprovisioning_disk_type
      }
    }
  }

  datapath_provider = local.derived_enable_dataplane_v2 ? "ADVANCED_DATAPATH" : "LEGACY_DATAPATH"

  enable_multi_networking = local.derived_enable_multi_networking

  enable_fqdn_network_policy = var.enable_fqdn_network_policy

  network_policy {
    # Enabling NetworkPolicy for clusters with DatapathProvider=ADVANCED_DATAPATH
    # is not allowed. Dataplane V2 will take care of network policy enforcement
    # instead.
    enabled = try(var.network_policy.enabled, false)
    # GKE Dataplane V2 support. This must be set to PROVIDER_UNSPECIFIED in
    # order to let the datapath_provider take effect.
    # https://github.com/terraform-google-modules/terraform-google-kubernetes-engine/issues/656#issuecomment-720398658
    provider = try(var.network_policy.provider, "PROVIDER_UNSPECIFIED")
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

  vertical_pod_autoscaling {
    enabled = var.enable_vertical_pod_autoscaling
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
    dynamic "daily_maintenance_window" {
      for_each = var.maintenance_start_time != null ? [1] : []
      content {
        start_time = var.maintenance_start_time
      }
    }

    dynamic "maintenance_exclusion" {
      for_each = var.maintenance_exclusions
      content {
        exclusion_name = maintenance_exclusion.value.name
        start_time     = coalesce(maintenance_exclusion.value.start_time, time_static.exclusion_start.rfc3339)
        end_time       = maintenance_exclusion.value.end_time
        exclusion_options {
          scope             = maintenance_exclusion.value.exclusion_scope
          end_time_behavior = maintenance_exclusion.value.exclusion_end_time_behavior
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
    slice_controller_config {
      enabled = var.enable_slice_controller
    }
    network_policy_config {
      disabled = !try(var.network_policy.enabled, false)
    }
  }

  confidential_nodes {
    enabled                    = var.enable_confidential_nodes
    confidential_instance_type = var.confidential_instance_type
  }


  timeouts {
    create = var.timeout_create
    update = var.timeout_update
  }


  dynamic "node_pool_defaults" {
    for_each = var.enable_gcfs ? [1] : []
    content {
      node_config_defaults {
        gcfs_config {
          enabled = true
        }
      }
    }
  }

  node_config {
    machine_type = var.enable_confidential_nodes ? var.system_node_pool_machine_type : "e2-medium"
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
    precondition {
      condition = (
        !var.enable_slice_controller ||
        module.slice_controller_version_check.is_greater_than_or_equal
      )
      error_message = "The GKE Slice Controller requires a GKE version of 1.35.0-gke.274500 or higher. Please update 'version_prefix' or 'min_master_version'."
    }
    precondition {
      condition     = !(local.derived_enable_dataplane_v2 && try(var.network_policy.enabled, false))
      error_message = "Enabling network policy (Calico) is not supported when GKE Dataplane V2 is enabled. Dataplane V2 automatically manages network policy enforcement."
    }
    precondition {
      condition     = !var.enable_fqdn_network_policy || local.derived_enable_dataplane_v2
      error_message = "FQDN Network Policy requires GKE Dataplane V2 to be enabled."
    }
    precondition {
      condition     = !var.enable_confidential_nodes || !var.system_node_pool_enabled || can(regex("^(n2d-|c2d-|c3d?-|t2d-|g4-)", var.system_node_pool_machine_type))
      error_message = "The system_node_pool_machine_type must be a confidential-compatible machine type (e.g., n2d, c2d, c3d, c3, t2d, g4) when enable_confidential_nodes is true and system_node_pool_enabled is true."
    }
  }

  monitoring_config {
    enable_components = var.enable_dcgm_monitoring ? distinct(concat(var.monitoring_components, ["DCGM"])) : var.monitoring_components
    managed_prometheus {
      enabled = true
      auto_monitoring_config {
        scope = var.auto_monitoring_scope
      }
    }
  }

  logging_config {
    enable_components = local.default_logging_component
  }

  dynamic "managed_machine_learning_diagnostics_config" {
    for_each = var.enable_ml_diagnostics ? [1] : []
    content {
      enabled = true
    }
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
    labels                      = var.system_node_pool_kubernetes_labels
    resource_labels             = local.labels
    service_account             = var.service_account_email
    oauth_scopes                = var.service_account_scopes
    machine_type                = var.system_node_pool_machine_type
    disk_size_gb                = var.system_node_pool_disk_size_gb
    disk_type                   = var.system_node_pool_disk_type
    enable_confidential_storage = var.enable_confidential_storage
    boot_disk_kms_key           = var.boot_disk_kms_key

    dynamic "confidential_nodes" {
      for_each = var.enable_confidential_nodes ? [1] : []
      content {
        enabled                    = true
        confidential_instance_type = var.confidential_instance_type
      }
    }

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
    precondition {
      condition     = !var.enable_confidential_storage || (var.boot_disk_kms_key != null && var.boot_disk_kms_key != "")
      error_message = "A valid boot_disk_kms_key must be provided when enable_confidential_storage is true to satisfy GKE Confidential Storage requirements."
    }
    precondition {
      condition     = !var.enable_confidential_storage || (var.system_node_pool_disk_type != null && can(regex("^hyperdisk", var.system_node_pool_disk_type)))
      error_message = "Confidential Storage (enable_confidential_storage = true) is only supported on Hyperdisks. Please set system_node_pool_disk_type to 'hyperdisk-balanced' or another hyperdisk type."
    }
  }
}

resource "google_container_node_pool" "cpu_np" {
  provider = google-beta
  count    = var.enable_pathways_for_tpus ? 1 : 0

  project        = var.project_id
  name           = "cpu-np"
  cluster        = var.cluster_reference_type == "NAME" ? google_container_cluster.gke_cluster.name : google_container_cluster.gke_cluster.self_link
  location       = var.cluster_availability_type == "ZONAL" ? var.zone : var.region
  node_locations = var.system_node_pool_zones
  version        = local.master_version

  autoscaling {
    total_min_node_count = 0
    total_max_node_count = 100
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }

  node_config {
    resource_labels = local.labels
    service_account = var.service_account_email
    oauth_scopes    = ["https://www.googleapis.com/auth/cloud-platform"]
    machine_type    = "n4-standard-64"
    image_type      = var.system_node_pool_image_type

    shielded_instance_config {
      enable_secure_boot          = var.system_node_pool_enable_secure_boot
      enable_integrity_monitoring = true
    }

    gvnic {
      enabled = var.system_node_pool_image_type == "COS_CONTAINERD"
    }

    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    metadata = {
      "disable-legacy-endpoints" = "true"
    }
  }

  lifecycle {
    ignore_changes = [
      version,
    ]
  }
}

resource "kubernetes_namespace" "user_namespace" {
  count = var.namespace != "default" ? 1 : 0

  metadata {
    name = var.namespace
  }

  depends_on = [
    google_container_cluster.gke_cluster
  ]
}

resource "kubernetes_labels" "workload_namespace_labels" {
  count       = var.enable_ml_diagnostics ? 1 : 0
  api_version = "v1"
  kind        = "Namespace"

  metadata {
    name = var.namespace
  }

  labels = {
    "managed-mldiagnostics-gke" = "true"
  }

  depends_on = [
    google_container_cluster.gke_cluster,
    kubernetes_namespace.user_namespace
  ]
}

module "workload_identity" {
  count   = var.configure_workload_identity_sa ? 1 : 0
  source  = "terraform-google-modules/kubernetes-engine/google//modules/workload-identity"
  version = ">= 40.0"

  use_existing_gcp_sa = true
  name                = var.k8s_service_account_name
  namespace           = var.namespace
  gcp_sa_name         = local.sa_email
  project_id          = var.project_id

  providers = {
    kubernetes = kubernetes
  }

  # https://github.com/terraform-google-modules/terraform-google-kubernetes-engine/issues/1059
  depends_on = [
    data.google_project.project,
    google_container_cluster.gke_cluster,
    kubernetes_namespace.user_namespace
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

resource "terraform_data" "validate_ml_diagnostics_version" {
  lifecycle {
    precondition {
      condition     = !var.enable_ml_diagnostics || module.mldiagnostics_version_check.is_greater_than_or_equal
      error_message = "GKE-managed ML Diagnostics requires a GKE version of ${local.mldiagnostics_minimum_version} or higher. Please update 'version_prefix' or 'min_master_version'."
    }
  }
}
