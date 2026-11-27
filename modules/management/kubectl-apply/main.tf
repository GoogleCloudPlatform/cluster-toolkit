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
  cluster_id_parts = split("/", var.cluster_id)
  cluster_name     = local.cluster_id_parts[5]
  cluster_location = local.cluster_id_parts[3]
  project_id       = var.project_id != null ? var.project_id : local.cluster_id_parts[1]

  asapd_lite_config_content = (
    local.install_asapd_lite && var.asapd_lite.config_path != null && var.asapd_lite.config_path != "" ?
    (
      endswith(var.asapd_lite.config_path, ".tftpl") || (var.asapd_lite.config_template_vars != null && length(var.asapd_lite.config_template_vars) > 0) ?
      templatefile(var.asapd_lite.config_path, var.asapd_lite.config_template_vars != null ? var.asapd_lite.config_template_vars : {}) :
      file(var.asapd_lite.config_path)
    ) : ""
  )

  # 1. First, Identify manifests that are explicitly enabled.
  enabled_manifests = {
    for index, manifest in coalesce(var.apply_manifests, []) : index => manifest
    if try(manifest.enable, true)
  }

  # 2. Identify URL-based manifests
  url_manifests = {
    for index, manifest in local.enabled_manifests : index => manifest
    if try(startswith(manifest.source, "http://") || startswith(manifest.source, "https://"), false)
  }

  # 3. Identify directory-based manifests
  directory_manifests = {
    for index, manifest in local.enabled_manifests : index => manifest
    if try(manifest.source, null) != null &&
    !contains(keys(local.url_manifests), index) &&
    (endswith(manifest.source, "/") || (!fileexists(manifest.source) && can(fileset(manifest.source, "*"))))
  }

  # Pre-calculate normalized names for each manifest
  manifest_names = {
    for index, manifest in local.enabled_manifests : index =>
    trim(replace(lower(
      (try(coalesce(manifest.name, ""), "") != "" ? manifest.name :
        "${substr((try(manifest.source, null) != null && manifest.source != "") ? replace(basename(manifest.source), "/(\\.(tftpl|yaml|yml))+$/", "") : "${var.module_id}-raw", 0, 30)}-${substr(sha1(join("|", [
          try(manifest.source != null ? manifest.source : "", ""),
          try(manifest.content != null ? manifest.content : "", ""),
          join(",", try(keys(manifest.template_vars), []))
        ])), 0, 7)}-${index}"
      )
    ), "/[^a-z0-9-]+/", "-"), "-")
  }

  # 4. Rebuild the map by populating the 'content' field for all manifests
  processed_apply_manifests_map = tomap({
    for index, manifest in local.enabled_manifests :
    local.manifest_names[index] => {
      content = (
        # Step A: Use the fetched body if it's a URL
        contains(keys(local.url_manifests), tostring(index)) ? data.http.manifest_from_url[tostring(index)].body :

        # Step B: Process directory files
        contains(keys(local.directory_manifests), index) ? (
          join("\n---\n", [
            # Use union() to combine the results of fileset (which are sets)
            for f in setunion(
              fileset(manifest.source, "*.yaml"),
              fileset(manifest.source, "*.yml"),
              fileset(manifest.source, "*.tftpl")
              ) : (
              endswith(f, ".tftpl") ?
              templatefile("${trimsuffix(manifest.source, "/")}/${f}", manifest.template_vars != null ? manifest.template_vars : {}) :
              file("${trimsuffix(manifest.source, "/")}/${f}")
            )
          ])
        ) :
        # Step C: Single file logic (implied if source is provided but not a URL or Dir)
        (manifest.source != null && manifest.source != "") ? (
          endswith(manifest.source, ".tftpl") || (manifest.template_vars != null && length(manifest.template_vars) > 0) ?
          templatefile(manifest.source, manifest.template_vars != null ? manifest.template_vars : {}) :
          file(manifest.source)
        )
        :
        # Step D: Fallback to the raw 'content' field
        coalesce(manifest.content, "")
      )
      wait_for_rollout = manifest.wait_for_rollout
      namespace        = manifest.namespace
    }
  })

  install_cert_manager             = try(var.cert_manager.install, false)
  install_k8s_aibom                = try(var.k8s_aibom.install, false)
  install_jobset                   = try(var.jobset.install, false)
  install_gpu_operator             = try(var.gpu_operator.install, false)
  install_nvidia_dra_driver        = try(var.nvidia_dra_driver.install, false)
  install_gib                      = try(var.gib.install, false)
  install_asapd_lite               = try(var.asapd_lite.install, false)
  install_checkpoint_configuration = var.enable_multi_tier_checkpointing && var.checkpoint_configuration != null

  jobset_controller_cpu    = try(var.jobset.controller_cpu, null) != null ? var.jobset.controller_cpu : (local.enable_slicing ? "4" : null)
  jobset_controller_memory = try(var.jobset.controller_memory, null) != null ? var.jobset.controller_memory : (local.enable_slicing ? "16Gi" : null)
}

