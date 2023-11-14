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

output "cluster_id" {
  description = "An identifier for the resource with format projects/<project_id>/locations/<region>/clusters/<name>."
  value       = google_container_cluster.gke_cluster.id
}

output "gke_cluster_exists" {
  description = "A static flag that signals to downstream modules that a cluster has been created. Needed by community/modules/scripts/kubernetes-operations."
  value       = true
  depends_on = [
    google_container_cluster.gke_cluster
  ]
}

locals {
  private_endpoint_message = trimspace(
    <<-EOT
      This cluster was created with 'enable_private_endpoint: true'. 
      It cannot be accessed from a public IP addressses.
      One way to access this cluster is from a VM created in the GKE cluster subnet.
    EOT
  )
  master_authorized_networks_message = length(var.master_authorized_networks) == 0 ? "" : trimspace(
    <<-EOT
    The following networks have been authorized to access this cluster:
    ${join("\n", [for x in var.master_authorized_networks : "  ${x.display_name}: ${x.cidr_block}"])}"
    EOT
  )
  public_endpoint_message = trimspace(
    <<-EOT
      To add authorized networks you can allowlist your IP with this command:
        gcloud container clusters update ${google_container_cluster.gke_cluster.name} \
          --region ${google_container_cluster.gke_cluster.location} \
          --project ${var.project_id} \
          --enable-master-authorized-networks \
          --master-authorized-networks <IP Address>/32
    EOT
  )
  allowlist_your_ip_message = var.enable_private_endpoint ? local.private_endpoint_message : local.public_endpoint_message
}

output "instructions" {
  description = "Instructions on how to connect to the created cluster."
  value = trimspace(
    <<-EOT
      ${local.master_authorized_networks_message}

      ${local.allowlist_your_ip_message}

      Use the following command to fetch credentials for the created cluster:
        gcloud container clusters get-credentials ${google_container_cluster.gke_cluster.name} \
          --region ${google_container_cluster.gke_cluster.location} \
          --project ${var.project_id}
    EOT
  )
}

output "k8s_service_account_name" {
  description = "Name of k8s service account."
  value       = one(module.workload_identity[*].k8s_service_account_name)
}
