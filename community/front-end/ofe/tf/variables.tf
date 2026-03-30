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
  description = "GCP Project in which to deploy the HPC Frontend."
}

variable "region" {
  type        = string
  description = "GCP Region for HPC Frontend deployment."
}

variable "zone" {
  type        = string
  description = "GCP Zone for HPC Frontend deployment."
}

variable "subnet" {
  type        = string
  default     = ""
  description = "Subnet in which to deploy HPC Frontend."
}

variable "static_ip" {
  type        = string
  default     = ""
  description = "Optional pre-configured static IP for HPC Frontend."
}

variable "deployment_name" {
  description = "Base \"name\" for the deployment."
  type        = string
}

variable "webserver_hostname" {
  description = "DNS Hostname for the webserver"
  default     = ""
  type        = string
}

variable "django_su_username" {
  description = "DJango Admin SuperUser username"
  type        = string
  default     = "admin"
}

variable "django_su_password" {
  description = "DJango Admin SuperUser password"
  type        = string
  sensitive   = true
}

variable "django_su_email" {
  description = "DJango Admin SuperUser email"
  type        = string
}

variable "server_instance_type" {
  default     = "e2-standard-2"
  type        = string
  description = "Instance size to use from HPC Frontend webserver"
}

variable "deployment_mode" {
  type        = string
  description = "Use a tarball of this directory, or download from git to deploy the server. Must be either 'tarball' or 'git'"
  default     = "tarball"
  validation {
    condition     = var.deployment_mode == "tarball" || var.deployment_mode == "git"
    error_message = "The variable 'deployment_mode' must be either 'tarball' or 'git'."
  }
}

variable "repo_branch" {
  default     = "main"
  type        = string
  description = "git branch to checkout when deploying the HPC Frontend"
}

variable "repo_fork" {
  default     = "GoogleCloudPlatform"
  type        = string
  description = "GitHub repository name in which to find the cluster-toolkit repo"
}

variable "deployment_key" {
  default     = ""
  type        = string
  description = "Name to identify resources from this deployment"
}


variable "extra_labels" {
  type        = map(any)
  default     = {}
  description = "Extra labels to apply to created GCP resources."
}