data "http" "manifest_from_url" {
  for_each = local.url_manifests
  url      = each.value.source
}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

data "google_client_config" "default" {}

module "kubectl_apply_manifests" {
  for_each   = local.processed_apply_manifests_map
  source     = "./helm_install"
  depends_on = [var.gke_cluster_exists]

  release_name  = "manifest-${each.key}"
  chart_name    = "${path.module}/raw-config-chart"
  chart_version = "0.1.0"
  namespace     = each.value.namespace
  atomic        = each.value.wait_for_rollout
  wait          = each.value.wait_for_rollout
  timeout       = 1200
  values_yaml = [
    yamlencode({
      # Pass the entire unbroken string to Helm. Helm will parse inner '---' natively.
      manifests = length(trimspace(each.value.content)) > 0 ? [each.value.content] : []
    })
  ]
}

module "install_jobset" {
  source           = "./helm_install"
  count            = local.install_jobset ? 1 : 0
  wait             = false
  timeout          = 1200
  release_name     = "jobset"
  chart_repository = "oci://registry.k8s.io/jobset/charts"
  chart_name       = "jobset"
  chart_version    = var.jobset.version
  namespace        = "jobset-system"
  create_namespace = true
  values_yaml = compact([
    file("${path.module}/jobset/jobset-helm-values.yaml"),
    local.jobset_controller_cpu != null || local.jobset_controller_memory != null ? yamlencode({
      controller = {
        resources = {
          requests = merge(
            local.jobset_controller_cpu != null ? { cpu = local.jobset_controller_cpu } : {},
            local.jobset_controller_memory != null ? { memory = local.jobset_controller_memory } : {}
          )
          limits = merge(
            local.jobset_controller_cpu != null ? { cpu = local.jobset_controller_cpu } : {},
            local.jobset_controller_memory != null ? { memory = local.jobset_controller_memory } : {}
          )
        }
      }
    }) : ""
  ])
  depends_on = [var.gke_cluster_exists, module.configure_kueue]
}

module "install_cert_manager" {
  source           = "./helm_install"
  count            = local.install_cert_manager ? 1 : 0
  wait_for_jobs    = true
  timeout          = 1200
  release_name     = "cert-manager"
  chart_repository = "https://charts.jetstack.io"
  chart_name       = "cert-manager"
  chart_version    = var.cert_manager.version
  namespace        = "cert-manager"
  create_namespace = true
  set_values       = [{ name = "installCRDs", value = "true", type = "auto" }]
  depends_on       = [var.gke_cluster_exists, module.configure_kueue, module.install_jobset]
}

# k8s-aibom ships two charts from the same release: the CRD chart first, so
# CRD changes are ordinary Helm upgrades, then the controller chart, which
# pins its image by digest. Namespaces are opted in by label by the
# platform admin; the module installs the controller only.
module "install_k8s_aibom_crds" {
  source           = "./helm_install"
  count            = local.install_k8s_aibom ? 1 : 0
  timeout          = 600
  release_name     = "k8s-aibom-crds"
  chart_repository = "oci://ghcr.io/googlecloudplatform/charts"
  chart_name       = "k8s-aibom-crds"
  chart_version    = var.k8s_aibom.version
  namespace        = "k8s-aibom-system"
  create_namespace = true
  depends_on       = [var.gke_cluster_exists]
}

