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
  description = "Project in which the load balancer is created. Must be its backend services' project."
  type        = string
}

variable "name" {
  description = "Name of the load balancer's forwarding rules, and prefix of its other resources."
  type        = string

  validation {
    # Leaves room for the suffixes of the resources named after it.
    condition     = can(regex("^[a-z]([-a-z0-9]{0,40}[a-z0-9])?$", var.name))
    error_message = "name must be a valid resource name of at most 42 characters: lowercase letters, digits and hyphens, starting with a letter."
  }
}

variable "address" {
  description = <<-EOT
    A reserved global external IPv4 address for the load balancer: the address
    itself, not its name. Leave null to reserve one, which is released with the
    load balancer, so DNS records pointing at it must then be updated.
    EOT
  type        = string
  default     = null
}

variable "routes" {
  description = <<-EOT
    Hostnames and the backend services that serve them, by self link, such as
    iap-backend-service's backend_service_self_link. paths defaults to every
    path. Requests for any other hostname go to the route whose host is "*",
    or without one, to the alphabetically first host's backend service.
    EOT
  type = list(object({
    host            = string
    backend_service = string
    paths           = optional(list(string), ["/*"])
  }))
  nullable = false

  validation {
    # A URL map needs a default backend service.
    condition     = length(var.routes) > 0
    error_message = "routes must name at least one route: a load balancer cannot be created without a backend service."
  }
  validation {
    condition     = alltrue([for route in var.routes : length(route.paths) > 0])
    error_message = "Each route's paths must name at least one path."
  }
}

variable "certificate_map" {
  description = <<-EOT
    ID of a Certificate Manager certificate map, such as managed-certificate's
    certificate_map. Use it for a wildcard certificate. Must be known at plan
    time, as whether to serve HTTPS depends on it.
    EOT
  type        = string
  default     = null

  validation {
    condition     = var.certificate_map == null || (length(var.ssl_certificates) == 0 && length(var.managed_ssl_certificate_domains) == 0)
    error_message = "Set certificate_map, or ssl_certificates and managed_ssl_certificate_domains, not both: a proxy takes certificates from one or the other."
  }
}

variable "ssl_certificates" {
  description = "Self links of existing Compute Engine SSL certificates."
  type        = list(string)
  default     = []
  nullable    = false
}

variable "managed_ssl_certificate_domains" {
  description = <<-EOT
    Domains for a Google-managed Compute Engine certificate, created here. It is
    issued once each domain resolves to the load balancer. It cannot cover a
    wildcard; use certificate_map for one.
    EOT
  type        = list(string)
  default     = []
  nullable    = false

  validation {
    condition     = alltrue([for domain in var.managed_ssl_certificate_domains : !startswith(domain, "*.")])
    error_message = "managed_ssl_certificate_domains cannot include a wildcard. Use managed-certificate and certificate_map instead."
  }
}

variable "ssl_policy" {
  description = "Self link of an SSL policy for the HTTPS proxy. Null uses Google's default policy."
  type        = string
  default     = null
}

variable "https_redirect" {
  description = "Whether to redirect HTTP requests to HTTPS. Ignored without a certificate, as the load balancer then serves HTTP only."
  type        = bool
  default     = true
}

variable "labels" {
  description = "Labels to add to the resources."
  type        = map(string)
  default     = {}
}
