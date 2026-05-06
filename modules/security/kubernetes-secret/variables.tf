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

variable "namespace" {
  description = "Kubernetes namespace"
  type        = string
}
variable "secret_name" {
  description = "Name of the Kubernetes secret"
  type        = string
}
variable "data" {
  description = "Key-value map of secret data"
  type        = map(string)
  sensitive   = true
}
variable "cluster_id" {
  description = "The full GCP resource ID of the GKE cluster in the format projects/PROJECT_ID/locations/LOCATION/clusters/CLUSTER_NAME"
  type        = string
}

variable "cluster_endpoint" {
  description = "The endpoint of the GKE cluster. Do not include the https:// prefix. If provided, ignores data source lookup."
  type        = string
  default     = null
}

variable "cluster_ca_certificate" {
  description = "The cluster CA certificate of the GKE cluster. Must be base64-encoded. If provided, ignores data source lookup."
  type        = string
  default     = null
}

variable "access_token" {
  description = "The access token for accessing the cluster. If provided, ignores data source lookup."
  type        = string
  sensitive   = true
  default     = null
}
