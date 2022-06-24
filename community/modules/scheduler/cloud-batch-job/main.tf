/**
 * Copyright 2022 Google LLC
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

locals {
  gcloud_version = var.gcloud_version == "" ? "" : "${var.gcloud_version} "

  job_template_contents = templatefile(
    "${path.module}/templates/batch-job-base.json.tftpl",
    {
      runnable          = var.runnable
      log_policy        = var.log_policy
      instance_template = var.instance_template
    }
  )

  job_template_output_path = "${path.root}/cloud-batch-${var.job_id}.json"
}

resource "local_file" "job_template" {
  content  = local.job_template_contents
  filename = local.job_template_output_path
}
