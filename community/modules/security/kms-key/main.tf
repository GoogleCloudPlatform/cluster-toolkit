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

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "kms-key", ghpc_role = "security" })
}

locals {
  # Exactly one of key_ring_name or key_ring_id is set, enforced by variable
  # validation, so the ring is either created here or adopted from the caller.
  create_key_ring = var.key_ring_id == null
  key_ring_id     = local.create_key_ring ? one(google_kms_key_ring.this[*].id) : var.key_ring_id
}

# Created only when the caller did not supply an existing ring. Adopting a ring
# matters because Cloud KMS key rings can never be deleted: without this,
# every deployment would strand another permanent ring, and a deployment torn
# down and recreated under the same name would fail on the retained ring.
resource "google_kms_key_ring" "this" {
  count = local.create_key_ring ? 1 : 0

  project  = var.project_id
  location = var.location
  name     = var.key_ring_name
}

resource "google_kms_crypto_key" "this" {
  name     = var.key_name
  key_ring = local.key_ring_id
  purpose  = "ENCRYPT_DECRYPT"

  rotation_period            = var.rotation_period
  destroy_scheduled_duration = var.destroy_scheduled_duration
  labels                     = local.labels

  # Defaults to ABANDON: ordinary blueprint teardown must not destroy key
  # material, because data encrypted with this key may outlive the
  # deployment. Note the consequence of ABANDON -- any change that forces
  # replacement of this resource (for example protection_level) cannot
  # succeed, since Cloud KMS will not reuse the abandoned name.
  deletion_policy = var.deletion_policy

  version_template {
    algorithm        = "GOOGLE_SYMMETRIC_ENCRYPTION"
    protection_level = var.protection_level
  }
}
