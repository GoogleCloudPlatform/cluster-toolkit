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
  description = "an identifier for the resource with format projects/<project_id>/locations/<region>/clusters/<name>."
  value       = google_container_cluster.gke_cluster.id
}

locals {
  private_endpoint_message = trimspace(
    <<-EOT
      This cluster was created with 'enable_private_endpoint: true'. 
      It cannot be accessed from a public IP addressses.
      One way to access this cluster is from a VM created in the GKE cluster subnet.
    EOT
  )
  public_endpoint_message = trimspace(
    <<-EOT
      To access this cluster from a public IP address you must allowlist your IP:
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
      ${local.allowlist_your_ip_message}

      Use the following command to fetch credentials for the created cluster:
        gcloud container clusters get-credentials ${google_container_cluster.gke_cluster.name} \
          --region ${google_container_cluster.gke_cluster.location} \
          --project ${var.project_id}
    EOT
  )
}
