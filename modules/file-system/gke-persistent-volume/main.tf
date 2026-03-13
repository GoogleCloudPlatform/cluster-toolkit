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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "gke-persistent-volume", ghpc_role = "file-system" })
}

locals {
  # Flags indicating which storage type is active based on input variables.
  storage_type_active = {
    gcs       = var.gcs_bucket_name != null
    lustre    = var.lustre_id != null
    filestore = var.filestore_id != null
  }

  # Determine the active storage type name.
  active_types = [for type, is_active in local.storage_type_active : type if is_active]

  # The precondition in kubectl_manifest.pv ensures exactly one type is active.
  storage_type = length(local.active_types) > 0 ? local.active_types[0] : "unknown"

  # Map containing the base name derivation logic for each storage type.
  base_name_map = {
    gcs       = var.gcs_bucket_name
    lustre    = var.lustre_id != null ? split("/", var.lustre_id)[5] : null
    filestore = var.filestore_id != null ? split("/", var.filestore_id)[5] : null
  }
  # Retrieve the base name for the active storage type.
  base_name = local.base_name_map[local.storage_type]

  # PV and PVC names
  pv_name  = var.pv_name != null ? var.pv_name : "${local.base_name}-pv"
  pvc_name = var.pvc_name != null ? var.pvc_name : "${local.base_name}-pvc"

  # Template file paths
  pv_templates = {
    gcs       = "${path.module}/templates/gcs-pv.yaml.tftpl"
    lustre    = "${path.module}/templates/managed-lustre-pv.yaml.tftpl"
    filestore = "${path.module}/templates/filestore-pv.yaml.tftpl"
  }
  pvc_templates = {
    gcs       = "${path.module}/templates/gcs-pvc.yaml.tftpl"
    lustre    = "${path.module}/templates/managed-lustre-pvc.yaml.tftpl"
    filestore = "${path.module}/templates/filestore-pvc.yaml.tftpl"
  }

  # Common variables for all PVC templates
  common_pvc_vars = {
    pv_name   = local.pv_name
    pvc_name  = local.pvc_name
    labels    = local.labels
    capacity  = "${var.capacity_gib}Gi"
    namespace = var.namespace
  }

  # Common variables for all PV templates
  common_pv_vars = {
    pv_name  = local.pv_name
    capacity = "${var.capacity_gib}Gi"
    labels   = local.labels
  }

  # Variables for PV templates, merging common vars with type-specific ones.
  pv_template_vars = {
    gcs = merge(local.common_pv_vars, {
      mount_options = var.gcs_bucket_name != null ? split(",", var.network_storage.mount_options) : []
      bucket_name   = var.gcs_bucket_name
      namespace     = var.namespace
      pvc_name      = local.pvc_name
    })
    lustre = merge(local.common_pv_vars, {
      location        = var.lustre_id != null ? split("/", var.lustre_id)[3] : null
      project         = split("/", var.cluster_id)[1]
      instance_name   = local.base_name
      server_ip       = var.lustre_id != null ? split("@", var.network_storage.server_ip)[0] : null
      filesystem_name = var.network_storage.remote_mount
      pvc_name        = local.pvc_name
      namespace       = var.namespace
    })
    filestore = merge(local.common_pv_vars, {
      location       = var.filestore_id != null ? split("/", var.filestore_id)[3] : null
      filestore_name = local.base_name
      share_name     = trimprefix(var.network_storage.remote_mount, "/")
      ip_address     = var.network_storage.server_ip
      pvc_name       = local.pvc_name
      namespace      = var.namespace
    })
  }

  # Rendered YAML contents
  pv_content = templatefile(
    local.pv_templates[local.storage_type],
    local.pv_template_vars[local.storage_type]
  )
  pvc_content = templatefile(
    local.pvc_templates[local.storage_type],
    local.common_pvc_vars
  )

  # GKE Cluster details
  cluster_name     = split("/", var.cluster_id)[5]
  cluster_location = split("/", var.cluster_id)[3]
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
  yaml_body = local.pv_content

  lifecycle {
    precondition {
      condition     = length(local.active_types) == 1
      error_message = "Exactly one of gcs_bucket_name, filestore_id, or lustre_id must be set."
    }
  }
}

resource "kubectl_manifest" "pvc" {
  yaml_body  = local.pvc_content
  depends_on = [kubectl_manifest.pv, kubectl_manifest.pvc_namespace]
}
