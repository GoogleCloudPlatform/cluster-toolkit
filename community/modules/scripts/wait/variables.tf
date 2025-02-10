/**
 * Copyright 2025 Google LLC
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

variable "create_duration" {
  description = " Time duration to delay resource creation. For example, 30s for 30 seconds or 5m for 5 minutes. Updating this value by itself will not trigger a delay."
  default     = null
  type        = string
}

variable "destroy_duration" {
  description = "Time duration to delay resource destroy. For example, 30s for 30 seconds or 5m for 5 minutes. Updating this value by itself will not trigger a delay. This value or any updates to it must be successfully applied into the Terraform state before destroying this resource to take effect."
  default     = null
  type        = string
}

variable "triggers" {
  description = "(Optional) Arbitrary map of values that, when changed, will run any creation or destroy delays again."
  type        = map(string)
}
