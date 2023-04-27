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
  description = "A list of strings that will be joined to create the job command."
  type        = list(string)
  default     = ["hostname"]
}

variable "image" {
  description = "The container image the job should use."
  type        = string
  default     = "debian"
}

variable "node_pool_name" {
  description = "The name of the node pool on which to run the job. Can be populated via `use` feild."
  type        = string
  default     = null
}

variable "cpu_per_node" {
  description = "The number of CPUs per node. Used to claim whole nodes. Generally populated from gke-node-pool via `use` field."
  type        = number
  default     = null
}

variable "tolerations" {
  description = "value"
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
  description = "Controls the number of retries before considering a Job as failed."
  type        = number
  default     = 3
}

variable "random_name_sufix" {
  description = "Appends a random suffix to the job name to avoid clashes."
  type        = bool
  default     = false
}
