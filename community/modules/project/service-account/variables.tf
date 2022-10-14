/**
 * Copyright 2022 Google LLC
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

variable "description" {
  description = "Default description of the created service accounts (defaults to no description)."
  type        = string
  default     = ""
}

variable "descriptions" {
  description = "List of descriptions of the created service accounts (elements default to the value of description)."
  type        = list(string)
  default     = []
}

variable "display_name" {
  description = "display names of the created service accounts."
  type        = string
  default     = ""
}

variable "generate_keys" {
  description = "Generate keys for service accounts."
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

variable "names" {
  description = "Names of the services accounts to create."
  type        = list(string)
  default     = []
}

variable "org_id" {
  description = "Id of the organization for org-level roles."
  type        = string
  default     = ""
}

variable "prefix" {
  description = "prefix applied to service account names"
  type        = string
  default     = ""
}

variable "project_id" {
  description = "ID of the project"
  type        = string
}

variable "project_roles" {
  description = "list of roles to apply to created service accounts"
  type        = list(string)
}
