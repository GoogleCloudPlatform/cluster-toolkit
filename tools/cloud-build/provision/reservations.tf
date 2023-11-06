# Copyright 2023 Google LLC
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

locals {
  reservation_description = "Reservation for running tests. Managed by Terraform. DO NOT modify manually."
}

resource "google_compute_reservation" "c2standard60_us_west4_c" {
  name        = "c2standard60-us-central1-a"
  zone        = "us-west4-c"
  description = local.reservation_description

  specific_reservation {
    count = 1
    instance_properties {
      machine_type = "c2-standard-60"
    }
  }
}

resource "google_compute_reservation" "n1standard8_with_tesla_t4_europe_west1_d" {
  name        = "n1standard8-with-tesla-t4-europe-west1-d"
  zone        = "europe-west1-d"
  description = local.reservation_description

  specific_reservation {
    count = 1
    instance_properties {
      machine_type = "n1-standard-8"
      guest_accelerators {
        accelerator_type  = "nvidia-tesla-t4-vws"
        accelerator_count = 1
      }
    }
  }
}
