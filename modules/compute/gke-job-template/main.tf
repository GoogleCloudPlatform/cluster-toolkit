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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "gke-job-template", ghpc_role = "compute" })
}

locals {
  # Start with the minimum cpu available of used node pools
  min_allocatable_cpu = min(var.allocatable_cpu_per_node...)
  full_node_cpu_request = (
    local.min_allocatable_cpu > 2 ?     # if large enough
    local.min_allocatable_cpu - 1 :     # leave headroom for 1 cpu
    local.min_allocatable_cpu / 2 + 0.1 # else take just over half
  ) - (local.any_gcs ? 0.25 : 0)        # save room for gcs side car

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
  cpu_request_string = local.millicpu >= 0 ? "${local.millicpu}m" : null
  full_node_request  = local.min_allocatable_cpu >= 0 && var.requested_cpu_per_pod < 0

  memory_request_value = try(sum([for ed in var.ephemeral_volumes :
    ed.size_gb
    if ed.type == "memory"
  ]), 0)
  memory_request_string = local.memory_request_value > 0 ? "${local.memory_request_value}Gi" : null

  ephemeral_request_value = try(sum([for ed in var.ephemeral_volumes :
    ed.size_gb
    if ed.type == "local-ssd"
  ]), 0)
  ephemeral_request_string = local.ephemeral_request_value > 0 ? "${local.ephemeral_request_value}Gi" : null

  uses_local_ssd = anytrue([for ed in var.ephemeral_volumes :
    ed.type == "local-ssd"
  ])
  local_ssd_node_selector = local.uses_local_ssd ? [{
    key   = "cloud.google.com/gke-ephemeral-storage-local-ssd"
    value = "true"
  }] : []

  # arbitrarily, user can edit in template.
  # May come from node pool in future.
  gpu_limit_string = alltrue(var.has_gpu) ? "1" : null

  empty_dir_volumes = [for ed in var.ephemeral_volumes :
    {
      name       = replace(trim(ed.mount_path, "/"), "/", "-")
      mount_path = ed.mount_path
      size_limit = "${ed.size_gb}Gi"
      in_memory  = ed.type == "memory"
    }
    if contains(["memory", "local-ssd"], ed.type)
  ]

  ephemeral_pd_volumes = [for pd in var.ephemeral_volumes :
    {
      name               = replace(trim(pd.mount_path, "/"), "/", "-")
      mount_path         = pd.mount_path
      storage_class_name = pd.type == "pd-ssd" ? "premium-rwo" : "standard-rwo"
      storage            = "${pd.size_gb}Gi"
    }
    if contains(["pd-balanced", "pd-ssd"], pd.type)
  ]

  pvc_volumes = [for pvc in var.persistent_volume_claims :
    {
      name       = replace(trim(pvc.mount_path, "/"), "/", "-")
      mount_path = pvc.mount_path
      claim_name = pvc.name
    }
  ]

  volume_mounts = [for v in concat(local.empty_dir_volumes, local.ephemeral_pd_volumes, local.pvc_volumes) :
    {
      name       = v.name
      mount_path = v.mount_path
    }
  ]

  suffix = var.random_name_sufix ? "-${random_id.resource_name_suffix.hex}" : ""
  machine_family_node_selector = var.machine_family != null ? [{
    key   = "cloud.google.com/machine-family"
    value = var.machine_family
  }] : []
  node_selectors = concat(local.machine_family_node_selector, local.local_ssd_node_selector, var.node_selectors)

  any_gcs = anytrue([for pvc in var.persistent_volume_claims :
    pvc.is_gcs
  ])

  job_template_contents = templatefile(
    "${path.module}/templates/gke-job-base.yaml.tftpl",
    {
      name                     = var.name
      suffix                   = local.suffix
      image                    = var.image
      command                  = var.command
      node_count               = var.node_count
      completion_mode          = var.completion_mode
      k8s_service_account_name = var.k8s_service_account_name
      node_pool_names          = var.node_pool_name
      node_selectors           = local.node_selectors
      full_node_request        = local.full_node_request
      cpu_request              = local.cpu_request_string
      gpu_limit                = local.gpu_limit_string
      restart_policy           = var.restart_policy
      backoff_limit            = var.backoff_limit
      tolerations              = distinct(var.tolerations)
      labels                   = local.labels

      empty_dir_volumes    = local.empty_dir_volumes
      ephemeral_pd_volumes = local.ephemeral_pd_volumes
      pvc_volumes          = local.pvc_volumes
      volume_mounts        = local.volume_mounts
      memory_request       = local.memory_request_string
      ephemeral_request    = local.ephemeral_request_string
      gcs_annotation       = local.any_gcs
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

  lifecycle {
    precondition {
      condition     = local.any_gcs ? var.k8s_service_account_name != null : true
      error_message = "When using GCS, a kubernetes service account with workload identity is required. gke-cluster module will perform this setup when var.configure_workload_identity_sa is set to true."
    }
  }
}
