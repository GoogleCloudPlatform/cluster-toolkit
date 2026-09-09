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

variable "project_id" {
  description = "GCP project ID"
  type        = string
  default     = "hpc-toolkit-dev"
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "GCP zone"
  type        = string
  default     = "us-central1-c"
}

variable "repo_uri" {
  description = "URI of GitHub repo"
  type        = string
  default     = "https://github.com/GoogleCloudPlatform/cluster-toolkit"
}

variable "daily_tests_project_id" {
  description = "The GCP project for daily tests"
  type        = string
  default     = "hpc-toolkit-dev-2"
}

variable "daily_tests_dev_exceptions" {
  description = "List of daily tests that must run in hpc-toolkit-dev in addition to hpc-toolkit-dev-2 (e.g. due to hardware reservations or quota constraints)"
  type        = set(string)
  default = [
    "gke-g4-confidential",
    "gke-tpu-7x",
  ]
}

variable "daily_tests_service_account" {
  description = "The service account to run daily tests under. If null, the default Cloud Build service account is used. For projects enforcing BYOSA (like hpc-toolkit-dev-2), you must set this via environment variable, e.g. export TF_VAR_daily_tests_service_account=\"projects/...\""
  type        = string
  default     = null
}
