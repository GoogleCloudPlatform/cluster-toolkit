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

# CMEK consumers must `use` THIS module, not the module that created the key.
#
# Every output here is the same CryptoKey id, ordered behind the grants above.
# A consumer that takes the id straight from kms-key gets it as soon as the key
# exists -- before its service agent can use it -- which surfaces as an
# intermittent KMS PERMISSION_DENIED that depends only on how Terraform happened
# to schedule the two. depends_on also reverses the order on destroy, so the
# consumer is removed before the grant it relies on.
#
# The several names exist because Cluster Toolkit's `use` keyword matches a used
# module's OUTPUT NAME against a using module's INPUT VARIABLE NAME, exactly
# (pkg/config/expand.go, useModule). Without a name per consumer,
# `use: [kms_key_iam]` matches nothing and the blueprint is rejected by the
# test_module_not_used validator. They are not redundant; deleting one breaks
# `use` for its consumer.
#
# depends_on is per-output and is not inherited, so each one repeats it.
#
# When var.skip_iam_role_grants is true, google_kms_crypto_key_iam_member.this
# has zero instances (see main.tf), so every depends_on below is vacuous and
# each output simply returns var.crypto_key_id immediately -- this is
# intentional, not a gap: skip_iam_role_grants exists for callers whose
# permissions are managed out-of-band, so there is no grant here to order
# behind in the first place.

output "crypto_key_id" {
  description = "The CryptoKey id, ordered after the encrypt/decrypt grants. This is the output to pass to any CMEK-consuming module whose input is not named below."
  value       = var.crypto_key_id

  depends_on = [google_kms_crypto_key_iam_member.this]
}

output "kms_key_name" {
  description = "The CryptoKey id under the name used by CMEK-capable storage modules, so `use` wires it automatically. Consumed by modules/file-system/filestore's kms_key_name."
  value       = var.crypto_key_id

  depends_on = [google_kms_crypto_key_iam_member.this]
}

output "disk_encryption_key" {
  description = "The CryptoKey id under the name used by Slurm instance modules, so `use` wires it automatically. Consumed by the schedmd-slurm-gcp-v6 controller, login and nodeset modules to encrypt boot disks. Leave their disk_encryption_key_service_account unset, so Compute Engine encrypts as the project's Compute Engine service agent -- that agent, not the instances' own service account, is the principal that needs the grant."
  value       = var.crypto_key_id

  depends_on = [google_kms_crypto_key_iam_member.this]
}

output "image_encryption_key" {
  description = "The CryptoKey id under the name used by the custom-image Packer module, so `use` wires it automatically. Consumed by modules/packer/custom-image's image_encryption_key. Grant the `compute` service agent: a Packer build creates a Compute Engine disk and then an image from it."
  value       = var.crypto_key_id

  depends_on = [google_kms_crypto_key_iam_member.this]
}

output "repository_kms_key_name" {
  description = "The CryptoKey id under the name used by the Artifact Registry module, so `use` wires it automatically. Consumed by community/modules/container/artifact-registry's repository_kms_key_name. Grant the `artifactregistry` service agent, and note the key location must match the repository location."
  value       = var.crypto_key_id

  depends_on = [google_kms_crypto_key_iam_member.this]
}

output "encryption_key_name" {
  description = "The CryptoKey id under the name used by the Cloud SQL federation module, so `use` wires it automatically. Consumed by community/modules/database/slurm-cloudsql-federation's encryption_key_name. Grant the `cloudsql` service agent."
  value       = var.crypto_key_id

  depends_on = [google_kms_crypto_key_iam_member.this]
}

output "slurm_bucket_kms_key" {
  description = "The CryptoKey id under the name used by the Slurm controller for its configuration bucket, so `use` wires it automatically. Consumed by schedmd-slurm-gcp-v6-controller's slurm_bucket_kms_key. The bucket is written by Cloud Storage, so that project's Cloud Storage service agent needs the grant."
  value       = var.crypto_key_id

  depends_on = [google_kms_crypto_key_iam_member.this]
}
