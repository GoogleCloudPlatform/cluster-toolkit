/**
  * Copyright 2023 Google LLC
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
  should_set_resources = var.cpu_per_node != null
  cpu_limit            = local.should_set_resources ? var.cpu_per_node : 0
  cpu_request          = local.cpu_limit > 2 ? local.cpu_limit - 1 : "${local.cpu_limit * 1000 / 2 + 10}m"

  suffix = var.random_name_sufix ? "-${random_id.resource_name_suffix.hex}" : ""

  job_template_contents = templatefile(
    "${path.module}/templates/gke-job-base.yaml.tftpl",
    {
      name                 = var.name
      suffix               = local.suffix
      image                = var.image
      command              = var.command
      node_count           = var.node_count
      machine_family       = var.machine_family
      node_pool_name       = var.node_pool_name
      node_selectors       = var.node_selectors
      should_set_resources = local.should_set_resources
      cpu_request          = local.cpu_request
      cpu_limit            = local.cpu_limit
      restart_policy       = var.restart_policy
      backoff_limit        = var.backoff_limit
      tolerations          = var.tolerations
    }
  )

  job_template_output_path = "${path.root}/gke-job.yaml"

}

resource "random_id" "resource_name_suffix" {
  byte_length = 2
  keepers = {
    timestamp = timestamp()
  }
}

resource "local_file" "job_template" {
  content  = local.job_template_contents
  filename = local.job_template_output_path
}