module "install_k8s_aibom" {
  source           = "./helm_install"
  count            = local.install_k8s_aibom ? 1 : 0
  timeout          = 600
  release_name     = "k8s-aibom"
  chart_repository = "oci://ghcr.io/googlecloudplatform/charts"
  chart_name       = "k8s-aibom"
  chart_version    = var.k8s_aibom.version
  namespace        = "k8s-aibom-system"
  create_namespace = false
  values_yaml = compact([
    file("${path.module}/k8s-aibom/k8s-aibom-helm-values.yaml"),
    var.k8s_aibom.controller_cpu != null || var.k8s_aibom.controller_memory != null ? yamlencode({
      resources = {
        requests = merge(
          var.k8s_aibom.controller_cpu != null ? { cpu = var.k8s_aibom.controller_cpu } : {},
          var.k8s_aibom.controller_memory != null ? { memory = var.k8s_aibom.controller_memory } : {}
        )
        limits = merge(
          var.k8s_aibom.controller_cpu != null ? { cpu = var.k8s_aibom.controller_cpu } : {},
          var.k8s_aibom.controller_memory != null ? { memory = var.k8s_aibom.controller_memory } : {}
        )
      }
    }) : ""
  ])
  dependencies = var.system_node_pool_id != null ? [var.system_node_pool_id] : []
  depends_on   = [var.gke_cluster_exists, module.install_k8s_aibom_crds, module.configure_kueue]
}

module "install_nvidia_dra_driver" {
  count      = local.install_nvidia_dra_driver ? 1 : 0
  depends_on = [module.kubectl_apply_manifests, var.gke_cluster_exists, module.configure_kueue]
  source     = "./helm_install"

  release_name     = "nvidia-dra-driver-gpu"              # The release name
  chart_repository = "https://helm.ngc.nvidia.com/nvidia" # The Helm repository URL for nvidia charts
  chart_name       = "nvidia-dra-driver-gpu"              # The chart name
  chart_version    = var.nvidia_dra_driver.version        # The chart version
  namespace        = "nvidia-dra-driver-gpu"              # The target namespace
  create_namespace = true                                 # Equivalent to --create-namespace

  # Use the 'values' argument to pass the YAML content
  # This corresponds to the -f <(cat <<EOF ... EOF) part
  values_yaml = [<<EOF
      nvidiaDriverRoot: /home/kubernetes/bin/nvidia
      resources:
        gpus:
          enabled: false

      controller:
        affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                - matchExpressions:
                  - key: "nvidia.com/gpu"
                    operator: "DoesNotExist"

      kubeletPlugin:
        affinity:
          nodeAffinity:
            requiredDuringSchedulingIgnoredDuringExecution:
              nodeSelectorTerms:
                - matchExpressions:
                    - key: cloud.google.com/gke-accelerator
                      operator: In
                      values:
                        - ${var.nvidia_dra_driver.accelerator_type}
                    - key: kubernetes.io/arch
                      operator: In
                      values:
                        - arm64

        tolerations:
          - key: nvidia.com/gpu
            operator: Equal
            value: present
            effect: NoSchedule
          - key: kubernetes.io/arch
            operator: Equal
            value: arm64
            effect: NoSchedule

      EOF
  ]

  atomic          = true
  cleanup_on_fail = true
}

module "install_gpu_operator" {
  count            = local.install_gpu_operator ? 1 : 0
  source           = "./helm_install"
  chart_repository = "https://helm.ngc.nvidia.com/nvidia"
  depends_on       = [module.kubectl_apply_manifests, var.gke_cluster_exists]

  namespace        = "gpu-operator"
  create_namespace = true

  release_name  = "gpu-operator"
  chart_name    = "gpu-operator"
  chart_version = var.gpu_operator.version
  wait          = true

