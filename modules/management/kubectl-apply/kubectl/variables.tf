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

variable "content" {
  description = "The YAML body to apply to gke cluster."
  type        = string
  default     = null
}

variable "source_path" {
  description = "The source for manifest(s) to apply to gke cluster. Acceptable sources are a local yaml or template (.tftpl) file path, a directory (ends with '/') containing yaml or template files, and a url for a yaml file."
  type        = string
  default     = null
}

variable "template_vars" {
  description = "The values to populate template file(s) with."
  type        = any
  default     = null
}

variable "server_side_apply" {
  description = "Allow using kubectl server-side apply method."
  type        = bool
  default     = false
}

variable "wait_for_rollout" {
  description = "Wait or not for Deployments and APIService to complete rollout. See [kubectl wait](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_wait/) for more details."
  type        = bool
  default     = true
}

variable "force_conflicts" {
  description = "The force_conflicts boolean, when true, compels kubectl apply (in server-side apply mode) to forcefully take ownership and override any resource fields managed by a different entity. For more information, see [Using Server-Side Apply in a controller](https://kubernetes.io/docs/reference/using-api/server-side-apply/#using-server-side-apply-in-a-controller)"
  type        = bool
  default     = false
}
