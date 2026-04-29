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
  description = "GCP Project ID"
  type        = string
}
variable "region" {
  description = "GCP Region"
  type        = string
}
variable "service_name" {
  description = "Cloud Run Service Name"
  type        = string
}
variable "image" {
  description = "Container Image URL"
  type        = string
}
variable "container_port" {
  description = "Port the container listens on"
  type        = number
  default     = 8080
}
variable "env_vars" {
  description = "Environment variables for the container"
  type        = map(string)
  default     = {}
}
variable "allow_unauthenticated" {
  description = "Whether to allow unauthenticated access"
  type        = bool
  default     = true
}

variable "ingress" {
  description = "Ingress traffic allowed for the service. Possible values: INGRESS_TRAFFIC_ALL, INGRESS_TRAFFIC_INTERNAL_ONLY, INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER."
  type        = string
  default     = "INGRESS_TRAFFIC_ALL"
  validation {
    condition     = contains(["INGRESS_TRAFFIC_ALL", "INGRESS_TRAFFIC_INTERNAL_ONLY", "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"], var.ingress)
    error_message = "The ingress variable must be one of INGRESS_TRAFFIC_ALL, INGRESS_TRAFFIC_INTERNAL_ONLY, or INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER."
  }
}

variable "labels" {
  description = "Labels to apply to the Cloud Run service"
  type        = any
  default     = {}
}
