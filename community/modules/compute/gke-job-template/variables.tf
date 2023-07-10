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

variable "name" {
  description = "The name of the job."
  type        = string
  default     = "my-job"
}

variable "node_count" {
  description = "How many nodes the job should run in parallel."
  type        = number
  default     = 1
}

variable "command" {
  description = "The command and arguments for the container that run in the Pod. The command field corresponds to entrypoint in some container runtimes."
  type        = list(string)
  default     = ["hostname"]
}

variable "image" {
  description = "The container image the job should use."
  type        = string
  default     = "debian"
}

variable "node_pool_name" {
  description = "A list of node pool names on which to run the job. Can be populated via `use` feild."
  type        = list(string)
  default     = []
}

variable "allocatable_cpu_per_node" {
  description = "The allocatable cpu per node. Used to claim whole nodes. Generally populated from gke-node-pool via `use` field."
  type        = list(number)
  default     = [-1]
}

variable "has_gpu" {
  description = "Indicates that the job should request nodes with GPUs. Typically supplied by a gke-node-pool module."
  type        = list(bool)
  default     = [false]
}

variable "requested_cpu_per_pod" {
  description = "The requested cpu per pod. If null, allocatable_cpu_per_node will be used to claim whole nodes. If provided will override allocatable_cpu_per_node."
  type        = number
  default     = -1
}

variable "tolerations" {
  description = "Tolerations allow the scheduler to schedule pods with matching taints. Generally populated from gke-node-pool via `use` field."
  type = list(object({
    key      = string
    operator = string
    value    = string
    effect   = string
  }))
  default = [
    {
      key      = "user-workload"
      operator = "Equal"
      value    = "true"
      effect   = "NoSchedule"
    }
  ]
}

variable "machine_family" {
  description = "The machine family to use in the node selector (example: `n2`). If null then machine family will not be used as selector criteria."
  type        = string
  default     = null
}

variable "node_selectors" {
  description = "A list of node selectors to use to place the job."
  type = list(object({
    key   = string
    value = string
  }))
  default = []
}

variable "restart_policy" {
  description = "Job restart policy. Only a RestartPolicy equal to `Never` or `OnFailure` is allowed."
  type        = string
  default     = "Never"
}

variable "backoff_limit" {
  description = "Controls the number of retries before considering a Job as failed. Set to zero for shared fate."
  type        = number
  default     = 0
}

variable "random_name_sufix" {
  description = "Appends a random suffix to the job name to avoid clashes."
  type        = bool
  default     = true
}

variable "persistent_volume_claims" {
  description = "A list of objects that describes a k8s PVC that is to be used and mounted on the job. Generally supplied by the gke-persistent-volume module."
  type = list(object({
    name          = string
    mount_path    = string
    mount_options = string
  }))
  default = []
}

variable "labels" {
  description = "Labels to add to the GKE job template. Key-value pairs."
  type        = map(string)
}
