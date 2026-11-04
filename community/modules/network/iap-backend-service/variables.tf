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
  description = "Project in which the backend service is created. Must be the load balancer's project."
  type        = string
}

variable "name" {
  description = <<-EOT
    Name of the backend service, and of its endpoint group and health check.
    Pass the same name to the desktop modules' iap_backend_service: the brokers
    look the backend service up by it to verify IAP assertions.
    EOT
  type        = string

  validation {
    condition     = can(regex("^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$", var.name))
    error_message = "name must be a valid resource name: lowercase letters, digits and hyphens, starting with a letter."
  }
}

variable "zone" {
  description = "Zone of the desktop instances. Every instance in endpoints must be in it."
  type        = string
}

variable "network_self_link" {
  description = "Network of the desktop instances."
  type        = string
}

variable "subnetwork_self_link" {
  description = "Subnetwork of the desktop instances. Optional when the network has one subnetwork in the zone's region."
  type        = string
  default     = null
}

variable "endpoints" {
  description = <<-EOT
    Desktop instances to serve, by name and primary internal IP, such as
    novnc-desktop's network_endpoints. Every instance must be in zone. May be
    empty, for endpoints attached later.
    EOT
  type = list(object({
    instance   = string
    ip_address = string
  }))
  default = []

  validation {
    condition     = length(distinct([for endpoint in var.endpoints : endpoint.instance])) == length(var.endpoints)
    error_message = "Each instance may appear in endpoints only once."
  }
}

variable "port" {
  description = "TCP port the desktop brokers serve the load balancer on: the desktop modules' iap_listen_port."
  type        = number
  default     = 6080
}

variable "health_check_path" {
  description = "Path the load balancer's health check requests. The broker answers it without authentication."
  type        = string
  default     = "/healthz"
}

variable "timeout_sec" {
  description = "Backend service timeout, which bounds how long a desktop's websocket can stay open."
  type        = number
  default     = 86400
}

variable "iap_members" {
  description = <<-EOT
    Principals allowed through Identity-Aware Proxy, such as
    "user:alice@example.com", "group:hpc@example.com" or "domain:example.com".
    They also need roles/compute.osLogin on the instances, or they get past IAP
    and are then refused a desktop.
    EOT
  type        = list(string)
  default     = []
}

variable "oauth2_client_id" {
  description = "OAuth client for IAP. Leave null, with oauth2_client_secret, to use a Google-managed client, which admits only users in your organization."
  type        = string
  default     = null
}

variable "oauth2_client_secret" {
  description = "Secret of oauth2_client_id."
  type        = string
  default     = null
  sensitive   = true

  validation {
    condition     = (var.oauth2_client_id == null) == (var.oauth2_client_secret == null)
    error_message = "Set oauth2_client_id and oauth2_client_secret together, or neither for a Google-managed client."
  }
}
