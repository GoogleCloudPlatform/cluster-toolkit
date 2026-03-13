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
resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

locals {
  name = "${var.name}-${random_id.resource_name_suffix.hex}"
}

resource "google_compute_resource_policy" "policy" {
  name     = local.name
  region   = var.region
  project  = var.project_id
  provider = google-beta

  dynamic "workload_policy" {
    for_each = var.workload_policy.type != null ? [1] : []

    content {
      type                  = var.workload_policy.type
      max_topology_distance = var.workload_policy.max_topology_distance
      accelerator_topology  = var.workload_policy.accelerator_topology
    }
  }

  dynamic "group_placement_policy" {
    for_each = var.group_placement_max_distance > 0 ? [1] : []

    content {
      collocation  = "COLLOCATED"
      max_distance = var.group_placement_max_distance
    }
  }
}
