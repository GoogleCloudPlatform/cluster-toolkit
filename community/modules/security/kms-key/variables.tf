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
  description = "The project in which to create the Cloud KMS key ring and CryptoKey. Pass the workload project for co-located keys, or a dedicated key project for centralized keys."
  type        = string
  nullable    = false
}

variable "location" {
  description = "The Cloud KMS location (region or multi-region) in which to create the key ring, e.g. \"us-central1\" or \"us\". Must be a location that can serve the resources being encrypted; Cloud KMS validates it."
  type        = string
  nullable    = false
}

variable "key_ring_name" {
  description = "The permanent name of a Cloud KMS key ring to create. Cloud KMS key ring names cannot be changed or reused once created. Leave null when adopting an existing ring with key_ring_id."
  type        = string
  default     = null

  validation {
    condition     = var.key_ring_name == null || length(trimspace(coalesce(var.key_ring_name, " "))) > 0
    error_message = "key_ring_name must not be empty; the key ring name is permanent and cannot be renamed later."
  }

  # Checked here rather than left to the API because the name is permanent:
  # a rejected name should cost nothing, not surface partway through an apply.
  validation {
    condition     = var.key_ring_name == null || can(regex("^[a-zA-Z0-9_-]{1,63}$", coalesce(var.key_ring_name, "x")))
    error_message = "key_ring_name must match [a-zA-Z0-9_-]{1,63} (letters, digits, underscores and hyphens only, at most 63 characters), as required by Cloud KMS."
  }
}

variable "key_ring_id" {
  description = <<-EOT
    The id of an existing Cloud KMS key ring to create the CryptoKey in, for
    example "projects/my-project/locations/us-central1/keyRings/my-keyring".
    Set this instead of key_ring_name to reuse a key ring rather than create
    one, which is what makes it possible to hold many CryptoKeys in a single
    long-lived ring and to redeploy after a teardown that retained the ring.
    Exactly one of key_ring_name or key_ring_id must be supplied.
    EOT
  type        = string
  default     = null

  # Cross-variable validation, so the mistake is caught at plan time rather
  # than as a confusing "key ring already exists" or missing-ring failure.
  validation {
    condition     = (var.key_ring_name == null) != (var.key_ring_id == null)
    error_message = "Supply exactly one of key_ring_name (to create a key ring) or key_ring_id (to use an existing one)."
  }

  validation {
    condition     = var.key_ring_id == null || can(regex("^projects/[^/]+/locations/[^/]+/keyRings/[a-zA-Z0-9_-]{1,63}$", var.key_ring_id))
    error_message = "key_ring_id must be a full key ring id of the form projects/<project>/locations/<location>/keyRings/<name>."
  }

  # An adopted ring already encodes its own project and location, so nothing
  # reads project_id or location in that mode. Rather than let a contradictory
  # value pass unnoticed, require that they agree with the ring being adopted.
  #
  # try() rather than a bare index: a malformed id has fewer than four
  # segments, and indexing past the end is an evaluation error that aborts
  # the plan before any validation message is printed -- so the caller
  # would never see the format error above, only a Terraform crash.
  validation {
    condition     = var.key_ring_id == null || try(split("/", var.key_ring_id)[1], "") == var.project_id
    error_message = "key_ring_id names a different project than project_id. The CryptoKey is created inside the supplied key ring, so project_id must be that ring's project."
  }

  validation {
    condition     = var.key_ring_id == null || try(split("/", var.key_ring_id)[3], "") == var.location
    error_message = "key_ring_id names a different location than location. The CryptoKey inherits its ring's location, so location must be that ring's location."
  }
}

variable "key_name" {
  description = "The permanent name of the symmetric CryptoKey. Cloud KMS CryptoKey names cannot be renamed and cannot be reused once destroyed."
  type        = string
  nullable    = false

  validation {
    condition     = length(trimspace(var.key_name)) > 0
    error_message = "key_name must not be empty; the CryptoKey name is permanent and cannot be renamed later."
  }

  # Checked here rather than left to the API because the name is permanent,
  # and because the key ring is created first: an invalid key name would
  # otherwise fail only after the ring already exists.
  validation {
    condition     = can(regex("^[a-zA-Z0-9_-]{1,63}$", var.key_name))
    error_message = "key_name must match [a-zA-Z0-9_-]{1,63} (letters, digits, underscores and hyphens only, at most 63 characters), as required by Cloud KMS."
  }
}

variable "protection_level" {
  # Changing this after creation cannot succeed: it is immutable in the Cloud
  # KMS API, so Terraform plans a replacement, and the retained CryptoKey then
  # blocks recreating the same name. The README's "Lifecycle and naming"
  # section explains the failure and the terraform import recovery.
  description = "The protection level for new CryptoKeyVersions, either \"SOFTWARE\" or \"HSM\". Chosen at creation and effectively immutable afterwards; use a new key_name to change it. See the module README."
  type        = string
  default     = "SOFTWARE"
  nullable    = false

  validation {
    condition     = contains(["SOFTWARE", "HSM"], var.protection_level)
    error_message = "protection_level must be one of \"SOFTWARE\" or \"HSM\"."
  }
}

