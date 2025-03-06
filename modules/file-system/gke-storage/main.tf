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

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "gke-storage", ghpc_role = "file-system" })
}

locals {
  storage_type       = lower(var.storage_type)
  storage_class_name = "${local.storage_type}-sc"
  pvc_name_prefix    = "${local.storage_type}-pvc"
}

check "private_vpc_connection_peering" {
  assert {
    condition     = lower(var.storage_type) != "parallelstore" ? true : var.private_vpc_connection_peering != null
    error_message = <<-EOT
    Parallelstore must be run within the same VPC as the GKE cluster and have private services access enabled.
    If using new VPC, please use community/modules/network/private-service-access to create private-service-access.
    If using existing VPC with private-service-access enabled, set this manually follow [user guide](https://cloud.google.com/parallelstore/docs/vpc).
    EOT
  }
}

module "kubectl_apply" {
  source = "../../management/kubectl-apply"

  cluster_id = var.cluster_id
  project_id = var.project_id

  # count = var.pvc_count
  apply_manifests = flatten(
    [
      # create StorageClass in the cluster
      {
        content = templatefile(
          "${path.module}/storage-class/${local.storage_class_name}.yaml.tftpl",
          {
            name                = local.storage_class_name
            labels              = local.labels
            volume_binding_mode = var.sc_volume_binding_mode
            reclaim_policy      = var.sc_reclaim_policy
            topology_zones      = var.sc_topology_zones
        })
      },
      # create PersistentVolumeClaim in the cluster
      flatten([
        for idx in range(var.pvc_count) : [
          {
            content = templatefile(
              "${path.module}/persistent-volume-claim/${(local.pvc_name_prefix)}.yaml.tftpl",
              {
                pvc_name           = "${local.pvc_name_prefix}-${idx}"
                labels             = local.labels
                capacity           = "${var.capacity_gb}Gi"
                access_mode        = var.access_mode
                storage_class_name = local.storage_class_name
              }
            )
          }
        ]
      ])
  ])
}
