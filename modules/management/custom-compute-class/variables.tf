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
  description = "The GCP project ID. If omitted, inferred from cluster_id."
  type        = string
  default     = null
}

variable "cluster_id" {
  description = "The GKE cluster ID (e.g. projects/PROJECT_ID/locations/LOCATION/clusters/CLUSTER_NAME)."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^projects/[^/]+/locations/[^/]+/clusters/[^/]+$", var.cluster_id))
    error_message = "The cluster_id must be in the format projects/PROJECT_ID/locations/LOCATION/clusters/CLUSTER_NAME."
  }
}

variable "cluster_endpoint" {
  description = "Cluster control plane endpoint host."
  type        = string
  default     = null
}

variable "access_token" {
  description = "GCP OAuth2 access token for authenticating to Kubernetes API."
  type        = string
  default     = null
  sensitive   = true
}

variable "cluster_ca_certificate" {
  description = "Base64 encoded cluster CA certificate."
  type        = string
  default     = null
}

variable "compute_classes" {
  description = "List of Custom Compute Classes to template and apply."
  type = list(object({
    name                    = string
    description             = optional(string, "")
    node_pool_auto_creation = optional(bool, true)
    autopilot               = optional(bool, false)

    priorities = list(object({
      machine_family        = string
      spot                  = optional(bool, false)
      min_cores             = optional(number)
      min_memory_gb         = optional(number)
      min_cpu_platform      = optional(string)
      placement_policy_name = optional(string)
      zones                 = optional(list(string), [])
    }))

    node_pool_config = optional(object({
      image_type      = optional(string, "cos_containerd")
      service_account = optional(string)
      node_labels     = optional(map(string), {})
      taints = optional(list(object({
        key    = string
        value  = string
        effect = string
      })), [])
      gvnic_enabled   = optional(bool, true)
      image_streaming = optional(bool, true)
    }))

    autoscaling_policy = optional(object({
      consolidation_delay_minutes = optional(number, 1)
      consolidation_threshold     = optional(number, 0)
    }))
  }))
  default  = []
  nullable = false
}

variable "custom_manifest_path" {
  description = "Optional path to a custom ComputeClass YAML file or template."
  type        = string
  default     = null
}

variable "custom_template_vars" {
  description = "Optional template variables for custom_manifest_path if it is a templatefile."
  type        = any
  default     = {}
  nullable    = false
}

variable "module_id" {
  description = "The ID of the module as defined in the blueprint. Injected by ghpc."
  type        = string
  default     = "custom-compute-class"
  nullable    = false
}
