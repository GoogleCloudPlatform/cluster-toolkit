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


  install_kueue             = try(var.kueue.install, false)
  install_jobset            = try(var.jobset.install, false)
  install_gpu_operator      = try(var.gpu_operator.install, false)
  install_nvidia_dra_driver = try(var.nvidia_dra_driver.install, false)
  install_gib               = try(var.gib.install, false)
  install_asapd_lite        = try(var.asapd_lite.install, false)

  enable_slice_controller      = var.kueue.enable_slice_controller
  kueue_super_slicing_cpu      = "16"
  kueue_super_slicing_memory   = "64Gi"
  kueue_super_slicing_replicas = 3
  jobset_super_slicing_cpu     = "4"
  jobset_super_slicing_memory  = "16Gi"
  kueue_cpu                    = var.kueue.controller_cpu != null ? var.kueue.controller_cpu : (local.enable_slice_controller ? local.kueue_super_slicing_cpu : null)
  kueue_memory                 = var.kueue.controller_memory != null ? var.kueue.controller_memory : (local.enable_slice_controller ? local.kueue_super_slicing_memory : null)
  kueue_replicas               = var.kueue.controller_replicas != null ? var.kueue.controller_replicas : (local.enable_slice_controller ? local.kueue_super_slicing_replicas : null)
  kueue_custom_resources       = var.kueue.controller_replicas != null || var.kueue.controller_cpu != null || var.kueue.controller_memory != null || local.enable_slice_controller

  jobset_cpu              = var.jobset.controller_cpu != null ? var.jobset.controller_cpu : (local.enable_slice_controller ? local.jobset_super_slicing_cpu : null)
  jobset_memory           = var.jobset.controller_memory != null ? var.jobset.controller_memory : (local.enable_slice_controller ? local.jobset_super_slicing_memory : null)
  jobset_custom_resources = var.jobset.controller_cpu != null || var.jobset.controller_memory != null || local.enable_slice_controller

  kueue_config_content = join("\n---\n", compact([
    local.enable_slice_controller ? templatefile("${path.module}/kueue/super-slicing.yaml.tftpl", {
      super_slice_topology_name      = "super-slice-topology"
      super_slice_cluster_queue_name = "cluster-queue"
    }) : "",
    var.kueue.enable_pathways_for_tpus ? templatefile("${path.module}/kueue/pathways.yaml.tftpl", {
      pathways_nodepool_name = "cpu-np"
      pathways_cpu_quota     = 480
      pathways_memory_quota  = "2000G"
    }) : "",
    var.kueue.config_path != null && var.kueue.config_path != "" ? (
      endswith(var.kueue.config_path, ".tftpl") || (var.kueue.config_template_vars != null && length(var.kueue.config_template_vars) > 0) ?
      templatefile(var.kueue.config_path, var.kueue.config_template_vars != null ? var.kueue.config_template_vars : {}) :
      file(var.kueue.config_path)
    ) : ""
  ]))
  configure_kueue = local.install_kueue && (try(var.kueue.config_path, "") != "" || var.kueue.enable_pathways_for_tpus || local.enable_slice_controller)

  asapd_lite_config_content = (
    var.asapd_lite.config_path != null && var.asapd_lite.config_path != "" ?
    (
      endswith(var.asapd_lite.config_path, ".tftpl") || (var.asapd_lite.config_template_vars != null && length(var.asapd_lite.config_template_vars) > 0) ?
      templatefile(var.asapd_lite.config_path, var.asapd_lite.config_template_vars != null ? var.asapd_lite.config_template_vars : {}) :
      file(var.asapd_lite.config_path)
    ) : ""
  )

  # 1. Combine splitting and decoding into one step
  kueue_parsed = [for doc in split("\n---", local.kueue_config_content) : yamldecode(doc) if trimspace(doc) != ""]

  # 2. Group ClusterQueues by name directly from the parsed list
  cluster_queue_groups = { for doc in local.kueue_parsed : doc.metadata.name => doc... if try(doc.kind, "") == "ClusterQueue" }

  # 3. Merge ClusterQueues by inheriting cqs[0] and overwriting only the spec
  final_cluster_queues = [
    for name, cqs in local.cluster_queue_groups : merge(cqs[0], {
      spec = merge(
        try(cqs[0].spec, {}),
        { resourceGroups = flatten([for cq in cqs : try(cq.spec.resourceGroups, [])]) },
        # Aggregated admissionChecks logic
        length(distinct(compact(flatten([for cq in cqs : try(cq.spec.admissionChecks, [])])))) > 0 ? {
          admissionChecks = distinct(compact(flatten([for cq in cqs : try(cq.spec.admissionChecks, [])])))
        } : {}
      )
    })
  ]

  # 4. Construct final manifest list
  final_kueue_manifests = concat(
    [for doc in local.kueue_parsed : yamlencode(doc) if try(doc.kind, "") != "ClusterQueue"],
    [for doc in local.final_cluster_queues : yamlencode(doc)]
  )

  # 1. First, Identify manifests that are explicitly enabled.
  enabled_manifests = {
    for index, manifest in coalesce(var.apply_manifests, []) : index => manifest
    if try(manifest.enable, true)
  }

  # 2. Identify URL-based manifests
  url_manifests = {
    for index, manifest in local.enabled_manifests : index => manifest
    if try(startswith(manifest.source, "http://") || startswith(manifest.source, "https://"), false)
  }

  # 3. Identify directory-based manifests
  directory_manifests = {
    for index, manifest in local.enabled_manifests : index => manifest
    if try(manifest.source, null) != null &&
    !contains(keys(local.url_manifests), index) &&
    (try(endswith(manifest.source, "/"), false) || try((!fileexists(manifest.source) && can(fileset(manifest.source, "*"))), false))
  }

  # Pre-calculate normalized names for each manifest
  manifest_names = {
    for index, manifest in local.enabled_manifests : index =>
    trim(replace(lower(
      (try(manifest.name, null) != null ? manifest.name :
        "${substr((manifest.source != null && manifest.source != "") ? replace(basename(manifest.source), "/(\\.(tftpl|yaml|yml))+$/", "") : "${var.module_id}-raw", 0, 30)}-${substr(sha1(jsonencode(manifest)), 0, 7)}"
      )
    ), "/[^a-z0-9-]+/", "-"), "-")
  }

  # 4. Rebuild the map by populating the 'content' field for all manifests
  processed_apply_manifests_map = tomap({
    for index, manifest in local.enabled_manifests :
    local.manifest_names[index] => {
      content = (
        # Step A: Use the fetched body if it's a URL
        contains(keys(local.url_manifests), tostring(index)) ? data.http.manifest_from_url[tostring(index)].body :

        # Step B: Process directory files 
        contains(keys(local.directory_manifests), index) ? (
          join("\n---\n", [
            # Use union() to combine the results of fileset (which are sets)
            for f in union(
              fileset(manifest.source, "*.yaml"),
              fileset(manifest.source, "*.yml"),
              fileset(manifest.source, "*.tftpl")
              ) : (
              endswith(f, ".tftpl") ?
              templatefile("${trimsuffix(manifest.source, "/")}/${f}", manifest.template_vars != null ? manifest.template_vars : {}) :
              file("${trimsuffix(manifest.source, "/")}/${f}")
            )
          ])
        ) :
        # Step C: Single file logic (implied if source is provided but not a URL or Dir)
        (manifest.source != null && manifest.source != "") ? (
          endswith(manifest.source, ".tftpl") || (manifest.template_vars != null && length(manifest.template_vars) > 0) ?
          templatefile(manifest.source, manifest.template_vars != null ? manifest.template_vars : {}) :
          file(manifest.source)
        )
        :
        # Step D: Fallback to the raw 'content' field
        coalesce(manifest.content, "")
      )
      wait_for_rollout = manifest.wait_for_rollout
      namespace        = manifest.namespace
    }
  })
}

