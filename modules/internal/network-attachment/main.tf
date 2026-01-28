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


variable "connection_preference" {
  type        = string
  description = "The connection preference of service attachment."
  default     = "ACCEPT_AUTOMATIC"
}

variable "subnetwork_self_links" {
  type        = list(string)
  description = " An array of selfLinks of subnets to use for endpoints in the producers that connect to this network attachment."
}

variable "name" {
  type        = string
  description = "Name of the resource. Provided by the client when the resource is created"
}

variable "project_id" {
  type        = string
  description = "The ID of the project in which the resource belongs."
}

variable "region" {
  type        = string
  description = "Region where the network attachment resides"
}


resource "google_compute_network_attachment" "self" {
  provider = google-beta

  project               = var.project_id
  region                = var.region
  name                  = var.name
  connection_preference = var.connection_preference
  subnetworks           = var.subnetwork_self_links
}


output "self_link" {
  value       = google_compute_network_attachment.self.self_link
  description = "Server-defined URL for the resource."
}

terraform {
  required_version = ">= 0.15.0"

  required_providers {
    google-beta = {
      source  = "hashicorp/google-beta"
      version = ">= 6.0.0"
    }
  }
}