  # Use the 'values' argument to pass the YAML content
  # This corresponds to the -f <(cat <<EOF ... EOF) part
  values_yaml = [<<EOF
      hostPaths:
        driverInstallDir: /home/kubernetes/bin/nvidia
      toolkit:
        installDir: /home/kubernetes/bin/nvidia
      cdi:
        enabled: true
        default: true
      driver:
        enabled: false

      daemonsets:
        tolerations:
          - key: nvidia.com/gpu
            operator: Equal
            value: present
            effect: NoSchedule
          - key: kubernetes.io/arch
            operator: Equal
            value: arm64
            effect: NoSchedule

      node-feature-discovery:
        worker:
          tolerations:
            - key: kubernetes.io/arch
              operator: Equal
              value: arm64
              effect: NoSchedule
            - key: "node-role.kubernetes.io/master"
              operator: "Equal"
              value: ""
              effect: "NoSchedule"
            - key: "node-role.kubernetes.io/control-plane"
              operator: "Equal"
              value: ""
              effect: "NoSchedule"
            - key: nvidia.com/gpu
              operator: Exists
              effect: NoSchedule
      EOF
  ]

  atomic          = true
  cleanup_on_fail = true

}

module "install_gib" {
  source = "./helm_install"
  count  = local.install_gib ? 1 : 0

  release_name = "nccl-gib"
  chart_name   = "${path.module}/raw-config-chart"
  namespace    = "kube-system"
  wait         = true
  depends_on   = [var.gke_cluster_exists]

  values_yaml = local.install_gib ? [
    yamlencode({
      manifests = [
        templatefile(var.gib.path, var.gib.template_vars)
      ]
    })
  ] : []
}

module "install_asapd_lite" {
  source        = "./helm_install"
  count         = local.install_asapd_lite ? 1 : 0
  release_name  = "asapd-lite"
  chart_name    = "${path.module}/raw-config-chart"
  chart_version = "0.1.0"
  namespace     = "kube-system"
  wait          = true
  depends_on    = [var.gke_cluster_exists]

  values_yaml = [
    yamlencode({
      manifests = length(trimspace(local.asapd_lite_config_content)) > 0 ? [local.asapd_lite_config_content] : []
    })
  ]
}

resource "kubernetes_annotations" "sa_patch" {
  for_each    = var.service_account_annotations
  depends_on  = [var.gke_cluster_exists]
  api_version = "v1"
  kind        = "ServiceAccount"

  metadata {
    name      = each.key
    namespace = each.value.namespace
  }

  annotations = {
    "iam.gke.io/gcp-service-account" = each.value.gcp_service_account_email
  }
}

locals {
  checkpoint_configuration_manifest = !local.install_checkpoint_configuration ? "" : yamlencode({
    apiVersion = "checkpointing.gke.io/v1"
    kind       = "CheckpointConfiguration"
    metadata   = { name = coalesce(var.checkpoint_configuration.name, "default") }
    # Unset and empty optional fields are left out of the spec.
    spec = { for k, v in {
      cloudStorageBucketName = var.checkpoint_configuration.cloud_storage_bucket_name
      inMemoryVolumeSize     = var.checkpoint_configuration.in_memory_volume_size
      nodeSelector           = var.checkpoint_configuration.node_selector
      tolerations            = var.checkpoint_configuration.tolerations == null ? null : [for t in var.checkpoint_configuration.tolerations : { for tk, tv in t : tk => tv if tv != null }]
      gcsFuseMountOptions    = var.checkpoint_configuration.gcs_fuse_mount_options
    } : k => v if v != null && length(v) > 0 }
  })
}

# The manifest hash in release_name replaces the release on any spec change, so the
# CheckpointConfiguration is re-created: GKE does not apply in-place updates to the driver.
module "install_checkpoint_configuration" {
  source        = "./helm_install"
  count         = local.install_checkpoint_configuration ? 1 : 0
  release_name  = "checkpoint-configuration-${substr(sha256(local.checkpoint_configuration_manifest), 0, 8)}"
  chart_name    = "${path.module}/raw-config-chart"
  chart_version = "0.1.0"
  wait          = true
  depends_on    = [var.gke_cluster_exists]

  values_yaml = [
    yamlencode({
      manifests = [local.checkpoint_configuration_manifest]
    })
  ]
}
