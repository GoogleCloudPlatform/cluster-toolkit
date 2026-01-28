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

data "google_storage_bucket" "this" {
  name = var.slurm_bucket[0].name

  depends_on = [var.slurm_bucket]
}

### Slurm Partition
locals {
  partition_conf = {
    "PowerDownOnIdle" = "NO"
    "SuspendTime"     = "INFINITE"
    "SuspendTimeout"  = var.has_tpu ? 240 : 120
    "ResumeTimeout"   = var.has_tpu ? 600 : 300
  }

  partition = {
    partition_name = var.partition_name
    partition_conf = local.partition_conf

    partition_nodeset     = [var.nodeset_name]
    partition_nodeset_tpu = []
    partition_nodeset_dyn = []
    # Options
    enable_job_exclusive = true
    power_down_on_idle   = false
  }
}

resource "google_storage_bucket_object" "parition_config" {
  bucket  = data.google_storage_bucket.this.name
  name    = "${var.slurm_bucket_dir}/partition_configs/${var.partition_name}.yaml"
  content = yamlencode(local.partition)
}
