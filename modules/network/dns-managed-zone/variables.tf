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
  description = "Project ID"
}
variable "zone_name" {
  type        = string
  description = "The name of the DNS zone"
}
variable "dns_name" {
  type        = string
  description = "The DNS name of this managed zone, e.g. 'example.com.'"
}
variable "description" {
  type        = string
  description = "A textual description of this managed zone"
  default     = "Managed by Cluster Toolkit"
}
variable "labels" {
  type        = map(string)
  description = "A set of key/value label pairs to assign to this ManagedZone"
  default     = {}
}
variable "recordsets" {
  type = list(object({
    name    = string
    type    = string
    ttl     = number
    rrdatas = list(string)
  }))
  description = "List of DNS record sets to create in the zone"
  default     = []
}
