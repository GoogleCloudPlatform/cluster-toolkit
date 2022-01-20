/**
 * Copyright 2021 Google LLC
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
  description = "ID of the project"
  type        = string
}

variable "project_roles" {
  description = "list of roles to apply to created service accounts"
  type        = list(string)
}

variable "names" {
  description = "names of the services accounts to create"
  type        = list(string)
}

variable "prefix" {
  description = "prefix applied to service account names"
  type        = string
  default     = ""
}

variable "display_name" {
  description = "display names of the created service accounts"
  type        = string
  default     = ""
}

