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
  description = "Project ID where the GKE cluster and Ingress reside."
}

variable "cluster_name" {
  type        = string
  description = "Name of the GKE cluster."
}

variable "location" {
  type        = string
  description = "Location (zone or region) of the GKE cluster."
}

variable "namespace" {
  type        = string
  description = "Kubernetes namespace where the Ingress is deployed."
}

variable "service_name" {
  type        = string
  description = "Name of the Kubernetes Service the BackendService is backing (e.g. 'http-service')."
}

variable "service_port" {
  type        = string
  description = "Port of the Kubernetes Service (e.g. '8080')."
}

variable "timeout_seconds" {
  type        = number
  description = "Maximum time to wait for the backend service to be provisioned (in seconds)."
  default     = 600
}
