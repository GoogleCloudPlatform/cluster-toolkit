# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "project_id" {
  description = "ID of the project the service account and its generated roles live in."
  type        = string
}

variable "deployment_name" {
  description = "Name of the deployment. With name, it builds the generated custom role ids. Use the deployment_name given to whatever created the account."
  type        = string

  validation {
    condition     = can(regex(local.id_part_pattern, var.deployment_name))
    error_message = "deployment_name must be lowercase letters, digits and dashes, not starting or ending with a dash: the generated custom role id is built from it and cannot be made unique otherwise."
  }
}

variable "name" {
  description = "Short name for this identity. With deployment_name, it builds the generated custom role ids. Use the name given to whatever created the account."
  type        = string

  validation {
    condition     = can(regex(local.id_part_pattern, var.name))
    error_message = "name must be lowercase letters, digits and dashes, not starting or ending with a dash: the generated custom role id is built from it and cannot be made unique otherwise."
  }

  validation {
    condition     = length(var.deployment_name) + length(var.name) + length(".scoped") + 1 <= 64
    error_message = "deployment_name and name together are too long: the generated custom role id \"<deployment_name>.<name>.scoped\" may be at most 64 bytes, so deployment_name plus name may be at most ${64 - length(".scoped") - 1} characters."
  }
}

variable "slurm_cluster_name" {
  description = <<-EOT
    For the Slurm controller profile: the same value given to
    schedmd-slurm-gcp-v6-controller's slurm_cluster_name input, if set. Slurm names every
    VM "<slurm_cluster_name>-...", and the controller does not re-normalise an explicit
    value, so passing the identical value here keeps the name-scoped role's instance-name
    condition correct without hand-deriving instance_name_prefixes. Unset mirrors the
    controller's own derived default (the first 10 characters of deployment_name,
    lowercase, letters and digits only). Ignored if instance_name_prefixes is also set.
  EOT
  type        = string
  default     = null

  validation {
    condition     = var.slurm_cluster_name == null || can(regex("^[a-z]([-a-z0-9]{0,19})$", var.slurm_cluster_name))
    error_message = "slurm_cluster_name must match ^[a-z]([-a-z0-9]{0,19})$, the same pattern schedmd-slurm-gcp-v6-controller's own slurm_cluster_name variable requires."
  }
}

variable "service_account_email" {
  description = <<-EOT
    E-mail address of the existing service account to grant this profile's permissions to.
    Usually set by use: [<account module id>] on a modules/project/service-account block, which
    supplies its service_account_email output; can also be set directly to any existing account's
    e-mail. This module does not create service accounts.
  EOT
  type        = string
}

# Cross-input rules live here. A validation may read other variables and
# locals, but not a value derived from its own variable (Terraform reports a
# cycle); such checks are preconditions in main.tf.
variable "profile" {
  description = <<-EOT
    Permission profile for this account, from the module's catalogue: the Slurm controller,
    login and compute profiles, the Packer image builder profile, or the custom profile. See
    the README for what each grants.
  EOT
  type        = string

  validation {
    condition     = contains(keys(local.profiles), var.profile)
    error_message = "profile must be one of: ${join(", ", keys(local.profiles))}."
  }

  validation {
    condition = alltrue([
      for capability in local.enabled_capabilities : contains(try(local.profiles[var.profile].accepted_capabilities, []), capability)
    ])
    error_message = "profile = \"${var.profile}\" does not accept one or more of the enabled capability flags. Accepted for this profile: ${length(try(local.profiles[var.profile].accepted_capabilities, [])) == 0 ? "none" : join(", ", try(local.profiles[var.profile].accepted_capabilities, []))}."
  }

  validation {
    condition     = try(local.profiles[var.profile].accepts_act_as, true) || length(var.act_as_service_accounts) == 0
    error_message = "act_as_service_accounts is accepted only with profile ${join(" or ", [for name, profile in local.profiles : "\"${name}\"" if profile.accepts_act_as])}, which attaches compute service accounts to the nodes it creates, not \"${var.profile}\". No other identity may act as another service account."
  }

  validation {
    condition     = try(local.profiles[var.profile].custom, true) || (var.custom_profile_name == null && length(var.custom_permissions) == 0 && length(var.custom_roles) == 0)
    error_message = "custom_profile_name, custom_permissions and custom_roles are accepted only with profile ${join(" or ", [for name, profile in local.profiles : "\"${name}\"" if profile.custom])}. \"${var.profile}\" grants a fixed permission set; grant data access through the resource-scoped *_bindings variables instead."
  }

  validation {
    condition     = var.slurm_cluster_name == null || var.profile == "slurm-controller"
    error_message = "slurm_cluster_name is accepted only with profile \"slurm-controller\", not \"${var.profile}\"."
  }

  validation {
    condition     = !try(local.profiles[var.profile].custom, false) || var.custom_profile_name != null
    error_message = "profile = \"${var.profile}\" requires custom_profile_name, which names the custom role."
  }

  validation {
    condition = !try(local.profiles[var.profile].custom, false) || anytrue([
      length(var.custom_permissions) > 0,
      length(var.custom_roles) > 0,
      length(local.enabled_capabilities) > 0,
      anytrue([for bindings in values(local.binding_inputs) : length(bindings) > 0]),
    ])
    error_message = "profile = \"${var.profile}\" needs at least one of custom_permissions, custom_roles, an enable_* capability flag or a *_bindings entry; without them it grants nothing beyond logging and monitoring."
  }

  validation {
    condition     = length(try(local.profiles[var.profile].accepted_capabilities, [])) == 0 || length(setintersection(var.custom_permissions, local.capability_grants)) == 0
    error_message = "custom_permissions repeats permissions that an enabled capability flag already grants: ${join(", ", setintersection(var.custom_permissions, local.capability_grants))}. Remove them from custom_permissions."
  }

  validation {
    condition     = var.custom_role_stage == null || length(concat(try(local.profiles[var.profile].permissions, ["?"]), local.capability_grants, var.custom_permissions)) > 0
    error_message = "custom_role_stage is set, but this account gets no generated custom role (its profile, capability flags and custom_permissions grant no permissions), so the stage would have nothing to apply to. Leave custom_role_stage unset."
  }
}

