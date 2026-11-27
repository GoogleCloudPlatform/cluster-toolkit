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
  install_kueue   = try(var.kueue.install, false)
  enable_pathways = var.enable_pathways_for_tpus || var.kueue.enable_pathways_for_tpus
  enable_slicing  = var.kueue.enable_dynamic_slicing_for_tpus

  kueue_default_config_template = lookup({
    "true-true"  = "${path.module}/kueue/kueue-configuration-dynamic-slicing-pathways.yaml.tftpl",
    "false-true" = "${path.module}/kueue/kueue-configuration-dynamic-slicing.yaml.tftpl",
    "true-false" = "${path.module}/kueue/kueue-configuration-pathways.yaml.tftpl",
  }, "${local.enable_pathways}-${local.enable_slicing}", "")

  kueue_config_template_vars = merge(
    {
      namespace               = "default"
      pathways_cpu_quota      = 480
      pathways_memory_quota   = "2000G"
      tpu_flavor_cpu_quota    = "999999" # High default to avoid limiting TPU pods by CPU
      tpu_flavor_memory_quota = "999999T"
      tpu_quota               = "999999" # Default high value if not set
      tpu_topology            = ""
    },
    var.kueue.config_template_vars != null ? var.kueue.config_template_vars : {}
  )

  kueue_default_config_content = (local.install_kueue && local.kueue_user_config_content == "" && local.kueue_default_config_template != "") ? (
    endswith(local.kueue_default_config_template, ".tftpl") ?
    templatefile(local.kueue_default_config_template, local.kueue_config_template_vars) :
    file(local.kueue_default_config_template)
  ) : ""

  kueue_user_config_content = (local.install_kueue && var.kueue.config_path != "" && var.kueue.config_path != null) ? (
    endswith(var.kueue.config_path, ".tftpl") || (var.kueue.config_template_vars != null && length(var.kueue.config_template_vars) > 0) ?
    templatefile(var.kueue.config_path, local.kueue_config_template_vars) :
    file(var.kueue.config_path)
  ) : ""

  kueue_config_content = local.kueue_user_config_content != "" ? local.kueue_user_config_content : local.kueue_default_config_content

  configure_kueue       = local.install_kueue && local.kueue_config_content != ""
  webhook_wait_duration = "60s"

  kueue_docs            = [for doc in split("\n---", local.kueue_config_content) : trimspace(doc) if length(trimspace(doc)) > 0]
  parsed_kueue_docs     = [for doc in local.kueue_docs : yamldecode(doc)]
  cluster_queues        = [for doc in local.parsed_kueue_docs : doc if try(doc.kind, "") == "ClusterQueue"]
  other_docs            = [for doc in local.parsed_kueue_docs : doc if try(doc.kind, "") != "ClusterQueue"]
  merged_cluster_queues = { for cq in local.cluster_queues : cq.metadata.name => cq... }
  final_cluster_queues = [
    for name, cqs in local.merged_cluster_queues : {
      apiVersion = cqs[0].apiVersion
      kind       = cqs[0].kind
      metadata   = merge([for cq in cqs : cq.metadata if try(cq.metadata, null) != null]...)
      spec = merge(
        merge([for cq in cqs : cq.spec if try(cq.spec, null) != null]...),
        {
          resourceGroups = flatten([for cq in cqs : try(cq.spec.resourceGroups, [])])
        }
      )
    }
  ]
  final_kueue_manifests = concat([for doc in local.other_docs : yamlencode(doc)], [for doc in local.final_cluster_queues : yamlencode(doc)])

  kueue_controller_cpu      = try(var.kueue.controller_cpu, null) != null ? var.kueue.controller_cpu : (local.enable_slicing ? "16" : null)
  kueue_controller_memory   = try(var.kueue.controller_memory, null) != null ? var.kueue.controller_memory : (local.enable_slicing ? "64Gi" : null)
  kueue_controller_replicas = try(var.kueue.controller_replicas, null) != null ? var.kueue.controller_replicas : (local.enable_slicing ? 3 : null)

  kueue_controller_sizing = merge(
    local.kueue_controller_replicas != null ? { replicas = local.kueue_controller_replicas } : {},
    local.kueue_controller_cpu != null || local.kueue_controller_memory != null ? {
      manager = {
        resources = {
          requests = merge(
            local.kueue_controller_cpu != null ? { cpu = local.kueue_controller_cpu } : {},
            local.kueue_controller_memory != null ? { memory = local.kueue_controller_memory } : {}
          )
          limits = merge(
            local.kueue_controller_cpu != null ? { cpu = local.kueue_controller_cpu } : {},
            local.kueue_controller_memory != null ? { memory = local.kueue_controller_memory } : {}
          )
        }
      }
    } : {}
  )

  kueue_enable_wait_for_pods_ready = (
    module.kueue_pods_ready_default_on_check.is_greater_than_or_equal &&
    (var.kueue.wait_for_pods_ready != null || local.enable_pathways)
  )
  kueue_pods_ready_recovery_timeout = (
    try(var.kueue.wait_for_pods_ready.recovery_timeout, null) != null ?
    var.kueue.wait_for_pods_ready.recovery_timeout :
    (local.enable_pathways && module.kueue_pods_ready_default_on_check.is_greater_than_or_equal ? "0s" : null)
  )
  kueue_wait_for_pods_ready = local.kueue_enable_wait_for_pods_ready ? merge(
    { timeout = coalesce(try(var.kueue.wait_for_pods_ready.timeout, null), "30m") },
    local.kueue_pods_ready_recovery_timeout != null ? { recoveryTimeout = local.kueue_pods_ready_recovery_timeout } : {},
    try(var.kueue.wait_for_pods_ready.block_admission, null) != null ? { blockAdmission = var.kueue.wait_for_pods_ready.block_admission } : {}
  ) : null

  # Base Kueue Helm chart values and custom values rendering/validation
  kueue_base_helm_values_raw = file("${path.module}/kueue/kueue-helm-values.yaml")
  kueue_base_helm_values     = yamldecode(local.kueue_base_helm_values_raw)

  kueue_helm_values_path        = try(var.kueue.helm_values_path, null) != null ? trimspace(var.kueue.helm_values_path) : ""
  kueue_helm_values_file_exists = local.install_kueue && local.kueue_helm_values_path != "" ? fileexists(local.kueue_helm_values_path) : false

  kueue_user_helm_values_raw = local.kueue_helm_values_file_exists ? (
    length(try(keys(var.kueue.helm_values_template_vars), [])) > 0 ?
    templatefile(local.kueue_helm_values_path, var.kueue.helm_values_template_vars) :
    can(regex("\\.tftpl(_[0-9a-f]{10})?$", local.kueue_helm_values_path)) ?
    templatefile(local.kueue_helm_values_path, {}) :
    file(local.kueue_helm_values_path)
  ) : ""
  kueue_user_helm_values_is_blank = (
    local.kueue_user_helm_values_raw == "" ||
    length(regexall("(?m)^\\s*[^#\\s]", local.kueue_user_helm_values_raw)) == 0
  )
  kueue_user_helm_values_content = (
    local.kueue_user_helm_values_is_blank || try(yamldecode(local.kueue_user_helm_values_raw) == null, false)
    ? ""
    : local.kueue_user_helm_values_raw
  )

  kueue_helm_values_is_mapping = (
    local.kueue_user_helm_values_content == "" ||
    can(keys(yamldecode(local.kueue_user_helm_values_content)))
  )
  kueue_user_helm_values_parsed = yamldecode(
    local.kueue_user_helm_values_content != "" && local.kueue_helm_values_is_mapping
    ? local.kueue_user_helm_values_content
    : "{}"
  )
  kueue_helm_values_not_k8s_manifest = (
    !contains(keys(local.kueue_user_helm_values_parsed), "apiVersion") &&
    !contains(keys(local.kueue_user_helm_values_parsed), "kind") &&
    !(
      !local.kueue_helm_values_is_mapping &&
      can(regex("(?m)^(apiVersion|kind)\\s*:", local.kueue_user_helm_values_content))
    )
  )
  kueue_helm_values_check_fields = (
    local.kueue_helm_values_file_exists &&
    local.kueue_helm_values_is_mapping &&
    local.kueue_helm_values_not_k8s_manifest
  )

  kueue_helm_values_preserves_fullname = alltrue([
    for k in ["fullnameOverride", "nameOverride"] :
    !contains(keys(local.kueue_user_helm_values_parsed), k) ||
    try(local.kueue_user_helm_values_parsed[k] == null, false) ||
    try(
      local.kueue_user_helm_values_parsed[k] == tostring(local.kueue_user_helm_values_parsed[k]) &&
      contains(["", "kueue"], tostring(local.kueue_user_helm_values_parsed[k])),
      false
    )
  ])
  kueue_user_controller_manager_valid = (
    !contains(keys(local.kueue_user_helm_values_parsed), "controllerManager") ||
    can(keys(local.kueue_user_helm_values_parsed.controllerManager))
  )
  kueue_user_manager_config_container_valid = (
    !contains(keys(local.kueue_user_helm_values_parsed), "managerConfig") ||
    can(keys(local.kueue_user_helm_values_parsed.managerConfig))
  )
  kueue_user_cert_manager_order_valid = (
    !local.install_cert_manager ||
    !contains(keys(local.kueue_user_helm_values_parsed), "enableCertManager") ||
    try(local.kueue_user_helm_values_parsed.enableCertManager == null, false) ||
    try(local.kueue_user_helm_values_parsed.enableCertManager == false, false)
  )

  # Feature-gate merging by .name (preserves TopologyAwareScheduling and Kueue >= 0.18 DRA disables unless explicitly overridden)
  kueue_has_user_feature_gates_field = contains(
    try(keys(local.kueue_user_helm_values_parsed.controllerManager), []), "featureGates"
  )
  kueue_user_feature_gates_valid = (
    !local.kueue_has_user_feature_gates_field || (
      try(local.kueue_user_helm_values_parsed.controllerManager.featureGates != null, false) &&
      !can(keys(local.kueue_user_helm_values_parsed.controllerManager.featureGates)) &&
      try(alltrue([
        for fg in local.kueue_user_helm_values_parsed.controllerManager.featureGates :
        can(keys(fg)) && fg.name != null && fg.name == tostring(fg.name) &&
        trimspace(tostring(fg.name)) != "" && (fg.enabled == true || fg.enabled == false)
      ]), false)
    )
  )
  kueue_user_feature_gates = try([
    for fg in local.kueue_user_helm_values_parsed.controllerManager.featureGates :
    { name = tostring(fg.name), enabled = tobool(fg.enabled) }
    if local.kueue_has_user_feature_gates_field && local.kueue_user_feature_gates_valid
  ], [])
  kueue_user_feature_gates_by_name = {
    for name, vals in { for fg in local.kueue_user_feature_gates : fg.name => fg.enabled... } :
    name => vals[length(vals) - 1]
  }
  kueue_base_feature_gates = concat(
    local.kueue_base_helm_values.controllerManager.featureGates,
    [
      for name in [
        "KueueDRAIntegration",
        "KueueDRARejectWorkloadsWhenDRADisabled",
        "KueueDRAIntegrationExtendedResource",
        "KueueDRAIntegrationPartitionableDevices",
      ] : { name = name, enabled = false }
      if module.kueue_dra_version_check.is_greater_than_or_equal
    ]
  )
  kueue_merged_feature_gates = concat(
    [
      for fg in local.kueue_base_feature_gates : {
        name    = fg.name
        enabled = lookup(local.kueue_user_feature_gates_by_name, fg.name, fg.enabled)
      }
    ],
    [
      for name in distinct([for fg in local.kueue_user_feature_gates : fg.name]) : {
        name    = name
        enabled = local.kueue_user_feature_gates_by_name[name]
      }
      if !contains([for fg in local.kueue_base_feature_gates : fg.name], name)
    ]
  )

  # Toleration preservation (keeps GKE system node-pool gke-managed-components toleration unless overridden)
  kueue_has_user_tolerations_field = contains(
    try(keys(local.kueue_user_helm_values_parsed.controllerManager), []), "tolerations"
  )
  kueue_user_tolerations_valid = (
    !local.kueue_has_user_tolerations_field || (
      try(local.kueue_user_helm_values_parsed.controllerManager.tolerations != null, false) &&
      !can(keys(local.kueue_user_helm_values_parsed.controllerManager.tolerations)) &&
      try(alltrue([
        for t in local.kueue_user_helm_values_parsed.controllerManager.tolerations :
        t != null && can(keys(t))
      ]), false)
    )
  )
  kueue_has_user_tolerations = local.kueue_has_user_tolerations_field && local.kueue_user_tolerations_valid
  kueue_user_tolerations = try([
    for t in local.kueue_user_helm_values_parsed.controllerManager.tolerations : t
    if local.kueue_has_user_tolerations
  ], [])
  kueue_merged_tolerations = concat([
    for t in local.kueue_base_helm_values.controllerManager.tolerations : t
    if !contains([for ut in local.kueue_user_tolerations : try(ut.key, "")], t.key)
  ], local.kueue_user_tolerations)

  # Structured merge for managerConfig.controllerManagerConfigYaml (maps merged, lists replaced, nulls omitted)
  kueue_has_user_manager_config_field = contains(
    try(keys(local.kueue_user_helm_values_parsed.managerConfig), []), "controllerManagerConfigYaml"
  )
  kueue_user_manager_config_valid_yaml = (
    !local.kueue_has_user_manager_config_field || try(
      local.kueue_user_helm_values_parsed.managerConfig.controllerManagerConfigYaml != null &&
      local.kueue_user_helm_values_parsed.managerConfig.controllerManagerConfigYaml == tostring(local.kueue_user_helm_values_parsed.managerConfig.controllerManagerConfigYaml) &&
      !can(keys(local.kueue_user_helm_values_parsed.managerConfig.controllerManagerConfigYaml)) &&
      trimspace(local.kueue_user_helm_values_parsed.managerConfig.controllerManagerConfigYaml) != "" &&
      can(keys(yamldecode(trimspace(local.kueue_user_helm_values_parsed.managerConfig.controllerManagerConfigYaml)))),
      false
    )
  )
  kueue_user_manager_config = yamldecode(
    local.kueue_has_user_manager_config_field && local.kueue_user_manager_config_valid_yaml
    ? trimspace(local.kueue_user_helm_values_parsed.managerConfig.controllerManagerConfigYaml)
    : "{}"
  )
  kueue_user_manager_config_valid_kind = (
    !local.kueue_has_user_manager_config_field ||
    !local.kueue_user_manager_config_valid_yaml || try(
      try(local.kueue_user_manager_config.apiVersion, "config.kueue.x-k8s.io/v1beta2") == "config.kueue.x-k8s.io/v1beta2" &&
      try(local.kueue_user_manager_config.kind, "Configuration") == "Configuration",
      false
    )
  )
  kueue_user_manager_config_no_feature_gates = (
    !local.kueue_has_user_manager_config_field ||
    !local.kueue_user_manager_config_valid_yaml ||
    !contains(keys(local.kueue_user_manager_config), "featureGates") ||
    try(local.kueue_user_manager_config.featureGates == null, false)
  )

  kueue_raw_base_manager_config = yamldecode(file("${path.module}/kueue/kueue-manager-config.yaml"))
  kueue_base_manager_config = merge(
    local.kueue_raw_base_manager_config,
    {
      controller = merge(
        local.kueue_raw_base_manager_config.controller,
        {
          groupKindConcurrency = {
            for k, v in local.kueue_raw_base_manager_config.controller.groupKindConcurrency : k => v
            if k != "Cohort.kueue.x-k8s.io" || module.kueue_pods_ready_default_on_check.is_greater_than_or_equal
          }
        }
      )
      integrations = merge(
        local.kueue_raw_base_manager_config.integrations,
        {
          frameworks = concat(
            [
              for f in local.kueue_raw_base_manager_config.integrations.frameworks : f
              if(f != "ray.io/rayservice" || !can(regex("^v?0\\.(15|16)\\.", var.kueue.version))) &&
              (!contains(["pod", "deployment", "statefulset", "leaderworkerset.x-k8s.io/leaderworkerset"], f) || !can(regex("^v?0\\.15\\.", var.kueue.version)))
            ],
            [
              for f in [
                "kubeflow.org/paddlejob",
                "kubeflow.org/pytorchjob",
                "kubeflow.org/tfjob",
                "kubeflow.org/xgboostjob",
                "kubeflow.org/jaxjob",
              ] : f
              if !module.kueue_pods_ready_default_on_check.is_greater_than_or_equal
            ]
          )
        }
      )
    },
    local.kueue_wait_for_pods_ready != null ? { waitForPodsReady = local.kueue_wait_for_pods_ready } : {}
  )

  kueue_user_manager_config_sections_valid = (
    !local.kueue_has_user_manager_config_field || !local.kueue_user_manager_config_valid_yaml || alltrue(flatten([
      for k, bv in merge({ waitForPodsReady = {} }, local.kueue_base_manager_config) : concat(
        [
          !contains(keys(local.kueue_user_manager_config), k) ||
          try(local.kueue_user_manager_config[k] == null, false) ||
          can(keys(local.kueue_user_manager_config[k]))
        ],
        [
          for sk, bsv in bv : (
            !can(keys(local.kueue_user_manager_config[k])) ||
            !contains(keys(local.kueue_user_manager_config[k]), sk) ||
            try(local.kueue_user_manager_config[k][sk] == null, false) ||
            can(keys(local.kueue_user_manager_config[k][sk]))
          )
          if can(keys(bsv))
        ]
      )
      if can(keys(bv))
    ]))
  )

  kueue_merged_manager_config = merge(
    { for k, v in local.kueue_base_manager_config : k => v if !contains(keys(local.kueue_user_manager_config), k) },
    { for k, v in local.kueue_user_manager_config : k => v if v != null },
    { for k, uv in local.kueue_user_manager_config : k => merge(
      { for sk, sv in local.kueue_base_manager_config[k] : sk => sv if !contains(keys(uv), sk) },
      { for sk, sv in uv : sk => sv if sv != null },
      { for sk, suv in uv : sk => merge(
        { for gk, gv in local.kueue_base_manager_config[k][sk] : gk => gv if !contains(keys(suv), gk) },
        { for gk, gv in suv : gk => gv if gv != null }
      ) if suv != null && can(keys(local.kueue_base_manager_config[k][sk])) && can(keys(suv)) }
    ) if uv != null && can(keys(local.kueue_base_manager_config[k])) && can(keys(uv)) }
  )

  kueue_manager_config_values_yaml = (
    local.kueue_wait_for_pods_ready != null || local.kueue_has_user_manager_config_field
    ? yamlencode({
      managerConfig = {
        controllerManagerConfigYaml = yamlencode(local.kueue_merged_manager_config)
      }
    })
    : ""
  )
}