data "http" "manifest_from_url" {
  for_each = local.url_manifests
  url      = each.value.source
}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

data "google_client_config" "default" {}

module "kubectl_apply_manifests" {
  for_each   = local.processed_apply_manifests_map
  source     = "./helm_install"
  depends_on = [var.gke_cluster_exists]

  release_name  = "manifest-${each.key}"
  chart_name    = "${path.module}/raw-config-chart"
  chart_version = "0.1.0"
  namespace     = each.value.namespace
  atomic        = true
  wait          = each.value.wait_for_rollout
  timeout       = 1200
  values_yaml = [
    yamlencode({
      # Pass the entire unbroken string to Helm. Helm will parse inner '---' natively.
      manifests = length(trimspace(each.value.content)) > 0 ? [each.value.content] : []
    })
  ]
}


module "super_slicing_kueue_version_check" {
  source = "../../internal/semver_compare"

  current_version = var.kueue.version
  minimum_version = "0.15.2"
}

resource "terraform_data" "kueue_slice_controller_version_check" {
  count = local.install_kueue ? 1 : 0
  lifecycle {
    precondition {
      condition     = !local.enable_slice_controller || module.super_slicing_kueue_version_check.is_greater_than_or_equal
      error_message = "Kueue super-slicing requires Kueue version >= 0.15.2."
    }
  }
}

