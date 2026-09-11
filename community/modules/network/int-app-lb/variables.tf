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
  description = "The project ID to deploy to."
  type        = string
  validation {
    condition     = can(regex("^[a-z0-9-]{6,30}$", var.project_id))
    error_message = "The project_id must be between 6 and 30 characters and only contain lowercase letters, numbers, and hyphens."
  }
}

variable "region" {
  description = "The region to deploy to."
  type        = string
  validation {
    condition     = can(regex("^[a-z]+-[a-z0-9]+$", var.region))
    error_message = "The region must be a valid Google Cloud region name (e.g., us-central1)."
  }
}

variable "network_self_link" {
  description = "The network self-link to attach the load balancer to."
  type        = string
}

variable "subnetwork_self_link" {
  description = "The subnetwork self-link to attach the load balancer to."
  type        = string
}


variable "lb_name" {
  description = "The name for the Load Balancer resources."
  type        = string
  default     = "internal-app-lb"
  validation {
    condition     = can(regex("^[a-z]([-a-z0-9]*[a-z0-9])?$", var.lb_name)) && length(var.lb_name) <= 63
    error_message = "The lb_name must be a valid resource name: 1-63 lowercase alphanumeric characters or hyphens, starting with a letter and ending with an alphanumeric character."
  }
}

variable "labels" {
  description = "Labels to add to the resources."
  type        = map(string)
  default     = {}
}

variable "ssl_certificates" {
  description = "List of SSL certificate self-links. If provided, the load balancer will be configured to use HTTPS."
  type        = list(string)
  default     = []
}

variable "backends" {
  description = "List of instance group self-links to use as backends for all endpoints."
  type        = list(string)
  validation {
    condition     = length(var.backends) > 0
    error_message = "You must provide at least one backend instance group."
  }
}

variable "endpoints" {
  description = "Map of endpoint names (used as named_ports) to their listening port and URL paths."
  type = map(object({
    port                      = number
    paths                     = list(string)
    health_check_type         = optional(string, "HTTP")
    health_check_request_path = optional(string, "/")
  }))
  validation {
    condition     = length(var.endpoints) > 0
    error_message = "You must provide at least one endpoint."
  }
  validation {
    condition     = alltrue([for k, v in var.endpoints : contains(["HTTP", "TCP"], coalesce(v.health_check_type, "HTTP"))])
    error_message = "The health_check_type for all endpoints must be either 'HTTP' or 'TCP'."
  }
}

variable "protocol" {
  description = "The protocol to use to talk to the backend. Can be HTTP, HTTPS, or HTTP2."
  type        = string
  default     = "HTTP"
  validation {
    condition     = contains(["HTTP", "HTTPS", "HTTP2"], var.protocol)
    error_message = "The protocol must be one of HTTP, HTTPS, or HTTP2."
  }
}

variable "timeout_sec" {
  description = "How long (in seconds) to wait for backend response."
  type        = number
  default     = 30
}

variable "connection_draining_timeout_sec" {
  description = "Time (in seconds) to wait for connections to drain before dropping them during VM termination."
  type        = number
  default     = 30
}

variable "log_config" {
  description = "Logging configuration for this backend service."
  type = object({
    enable      = bool
    sample_rate = optional(number, 1.0)
  })
  default = null
}

variable "iap_config" {
  description = "Settings for enabling Identity-Aware Proxy (IAP) on the backend service."
  type = object({
    oauth2_client_id     = string
    oauth2_client_secret = string
  })
  default   = null
  sensitive = true
}

variable "health_check_interval_sec" {
  description = "How often (in seconds) to send a health check."
  type        = number
  default     = 10
}

variable "health_check_timeout_sec" {
  description = "How long (in seconds) to wait before claiming failure."
  type        = number
  default     = 5
}

variable "healthy_threshold" {
  description = "A consecutive number of successful checks before marking as healthy."
  type        = number
  default     = 2
}

variable "unhealthy_threshold" {
  description = "A consecutive number of failed checks before marking as unhealthy."
  type        = number
  default     = 3
}

variable "ip_address" {
  description = "The static internal IP address to assign to the forwarding rule. If null, one will be automatically assigned."
  type        = string
  default     = null
}
