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

variable "name" {
  type        = string
  description = "Name of the peering."
}

variable "network_self_link" {
  type        = string
  description = "The primary network of the peering."
}

variable "peer_network_self_link" {
  type        = string
  description = "The peer network in the peering. The peer network may belong to a different project."
}

variable "export_custom_routes" {
  type        = bool
  description = "(Optional) Whether to export the custom routes to the peer network. Defaults to false."
  default     = null
}

variable "import_custom_routes" {
  type        = bool
  description = "(Optional) Whether to import the custom routes from the peer network. Defaults to false."
  default     = null
}

variable "import_subnet_routes_with_public_ip" {
  type        = bool
  description = "(Optional) Whether subnet routes with public IP range are imported. "
  default     = null
}

variable "stack_type" {
  type        = string
  description = "(Optional) Which IP version(s) of traffic and routes are allowed to be imported or exported between peer networks. "
  default     = null
}

resource "google_compute_network_peering" "peering" {
  name                                = var.name
  network                             = var.network_self_link
  peer_network                        = var.peer_network_self_link
  export_custom_routes                = var.export_custom_routes
  import_custom_routes                = var.import_custom_routes
  import_subnet_routes_with_public_ip = var.import_subnet_routes_with_public_ip
  stack_type                          = var.stack_type
}

output "peering_name" {
  value       = google_compute_network_peering.peering.name
  description = "Name of the peering."
}

terraform {
  required_version = ">= 0.15.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 3.83"
    }
  }
}
