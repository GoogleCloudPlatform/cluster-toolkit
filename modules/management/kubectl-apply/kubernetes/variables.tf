/**
 * Copyright 2024 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the sific language governing permissions and
 * limitations under the License.
 */

# variable "cluster_id" {
#   type        = string
#   description = "The full ID of the GKE cluster (e.g., projects/my-project/locations/us-central1/clusters/my-cluster)."
# }

# variable "project_id" {
#   type        = string
#   description = "The GCP project ID where the GKE cluster resides."
#   nullable    = true
#   default     = null
# }

variable "source_path" {
  description = "Path to a single manifest file (.yaml or .tftpl) or a directory of manifests. For a directory, the path must end with a '/'."
  type        = string
  default     = null
}

variable "content" {
  description = "Direct content of a YAML manifest. Has precedence over source_path."
  type        = string
  default     = null
}

variable "template_vars" {
  description = "A map of variables to be used when rendering .tftpl template files."
  type        = map(any)
  default     = {}
}

# variable "manifests" {
#   description = "A list of manifest objects to apply. Each object must have a 'content' key with the YAML string."
#   type        = list(object({ content = string }))
#   default     = []
# }
