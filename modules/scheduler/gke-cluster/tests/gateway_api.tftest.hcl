# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

mock_provider "google" {
  mock_data "google_compute_zones" {
    defaults = { names = ["us-central1-a"] }
  }
}
mock_provider "google-beta" {
  mock_data "google_container_engine_versions" {
    defaults = {
      latest_master_version          = "1.35.0-gke.3065000"
      release_channel_latest_version = {}
    }
  }
}
mock_provider "kubernetes" {}
mock_provider "time" {}
mock_provider "helm" {}
mock_provider "http" {}

# The child installs CRDs against a live API. Inspect its inputs here instead.
override_module {
  target  = module.kubectl_apply
  outputs = { k8s_prerequisites_ready = true }
}

variables {
  project_id                    = "test-project"
  deployment_name               = "gateway-test"
  region                        = "us-central1"
  network_id                    = "projects/test-project/global/networks/test"
  subnetwork_self_link          = "https://www.googleapis.com/compute/v1/projects/test-project/regions/us-central1/subnetworks/test"
  system_node_pool_machine_type = "e2-standard-4"
  system_node_pool_enabled      = false
  labels                        = {}
}

run "gateway_false_inference_false" {
  command = plan
  variables {
    enable_gateway_api       = false
    enable_inference_gateway = false
  }
  assert {
    condition = length(local.inference_gateway_manifests) == 0
    error_message = "Generic Gateway API enablement must not install inference CRDs."
  }
  assert {
    condition     = length(google_container_cluster.gke_cluster.gateway_api_config) == 0
    error_message = "Gateway API must be enabled by either flag, without changing the default."
  }
}

run "gateway_true_inference_false" {
  command = plan
  variables {
    enable_gateway_api       = true
    enable_inference_gateway = false
  }
  assert {
    condition = length(local.inference_gateway_manifests) == 0
    error_message = "Generic Gateway API enablement must not install inference CRDs."
  }
  assert {
    condition     = length(google_container_cluster.gke_cluster.gateway_api_config) == 1
    error_message = "Gateway API must be enabled by either flag, without changing the default."
  }
  assert {
    condition     = google_container_cluster.gke_cluster.gateway_api_config[0].channel == "CHANNEL_STANDARD" && google_container_cluster.gke_cluster.addons_config[0].http_load_balancing[0].disabled == false
    error_message = "Both Gateway features require the Standard channel and HTTP load balancing."
  }
}

run "gateway_false_inference_true" {
  command = plan
  variables {
    enable_gateway_api       = false
    enable_inference_gateway = true
  }
  assert {
    condition = length(local.inference_gateway_manifests) == 1
    error_message = "Generic Gateway API enablement must not install inference CRDs."
  }
  assert {
    condition     = local.inference_gateway_manifests[0].name == "inference-gateway"
    error_message = "The inference manifest name must remain stable to avoid Helm release renames."
  }
  assert {
    condition = local.inference_gateway_manifests[0].source == "https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/download/v1.0.0/manifests.yaml"
    error_message = "Existing inference deployments must retain their CRD version."
  }
  assert {
    condition     = length(google_container_cluster.gke_cluster.gateway_api_config) == 1
    error_message = "Gateway API must be enabled by either flag, without changing the default."
  }
  assert {
    condition     = google_container_cluster.gke_cluster.gateway_api_config[0].channel == "CHANNEL_STANDARD" && google_container_cluster.gke_cluster.addons_config[0].http_load_balancing[0].disabled == false
    error_message = "Both Gateway features require the Standard channel and HTTP load balancing."
  }
}

run "gateway_true_inference_true" {
  command = plan
  variables {
    enable_gateway_api       = true
    enable_inference_gateway = true
  }
  assert {
    condition = length(local.inference_gateway_manifests) == 1
    error_message = "Generic Gateway API enablement must not install inference CRDs."
  }
  assert {
    condition     = local.inference_gateway_manifests[0].name == "inference-gateway"
    error_message = "The inference manifest name must remain stable to avoid Helm release renames."
  }
  assert {
    condition = local.inference_gateway_manifests[0].source == "https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/download/v1.0.0/manifests.yaml"
    error_message = "Existing inference deployments must retain their CRD version."
  }
  assert {
    condition     = length(google_container_cluster.gke_cluster.gateway_api_config) == 1
    error_message = "Gateway API must be enabled by either flag, without changing the default."
  }
  assert {
    condition     = google_container_cluster.gke_cluster.gateway_api_config[0].channel == "CHANNEL_STANDARD" && google_container_cluster.gke_cluster.addons_config[0].http_load_balancing[0].disabled == false
    error_message = "Both Gateway features require the Standard channel and HTTP load balancing."
  }
}
