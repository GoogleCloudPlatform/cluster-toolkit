/**
  * Copyright 2024 Google LLC
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
  description = "The project ID for the resource policy."
  type        = string
}

variable "region" {
  description = "The region for the the resource policy."
  type        = string
}

variable "name" {
  description = "The resource policy's name."
  type        = string
}

variable "group_placement_max_distance" {
  description = <<-EOT
  The max distance for group placement policy to use for the node pool's nodes. If set it will add a compact group placement policy.
  Note: Placement policies have the [following](https://cloud.google.com/compute/docs/instances/placement-policies-overview#restrictions-compact-policies) restrictions.
  EOT

  type    = number
  default = 0
}
