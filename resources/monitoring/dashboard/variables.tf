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
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "deployment_name" {
  description = "The name of the current deployment"
  type        = string
}

variable "base_dashboard" {
  description = "Baseline dashboard template, either custom or from ./dashboards"
  type        = string
  default     = "HPC"
}

variable "widgets" {
  description = "List of additional widgets to add to the base dashboard."
  type        = list(string)
  default     = []
}
