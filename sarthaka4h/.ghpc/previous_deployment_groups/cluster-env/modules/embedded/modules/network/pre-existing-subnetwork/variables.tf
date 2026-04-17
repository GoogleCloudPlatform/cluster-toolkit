/**
 * Copyright 2022 Google LLC
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

variable "subnetwork_self_link" {
  description = "Self-link of the subnet in the VPC"
  type        = string
  default     = null
}

variable "project" {
  description = "Name of the project that owns the subnetwork"
  type        = string
  default     = null
}

variable "subnetwork_name" {
  description = "Name of the pre-existing VPC subnetwork"
  type        = string
  default     = null
}

variable "region" {
  description = "Region in which to search for primary subnetwork"
  type        = string
  default     = null
}
