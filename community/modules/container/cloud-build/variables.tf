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
  description = "GCP project ID where Cloud Build executes."
  type        = string
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{4,28}[a-z0-9]$", var.project_id))
    error_message = "Project ID must be 6 to 30 characters long, start with a lowercase letter, end with a letter or digit, and contain only lowercase letters, digits, and hyphens."
  }
}

variable "region" {
  description = "GCP region where Artifact Registry and Cloud Build execute."
  type        = string
  default     = "us-central1"
  validation {
    condition     = can(regex("^[a-z0-9-]+$", var.region))
    error_message = "Region must be a valid GCP region identifier (e.g. 'us-central1', 'europe-west1')."
  }
}

variable "cloud_build_dir" {
  description = "Directory containing source code for Cloud Build. When repo_url is set, this is the relative subfolder in the remote repository (e.g. 'src'). When repo_url is omitted, this is the local directory uploaded to Cloud Build."
  type        = string
  default     = "."
}

variable "cloud_build_template_path" {
  description = "Path to the cloudbuild.yaml template file (.tfpl or .yaml) to be rendered with template_vars."
  type        = string
  default     = null
}

variable "cloud_build_content" {
  description = "Raw YAML content for Cloud Build config (alternative to cloud_build_template_path)."
  type        = string
  default     = null
}

variable "template_vars" {
  description = "Map of variables to pass into templatefile when rendering cloud_build_template_path."
  type        = map(any)
  default     = {}
}

variable "substitutions" {
  description = "Map of Cloud Build substitution variables (_KEY=VALUE)."
  type        = map(string)
  default     = {}
  validation {
    condition     = alltrue([for k in keys(var.substitutions) : can(regex("^_", k))])
    error_message = "All Cloud Build substitution keys must start with an underscore (e.g. '_MY_VAR')."
  }
}

variable "gcs_staging_dir" {
  description = "Optional GCS bucket path for Cloud Build source staging (e.g. gs://bucket/staging)."
  type        = string
  default     = null
  validation {
    condition     = var.gcs_staging_dir == null || can(regex("^gs://[a-z0-9_.-]+(/.*)?$", var.gcs_staging_dir))
    error_message = "gcs_staging_dir must be null or a valid GCS URI starting with 'gs://'."
  }
}

variable "service_account" {
  description = "Optional service account email to execute Cloud Build."
  type        = string
  default     = null
  validation {
    condition     = var.service_account == null || var.service_account == "" || can(regex("^[a-z0-9-]+@[a-z0-9-]+\\.iam\\.gserviceaccount\\.com$", var.service_account)) || can(regex("^[a-zA-Z0-9_-]+$", var.service_account))
    error_message = "service_account must be null, empty, a full service account email, or a service account name."
  }
}

variable "repo_url" {
  description = "Optional Git repository URL (e.g. https://github.com/GoogleCloudPlatform/scientific-computing-examples.git). When specified, Cloud Build runs with --no-source and the repository is cloned directly within the Cloud Build pipeline."
  type        = string
  default     = null
}

variable "repo_ref" {
  description = "Git branch name, release tag, or commit SHA to check out when repo_url is specified."
  type        = string
  default     = "main"
}

variable "triggers" {
  description = "A map of arbitrary strings or outputs that, when changed, force this module to re-run."
  type        = map(string)
  default     = {}
}

variable "skip_if_exists" {
  description = "List of container image URLs to check in Artifact Registry before building. If all images exist, the build is skipped. Note that his feature only checks if the image/tag exists in Artifact Registry, it does not detect content changes. Only use with immutable or versioned/content-hashed tags (e.g., :v1.0.0 or :sha256-...), and never with mutable tags like :latest."
  type        = list(string)
  default     = []
}
