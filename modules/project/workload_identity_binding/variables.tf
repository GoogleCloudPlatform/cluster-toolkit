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

variable "project_id" {
  type        = string
  description = "The GCP project ID where the cluster is located."
}
variable "service_account_email" {
  type        = string
  description = "The email of the Google Service Account (GSA) to bind."
}
variable "namespace" {
  type        = string
  description = "The Kubernetes namespace where the KSA is located."
  default     = "default"
}
variable "k8s_service_account_name" {
  type        = string
  description = "The name of the Kubernetes Service Account (KSA) to bind."
}
