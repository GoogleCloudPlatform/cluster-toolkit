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

# This module creates a key; it does not grant anyone access to it. Pass
# crypto_key_id to a kms-key-iam module, and have CMEK consumers `use`
# that module. Consuming this one directly gets the key id as soon as the
# key exists -- before any service agent can use it -- which surfaces as
# an intermittent KMS PERMISSION_DENIED that depends only on how
# Terraform happened to schedule the two.

output "crypto_key_id" {
  description = "The canonical resource ID of the CryptoKey. Pass this to a kms-key-iam module; CMEK consumers should `use` that module rather than this one, so they are ordered behind the grants."
  value       = google_kms_crypto_key.this.id
}

output "key_ring_id" {
  description = "The canonical resource ID of the Cloud KMS key ring holding the CryptoKey, whether this module created it or adopted an existing one."
  value       = local.key_ring_id
}

output "primary_crypto_key_version_id" {
  description = "The resource name of the CryptoKey's current primary CryptoKeyVersion."

  # Indexing [0] is safe only because the key is always ENCRYPT_DECRYPT with an
  # initial version: Cloud KMS leaves `primary` unset for other purposes, which
  # would make this fail with an index error. If a `purpose` input is ever
  # added, this must become a conditional (for example a one() or try()).
  value = google_kms_crypto_key.this.primary[0].name
}
