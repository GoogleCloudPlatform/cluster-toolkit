# Copyright 2026 "Google LLC"
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

terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
      # >= 6.39.0: google_bigquery_dataset_iam_member no longer removes
      # authorized views and routines on apply (fixed in
      # hashicorp/terraform-provider-google#23177). Without this floor,
      # applying bigquery_dataset_bindings against a dataset that has
      # existing authorized views/routines can silently delete them.
      version = ">= 6.39.0"
    }
    time = {
      source  = "hashicorp/time"
      version = ">= 0.9"
    }
  }
  required_version = ">= 1.12.2"
}
