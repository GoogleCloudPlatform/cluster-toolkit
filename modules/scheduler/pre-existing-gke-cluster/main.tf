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

data "google_container_cluster" "existing_gke_cluster" {
  name     = var.cluster_name
  project  = var.project_id
  location = var.region
}

data "google_client_config" "default" {}

provider "kubectl" {
  host                   = "https://${data.google_container_cluster.existing_gke_cluster.endpoint}"
  token                  = data.google_client_config.default.access_token
  cluster_ca_certificate = base64decode(data.google_container_cluster.existing_gke_cluster.master_auth[0].cluster_ca_certificate)
  load_config_file       = false
}

resource "kubectl_manifest" "additional_net_params" {
  for_each = { for idx, network_info in var.additional_networks : idx => network_info }

  depends_on = [data.google_container_cluster.existing_gke_cluster]

  yaml_body = <<YAML
apiVersion: networking.gke.io/v1
kind: GKENetworkParamSet
metadata:
  name: vpc${each.key + 1}
spec:
  vpc: ${each.value.network}
  vpcSubnet: ${each.value.subnetwork}
  deviceMode: NetDevice
YAML

  provider = kubectl
}

resource "kubectl_manifest" "additional_nets" {
  for_each = { for idx, network_info in var.additional_networks : idx => network_info }

  depends_on = [data.google_container_cluster.existing_gke_cluster, kubectl_manifest.additional_net_params]

  yaml_body = <<YAML
apiVersion: networking.gke.io/v1
kind: Network
metadata:
  name: vpc${each.key + 1}
spec:
  parametersRef:
    group: networking.gke.io
    kind: GKENetworkParamSet
    name: vpc${each.key + 1}
  type: Device
YAML

  provider = kubectl
}
