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

data "google_kms_key_ring" "this" {
  project  = var.project_id
  location = var.location
  name     = var.key_ring_name
}

data "google_kms_crypto_key" "this" {
  key_ring = data.google_kms_key_ring.this.id
  name     = var.key_name

  lifecycle {
    # A key that is absent, or present but not usable for symmetric
    # encryption, fails here rather than when the first resource that
    # depends on it is created -- by which point the error names the
    # consumer rather than the key.
    postcondition {
      condition     = self.purpose == "ENCRYPT_DECRYPT"
      error_message = "CryptoKey ${var.key_name} has purpose ${self.purpose}; CMEK requires ENCRYPT_DECRYPT."
    }
  }
}
