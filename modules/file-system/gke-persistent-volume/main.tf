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
  labels = merge(var.labels, { ghpc_module = "gke-persistent-volume", ghpc_role = "file-system" })
}

locals {
  is_gcs    = (var.gcs_bucket_name != null)
  is_lustre = (var.lustre_id != null)

  filestore_id = (
    !local.is_gcs && !local.is_lustre ?              # If not using gcs or lustre
    var.filestore_id :                               # Then filestore_id must be provided
    "projects/empty/locations/empty/instances/empty" # Otherwise use something arbitrary as it will not be used
  )

  lustre_id = (
    local.is_lustre ?
    var.lustre_id :
    "projects/empty/locations/empty/instances/empty"
  )
  location             = local.is_lustre ? split("/", local.lustre_id)[3] : split("/", local.filestore_id)[3]
  filestore_name       = split("/", local.filestore_id)[5]
  filestore_share_name = trimprefix(var.network_storage.remote_mount, "/")

  # Determine the base_name based on which storage type is used
  base_name          = local.is_gcs ? var.gcs_bucket_name : (local.is_lustre ? split("/", local.lustre_id)[5] : local.filestore_name)
  pv_name            = var.pv_name != null ? var.pv_name : "${local.base_name}-pv"
  pvc_name           = var.pvc_name != null ? var.pvc_name : "${local.base_name}-pvc"
  list_mount_options = split(",", var.network_storage.mount_options)

  filestore_pv_contents = templatefile(
    "${path.module}/templates/filestore-pv.yaml.tftpl",
    {
      pv_name        = local.pv_name
      capacity       = "${var.capacity_gib}Gi"
      location       = local.location
      filestore_name = local.filestore_name
      share_name     = local.filestore_share_name
      ip_address     = var.network_storage.server_ip
      labels         = local.labels
      pvc_name       = local.pvc_name
      namespace      = var.namespace
    }
  )

  filestore_pvc_contents = templatefile(
    "${path.module}/templates/filestore-pvc.yaml.tftpl",
    {
      pv_name   = local.pv_name
      capacity  = "${var.capacity_gib}Gi"
      pvc_name  = local.pvc_name
      labels    = local.labels
      namespace = var.namespace
    }
  )

  gcs_pv_contents = templatefile(
    "${path.module}/templates/gcs-pv.yaml.tftpl",
    {
      pv_name       = local.pv_name
      capacity      = "${var.capacity_gib}Gi"
      labels        = local.labels
      mount_options = local.is_gcs ? local.list_mount_options : null
      bucket_name   = local.is_gcs ? var.gcs_bucket_name : ""
      namespace     = var.namespace
      pvc_name      = local.pvc_name
    }
  )

  gcs_pvc_contents = templatefile(
    "${path.module}/templates/gcs-pvc.yaml.tftpl",
    {
      pv_name   = local.pv_name
      pvc_name  = local.pvc_name
      labels    = local.labels
      capacity  = "${var.capacity_gib}Gi"
      namespace = var.namespace
    }
  )

  lustre_pv_contents = templatefile(
    "${path.module}/templates/managed-lustre-pv.yaml.tftpl",
    {
      pv_name         = local.pv_name
      capacity        = "${var.capacity_gib}Gi"
      location        = local.location
      project         = split("/", var.cluster_id)[1]
      labels          = local.labels
      instance_name   = local.base_name
      server_ip       = split("@", var.network_storage.server_ip)[0]
      filesystem_name = var.network_storage.remote_mount
      pvc_name        = local.pvc_name
      namespace       = var.namespace
    }
  )

  lustre_pvc_contents = templatefile(
    "${path.module}/templates/managed-lustre-pvc.yaml.tftpl",
    {
      pv_name   = local.pv_name
      pvc_name  = local.pvc_name
      labels    = local.labels
      capacity  = "${var.capacity_gib}Gi"
      namespace = var.namespace
    }
  )

  cluster_name     = split("/", var.cluster_id)[5]
  cluster_location = split("/", var.cluster_id)[3]
}

resource "local_file" "debug_file" {
  content  = <<-EOF
    ${local.filestore_pv_contents}
    ${local.filestore_pvc_contents}
    EOF
  filename = "${path.root}/pv-pvc-debug-file-${local.filestore_name}.yaml"
}

data "google_container_cluster" "gke_cluster" {
  name     = local.cluster_name
  location = local.cluster_location
}

data "google_client_config" "default" {}

provider "kubectl" {
  host                   = "https://${data.google_container_cluster.gke_cluster.endpoint}"
  cluster_ca_certificate = base64decode(data.google_container_cluster.gke_cluster.master_auth[0].cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
  load_config_file       = false
}

resource "kubectl_manifest" "pvc_namespace" {
  count = var.namespace != "default" ? 1 : 0

  yaml_body = templatefile("${path.module}/templates/namespace.yaml.tftpl", {
    namespace = var.namespace
  })
}

resource "kubectl_manifest" "pv" {
  yaml_body = local.is_gcs ? local.gcs_pv_contents : (local.is_lustre ? local.lustre_pv_contents : local.filestore_pv_contents)

  lifecycle {
    precondition {
      condition     = ((var.gcs_bucket_name != null ? 1 : 0) + (var.filestore_id != null ? 1 : 0) + (var.lustre_id != null ? 1 : 0)) == 1
      error_message = "Either gcs_bucket_name, filestore_id, or lustre_id must be set."
    }
  }
}

resource "kubectl_manifest" "pvc" {
  yaml_body  = local.is_gcs ? local.gcs_pvc_contents : (local.is_lustre ? local.lustre_pvc_contents : local.filestore_pvc_contents)
  depends_on = [kubectl_manifest.pv, kubectl_manifest.pvc_namespace]
}
