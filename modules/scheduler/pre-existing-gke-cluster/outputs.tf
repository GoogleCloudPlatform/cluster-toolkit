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

output "cluster_id" {
  description = "An identifier for the gke cluster with format projects/{{project_id}}/locations/{{region}}/clusters/{{name}}."
  value       = data.google_container_cluster.existing_gke_cluster.id
}

output "gke_cluster_exists" {
  description = "A static flag that signals to downstream modules that a cluster exists."
  value       = true
  depends_on = [
    data.google_container_cluster.existing_gke_cluster
  ]
}

output "gke_version" {
  description = "GKE cluster's version."
  value       = data.google_container_cluster.existing_gke_cluster.master_version
}
