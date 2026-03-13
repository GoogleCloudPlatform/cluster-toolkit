# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Description: Input variables for the generic Helm release module.

variable "content" {
  description = "The YAML body to apply to gke cluster."
  type        = string
  default     = null
}

variable "source_path" {
  description = "The source for manifest(s) to apply to gke cluster. Acceptable sources are a local yaml or template (.tftpl) file path, a directory (ends with '/') containing yaml or template files, and a url for a yaml file."
  type        = string
  default     = ""
}

variable "template_vars" {
  description = "The values to populate template file(s) with."
  type        = any
  default     = null
}

variable "wait_for_rollout" {
  description = "Wait or not for Deployments and APIService to complete rollout. See [kubectl wait](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_wait/) for more details."
  type        = bool
  default     = true
}


variable "wait_for_fields" {
  description = "(Optional) A map of attribute paths and desired patterns to be matched. After each apply the provider will wait for all attributes listed here to reach a value that matches the desired pattern."
  type        = map(string)
  default     = {}
}

variable "resource_timeouts" {
  description = "(Optional) Configure custom timeouts for the create, update, and delete operations of the resource. These timeouts also govern the duration for any 'wait' conditions to be met."
  type = object({
    create = optional(string, null)
    update = optional(string, null)
    delete = optional(string, null)
  })
  default = {
    create = "15m" # Default create timeout, also covers waiting for initial conditions
    update = "10m" # Default update timeout, also covers waiting for update conditions
    delete = "5m"  # Default delete timeout
  }
}

variable "field_manager" {
  description = "(Optional) Configure field manager options. The `name` is the name of the field manager. The `force_conflicts` flag allows overriding conflicts."
  type = object({
    name            = optional(string, null)
    force_conflicts = optional(bool, false)
  })
  default = null
}
