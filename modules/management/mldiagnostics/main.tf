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

  install_cert_manager                      = try(var.cert_manager.install, false)
  install_mldiagnostics_connection_operator = try(var.mldiagnostics_connection_operator.install, false)
  install_mldiagnostics_webhook             = try(var.mldiagnostics_webhook.install, false)
}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

data "google_client_config" "default" {}

resource "kubectl_manifest" "mldiagnostics_namespace" {
  yaml_body = <<YAML
apiVersion: v1
kind: Namespace
metadata:
  name: ${var.namespace}
  labels:
    diagon-enabled: "true"
    managed-mldiagnostics-gke: "true"
YAML

  depends_on = [var.gke_cluster_exists]
}

module "install_cert_manager" {
  source           = "../kubectl-apply/helm_install"
  count            = local.install_cert_manager ? 1 : 0
  wait_for_jobs    = true
  timeout          = 1200
  release_name     = "cert-manager"
  chart_repository = "https://charts.jetstack.io"
  chart_name       = "cert-manager"
  namespace        = "cert-manager"
  set_values       = [{ name = "installCRDs", value = "true", type = "auto" }]
  depends_on       = [var.gke_cluster_exists, var.workload_manager_wait]
}

module "install_mldiagnostics_webhook" {
  source           = "../kubectl-apply/helm_install"
  count            = local.install_mldiagnostics_webhook ? 1 : 0
  wait_for_jobs    = true
  timeout          = 1200
  release_name     = "mld-webhook"
  chart_name       = "oci://us-docker.pkg.dev/ai-on-gke/mldiagnostics-webhook-and-operator-helm/mldiagnostics-injection-webhook"
  chart_repository = ""
  namespace        = "gke-mldiagnostics"
  create_namespace = true
  depends_on       = [var.gke_cluster_exists, module.install_cert_manager]
}

module "install_mldiagnostics_connection_operator" {
  source           = "../kubectl-apply/helm_install"
  count            = local.install_mldiagnostics_connection_operator ? 1 : 0
  wait_for_jobs    = true
  timeout          = 1200
  release_name     = "mld-op"
  chart_name       = "oci://us-docker.pkg.dev/ai-on-gke/mldiagnostics-webhook-and-operator-helm/mldiagnostics-connection-operator"
  chart_repository = ""
  namespace        = "gke-mldiagnostics"
  create_namespace = false
  set_values       = [{ name = "fullnameOverride", value = "mld-op" }]
  depends_on       = [var.gke_cluster_exists, module.install_cert_manager, module.install_mldiagnostics_webhook]
}
