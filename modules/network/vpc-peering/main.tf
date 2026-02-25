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

resource "google_compute_network_peering" "local_to_remote" {
  name                                = var.peering_name != null ? var.peering_name : "${var.deployment_name}-peering-local"
  network                             = var.local_network_self_link
  peer_network                        = var.remote_network_self_link
  export_custom_routes                = var.export_custom_routes
  import_custom_routes                = var.import_custom_routes
  import_subnet_routes_with_public_ip = var.import_subnet_routes_with_public_ip
  stack_type                          = var.stack_type
}

resource "google_compute_network_peering" "remote_to_local" {
  count                               = var.create_remote_peering ? 1 : 0
  name                                = var.remote_peering_name != null ? var.remote_peering_name : "${var.deployment_name}-peering-remote"
  network                             = var.remote_network_self_link
  peer_network                        = var.local_network_self_link
  export_custom_routes                = var.export_custom_routes
  import_custom_routes                = var.import_custom_routes
  import_subnet_routes_with_public_ip = var.import_subnet_routes_with_public_ip
  stack_type                          = var.stack_type
}