module "install_kueue" {
  source           = "./helm_install"
  count            = local.install_kueue ? 1 : 0
  wait             = true
  timeout          = 1200
  release_name     = "kueue"
  chart_repository = "oci://registry.k8s.io/kueue/charts"
  chart_name       = "kueue"
  chart_version    = var.kueue.version
  namespace        = "kueue-system"
  create_namespace = true
  values_yaml = compact([
    file("${path.module}/kueue/kueue-helm-values.yaml"),
    local.kueue_custom_resources ? yamlencode({
      controllerManager = merge(
        local.kueue_replicas != null ? { replicas = local.kueue_replicas } : {},
        {
          manager = {
            resources = {
              limits = {
                for k, v in {
                  cpu    = local.kueue_cpu
                  memory = local.kueue_memory
                } : k => v if v != null
              }
              requests = {
                for k, v in {
                  cpu    = local.kueue_cpu
                  memory = local.kueue_memory
                } : k => v if v != null
              }
            }
          }
        }
      )
    }) : ""
  ])

  dependencies = var.system_node_pool_id != null ? [var.system_node_pool_id] : []

  depends_on = [var.gke_cluster_exists]
}


module "configure_kueue" {
  source           = "./helm_install"
  count            = local.configure_kueue ? 1 : 0
  release_name     = "kueue-config"
  chart_name       = "${path.module}/raw-config-chart"
  chart_version    = "0.1.0"
  namespace        = "kueue-system"
  create_namespace = true
  wait             = false # Configuration resources (Queues) usually don't need wait

  values_yaml = [
    yamlencode({
      manifests = local.final_kueue_manifests
    })
  ]

  depends_on = [module.install_kueue]

}

module "install_jobset" {
  source           = "./helm_install"
  count            = local.install_jobset ? 1 : 0
  wait             = false
  timeout          = 1200
  release_name     = "jobset"
  chart_repository = "oci://registry.k8s.io/jobset/charts"
  chart_name       = "jobset"
  chart_version    = var.jobset.version
  namespace        = "jobset-system"
  create_namespace = true
  values_yaml = compact([
    file("${path.module}/jobset/jobset-helm-values.yaml"),
    local.jobset_custom_resources ? yamlencode({
      controller = {
        resources = {
          limits = {
            for k, v in {
              cpu    = local.jobset_cpu
              memory = local.jobset_memory
            } : k => v if v != null
          }
          requests = {
            for k, v in {
              cpu    = local.jobset_cpu
              memory = local.jobset_memory
            } : k => v if v != null
          }
        }
      }
    }) : ""
  ])
  depends_on = [var.gke_cluster_exists, module.configure_kueue]
}

module "install_nvidia_dra_driver" {
  count      = local.install_nvidia_dra_driver ? 1 : 0
  depends_on = [module.kubectl_apply_manifests, var.gke_cluster_exists, module.configure_kueue]
  source     = "./helm_install"

