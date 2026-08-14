/**
* Copyright 2026 Google LLC
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

variable "project_id" {
  description = "The project ID to deploy to."
  type        = string
}

variable "instance_name" {
  description = "The Spanner instance name."
  type        = string
}

variable "database_name" {
  description = "The Spanner database name."
  type        = string
}

variable "migrations_dir" {
  description = "The migrations directory."
  type        = string
}

variable "sub_directory" {
  description = "Optional sub-directory within migrations_dir to search for SQL files."
  type        = string
  default     = ""
}

variable "proto_descriptors_file" {
  description = "Optional path to a compiled proto descriptors file (.pb) needed for custom types in migrations."
  type        = string
  default     = null
}
