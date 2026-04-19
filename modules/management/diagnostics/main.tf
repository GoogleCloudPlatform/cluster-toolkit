/**
  * Copyright 2026 Google LLC
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
  cluster_id_parts = split("/", var.cluster_id)
  cluster_name     = local.cluster_id_parts[5]
  cluster_location = local.cluster_id_parts[3]
  project_id       = var.project_id != null ? var.project_id : local.cluster_id_parts[1]

  mldiagnostics_namespace = "gke-mldiagnostics"

}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

data "google_client_config" "default" {}

data "kubernetes_all_namespaces" "all" {
  count      = var.mldiagnostics.enable ? 1 : 0
  depends_on = [var.kubectl_apply_ready]
}

data "kubernetes_service_account_v1" "workload_sa" {
  count = var.mldiagnostics.enable ? 1 : 0
  metadata {
    name      = var.k8s_service_account_name
    namespace = var.namespace
  }
  depends_on = [var.kubectl_apply_ready]
}

resource "kubernetes_labels" "workload_namespace_labels" {
  count       = var.mldiagnostics.enable ? 1 : 0
  api_version = "v1"
  kind        = "Namespace"

  metadata {
    name = var.namespace
  }

  labels = {
    "managed-mldiagnostics-gke" = "true"
  }

  depends_on = [
    terraform_data.validate_namespace,
    terraform_data.validate_sa,
    terraform_data.validate_cert_manager
  ]
}

module "install_mldiagnostics_webhook" {
  source           = "../kubectl-apply/helm_install"
  count            = var.mldiagnostics.enable ? 1 : 0
  wait             = true
  timeout          = 1200
  release_name     = "mld-webhook"
  chart_name       = "oci://us-docker.pkg.dev/ai-on-gke/mldiagnostics-webhook-and-operator-helm/mldiagnostics-injection-webhook"
  chart_repository = ""
  chart_version    = var.mldiagnostics.injection_webhook_version
  namespace        = local.mldiagnostics_namespace
  create_namespace = true
  depends_on       = [var.gke_cluster_exists, var.kubectl_apply_ready, kubernetes_labels.workload_namespace_labels]
}

module "install_mldiagnostics_connection_operator" {
  source           = "../kubectl-apply/helm_install"
  count            = var.mldiagnostics.enable ? 1 : 0
  wait             = true
  timeout          = 1200
  release_name     = "mld-op"
  chart_name       = "oci://us-docker.pkg.dev/ai-on-gke/mldiagnostics-webhook-and-operator-helm/mldiagnostics-connection-operator"
  chart_repository = ""
  chart_version    = var.mldiagnostics.connection_operator_version
  namespace        = local.mldiagnostics_namespace
  create_namespace = false
  set_values       = [{ name = "fullnameOverride", value = "mld-op" }]
  depends_on       = [var.gke_cluster_exists, var.kubectl_apply_ready, module.install_mldiagnostics_webhook]
}