variable "custom_profile_name" {
  description = "For the custom profile: a short name for what this identity is for, used as the custom role's title."
  type        = string
  default     = null

  validation {
    condition     = var.custom_profile_name == null || can(regex("^[A-Za-z0-9][-A-Za-z0-9 _.]{0,63}$", coalesce(var.custom_profile_name, "x")))
    error_message = "custom_profile_name must be 1-64 characters: letters, digits, spaces, '-', '_' and '.', starting with a letter or digit."
  }
}

variable "custom_permissions" {
  description = <<-EOT
    For the custom profile: IAM permissions (e.g. "compute.instances.get") for this account's
    generated custom role.
  EOT
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for permission in var.custom_permissions : !strcontains(permission, "/")])
    error_message = "custom_permissions takes permission names such as \"compute.instances.get\", not roles. Put existing custom role ids in custom_roles."
  }

  validation {
    condition     = length(distinct(var.custom_permissions)) == length(var.custom_permissions)
    error_message = "custom_permissions lists a permission more than once: ${join(", ", distinct([for index, permission in var.custom_permissions : permission if contains(slice(var.custom_permissions, 0, index), permission)]))}."
  }
}

variable "custom_roles" {
  description = <<-EOT
    For the custom profile: ids of existing custom roles to bind to this account, e.g.
    "projects/<project_id>/roles/<role_id>" or "organizations/<org_id>/roles/<role_id>". Can be combined
    with custom_permissions. Predefined roles are not accepted.
  EOT
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for role in var.custom_roles : can(regex(local.custom_role_id_pattern, role))])
    error_message = "custom_roles takes custom role ids of the form projects/<project>/roles/<id> or organizations/<org>/roles/<id>; predefined roles are not accepted. Invalid: ${join(", ", [for role in var.custom_roles : role if !can(regex(local.custom_role_id_pattern, role))])}."
  }

  validation {
    condition     = length(distinct(var.custom_roles)) == length(var.custom_roles)
    error_message = "custom_roles lists a role more than once: ${join(", ", distinct([for index, role in var.custom_roles : role if contains(slice(var.custom_roles, 0, index), role)]))}."
  }
}