  release_name     = "nvidia-dra-driver-gpu"              # The release name
  chart_repository = "https://helm.ngc.nvidia.com/nvidia" # The Helm repository URL for nvidia charts
  chart_name       = "nvidia-dra-driver-gpu"              # The chart name
  chart_version    = var.nvidia_dra_driver.version        # The chart version
  namespace        = "nvidia-dra-driver-gpu"              # The target namespace
  create_namespace = true                                 # Equivalent to --create-namespace

  # Use the 'values' argument to pass the YAML content
  # This corresponds to the -f <(cat <<EOF ... EOF) part
  values_yaml = [<<EOF
      nvidiaDriverRoot: /home/kubernetes/bin/nvidia
      resources:
        gpus:
          enabled: false

      controller:
        affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                - matchExpressions:
                  - key: "nvidia.com/gpu"
                    operator: "DoesNotExist"

      kubeletPlugin:
        affinity:
          nodeAffinity:
            requiredDuringSchedulingIgnoredDuringExecution:
              nodeSelectorTerms:
                - matchExpressions:
                    - key: cloud.google.com/gke-accelerator
                      operator: In
                      values:
                        - ${var.nvidia_dra_driver.accelerator_type}
                    - key: kubernetes.io/arch
                      operator: In
                      values:
                        - arm64

        tolerations:
          - key: nvidia.com/gpu
            operator: Equal
            value: present
            effect: NoSchedule
          - key: kubernetes.io/arch
            operator: Equal
            value: arm64
            effect: NoSchedule

      EOF
  ]

  atomic          = true
  cleanup_on_fail = true
}

module "install_gpu_operator" {
  count            = local.install_gpu_operator ? 1 : 0
  source           = "./helm_install"
  chart_repository = "https://helm.ngc.nvidia.com/nvidia"
  depends_on       = [module.kubectl_apply_manifests, var.gke_cluster_exists]

  namespace        = "gpu-operator"
  create_namespace = true

  release_name  = "gpu-operator"
  chart_name    = "gpu-operator"
  chart_version = var.gpu_operator.version
  wait          = true

  # Use the 'values' argument to pass the YAML content
  # This corresponds to the -f <(cat <<EOF ... EOF) part
  values_yaml = [<<EOF
      hostPaths:
        driverInstallDir: /home/kubernetes/bin/nvidia
      toolkit:
        installDir: /home/kubernetes/bin/nvidia
      cdi:
        enabled: true
        default: true
      driver:
        enabled: false

      daemonsets:
        tolerations:
          - key: nvidia.com/gpu
            operator: Equal
            value: present
            effect: NoSchedule
          - key: kubernetes.io/arch
            operator: Equal
            value: arm64
            effect: NoSchedule

      node-feature-discovery:
        worker:
          tolerations:
            - key: kubernetes.io/arch
              operator: Equal
              value: arm64
              effect: NoSchedule
            - key: "node-role.kubernetes.io/master"
              operator: "Equal"
              value: ""
              effect: "NoSchedule"
            - key: "node-role.kubernetes.io/control-plane"
              operator: "Equal"
              value: ""
              effect: "NoSchedule"
            - key: nvidia.com/gpu
              operator: Exists
              effect: NoSchedule
      EOF
  ]

  atomic          = true
  cleanup_on_fail = true

}

module "install_gib" {
  source = "./helm_install"
  count  = local.install_gib ? 1 : 0

  release_name = "nccl-gib"
  chart_name   = "${path.module}/raw-config-chart"
  namespace    = "kube-system"
  wait         = true
  depends_on   = [var.gke_cluster_exists]

  values_yaml = local.install_gib ? [
    yamlencode({
      manifests = [
        templatefile(var.gib.path, var.gib.template_vars)
      ]
    })
  ] : []
}

module "install_asapd_lite" {
  source        = "./helm_install"
  count         = local.install_asapd_lite ? 1 : 0
  release_name  = "asapd-lite"
  chart_name    = "${path.module}/raw-config-chart"
  chart_version = "0.1.0"
  namespace     = "kube-system"
  wait          = true
  depends_on    = [var.gke_cluster_exists]

  values_yaml = [
    yamlencode({
      manifests = length(trimspace(local.asapd_lite_config_content)) > 0 ? [local.asapd_lite_config_content] : []
    })
  ]
}