resource "terraform_data" "kueue_validations" {
  lifecycle {
    precondition {
      condition     = !var.kueue.install || contains(local.kueue_supported_versions, var.kueue.version)
      error_message = "Supported version of Kueue are ${join(", ", local.kueue_supported_versions)}"
    }
    precondition {
      condition     = !var.kueue.install || !(var.enable_pathways_for_tpus || try(var.kueue.enable_pathways_for_tpus, false)) || (var.kueue.config_path != null && var.kueue.config_path != "") || contains(keys(coalesce(var.kueue.config_template_vars, {})), "accelerator_type")
      error_message = "accelerator_type must be set in kueue.config_template_vars when using the default pathways configuration."
    }
    precondition {
      condition     = !var.kueue.enable_dynamic_slicing_for_tpus || var.kueue.accelerator_topology_mode == "PROVISION_ONLY"
      error_message = "When enable_dynamic_slicing_for_tpus is true, accelerator_topology_mode must be 'PROVISION_ONLY'."
    }
    precondition {
      condition     = !var.kueue.enable_dynamic_slicing_for_tpus || (var.kueue.machine_type != null && length(regexall("^(tpu|ct)", var.kueue.machine_type)) > 0)
      error_message = "When enable_dynamic_slicing_for_tpus is true, machine_type must be a TPU machine type."
    }
    precondition {
      condition     = !var.kueue.enable_dynamic_slicing_for_tpus || (var.kueue.config_path != null && var.kueue.config_path != "") || (var.kueue.config_template_vars != null && contains(keys(var.kueue.config_template_vars), "accelerator_type"))
      error_message = "accelerator_type must be set in kueue.config_template_vars when using the default dynamic slicing configuration."
    }
    precondition {
      condition     = !(var.kueue.enable_dynamic_slicing_for_tpus && !var.kueue.install)
      error_message = "Slice controller requires Kueue to be installed. Set kueue.install to true when kueue.enable_dynamic_slicing_for_tpus is true."
    }
    precondition {
      condition     = !(var.kueue.enable_dynamic_slicing_for_tpus && !var.jobset.install)
      error_message = "Slice controller requires Jobset to be installed. Set jobset.install to true when kueue.enable_dynamic_slicing_for_tpus is true."
    }
    precondition {
      condition     = !var.kueue.install || var.kueue.wait_for_pods_ready == null || module.kueue_pods_ready_default_on_check.is_greater_than_or_equal
      error_message = "kueue.wait_for_pods_ready is only supported on Kueue >= 0.19.0."
    }
    precondition {
      condition     = !local.install_kueue || local.kueue_helm_values_path == "" || local.kueue_helm_values_file_exists
      error_message = "kueue.helm_values_path '${local.kueue_helm_values_path}' does not exist. Ensure the file exists and is staged with ghpc_stage(...) when referenced from a blueprint."
    }
    precondition {
      condition     = !local.kueue_helm_values_file_exists || !local.kueue_helm_values_not_k8s_manifest || local.kueue_helm_values_is_mapping
      error_message = "kueue.helm_values_path must render to a valid single-document YAML mapping of Kueue Helm chart values."
    }
    precondition {
      condition     = !local.kueue_helm_values_file_exists || local.kueue_helm_values_not_k8s_manifest
      error_message = "kueue.helm_values_path expects Kueue Helm chart values (for example managerConfig.controllerManagerConfigYaml and controllerManager.featureGates), not a Kubernetes manifest with top-level 'apiVersion' or 'kind'. Wrap Kueue Configuration YAML under managerConfig.controllerManagerConfigYaml, or use kueue.config_path for post-install resources such as ClusterQueue and LocalQueue."
    }
    precondition {
      condition     = !local.kueue_helm_values_check_fields || local.kueue_helm_values_preserves_fullname
      error_message = "fullnameOverride and nameOverride in kueue.helm_values_path must not be customized (leave unset, empty, or 'kueue') so gcluster job can locate kueue-controller-manager and kueue-webhook-service."
    }
    precondition {
      condition     = !local.kueue_helm_values_check_fields || local.kueue_user_cert_manager_order_valid
      error_message = "enableCertManager: true in kueue.helm_values_path cannot be combined with cert_manager.install = true in the same kubectl-apply module because Kueue is installed before cert-manager; pre-install cert-manager in an earlier module or deployment group first."
    }
    precondition {
      condition     = !local.kueue_helm_values_check_fields || local.kueue_user_controller_manager_valid
      error_message = "controllerManager in kueue.helm_values_path must be a YAML mapping."
    }
    precondition {
      condition     = !local.kueue_helm_values_check_fields || local.kueue_user_feature_gates_valid
      error_message = "controllerManager.featureGates in kueue.helm_values_path must be a YAML list of {name, enabled} objects with non-empty string name and boolean enabled (for example: [{name: 'VisibilityOnDemand', enabled: true}])."
    }
    precondition {
      condition     = !local.kueue_helm_values_check_fields || local.kueue_user_tolerations_valid
      error_message = "controllerManager.tolerations in kueue.helm_values_path must be a YAML list of toleration objects."
    }
    precondition {
      condition     = !local.kueue_helm_values_check_fields || local.kueue_user_manager_config_container_valid
      error_message = "managerConfig in kueue.helm_values_path must be a YAML mapping."
    }
    precondition {
      condition     = !local.kueue_helm_values_check_fields || local.kueue_user_manager_config_valid_yaml
      error_message = "managerConfig.controllerManagerConfigYaml in kueue.helm_values_path must be a YAML string (use '|') containing a valid single-document Kueue Configuration mapping."
    }
    precondition {
      condition     = !local.kueue_helm_values_check_fields || !local.kueue_user_manager_config_valid_yaml || local.kueue_user_manager_config_valid_kind
      error_message = "managerConfig.controllerManagerConfigYaml in kueue.helm_values_path must have apiVersion 'config.kueue.x-k8s.io/v1beta2' and kind 'Configuration' (or omit them to inherit defaults)."
    }
    precondition {
      condition     = !local.kueue_helm_values_check_fields || !local.kueue_user_manager_config_valid_yaml || !local.kueue_user_manager_config_valid_kind || local.kueue_user_manager_config_no_feature_gates
      error_message = "featureGates cannot be set inside managerConfig.controllerManagerConfigYaml because Kueue rejects specifying feature gates in both CLI flags and Configuration; set controllerManager.featureGates in kueue.helm_values_path instead."
    }
    precondition {
      condition     = !local.kueue_helm_values_check_fields || !local.kueue_user_manager_config_valid_yaml || !local.kueue_user_manager_config_valid_kind || local.kueue_user_manager_config_sections_valid
      error_message = "Nested configuration sections in managerConfig.controllerManagerConfigYaml (health, metrics, webhook, leaderElection, clientConnection, integrations, controller, controller.groupKindConcurrency, waitForPodsReady) must be YAML mappings when set."
    }
  }
}