variable "act_as_service_accounts" {
  description = <<-EOT
    For the Slurm controller profile: e-mail addresses of compute service accounts in
    project_id (name@<project_id>.iam.gserviceaccount.com) that the controller attaches to
    the nodes it creates. Grants "roles/iam.serviceAccountUser" on each target account
    individually, never project-wide. The default Compute Engine, App Engine and Google APIs
    accounts, accounts in another project, and Google-managed service agents are all refused:
    a designated compute account this controller creates and attaches, and nothing else.
  EOT
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for account_email in var.act_as_service_accounts : can(regex(local.act_as_target_pattern, account_email))])
    error_message = "act_as_service_accounts takes service account e-mail addresses of the form <name>@${var.project_id}.iam.gserviceaccount.com: accounts you create in this project, e.g. with modules/project/service-account. Accounts in another project and Google-managed service agents (for example service-<n>@gcp-sa-*.iam.gserviceaccount.com) are refused. Invalid: ${join(", ", [for account_email in var.act_as_service_accounts : account_email if !can(regex(local.act_as_target_pattern, account_email))])}."
  }

  validation {
    condition     = length(distinct(var.act_as_service_accounts)) == length(var.act_as_service_accounts)
    error_message = "act_as_service_accounts lists an account more than once: ${join(", ", distinct([for index, account_email in var.act_as_service_accounts : account_email if contains(slice(var.act_as_service_accounts, 0, index), account_email)]))}."
  }
}

variable "bucket_bindings" {
  description = "Roles to grant on individual Cloud Storage buckets: a map, keyed arbitrarily, of { bucket, role }."
  type = map(object({
    bucket = string
    role   = string
  }))
  default = {}
}

variable "secret_bindings" {
  description = "Roles to grant on individual Secret Manager secrets: a map of { project (default project_id), secret_id, role }."
  type = map(object({
    project   = optional(string)
    secret_id = string
    role      = string
  }))
  default = {}
}

variable "artifact_registry_bindings" {
  description = "Roles to grant on individual Artifact Registry repositories: a map of { project (default project_id), location, repository_id, role }."
  type = map(object({
    project       = optional(string)
    location      = string
    repository_id = string
    role          = string
  }))
  default = {}
}

variable "bigquery_dataset_bindings" {
  description = "Roles to grant on individual BigQuery datasets: a map of { project (default project_id), dataset_id, role }."
  type = map(object({
    project    = optional(string)
    dataset_id = string
    role       = string
  }))
  default = {}
}

variable "pubsub_topic_bindings" {
  description = "Roles to grant on individual Pub/Sub topics: a map of { project (default project_id), topic, role }."
  type = map(object({
    project = optional(string)
    topic   = string
    role    = string
  }))
  default = {}
}

variable "pubsub_subscription_bindings" {
  description = "Roles to grant on individual Pub/Sub subscriptions: a map of { project (default project_id), subscription, role }."
  type = map(object({
    project      = optional(string)
    subscription = string
    role         = string
  }))
  default = {}
}

variable "subnetwork_bindings" {
  description = "Roles to grant on individual Compute Engine subnetworks: a map of { project (default project_id; the host project for Shared VPC), region, subnetwork, role }."
  type = map(object({
    project    = optional(string)
    region     = string
    subnetwork = string
    role       = string
  }))
  default = {}
}

variable "enable_placement_policies" {
  description = "Capability: add the permissions to create and use placement policies."
  type        = bool
  default     = false
}

variable "enable_public_ips" {
  description = <<-EOT
    Capability: add compute.subnetworks.useExternalIp, needed to resume nodes that get an
    external IP (a nodeset with enable_public_ips or an access_config).
  EOT
  type        = bool
  default     = false
}

variable "enable_mig" {
  description = <<-EOT
    Capability: add the permissions to manage nodesets backed by a managed instance group that
    Terraform creates (provisioning_engine = "MIG"). Includes compute.instances.setMetadata
    without a name condition, because Google requires it for instance group manager patch,
    whose target is a group rather than an instance.
  EOT
  type        = bool
  default     = false
}

variable "enable_mig_flex" {
  description = <<-EOT
    Capability: add the regional instance-group permissions needed to manage DWS Flex nodesets
    (a nodeset with dws_flex enabled). Includes everything enable_mig adds, including the
    unscoped compute.instances.setMetadata.
  EOT
  type        = bool
  default     = false
}

variable "enable_bigquery_load" {
  description = "Capability: add the BigQuery permissions for the Slurm controller's enable_bigquery_load job accounting."
  type        = bool
  default     = false
}

variable "enable_tpu" {
  description = "Capability: add the Cloud TPU API permissions needed to manage TPU VM nodesets (schedmd-slurm-gcp-v6-nodeset-tpu)."
  type        = bool
  default     = false
}

variable "enable_tpu_gce" {
  description = <<-EOT
    Capability: add the permissions to manage TPU machine types on Compute Engine (a regular
    nodeset whose machine type is in a ct or tpu family), which the controller provisions through
    a MIG per slice. Includes everything enable_mig_flex adds, including the unscoped
    compute.instances.setMetadata.
  EOT
  type        = bool
  default     = false
}

