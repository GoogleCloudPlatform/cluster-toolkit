/**
* Copyright 2026 Google LLC
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */


variable "project_id" {
  type        = string
  description = "Project ID of the GKE cluster."
}

variable "cluster_name" {
  type        = string
  description = "Name of the GKE cluster."
}

variable "location" {
  type        = string
  description = "Location (region or zone) of the GKE cluster."
}

variable "release_name" {
  type        = string
  description = "Name of the Helm release."
}

variable "chart_name" {
  type        = string
  description = "Name of the Helm chart to install or upgrade."
}

variable "namespace" {
  type        = string
  description = "Kubernetes namespace to install the release into."
}

variable "values_yaml" {
  type        = list(string)
  description = "List of paths to values.yaml files to pass to helm upgrade."
}

variable "set_values" {
  type = list(object({
    name  = string
    value = string
  }))
  description = "List of key-value pairs to set in the helm chart."
}
