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

variable "billing_account_id" {
  description = "If assigning billing role, specify a billing account (default is to assign at the organizational level)."
  type        = string
  default     = ""
}

variable "conditional_project_roles" {
  description = "List of IAM roles to grant to the service account with IAM conditions (use role names without the \"roles/\" prefix, e.g. \"storage.objectUser\")."
  type = list(object({
    role        = string
    title       = string
    description = optional(string, "")
    expression  = string
  }))
  default  = []
  nullable = false

  validation {
    condition = alltrue([
      for r in var.conditional_project_roles :
      r != null ? (r.role != null ? (!startswith(r.role, "roles/") && !contains(["owner", "editor", "viewer"], r.role) && can(regex("^[A-Za-z0-9._-]+$", r.role))) : false) : false
    ])
    error_message = "Each conditional_project_roles entry must specify a non-empty role without a leading \"roles/\" prefix (matching ^[A-Za-z0-9._-]+$, e.g. \"storage.objectUser\"); basic roles (owner, editor, viewer) do not support IAM conditions."
  }

  validation {
    condition = alltrue([
      for r in var.conditional_project_roles :
      r != null ? (r.title != null ? length(trimspace(r.title)) > 0 && length(r.title) <= 100 : false) : false
    ])
    error_message = "Each conditional_project_roles entry must have a non-empty title of at most 100 characters."
  }

  validation {
    condition = alltrue([
      for r in var.conditional_project_roles :
      r != null ? (r.description != null ? length(r.description) <= 300 : true) : false
    ])
    error_message = "Each conditional_project_roles entry description must be at most 300 characters."
  }

  validation {
    condition = alltrue([
      for r in var.conditional_project_roles :
      r != null ? (r.expression != null ? length(trimspace(r.expression)) > 0 : false) : false
    ])
    error_message = "Each conditional_project_roles entry must have a non-empty CEL expression."
  }

  validation {
    condition = length(distinct([
      for idx, r in var.conditional_project_roles :
      r == null ? "null::${idx}" : "${r.role == null ? "" : r.role}::${r.title == null ? "" : r.title}"
    ])) == length(var.conditional_project_roles)
    error_message = "Each conditional_project_roles entry must have a unique (role, title) combination."
  }
}

variable "deployment_name" {
  description = "Name of the deployment (will be prepended to service account name)"
  type        = string
}

variable "description" {
  description = "Description of the created service account."
  type        = string
  default     = "Service Account"
}

# tflint-ignore: terraform_unused_declarations
variable "descriptions" {
  description = "Deprecated; create single service accounts using var.description."
  type        = list(string)
  default     = null

  validation {
    condition     = var.descriptions == null
    error_message = "var.descriptions has been deprecated in favor of creating single accounts with var.description"
  }
}

variable "display_name" {
  description = "Display name of the created service account."
  type        = string
  default     = "Service Account"
}

variable "generate_keys" {
  description = "Generate keys for service account."
  type        = bool
  default     = false
}

variable "grant_billing_role" {
  description = "Grant billing user role."
  type        = bool
  default     = false
}

variable "grant_xpn_roles" {
  description = "Grant roles for shared VPC management."
  type        = bool
  default     = true
}

variable "name" {
  description = "Name of the service account to create."
  type        = string
}

# tflint-ignore: terraform_unused_declarations
variable "names" {
  description = "Deprecated; create single service accounts using var.name."
  type        = list(string)
  default     = null

  validation {
    condition     = var.names == null
    error_message = "var.names has been deprecated in favor of creating single accounts with var.name"
  }
}

variable "org_id" {
  description = "Id of the organization for org-level roles."
  type        = string
  default     = ""
}

# tflint-ignore: terraform_unused_declarations
variable "prefix" {
  description = "Deprecated; prefix now set using var.deployment_name"
  type        = string
  default     = null

  validation {
    condition     = var.prefix == null
    error_message = "var.prefix has been deprecated in favor of setting prefix with var.deployment_name"
  }
}

variable "project_id" {
  description = "ID of the project"
  type        = string
}

variable "project_roles" {
  description = "List of roles to grant to service account (e.g. \"storage.objectViewer\" or \"compute.instanceAdmin.v1\""
  type        = list(string)
}
