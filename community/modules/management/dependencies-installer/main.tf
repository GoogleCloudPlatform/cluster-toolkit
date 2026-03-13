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

  install_gpu_operator      = try(var.gpu_operator.install, false)
  install_nvidia_dra_driver = try(var.nvidia_dra_driver.install, false)
}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

data "google_client_config" "default" {}

module "install_kueue" {
  source     = "./helm_install"
  depends_on = [var.gke_cluster_exists]

  release_name = "kueue"

  chart_name    = "oci://registry.k8s.io/kueue/charts/kueue"
  chart_version = var.kueue.version # Specify your desired Kueue version

  create_namespace = true # Helm can also create the namespace
  wait             = true
  timeout          = 600 # seconds
}

module "install_jobset" {
  source           = "./helm_install"
  depends_on       = [var.gke_cluster_exists, module.install_kueue]
  release_name     = "jobset-controller"                          # The release name for your JobSet installation
  chart_name       = "oci://registry.k8s.io/jobset/charts/jobset" # The Helm repository URL for nvidia charts
  chart_version    = var.jobset.version
  create_namespace = true
  namespace        = "jobset-system"
}

module "install_nvidia_dra_driver" {
  count      = local.install_nvidia_dra_driver ? 1 : 0
  depends_on = [var.gke_cluster_exists]
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
      nvidiaCtkPath: /home/kubernetes/bin/nvidia/nvidia-ctk
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
  depends_on       = [var.gke_cluster_exists]

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
