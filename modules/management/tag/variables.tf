/*
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

variable "tag_key_parent" {
  description = "The resource name of the new TagKey's parent. Must be of the form organizations/{org_id} or projects/{project_id_or_number}."
  type        = string

  validation {
    condition     = can(regex("^(organizations/[0-9]+|projects/[a-z0-9-]+)$", var.tag_key_parent))
    error_message = "The tag_key_parent must be in the format 'organizations/{org_id}' (numeric) or 'projects/{project_id_or_number}' (alphanumeric/hyphens)."
  }
}

variable "tag_key_short_name" {
  description = "The user friendly name for a TagKey. The short name should be unique for TagKeys within the same tag namespace. The short name can have a maximum length of 256 characters. The permitted character set for the shortName includes all UTF-8 encoded Unicode characters except single quotes ('), double quotes (\"), backslashes (\\), and forward slashes (/)."
  type        = string

  validation {
    condition     = length(var.tag_key_short_name) <= 256
    error_message = "The tag_key_short_name must not exceed 256 characters."
  }

  validation {
    # Matches any of the forbidden characters: ' " / \
    condition     = !can(regex("['\"/\\\\]", var.tag_key_short_name))
    error_message = "The tag_key_short_name cannot contain single quotes ('), double quotes (\"), backslashes (\\), or forward slashes (/)."
  }
}

variable "tag_key_description" {
  description = "User-assigned description of the TagKey. Must not exceed 256 characters."
  type        = string
  default     = ""

  validation {
    condition     = length(var.tag_key_description) <= 256
    error_message = "The tag_key_description must not exceed 256 characters."
  }
}

variable "tag_key_purpose" {
  description = "A purpose cannot be changed once set. A purpose denotes that this Tag is intended for use in policies of a specific policy engine, and will involve that policy engine in management operations involving this Tag. Possible values are: GCE_FIREWALL, DATA_GOVERNANCE."
  type        = string
  default     = null

  validation {
    # Allow null (since it has a default of null) or check if it's in the allowed list
    condition     = var.tag_key_purpose == null ? true : contains(["GCE_FIREWALL", "DATA_GOVERNANCE"], var.tag_key_purpose)
    error_message = "The tag_key_purpose must be either 'GCE_FIREWALL' or 'DATA_GOVERNANCE'."
  }
}

variable "tag_key_purpose_data" {
  description = "Purpose data cannot be changed once set. Purpose data corresponds to the policy system that the tag is intended for. For example, the GCE_FIREWALL purpose expects data in the following map format: network = \"<project-id>/<network-id>\" (or) Network URI (or) selfLinkWithId."
  type        = map(string)
  default     = null
}

variable "tag_values" {
  description = <<-EOT
  A list of TagValues to create as children of the TagKey. TagValues are used to group cloud resources for the purpose of controlling them using policies. Each object in the list should have the following attributes:
  - `short_name`: User-assigned short name for the TagValue. Must be unique for TagValues within the same parent TagKey. Maximum length of 256 characters. The permitted character set includes all UTF-8 encoded Unicode characters except single quotes ('), double quotes ("), backslashes (\\), and forward slashes (/).
  - `description`: User-assigned description of the TagValue. Must not exceed 256 characters.
  EOT
  type = list(object({
    short_name  = string
    description = string
  }))
  default = []

  validation {
    condition = alltrue([
      for v in var.tag_values :
      length(v.short_name) <= 256 &&
      !can(regex("['\"/\\\\]", v.short_name))
    ])
    error_message = "Each tag_values.short_name must be <= 256 characters and cannot contain quotes ('), double quotes (\"), backslashes (\\), or forward slashes (/)."
  }

  validation {
    condition = alltrue([
      for v in var.tag_values : length(v.description) <= 256
    ])
    error_message = "Each tag_values.description must not exceed 256 characters."
  }
}
