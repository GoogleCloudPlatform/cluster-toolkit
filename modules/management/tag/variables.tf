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
}

variable "tag_key_short_name" {
  description = "The user friendly name for a TagKey. The short name should be unique for TagKeys within the same tag namespace. The short name can have a maximum length of 256 characters. The permitted character set for the shortName includes all UTF-8 encoded Unicode characters except single quotes ('), double quotes (\"), backslashes (\\), and forward slashes (/)."
  type        = string
}

variable "tag_key_description" {
  description = "User-assigned description of the TagKey. Must not exceed 256 characters."
  type        = string
  default     = ""
}

variable "tag_key_purpose" {
  description = "A purpose cannot be changed once set. A purpose denotes that this Tag is intended for use in policies of a specific policy engine, and will involve that policy engine in management operations involving this Tag. Possible values are: GCE_FIREWALL, DATA_GOVERNANCE."
  type        = string
  default     = null
}

variable "tag_key_purpose_data" {
  description = "Purpose data cannot be changed once set. Purpose data corresponds to the policy system that the tag is intended for. For example, the GCE_FIREWALL purpose expects data in the following map format: network = \"<project-id>/<network-id>\" (or) Network URI (or) selfLinkWithId."
  type        = map(string)
  default     = null
}

variable "tag_value" {
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
}
