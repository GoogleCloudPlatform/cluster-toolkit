/**
  * Copyright 2024 Google LLC
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

  apply_manifests_map = tomap({
    for index, manifest in var.apply_manifests : index => manifest
  })

  install_kueue             = try(var.kueue.install, false)
  install_jobset            = try(var.jobset.install, false)
  install_gpu_operator      = try(var.gpu_operator.install, false)
  install_nvidia_dra_driver = try(var.nvidia_dra_driver.install, false)
  kueue_install_source      = format("${path.module}/manifests/kueue-%s.yaml", try(var.kueue.version, ""))
  jobset_install_source     = format("${path.module}/manifests/jobset-%s.yaml", try(var.jobset.version, ""))
}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

data "google_client_config" "default" {}

module "kubectl_apply_manifests" {
  for_each = local.apply_manifests_map
  source   = "./kubectl"

  content           = each.value.content
  source_path       = each.value.source
  template_vars     = each.value.template_vars
  server_side_apply = each.value.server_side_apply
  wait_for_rollout  = each.value.wait_for_rollout

  providers = {
    kubectl = kubectl
    http    = http.h
  }
}

module "install_kueue" {
  source            = "./kubectl"
  source_path       = local.install_kueue ? local.kueue_install_source : null
  server_side_apply = true

  providers = {
    kubectl = kubectl
    http    = http.h
  }
}

module "install_jobset" {
  source            = "./kubectl"
  source_path       = local.install_jobset ? local.jobset_install_source : null
  server_side_apply = true

  providers = {
    kubectl = kubectl
    http    = http.h
  }
}

module "configure_kueue" {
  source        = "./kubectl"
  source_path   = local.install_kueue ? try(var.kueue.config_path, "") : null
  template_vars = local.install_kueue ? try(var.kueue.config_template_vars, null) : null
  depends_on    = [module.install_kueue]

  server_side_apply = true
  wait_for_rollout  = true

  providers = {
    kubectl = kubectl
    http    = http.h
  }
}

module "install_nvidia_dra_driver" {
  count      = local.install_nvidia_dra_driver ? 1 : 0
  depends_on = [module.kubectl_apply_manifests, var.gke_cluster_exists]
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
      nvidiaCtkPath: /home/kubernetes/bin/nvidia/toolkit/nvidia-ctk
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
                    - key: feature.node.kubernetes.io/pci-10de.present
                      operator: In
                      values:
                        - "true"
                - matchExpressions:
                    - key: feature.node.kubernetes.io/cpu-model.vendor_id
                      operator: In
                      values:
                        - "ARM"
                - matchExpressions:
                    - key: "nvidia.com/gpu.present"
                      operator: In
                      values:
                        - "true"
        tolerations:
          - key: nvidia.com/gpu
            operator: Equal
            value: present
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

  set_values = [
    {
      name  = "hostPaths.driverInstallDir",
      value = "/home/kubernetes/bin/nvidia"
    },
    {
      name  = "toolkit.installDir"
      value = "/home/kubernetes/bin/nvidia"
    },
    {
      name  = "cdi.enabled"
      value = true
    },
    {
      name  = "cdi.default"
      value = true
    },
    {
      name  = "driver.enabled"
      value = false
  }]
}