module "kueue_pods_ready_default_on_check" {
  source          = "../../internal/semver_compare"
  current_version = var.kueue.version
  minimum_version = "0.19.0"
}

module "kueue_dra_version_check" {
  source          = "../../internal/semver_compare"
  current_version = var.kueue.version
  minimum_version = "0.18.0"
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
  values_yaml = local.kueue_user_helm_values_content == "" ? compact([
    local.kueue_base_helm_values_raw,
    local.kueue_manager_config_values_yaml,
    length(keys(local.kueue_controller_sizing)) > 0 || module.kueue_dra_version_check.is_greater_than_or_equal ? yamlencode({
      controllerManager = merge(
        module.kueue_dra_version_check.is_greater_than_or_equal ? { featureGates = local.kueue_base_feature_gates } : {},
        local.kueue_controller_sizing,
      )
    }) : "",
    ]) : compact([
    local.kueue_base_helm_values_raw,
    length(keys(local.kueue_controller_sizing)) > 0 ? yamlencode({
      controllerManager = local.kueue_controller_sizing
    }) : "",
    local.kueue_user_helm_values_content,
    (module.kueue_dra_version_check.is_greater_than_or_equal || local.kueue_has_user_feature_gates_field) ? yamlencode({
      controllerManager = {
        featureGates = local.kueue_merged_feature_gates
      }
    }) : "",
    local.kueue_has_user_tolerations ? yamlencode({
      controllerManager = {
        tolerations = local.kueue_merged_tolerations
      }
    }) : "",
    local.kueue_manager_config_values_yaml,
  ])

