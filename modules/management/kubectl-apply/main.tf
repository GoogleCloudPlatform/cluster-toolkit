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
  kueue_config_content = (
    var.kueue.config_path != null && var.kueue.config_path != "" ?
    (
      endswith(var.kueue.config_path, ".tftpl") || length(try(var.kueue.config_template_vars, {})) > 0 ?
      templatefile(var.kueue.config_path, try(var.kueue.config_template_vars, {})) :
      file(var.kueue.config_path)
    ) : ""
  )
  configure_kueue = local.install_kueue && try(var.kueue.config_path, "") != ""

  # 1. First, Identify manifests that are explicitly enabled.
  enabled_manifests = {
    for index, manifest in var.apply_manifests : index => manifest
    if try(manifest.enable, true)
  }

  # 2. Identify URL-based manifests
  url_manifests = {
    for index, manifest in local.enabled_manifests : index => manifest
    if try(manifest.source, null) != null && (startswith(manifest.source, "http://") || startswith(manifest.source, "https://"))
  }

  # 3. Rebuild the map by populating the 'content' field for URLs based manifest
  processed_apply_manifests_map = tomap({
    for index, manifest in local.enabled_manifests : tostring(index) => {
      # If this manifest was a URL, its content is the body from the HTTP call.
      content = contains(keys(local.url_manifests), tostring(index)) ? data.http.manifest_from_url[tostring(index)].body : manifest.content

      # If this was a URL, its source path is now null. Otherwise, use original.
      source = contains(keys(local.url_manifests), tostring(index)) ? null : manifest.source

      # Pass other vars
      template_vars     = manifest.template_vars
      server_side_apply = manifest.server_side_apply
      wait_for_rollout  = manifest.wait_for_rollout
    }
    }

  )

  install_kueue             = try(var.kueue.install, false)
  install_jobset            = try(var.jobset.install, false)
  install_gpu_operator      = try(var.gpu_operator.install, false)
  install_nvidia_dra_driver = try(var.nvidia_dra_driver.install, false)
  install_gib               = try(var.gib.install, false)
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
  source     = "./kubectl"
  depends_on = [var.gke_cluster_exists]

  content           = each.value.content
  source_path       = each.value.source
  template_vars     = each.value.template_vars
  server_side_apply = each.value.server_side_apply
  wait_for_rollout  = each.value.wait_for_rollout

  providers = {
    kubectl = kubectl
  }
}

module "install_kueue" {
  source           = "./helm_install"
  count            = local.install_kueue ? 1 : 0
  wait             = true
  timeout          = 1200
  release_name     = "kueue"
  chart_repository = "oci://registry.k8s.io/kueue/charts"
  chart_name       = "kueue"
  chart_version    = var.kueue.version
  namespace        = "kueue-system"
  create_namespace = true
  values_yaml = [
    file("${path.module}/kueue/kueue-helm-values.yaml")
  ]

  dependencies = var.system_node_pool_id != null ? [var.system_node_pool_id] : []

  depends_on = [var.gke_cluster_exists]
}

module "configure_kueue" {
  source           = "./helm_install"
  count            = local.configure_kueue ? 1 : 0
  release_name     = "kueue-config"
  chart_name       = "${path.module}/raw-config-chart"
  chart_version    = "0.1.0"
  namespace        = "kueue-system"
  create_namespace = true
  wait             = false # Configuration resources (Queues) usually don't need wait

  values_yaml = [
    yamlencode({
      manifests = [for doc in split("\n---", local.kueue_config_content) : trimspace(doc) if length(trimspace(doc)) > 0]
    })
  ]

  depends_on = [module.install_kueue]

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
  values_yaml = [
    file("${path.module}/jobset/jobset-helm-values.yaml")
  ]
  depends_on = [var.gke_cluster_exists, module.configure_kueue]
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
                        - nvidia-gb200
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
