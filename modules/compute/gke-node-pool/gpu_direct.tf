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

# Enable GPUDirect for A3 and A3Mega VMs, this involve multiple kubectl steps to integrate with the created cluster
# 1. Install NCCL plugin daemonset
# 2. Install NRI plugin daemonset
# 3. Update provided workload to inject rxdm sidecar and other required annotation, volume etc.
locals {
  workload_path_tcpx  = "${path.module}/gpu-direct-workload/sample-tcpx-workload-job.yaml"
  workload_path_tcpxo = "${path.module}/gpu-direct-workload/sample-tcpxo-workload-job.yaml"

  gpu_direct_settings = {
    "a3-highgpu-8g" = {
      # Manifest to be installed for enabling TCPX on a3-highgpu-8g machines
      gpu_direct_manifests = [
        "https://raw.githubusercontent.com/GoogleCloudPlatform/container-engine-accelerators/fee883360a660f71ba07478db95d5c1325322f77/gpudirect-tcpx/nccl-tcpx-installer.yaml",      # nccl_plugin v3.1.9 for tcpx
        "https://raw.githubusercontent.com/GoogleCloudPlatform/container-engine-accelerators/fee883360a660f71ba07478db95d5c1325322f77/gpudirect-tcpx/nccl-config.yaml",              # nccl_configmap
        "https://raw.githubusercontent.com/GoogleCloudPlatform/container-engine-accelerators/fee883360a660f71ba07478db95d5c1325322f77/nri_device_injector/nri-device-injector.yaml", # nri_plugin
      ]
      updated_workload_path   = replace(local.workload_path_tcpx, ".yaml", "-tcpx.yaml")
      rxdm_version            = "v2.0.12" # matching nccl-tcpx-installer version v3.1.9
      min_additional_networks = 4
      major_minor_version_acceptable_map = {
        "1.27" = "1.27.7-gke.1121000"
        "1.28" = "1.28.8-gke.1095000"
        "1.29" = "1.29.3-gke.1093000"
        "1.30" = "1.30.2-gke.1023000"
      }
    }
    "a3-megagpu-8g" = {
      # Manifest to be installed for enabling TCPXO on a3-megagpu-8g machines
      gpu_direct_manifests = [
        "https://raw.githubusercontent.com/GoogleCloudPlatform/container-engine-accelerators/39308db7574925ea3c14f9113fcf87f70a6fcc26/gpudirect-tcpxo/nccl-tcpxo-installer.yaml",    # nccl_plugin v1.0.8-1 for tcpxo
        "https://raw.githubusercontent.com/GoogleCloudPlatform/container-engine-accelerators/39308db7574925ea3c14f9113fcf87f70a6fcc26/nri_device_injector/nri-device-injector.yaml", # nri_plugin
      ]
      updated_workload_path   = replace(local.workload_path_tcpxo, ".yaml", "-tcpxo.yaml")
      rxdm_version            = "v1.0.14" # matching nccl-tcpxo-installer version v1.0.8-1
      min_additional_networks = 8
      major_minor_version_acceptable_map = {
        "1.28" = "1.28.9-gke.1250000"
        "1.29" = "1.29.4-gke.1542000"
        "1.30" = "1.30.4-gke.1129000"
        "1.31" = "1.31.1-gke.2008000"
        "1.32" = "1.32.2-gke.1489001"
      }
    }
  }
}
