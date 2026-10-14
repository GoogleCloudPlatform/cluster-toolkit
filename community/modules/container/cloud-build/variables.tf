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
}

variable "region" {
  description = "GCP region where Artifact Registry and Cloud Build execute."
  type        = string
  default     = "us-central1"
}

variable "cloud_build_dir" {
  description = "Directory containing source code for Cloud Build. When repo_url is set, this is the relative subfolder in the remote repository (e.g. 'src'). When repo_url is omitted, this is the local directory uploaded to Cloud Build."
  type        = string
  default     = "."
}

variable "target_dir" {
  description = "Optional path to a target or overlay directory (either inside the remote Git repository or on the local machine, controlled by is_target_dir_local). Automatically exposed as _TARGET_DIR in template_vars."
  type        = string
  default     = null
}

variable "is_target_dir_local" {
  description = "Whether target_dir resides on the local machine. When true, target_dir is staged from the local filesystem and uploaded to Cloud Build. When false, target_dir is assumed to reside inside the remote Git repository."
  type        = bool
  default     = false
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

variable "service_account_email" {
  description = "Optional service account email to execute Cloud Build."
  type        = string
  default     = null
  validation {
    condition     = var.service_account_email == null || var.service_account_email == "" || can(regex("^[^@]+@[^@]+\\.[^@]+$", var.service_account_email))
    error_message = "service_account_email must be null, empty, or a full service account email address."
  }
}

variable "repo_url" {
  description = "Optional Git repository URL (e.g. https://github.com/my-org/my-repo.git). When specified, Cloud Build runs with --no-source (unless target_dir is provided with is_target_dir_local = true) and the repository is cloned directly within the Cloud Build pipeline."
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
  description = "List of container image URLs to check in Artifact Registry before building. If all images exist, the build is skipped. Note that this feature only checks if the image/tag exists in Artifact Registry, it does not detect content changes. Only use with immutable or versioned/content-hashed tags (e.g., :v1.0.0 or :sha256-...), and never with mutable tags like :latest."
  type        = list(string)
  default     = []
}
