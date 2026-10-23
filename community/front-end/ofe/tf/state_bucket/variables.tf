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
  description = "GCP Project in which to create the Terraform state bucket."
}

variable "region" {
  type        = string
  description = "GCP Region (bucket location) for the Terraform state bucket."
}

variable "deployment_name" {
  description = "Base \"name\" for the deployment."
  type        = string
}

variable "state_bucket_writers" {
  type        = list(string)
  default     = []
  description = <<-EOT
    IAM members, in full "serviceAccount:name@project.iam.gserviceaccount.com"
    form, granted roles/storage.objectUser on the state bucket: the identities
    that run Terraform for the front end. Set in admin.auto.tfvars, which
    deploy.sh creates once and never overwrites. Add any provisioning
    credential from another project here, or it cannot read or write its
    resources' state.
  EOT
}

variable "default_writers" {
  type        = list(string)
  default     = []
  description = "Writers managed by deploy.sh: its provisioning service account, when that account exists. Regenerated on every deploy; do not set by hand."
}

variable "noncurrent_version_retention_days" {
  type        = number
  default     = 90
  description = "Days to retain non-current (superseded) state file versions before deletion."
}

variable "extra_labels" {
  type        = map(any)
  default     = {}
  description = "Extra labels to apply to created GCP resources."
}
