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

variable "control_bucket_location" {
  type        = string
  default     = ""
  description = <<-EOT
    Location of the control bucket. Empty: the bucket is created in region.
    deploy.sh sets it to an existing bucket's location when it is run again
    on a deployment, because Terraform replaces a bucket whose location
    changes, deleting its files.
    EOT
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
  nullable    = false
  validation {
    condition     = length(var.deployment_name) >= 3 && length(var.deployment_name) <= 24 && can(regex("^[a-z][-a-z0-9]*[a-z0-9]$", var.deployment_name))
    error_message = "The variable 'deployment_name' must be 3 to 24 characters long, contain only a-z (lowercase), 0-9 and '-', start with a letter and end with a letter or digit, so that the service account ID \"<name>-fe-sa\" fits within 30 characters."
  }
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
  default     = "n2d-standard-2"
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

variable "extra_labels" {
  type        = map(any)
  default     = {}
  description = "Extra labels to apply to created GCP resources."
}

variable "provisioning_service_account" {
  type        = string
  default     = ""
  description = <<-EOT
    E-mail address of the keyless service account, in project_id, that the web
    server acts as to create clusters and other resources (deploy.sh creates
    <deployment_name>-prov-sa). The web server's account is granted the
    Service Account Token Creator role on this account only, and the server
    reads the address from its instance metadata. Empty: no account is set and
    no grant is made.
  EOT

  validation {
    condition = (
      var.provisioning_service_account == "" || (
        can(regex("^[a-z][-a-z0-9]{4,28}[a-z0-9]@", var.provisioning_service_account)) &&
        endswith(var.provisioning_service_account, "@${var.project_id}.iam.gserviceaccount.com")
      )
    )
    error_message = "provisioning_service_account must be empty or the e-mail address of a service account in project ${var.project_id}: <name>@${var.project_id}.iam.gserviceaccount.com."
  }
}

variable "terraform_state_bucket" {
  type        = string
  description = <<-EOT
    Name of the GCS bucket holding Terraform state for resources the front end
    provisions, created by the separate tf/state_bucket root and passed in by
    deploy.sh. No default: silently falling back to local state would bring
    back the risk this bucket exists to remove.
  EOT
  validation {
    condition     = length(trimspace(var.terraform_state_bucket)) > 0
    error_message = "The variable 'terraform_state_bucket' must not be empty. Apply tf/state_bucket first and pass its bucket_name output."
  }
}
