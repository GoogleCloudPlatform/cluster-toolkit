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

variable "content" {
  description = "Raw string content of instructions to display. If template_path is also provided, template_path takes precedence."
  type        = string
  default     = null
}

variable "template_path" {
  description = "Path to a template file (.tftpl) to render using template_vars."
  type        = string
  default     = null
}

variable "template_vars" {
  description = "Variables to pass to templatefile when template_path is provided."
  type        = any
  default     = {}
}

variable "banner_title" {
  description = "Optional title to display in a decorative banner box around the instructions."
  type        = string
  default     = null
}

variable "write_to_file" {
  description = "Optional destination file path on the local filesystem to write the formatted instructions. Relative paths resolve relative to the deployment root directory ($${path.root})."
  type        = string
  default     = null
}

variable "print_to_terminal" {
  description = "Whether to print the formatted instructions directly to terminal during terraform apply via local-exec."
  type        = bool
  default     = false
}
