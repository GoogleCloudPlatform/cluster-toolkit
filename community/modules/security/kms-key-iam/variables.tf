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

variable "service_agent_principals" {
  description = "Fully qualified principal strings granted roles/cloudkms.cryptoKeyEncrypterDecrypter on the CryptoKey, e.g. \"serviceAccount:service-PROJECT_NUMBER@compute-system.iam.gserviceaccount.com\". Use this for principals service_agents cannot derive: agents belonging to a different project, or a user-managed service account. Unioned with service_agents; each principal must already exist."
  type        = set(string)
  default     = []
  nullable    = false

  validation {
    condition     = alltrue([for p in var.service_agent_principals : startswith(p, "serviceAccount:")])
    error_message = "Every value in service_agent_principals must begin with \"serviceAccount:\"."
  }
}
