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

# These deliberately match kms-key's outputs, so a blueprint can swap
# between creating a key and adopting one without changing anything
# downstream. Neither module grants anything, so pass crypto_key_id to a
# kms-key-iam module and have consumers `use` that.

output "crypto_key_id" {
  description = "The canonical resource ID of the adopted CryptoKey. Pass this to a kms-key-iam module; CMEK consumers should `use` that module rather than this one, so they are ordered behind the grants."
  value       = data.google_kms_crypto_key.this.id
}

output "key_ring_id" {
  description = "The canonical resource ID of the key ring holding the CryptoKey."
  value       = data.google_kms_key_ring.this.id
}

output "primary_crypto_key_version_id" {
  description = "The resource name of the CryptoKey's current primary CryptoKeyVersion."

  # Indexing [0] is safe because the postcondition in main.tf rejects any
  # key that is not ENCRYPT_DECRYPT, and Cloud KMS always reports a
  # primary version for those.
  value = data.google_kms_crypto_key.this.primary[0].name
}