# Both duration inputs below set `nullable = false`. Terraform only substitutes
# a variable's default when the variable is omitted: an explicitly assigned null
# is preserved for a nullable variable. Cloud KMS treats an absent
# rotation_period as "no automatic rotation", so without `nullable = false` a
# caller passing null would silently create a key with automatic rotation
# disabled -- the opposite of this module's default posture, and not something
# that should happen without asking for it by name. Widening these to accept
# null later, to express "no rotation" deliberately, is not a breaking change.
#
# Their format is validated by the provider and the API rather than here: the
# provider rejects a malformed rotation_period during plan, and the API's error
# for destroy_scheduled_duration already names the offending field.
variable "rotation_period" {
  description = "The interval between automatic CryptoKeyVersion rotations, expressed as a duration string ending in \"s\" (seconds), e.g. \"7776000s\" for 90 days. Must be greater than one day (86400s). Cannot currently be disabled."
  type        = string
  default     = "7776000s"
  nullable    = false

  # Cloud KMS rejects anything under 24h, and the provider only accepts a
  # seconds-suffixed duration. Checking here names the variable; failing in
  # the API names the resource.
  validation {
    condition     = can(regex("^[0-9]+s$", var.rotation_period)) && tonumber(trimsuffix(var.rotation_period, "s")) >= 86400
    error_message = "rotation_period must be a duration string of whole seconds ending in \"s\" (e.g. \"7776000s\") and at least 86400s (24 hours)."
  }
}

variable "destroy_scheduled_duration" {
  # Immutable in the Cloud KMS API and ForceNew in the provider, so changing it
  # after creation plans a replacement that cannot succeed: deletion_policy =
  # "ABANDON" retains the old CryptoKey and Cloud KMS will not reuse its name.
  # This is the same trap as protection_level; the README's "Lifecycle and
  # naming" section documents both and the terraform import recovery.
  description = "The period a CryptoKeyVersion spends in DESTROY_SCHEDULED before transitioning to DESTROYED, expressed as a duration string ending in \"s\" (seconds), e.g. \"2592000s\" for 30 days. Chosen at creation and immutable afterwards; use a new key_name to change it. See the module README."
  type        = string
  default     = "2592000s"
  nullable    = false

  # Cloud KMS accepts 24 hours to 120 days. Worth catching at plan time
  # because this input is immutable: a rejected value applied by mistake
  # cannot be corrected in place, only by creating a new key.
  validation {
    condition = can(regex("^[0-9]+s$", var.destroy_scheduled_duration)) && (
      tonumber(trimsuffix(var.destroy_scheduled_duration, "s")) >= 86400 &&
      tonumber(trimsuffix(var.destroy_scheduled_duration, "s")) <= 10368000
    )
    error_message = "destroy_scheduled_duration must be a duration string of whole seconds ending in \"s\" (e.g. \"2592000s\"), between 86400s (24 hours) and 10368000s (120 days)."
  }
}

variable "deletion_policy" {
  description = <<-EOT
    What `terraform destroy` does with the CryptoKey this module created.
    Required -- there is no default. The consequences of each value are
    severe and opposite enough (permanent data loss vs. a key that outlives
    every deployment) that picking one silently, for you, is worse than
    making every blueprint author decide and write it down.

      ABANDON  drop it from Terraform state, leaving the CryptoKey and
               every key version intact and enabled in Cloud KMS
      DELETE   destroy all key versions, rendering data encrypted with
               them permanently unrecoverable

    ABANDON is the recommended choice for most blueprints: key material
    routinely outlives the deployment that created it (a Filestore instance
    or bucket meant to survive `terraform destroy`, a key shared by more
    than this one deployment), and destroying versions cannot be undone.
    Choose DELETE only when you have deliberately decided the data this key
    protects is disposable and should not outlive this deployment -- for
    example short-lived scratch resources recreated from scratch on every
    deploy.

    Neither setting frees the CryptoKey name: Cloud KMS never deletes a
    CryptoKey resource itself, only DELETE additionally destroys its
    version(s). A key adopted with the pre-existing-kms-key module is never
    affected by this variable, since that module never creates a
    google_kms_crypto_key resource for `terraform destroy` to act on.

    Changing this is an in-place update, so it can be set on an existing
    key by re-applying -- unlike protection_level and
    destroy_scheduled_duration, which are fixed at creation.
    EOT
  type        = string
  nullable    = false

  validation {
    condition     = contains(["ABANDON", "DELETE"], var.deletion_policy)
    error_message = "deletion_policy must be one of \"ABANDON\" or \"DELETE\"."
  }
}

variable "labels" {
  description = "Labels to add to the CryptoKey. Key-value pairs. Cloud KMS key rings and IAM members do not support labels."
  type        = map(string)
  default     = {}
  nullable    = false
}
