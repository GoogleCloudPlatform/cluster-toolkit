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
  description = "GCP project ID."
  type        = string
}

variable "region" {
  description = "GCP region where the Colab runtime and template will be provisioned."
  type        = string
}

variable "deployment_name" {
  description = "Deployment name used as prefix for runtime template and runtime IDs."
  type        = string
  validation {
    condition     = can(regex("^[a-zA-Z0-9_-]+$", var.deployment_name))
    error_message = "deployment_name must be a non-empty string containing only alphanumeric characters, hyphens, or underscores."
  }
}

variable "module_instance_id" {
  description = "Unique ID of this module instance (automatically populated by gcluster)."
  type        = string
  default     = "colab"
  validation {
    condition     = can(regex("^[a-zA-Z0-9_-]+$", var.module_instance_id))
    error_message = "module_instance_id must be a non-empty string containing only alphanumeric characters, hyphens, or underscores."
  }
}

variable "runtime_user" {
  description = "The user email for the Colab runtime (required by Colab Enterprise, e.g. 'user@example.com')."
  type        = string
}

variable "mount_gcs_bucket" {
  description = "Optional GCS bucket name to automatically mount to the VM via gcsfuse on startup."
  type        = string
  default     = null
}

variable "mount_path" {
  description = "Local VM directory path where mount_gcs_bucket will be mounted."
  type        = string
  default     = "/home/jupyter/gcs"
}

variable "environment_variables" {
  description = "Map of environment variables to inject into the Colab runtime environment."
  type        = map(string)
  default     = {}
}

variable "post_startup_script_behavior" {
  description = "Execution behavior for post_startup_script ('RUN_ONCE', 'RUN_EVERY_START', 'DOWNLOAD_AND_RUN_EVERY_START')."
  type        = string
  default     = "DOWNLOAD_AND_RUN_EVERY_START"

  validation {
    condition     = contains(["RUN_ONCE", "RUN_EVERY_START", "DOWNLOAD_AND_RUN_EVERY_START"], var.post_startup_script_behavior)
    error_message = "post_startup_script_behavior must be one of: RUN_ONCE, RUN_EVERY_START, DOWNLOAD_AND_RUN_EVERY_START."
  }
}

variable "colab_machine_type" {
  description = "Machine type for the Colab runtime VM (e.g., 'n2-standard-4')."
  type        = string
  default     = "n2-standard-4"
  validation {
    condition     = can(regex("^[a-z0-9-]+$", var.colab_machine_type))
    error_message = "colab_machine_type must be a valid GCP machine type string (e.g. 'n2-standard-4')."
  }
}

variable "accelerator_type" {
  description = "Optional GPU accelerator type (e.g. 'NVIDIA_TESLA_T4', 'NVIDIA_L4')."
  type        = string
  default     = null
}

variable "accelerator_count" {
  description = "Optional number of GPU accelerators to attach to the VM."
  type        = number
  default     = null
}

variable "disk_type" {
  description = "Persistent disk type for the Colab runtime VM ('pd-standard', 'pd-balanced', 'pd-ssd', 'hyperdisk-balanced')."
  type        = string
  default     = "pd-balanced"

  validation {
    condition     = contains(["pd-standard", "pd-balanced", "pd-ssd", "hyperdisk-balanced"], var.disk_type)
    error_message = "disk_type must be one of: pd-standard, pd-balanced, pd-ssd, hyperdisk-balanced."
  }
}

variable "disk_size_gb" {
  description = "Size of the user persistent disk in GB."
  type        = number
  default     = 20
  validation {
    condition     = var.disk_size_gb >= (var.disk_type == "hyperdisk-balanced" ? 4 : 10)
    error_message = "disk_size_gb must be at least 10 GB (or 4 GB for hyperdisk-balanced)."
  }
}

variable "enable_internet_access" {
  description = "Enable internet access for the Colab runtime VM."
  type        = bool
  default     = true
}

variable "network" {
  description = "Optional VPC network resource name or ID for private networking."
  type        = string
  default     = null
}

variable "subnetwork" {
  description = "Optional VPC subnet resource name or ID for private networking."
  type        = string
  default     = null
}

variable "idle_timeout" {
  description = "Optional idle timeout duration after which the runtime is automatically shut down (e.g. '14400s' for 4 hours). Valid range is [10m, 24h] (600s to 86400s). An input of '0s' disables the idle shutdown feature."
  type        = string
  default     = null
  validation {
    condition     = var.idle_timeout == null || can(regex("^[0-9]+s$", var.idle_timeout))
    error_message = "idle_timeout must be a valid duration string ending in 's' (e.g., '3600s', '0s')."
  }
}

variable "labels" {
  description = "Labels to assign to the Colab runtime template."
  type        = map(string)
  default     = {}
}
