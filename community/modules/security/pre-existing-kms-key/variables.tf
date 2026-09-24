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
  description = "The project holding the Cloud KMS key ring. This is the key project, which need not be the project the encrypted resources live in."
  type        = string
  nullable    = false
}

variable "location" {
  description = "The Cloud KMS location of the key ring, e.g. \"us-central1\" or \"global\". A key can only encrypt resources in its own location, except for \"global\" keys."
  type        = string
  nullable    = false
}

variable "key_ring_name" {
  description = "The name of the existing Cloud KMS key ring holding the CryptoKey."
  type        = string
  nullable    = false
}

variable "key_name" {
  description = "The name of the existing symmetric CryptoKey."
  type        = string
  nullable    = false
}
