# Copyright 2026 Google LLC
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

variable "has_tpu" {
  description = "If set to true, the nodeset template's Pod spec will contain request/limit for TPU resource, open port 8740 for TPU communication and add toleration for google.com/tpu."
  type        = bool
  default     = false
}

variable "nodeset_name" {
  description = "The nodeset name"
  type        = string
  default     = "gkenodeset"
}

variable "partition_name" {
  description = "The partition name"
  type        = string
  default     = "gke"
}

variable "slurm_bucket_dir" {
  description = "Path directory within `bucket_name` for Slurm cluster file storage."
  type        = string
  nullable    = false
}

variable "slurm_bucket" {
  description = "GCS Bucket of Slurm cluster file storage."
  type        = any
  nullable    = true
}
