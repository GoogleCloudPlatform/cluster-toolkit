# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "project_id" {
  description = "Project in which the certificate is created. Must be the load balancer's project."
  type        = string
}

variable "name" {
  description = "Name of the certificate and its certificate map, and prefix of its DNS authorizations and map entries."
  type        = string

  validation {
    # Leaves room for the 9-character suffix of the authorizations and entries.
    condition     = can(regex("^[a-z]([-a-z0-9]{0,52}[a-z0-9])?$", var.name))
    error_message = "name must be a valid resource name of at most 54 characters: lowercase letters, digits and hyphens, starting with a letter."
  }
}

variable "domains" {
  description = <<-EOT
    Domains the certificate covers, such as ["*.desktops.example.com"] or
    ["login.example.com", "viz.example.com"]. A wildcard covers one level of
    subdomain only.
    EOT
  type        = list(string)
  nullable    = false

  validation {
    condition     = length(var.domains) > 0
    error_message = "domains must name at least one domain."
  }
  validation {
    condition = alltrue([
      for domain in var.domains :
      can(regex("^(\\*\\.)?([a-z0-9]([-a-z0-9]*[a-z0-9])?\\.)+[a-z][-a-z0-9]*[a-z0-9]$", domain))
    ])
    error_message = "Each domain must be a lowercase DNS name, optionally starting with \"*.\", without a trailing dot."
  }
}

variable "labels" {
  description = "Labels to add to the resources."
  type        = map(string)
  default     = {}
}
