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

variable "crypto_key_id" {
  description = "The id of the CryptoKey to grant on, of the form projects/PROJECT/locations/LOCATION/keyRings/RING/cryptoKeys/KEY. Take this from a kms-key or pre-existing-kms-key module rather than writing it out, so the grants are ordered against the key."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^projects/[^/]+/locations/[^/]+/keyRings/[a-zA-Z0-9_-]{1,63}/cryptoKeys/[a-zA-Z0-9_-]{1,63}$", var.crypto_key_id))
    error_message = "crypto_key_id must be a full CryptoKey id of the form projects/<project>/locations/<location>/keyRings/<ring>/cryptoKeys/<key>."
  }
}

variable "project_id" {
  description = "The project whose service agents are granted on the key. This is the project holding the resources being encrypted, which need not be the project holding the key."
  type        = string
  nullable    = false
}

variable "service_agents" {
  description = <<-EOT
    Short names of the Google service agents to grant on the key. Their
    addresses are derived from project_id, so the project number does not
    have to be looked up and pasted in. One of:

      compute            Compute Engine disks, images and snapshots
      storage            Cloud Storage buckets and objects
      filestore          Filestore instances
      cloudsql           Cloud SQL instances
      artifactregistry   Artifact Registry repositories
      secretmanager      Secret Manager secrets
      pubsub             Pub/Sub topics
      notebooks          Vertex AI Workbench instances

    Grant `compute` for Slurm boot and additional disks: with
    disk_encryption_key_service_account left unset, Compute Engine
    encrypts as its own service agent rather than as the instance's
    service account.
    EOT
  type        = set(string)
  default     = []
  nullable    = false

  validation {
    condition = alltrue([for a in var.service_agents : contains([
      "compute", "storage", "filestore", "cloudsql",
      "artifactregistry", "secretmanager", "pubsub", "notebooks",
    ], a)])
    error_message = "Each value in service_agents must be one of: compute, storage, filestore, cloudsql, artifactregistry, secretmanager, pubsub, notebooks."
  }
}

variable "custom_service_accounts" {
  description = "Bare email addresses of user-managed service accounts to grant roles/cloudkms.cryptoKeyEncrypterDecrypter on the CryptoKey, e.g. \"my-sa@my-project.iam.gserviceaccount.com\". Use this for identities service_agents cannot derive: a custom disk_encryption_key_service_account, or an agent belonging to a different project. Do not include the \"serviceAccount:\" prefix; the module adds it. Unioned with service_agents; each account must already exist."
  type        = list(string)
  default     = []
  nullable    = false

  validation {
    condition     = alltrue([for a in var.custom_service_accounts : !startswith(a, "serviceAccount:")])
    error_message = "custom_service_accounts must be bare email addresses, not principal strings -- omit the \"serviceAccount:\" prefix, which the module adds automatically."
  }

  validation {
    condition     = alltrue([for a in var.custom_service_accounts : can(regex("^[^@[:space:]]+@[^@[:space:]]+\\.gserviceaccount\\.com$", a))])
    error_message = "Each value in custom_service_accounts must be a service account email ending in \".gserviceaccount.com\", e.g. my-sa@my-project.iam.gserviceaccount.com."
  }
}

variable "skip_iam_role_grants" {
  description = <<-EOT
    Skip creating the IAM grants this module normally creates, while every
    output still resolves crypto_key_id as usual. Set this when permissions
    on the key are managed out-of-band by someone else -- for example a
    security team granting roles/cloudkms.cryptoKeyEncrypterDecrypter on a
    pre-existing-kms-key key directly -- and the identity running this
    module lacks cloudkms.admin/setIamPolicy on it. Without this, Terraform
    would attempt the grant anyway and fail with a 403, even though the
    caller only wanted this module's `use`-wiring convenience.

    service_agents and custom_service_accounts must both be empty when this
    is true. Setting either alongside skip_iam_role_grants would otherwise
    look like a grant request that silently does nothing, which is exactly
    the kind of surprising behavior this variable exists to prevent.
    EOT
  type        = bool
  default     = false
  nullable    = false

  validation {
    condition = !var.skip_iam_role_grants || (
      length(var.service_agents) == 0 && length(var.custom_service_accounts) == 0
    )
    error_message = "service_agents and custom_service_accounts must be empty when skip_iam_role_grants is true -- there is nothing to grant when this module isn't managing IAM."
  }
}