  dependencies = var.system_node_pool_id != null ? [var.system_node_pool_id] : []

  depends_on = [var.gke_cluster_exists, terraform_data.kueue_validations]
}

# This sleep ensures that subsequent configuration of Kueue custom resources
# do not fail due to the webhook not being available.
resource "time_sleep" "wait_for_webhook" {
  count           = local.install_kueue ? 1 : 0
  create_duration = local.webhook_wait_duration
  depends_on      = [module.install_kueue]
}

module "configure_kueue" {
  source           = "./helm_install"
  count            = local.configure_kueue ? 1 : 0
  release_name     = "kueue-config"
  chart_name       = "${path.module}/raw-config-chart"
  chart_version    = "0.1.0"
  namespace        = "kueue-system"
  create_namespace = true
  wait             = true

  values_yaml = [
    yamlencode({
      manifests = local.final_kueue_manifests
    })
  ]

  depends_on = [time_sleep.wait_for_webhook]
}

module "install_slice_controller" {
  source        = "./helm_install"
  count         = local.enable_slicing ? 1 : 0
  release_name  = "slice-controller"
  chart_name    = "${path.module}/raw-config-chart"
  chart_version = "0.1.0"
  wait          = true

  values_yaml = [
    yamlencode({
      manifests = [templatefile("${path.module}/kueue/slice-controller.yaml.tftpl", {
        cpu_request    = var.kueue.slice_controller_cpu_request
        memory_request = var.kueue.slice_controller_memory_request
        cpu_limit      = var.kueue.slice_controller_cpu_limit
        memory_limit   = var.kueue.slice_controller_memory_limit
      })]
    })
  ]

  depends_on = [var.gke_cluster_exists, module.configure_kueue, module.install_jobset]
}
