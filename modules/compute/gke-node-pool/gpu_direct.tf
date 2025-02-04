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
        "https://raw.githubusercontent.com/GoogleCloudPlatform/container-engine-accelerators/b324ec8994aa98ca320438dd2d01ff6d7f9165bb/gpudirect-tcpxo/nccl-tcpxo-installer.yaml",    # nccl_plugin v1.0.7 for tcpxo
        "https://raw.githubusercontent.com/GoogleCloudPlatform/container-engine-accelerators/b324ec8994aa98ca320438dd2d01ff6d7f9165bb/nri_device_injector/nri-device-injector.yaml", # nri_plugin
      ]
      updated_workload_path   = replace(local.workload_path_tcpxo, ".yaml", "-tcpxo.yaml")
      rxdm_version            = "v1.0.13_1" # matching nccl-tcpxo-installer version v1.0.7
      min_additional_networks = 8
      major_minor_version_acceptable_map = {
        "1.28" = "1.28.9-gke.1250000"
        "1.29" = "1.29.4-gke.1542000"
        "1.30" = "1.30.4-gke.1129000"
        "1.31" = "1.31.1-gke.2008000"
      }
    }
  }

  min_additional_networks = try(local.gpu_direct_settings[var.machine_type].min_additional_networks, 0)

  gke_version_regex = "(\\d+\\.\\d+)\\.(\\d+)-gke\\.(\\d+)" # GKE version format: 1.X.Y-gke.Z , regex output: ["1.X" , "Y", "Z"]

  gke_version_parts = regex(local.gke_version_regex, var.gke_version)
  gke_version_major = local.gke_version_parts[0]

  major_minor_version_acceptable_map = try(local.gpu_direct_setting[var.machine_type].major_minor_version_acceptable_map, null)
  minor_version_acceptable           = try(contains(keys(local.major_minor_version_acceptable_map), local.gke_version_major), false) ? local.major_minor_version_acceptable_map[local.gke_version_major] : "1.0.0-gke.0"
  minor_version_acceptable_parts     = regex(local.gke_version_regex, local.minor_version_acceptable)
  gke_gpudirect_compatible           = local.gke_version_parts[1] > local.minor_version_acceptable_parts[1] || (local.gke_version_parts[1] == local.minor_version_acceptable_parts[1] && local.gke_version_parts[2] >= local.minor_version_acceptable_parts[2])
}

check "gpu_direct_check_multi_vpc" {
  assert {
    condition     = length(var.additional_networks) >= local.min_additional_networks
    error_message = "To achieve optimal performance for ${var.machine_type} machine, at least ${local.min_additional_networks} additional vpc is recommended. You could configure it in the blueprint through modules/network/multivpc with network_count set as ${local.min_additional_networks}"
  }
}

check "gke_version_requirements" {
  assert {
    condition     = local.gke_gpudirect_compatible
    error_message = "GPUDirect is not supported on GKE version ${var.gke_version} for ${var.machine_type} machine. For supported version details visit https://cloud.google.com/kubernetes-engine/docs/how-to/gpu-bandwidth-gpudirect-tcpx#requirements"
  }
}