variable "custom_role_stage" {
  description = <<-EOT
    Launch stage of the generated custom role: "ALPHA", "BETA", "GA", "DEPRECATED",
    "DISABLED" or "EAP". Unset means the provider default, "GA". A "DISABLED" role stays
    bound but grants nothing. Only valid when the account gets a generated custom role.
  EOT
  type        = string
  default     = null

  validation {
    condition     = var.custom_role_stage == null || contains(local.role_stages, coalesce(var.custom_role_stage, "GA"))
    error_message = "custom_role_stage must be null or one of ${join(", ", [for stage in local.role_stages : "\"${stage}\""])}."
  }
}

variable "custom_role_deletion_policy" {
  description = <<-EOT
    Deletion policy for the generated custom role. Unset means the provider default,
    "DELETE": destroy soft-deletes the role, a later apply within 7 days undeletes it, and its
    id cannot be reused until Google's 44-day deletion process ends. "ABANDON" leaves the role
    active, so a later apply fails until the role is imported or removed. "PREVENT" blocks
    deletion.
  EOT
  type        = string
  default     = null

  validation {
    condition     = var.custom_role_deletion_policy == null || contains(local.role_deletion_policies, coalesce(var.custom_role_deletion_policy, "DELETE"))
    error_message = "custom_role_deletion_policy must be null or one of ${join(", ", [for policy in local.role_deletion_policies : "\"${policy}\""])}."
  }
}

variable "instance_name_prefixes" {
  description = <<-EOT
    For profiles that get a name-scoped role (slurm-controller, image-builder): prefixes of the
    Compute Engine instance names that role covers, 4-62 characters each. Unset uses the
    profile default: slurm-controller "<slurm_cluster_name, if set, or the first 10 characters
    of deployment_name, lowercase, letters and digits only>-"; image-builder "packer-". For the
    controller, prefer setting slurm_cluster_name over this variable when the controller's own
    slurm_cluster_name is set: it takes the exact value, with no risk of re-deriving it
    differently here. Scoping is by name only: any other VM in the project with a matching name
    is covered too; a short prefix makes an accidental match more likely, hence the 4-character
    floor.
  EOT
  type        = list(string)
  default     = null

  validation {
    condition = var.instance_name_prefixes == null || (
      length(var.instance_name_prefixes) > 0 &&
      length(var.instance_name_prefixes) <= 5 &&
      length(distinct(var.instance_name_prefixes)) == length(var.instance_name_prefixes) &&
      alltrue([for prefix in var.instance_name_prefixes : can(regex(local.instance_name_prefix_pattern, prefix))])
    )
    error_message = "instance_name_prefixes takes 1 to 5 distinct prefixes, each starting with a lowercase letter and containing only lowercase letters, digits and dashes, 4 to 62 characters (short enough to identify a real cluster or build, long enough that an accidental match in a shared project is unlikely rather than probable)."
  }

  validation {
    condition     = var.instance_name_prefixes == null || length(try(local.profiles[var.profile].scoped_permissions, [])) > 0
    error_message = "instance_name_prefixes is accepted only with a profile that gets a name-scoped role (${join(" or ", [for name, profile in local.profiles : "\"${name}\"" if length(profile.scoped_permissions) > 0])}), not \"${var.profile}\"."
  }

  validation {
    condition     = var.instance_name_prefixes != null || alltrue([for prefix in try(local.profiles[var.profile].default_instance_name_prefixes, []) : can(regex(local.instance_name_prefix_pattern, prefix))])
    error_message = "deployment_name \"${var.deployment_name}\" is too short or normalises to too little (the derived slurm-controller prefix must be 4-62 characters, including the trailing dash). Set instance_name_prefixes or slurm_cluster_name explicitly."
  }
}

variable "iam_propagation_wait" {
  description = <<-EOT
    How long the service_account_email output waits after this module's grants are created, as a Go duration
    ("30s", "2m"). Best effort: Google documents IAM changes as eventually consistent, "typically
    2 minutes, potentially 7 minutes or longer", so no value guarantees the grants are effective.
    Raise it if the first use of the account fails with PERMISSION_DENIED right after apply.
  EOT
  type        = string
  default     = "30s"

  validation {
    condition     = can(regex("^([0-9]+(ns|us|ms|s|m|h))+$", var.iam_propagation_wait))
    error_message = "iam_propagation_wait must be a duration such as \"30s\", \"2m\" or \"1m30s\"."
  }
}
