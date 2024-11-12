/**
 * Copyright 2024 Google LLC
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

## Required variables:
#  local_ssd_count_ephemeral_storage
#  local_ssd_count_nvme_block
#  machine_type

locals {

  local_ssd_machines = {
    "a3-highgpu-8g"  = { local_ssd_count_ephemeral_storage = null, local_ssd_count_nvme_block = 16 },
    "a3-megagpu-8g"  = { local_ssd_count_ephemeral_storage = null, local_ssd_count_nvme_block = 16 },
    "a3-ultragpu-8g" = { local_ssd_count_ephemeral_storage = null, local_ssd_count_nvme_block = 32 },
  }

  generated_local_ssd_config = lookup(local.local_ssd_machines, var.machine_type, { local_ssd_count_ephemeral_storage = null, local_ssd_count_nvme_block = null })

  # Select in priority order:
  # (1) var.local_ssd_count_ephemeral_storage and var.local_ssd_count_nvme_block if any is not null
  # (2) local.local_ssd_machines if not empty
  # (3) default to null value for both local_ssd_count_ephemeral_storage and local_ssd_count_nvme_block
  local_ssd_config = (var.local_ssd_count_ephemeral_storage == null && var.local_ssd_count_nvme_block == null) ? local.generated_local_ssd_config : { local_ssd_count_ephemeral_storage = var.local_ssd_count_ephemeral_storage, local_ssd_count_nvme_block = var.local_ssd_count_nvme_block }
}
