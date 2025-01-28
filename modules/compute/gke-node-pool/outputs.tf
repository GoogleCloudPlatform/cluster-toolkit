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

output "node_pool_name" {
  description = "Name of the node pool."
  value       = google_container_node_pool.node_pool.name
}

locals {
  is_a_series = local.machine_family == "a2"
  last_digit  = trimsuffix(try(local.machine_vals[2], 0), "g")

  # Shared core machines only have 1 cpu allocatable, even if they have 2 cpu capacity
  vcpu        = local.machine_shared_core ? 1 : local.is_a_series ? local.last_digit * 12 : local.last_digit
  useable_cpu = local.set_threads_per_core ? local.threads_per_core * local.vcpu / 2 : local.vcpu

  # allocatable resource definition: https://cloud.google.com/kubernetes-engine/docs/concepts/plan-node-sizes#cpu_reservations
  second_core       = local.useable_cpu > 1 ? 1 : 0
  third_fourth_core = local.useable_cpu == 3 ? 1 : local.useable_cpu > 3 ? 2 : 0
  cores_above_four  = local.useable_cpu > 4 ? local.useable_cpu - 4 : 0

  allocatable_cpu = 0.94 + (0.99 * local.second_core) + (0.995 * local.third_fourth_core) + (0.9975 * local.cores_above_four)
}

output "allocatable_cpu_per_node" {
  description = "Number of CPUs available for scheduling pods on each node."
  value       = local.allocatable_cpu
}

output "has_gpu" {
  description = "Boolean value indicating whether nodes in the pool are configured with GPUs."
  value       = local.has_gpu
}

output "allocatable_gpu_per_node" {
  description = "Number of GPUs available for scheduling pods on each node."
  value       = local.allocatable_gpu_per_node
}

output "static_gpu_count" {
  description = "Total number of GPUs in the node pool. Available only for static node pools."
  value       = local.static_gpu_count
}

locals {
  translate_toleration = {
    PREFER_NO_SCHEDULE = "PreferNoSchedule"
    NO_SCHEDULE        = "NoSchedule"
    NO_EXECUTE         = "NoExecute"
  }
  taints = google_container_node_pool.node_pool.node_config[0].taint
  tolerations = [for taint in local.taints : {
    key      = taint.key
    operator = "Equal"
    value    = taint.value
    effect   = lookup(local.translate_toleration, taint.effect, null)
  }]
}

output "tolerations" {
  description = "Tolerations needed for a pod to be scheduled on this node pool."
  value       = local.tolerations
}

locals {
  gpu_direct_enabled = var.machine_type == "a3-highgpu-8g" || var.machine_type == "a3-megagpu-8g"
  script_path = {
    a3-highgpu-8g = "enable-tcpx-in-workload.py",
    a3-megagpu-8g = "enable-tcpxo-in-workload.py"
  }
  gpu_direct_instruction = <<-EOT
    Since you are using ${var.machine_type} machine type that has GPUDirect support, your nodepool had been configured with the required plugins.
    To fully utilize GPUDirect you will need to add some components into your workload manifest. Details below:

    A sample GKE job that has GPUDirect enabled and NCCL test included has been generated locally at:
      ${abspath(local.gpu_direct_setting.updated_workload_path)}

    You can use the following commands to submit the sample job:
      kubectl create -f ${abspath(local.gpu_direct_setting.updated_workload_path)}
    After submitting the sample job, you can validate the GPU performance by initiating NCCL test included in the sample workload:
      NCCL test can be initiated from any one of the sample job Pods and coordinate with the peer Pods:
      export POD_NAME=$(kubectl get pods -l job-name=my-sample-job -o go-template='{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}' | head -n 1)
      export PEER_POD_IPS=$(kubectl get pods -l job-name=my-sample-job -o go-template='{{range .items}}{{.status.podIP}}{{" "}}{{end}}')
      kubectl exec --stdin --tty --container=nccl-test $POD_NAME -- /scripts/allgather.sh $PEER_POD_IPS

    If you would like to enable GPUDirect for your own workload, please follow the below steps:
      export WORKLOAD_PATH=<>
      python3 ${abspath("${path.module}/gpu-direct-workload/scripts/${lookup(local.script_path, var.machine_type, "")}")} --file $WORKLOAD_PATH --rxdm ${local.gpu_direct_setting.rxdm_version}
    **WARNING**
    The "--rxdm" version is tied to the nccl-tcpx/o-installer that had been deployed to your cluster, changing it to other value might have impact on performance
    **WARNING**

    Or you can also follow our GPUDirect user guide to update your workload
    https://cloud.google.com/kubernetes-engine/docs/how-to/gpu-bandwidth-gpudirect-tcpx#add-gpudirect-manifests
  EOT
}

output "instructions" {
  description = "Instructions for submitting the sample GPUDirect enabled job."
  value       = local.gpu_direct_enabled ? local.gpu_direct_instruction : null
}
