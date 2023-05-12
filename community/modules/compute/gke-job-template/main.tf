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
  # Start with the minimum cpu available of used node pools
  min_allocatable_cpu = min(var.allocatable_cpu_per_node...)
  full_node_cpu_request = (
    local.min_allocatable_cpu > 2 ?     # if large enough
    local.min_allocatable_cpu - 1 :     # leave headroom for 1 cpu
    local.min_allocatable_cpu / 2 + 0.1 # else take just over half
  )

  cpu_request = (
    var.requested_cpu_per_pod >= 0 ?   # if user supplied requested cpu
    var.requested_cpu_per_pod :        # then honor it
    (                                  # else
      local.min_allocatable_cpu >= 0 ? # if allocatable cpu was supplied
      local.full_node_cpu_request :    # then claim the full node
      -1                               # else do not set a limit
    )
  )
  millicpu           = floor(local.cpu_request * 1000)
  should_request_cpu = local.millicpu >= 0
  full_node_request  = local.min_allocatable_cpu >= 0 && var.requested_cpu_per_pod < 0

  should_request_gpu = alltrue(var.has_gpu)
  # arbitrarily, user can edit in template.
  # May come from node pool in future.
  gpu_limit = 1

  suffix = var.random_name_sufix ? "-${random_id.resource_name_suffix.hex}" : ""

  job_template_contents = templatefile(
    "${path.module}/templates/gke-job-base.yaml.tftpl",
    {
      name               = var.name
      suffix             = local.suffix
      image              = var.image
      command            = var.command
      node_count         = var.node_count
      machine_family     = var.machine_family
      node_pool_names    = var.node_pool_name
      node_selectors     = var.node_selectors
      should_request_cpu = local.should_request_cpu
      full_node_request  = local.full_node_request
      millicpu_request   = "${local.millicpu}m"
      should_request_gpu = local.should_request_gpu
      gpu_limit          = local.gpu_limit
      restart_policy     = var.restart_policy
      backoff_limit      = var.backoff_limit
      tolerations        = distinct(var.tolerations)
    }
  )

  job_template_output_path = "${path.root}/${var.name}${local.suffix}.yaml"

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
