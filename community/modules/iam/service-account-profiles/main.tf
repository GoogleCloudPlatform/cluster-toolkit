# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

locals {
  baseline_project_roles = ["logging.logWriter", "monitoring.metricWriter"]

  # Input patterns shared by several variable validations (variables.tf).
  id_part_pattern              = "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
  instance_name_prefix_pattern = "^[a-z][-a-z0-9]{3,61}$"
  custom_role_id_pattern       = "^(projects|organizations)/[^/]+/roles/[a-zA-Z0-9_.]{3,64}$"
  act_as_target_pattern        = "^[^@[:space:]]+@${var.project_id}[.]iam[.]gserviceaccount[.]com$"
  role_stages                  = ["ALPHA", "BETA", "GA", "DEPRECATED", "DISABLED", "EAP"]
  role_deletion_policies       = ["PREVENT", "ABANDON", "DELETE"]

  # Slurm names every VM "<slurm_cluster_name>-...". Use the controller's
  # explicit slurm_cluster_name, else mirror its derived default. Not
  # coalesce(): the derived value can be "", which coalesce() rejects.
  slurm_cluster_name_default = (
    var.slurm_cluster_name != null
    ? var.slurm_cluster_name
    : substr(replace(lower(var.deployment_name), "/^[^a-z]*|[^a-z0-9]/", ""), 0, 10)
  )

  mig_permissions = [
    "compute.instanceGroupManagers.get",
    "compute.instanceGroupManagers.list",
    "compute.instanceGroupManagers.update",
    "compute.instances.setMetadata",
  ]
  # Creating and deleting a whole MIG, for capabilities whose controller code
  # creates one per job or slice. Deleting the MIG also deletes its instance
  # group, which is checked separately.
  mig_group_lifecycle_permissions = [
    "compute.instanceGroupManagers.create",
    "compute.instanceGroupManagers.delete",
    "compute.instanceGroupManagers.use",
    "compute.instanceGroups.delete",
  ]

  # Profile catalogue: predefined project roles (without "roles/"), the
  # generated role's permissions, the name-scoped role's permissions, and the
  # optional inputs each profile accepts.
  profiles = {
    "slurm-controller" = {
      title         = "Slurm Controller"
      description   = "Least-privilege permissions for the Slurm controller."
      project_roles = local.baseline_project_roles
      permissions = [
        "compute.instances.create",
        "compute.instances.delete",
        "compute.instances.get",
        "compute.instances.list",
        "compute.instances.setLabels",
        "compute.instances.setServiceAccount",
        "compute.instances.setTags",
        "compute.instances.start",
        "compute.instances.use",
        "compute.instanceTemplates.get",
        "compute.instanceTemplates.useReadOnly",
        # Needed for nodes from a custom image in this project; public images
        # grant it to everyone.
        "compute.images.useReadOnly",
        "compute.disks.create",
        "compute.disks.setLabels",
        "compute.disks.use",
        "compute.subnetworks.use",
        # networks.use covers a template that sets a network without a
        # subnetwork. networks.get is undocumented for insert/bulkInsert but
        # not yet shown unneeded.
        "compute.networks.use",
        "compute.networks.get",
        "compute.machineTypes.get",
        "compute.machineTypes.list",
        "compute.zones.get",
        "compute.zones.list",
        "compute.regions.get",
        "compute.regions.list",
        "compute.projects.get",
        "compute.acceleratorTypes.get",
        "compute.acceleratorTypes.list",
        "compute.zoneOperations.get",
        "compute.zoneOperations.list",
        "compute.regionOperations.get",
        "compute.globalOperations.get",
        # repair.py's unscoped `gcloud compute operations list` reads global,
        # regional and zonal operations; slurmsync swallows the error if one
        # is missing.
        "compute.regionOperations.list",
        "compute.globalOperations.list",
        "compute.reservations.get",
        "compute.reservations.list",
        "compute.futureReservations.get",
        # slurmsync cleans up orphaned placement policies on every cycle,
        # whether or not any partition requests placement.
        "compute.resourcePolicies.list",
        "compute.resourcePolicies.delete",
      ]
      # Lets the holder add SSH keys to a VM, so it is granted only through the
      # name-scoped role. bulkInsert requires it even though the controller
      # sets no metadata itself.
      scoped_permissions             = ["compute.instances.setMetadata"]
      default_instance_name_prefixes = ["${local.slurm_cluster_name_default}-"]
      accepted_capabilities          = ["placement_policies", "public_ips", "mig", "mig_flex", "bigquery_load", "tpu", "tpu_gce"]
      accepts_act_as                 = true
      self_act_as                    = false
      custom                         = false
    }
    "slurm-login" = {
      title                          = "Slurm Login"
      description                    = "Least-privilege permissions for Slurm login nodes."
      project_roles                  = local.baseline_project_roles
      permissions                    = []
      scoped_permissions             = []
      default_instance_name_prefixes = []
      accepted_capabilities          = []
      accepts_act_as                 = false
      self_act_as                    = false
      custom                         = false
    }
    "slurm-compute" = {
      title                          = "Slurm Compute"
      description                    = "Least-privilege permissions for Slurm compute nodes."
      project_roles                  = local.baseline_project_roles
      permissions                    = []
      scoped_permissions             = []
      default_instance_name_prefixes = []
      accepted_capabilities          = []
      accepts_act_as                 = false
      self_act_as                    = false
      custom                         = false
    }
    "image-builder" = {
      title       = "Image Builder"
      description = "Least-privilege permissions for a Packer image-building VM."
      # No project-wide storage access: grant read on the buckets a build uses,
      # e.g. startup-script's bucket_viewers for its staging bucket.
      project_roles = local.baseline_project_roles
      permissions = [
        "compute.instances.get",
      ]
      # Packer's startup-script wrapper signals completion with `gcloud compute
      # instances add-metadata` on its own packer-<hex> VM, which also needs
      # actAs on itself.
      scoped_permissions             = ["compute.instances.setMetadata"]
      default_instance_name_prefixes = ["packer-"]
      accepted_capabilities          = []
      accepts_act_as                 = false
      self_act_as                    = true
      custom                         = false
    }
    "custom" = {
      title                          = null
      description                    = null
      project_roles                  = local.baseline_project_roles
      permissions                    = []
      scoped_permissions             = []
      default_instance_name_prefixes = []
      # No mig/mig_flex/tpu_gce: their setMetadata cannot be name-scoped, and
      # custom_permissions denies setMetadata.
      accepted_capabilities = ["placement_policies", "public_ips", "bigquery_load", "tpu"]
      accepts_act_as        = false
      self_act_as           = false
      custom                = true
    }
  }

  # Opt-in capabilities, each adding only the permissions it needs.
  capability_permissions = {
    placement_policies = [
      "compute.resourcePolicies.create",
      "compute.resourcePolicies.use",
    ]
    # Separate from instances.create; without it, external-IP nodesets fail to
    # resume.
    public_ips = [
      "compute.subnetworks.useExternalIp",
    ]
    # Terraform-created MIG nodesets (provisioning_engine = "MIG"): resume and
    # suspend read the group, patch its template, and create and delete
    # instances in it (all instanceGroupManagers.update). Google requires
    # setMetadata for instanceGroupManagers.patch, and IAM Conditions cannot
    # scope an instance group manager, so it is granted unscoped.
    mig = local.mig_permissions
    # DWS Flex nodesets: mig_flex.py creates, resizes and deletes a regional
    # MIG per job.
    mig_flex = concat(local.mig_permissions, local.mig_group_lifecycle_permissions)
    # load_bq.py creates the dataset and table, updates the table schema and
    # streams rows (tabledata.insertAll); none of that creates a BigQuery job.
    bigquery_load = [
      "bigquery.datasets.create",
      "bigquery.datasets.get",
      "bigquery.tables.create",
      "bigquery.tables.get",
      "bigquery.tables.update",
      "bigquery.tables.updateData",
    ]
    # tpu.py never calls UpdateNode, so tpu.nodes.update is deliberately absent.
    tpu = [
      "tpu.acceleratortypes.get",
      "tpu.nodes.create",
      "tpu.nodes.delete",
      "tpu.nodes.get",
      "tpu.nodes.list",
      "tpu.nodes.start",
      "tpu.nodes.stop",
      "tpu.operations.get",
      "tpu.runtimeversions.get",
    ]
    # TPU machine types on Compute Engine (ct*/tpu* in a regular nodeset):
    # mig_flex.py copies the nodeset's template, ensures a workload policy and
    # creates a MIG per slice, deleting it on suspend.
    tpu_gce = concat(local.mig_permissions, local.mig_group_lifecycle_permissions, [
      "compute.images.get",
      "compute.instanceGroups.create",
      "compute.instanceTemplates.create",
      "compute.resourcePolicies.create",
      "compute.resourcePolicies.get",
      "compute.resourcePolicies.use",
      "compute.subnetworks.get",
    ])
  }
  enabled_capabilities = [for capability, enabled in {
    placement_policies = var.enable_placement_policies
    public_ips         = var.enable_public_ips
    mig                = var.enable_mig
    mig_flex           = var.enable_mig_flex
    bigquery_load      = var.enable_bigquery_load
    tpu                = var.enable_tpu
    tpu_gce            = var.enable_tpu_gce
  } : capability if enabled]
  capability_grants = flatten([for capability in local.enabled_capabilities : local.capability_permissions[capability]])

  # Denylist for custom_permissions only; the built-in catalogue is reviewed by
  # hand. Best effort: it blocks known escalation paths, not every one.
  denied_permissions = [
    "iam.serviceAccounts.actAs",
    "iam.serviceAccounts.getAccessToken",
    "iam.serviceAccounts.getOpenIdToken",
    "iam.serviceAccounts.signBlob",
    "iam.serviceAccounts.signJwt",
    "iam.serviceAccounts.implicitDelegation",
    "iam.serviceAccounts.create",
    "iam.serviceAccounts.update",
    "iam.roles.create",
    "iam.roles.update",
    "iam.roles.delete",
    "iam.roles.undelete",
    "resourcemanager.projects.update",
    "serviceusage.services.enable",
    "orgpolicy.policy.set",
    "compute.projects.setCommonInstanceMetadata",
    "compute.instances.setMetadata",
    "compute.instances.osLogin",
    "compute.instances.osAdminLogin",
    "secretmanager.versions.access",
    "cloudkms.cryptoKeyVersions.useToDecrypt",
    "storage.hmacKeys.create",
    "storage.hmacKeys.update",
    "apikeys.keys.create",
    "serviceusage.apiKeys.create",
    "cloudbuild.builds.create",
    "cloudbuild.builds.update",
    "deploymentmanager.deployments.create",
    "deploymentmanager.deployments.update",
  ]
  denied_permission_prefixes = ["iam.serviceAccountKeys.", "iam.workloadIdentityPool"]
  # Google defines over 300 *.setIamPolicy permissions; the suffix rule covers
  # all of them without a list to keep current.
  denied_permission_suffixes = [".setIamPolicy"]
  # Open Front End creates a subscription per cluster at run time, so this
  # cannot be bound on a named subscription in advance.
  allowed_despite_suffix = ["pubsub.subscriptions.setIamPolicy"]
  denied_custom_permissions = [
    for permission in var.custom_permissions : permission if contains(local.denied_permissions, permission) ||
    anytrue([for prefix in local.denied_permission_prefixes : startswith(permission, prefix)]) ||
    (anytrue([for suffix in local.denied_permission_suffixes : endswith(permission, suffix)]) && !contains(local.allowed_despite_suffix, permission))
  ]

  # The seven resource-scoped binding inputs, defined once. project defaults to
  # project_id for every kind except buckets, which have no project.
  project_binding_inputs = {
    secret              = var.secret_bindings
    artifact_registry   = var.artifact_registry_bindings
    bigquery_dataset    = var.bigquery_dataset_bindings
    pubsub_topic        = var.pubsub_topic_bindings
    pubsub_subscription = var.pubsub_subscription_bindings
    subnetwork          = var.subnetwork_bindings
  }
  binding_inputs = merge({ bucket = var.bucket_bindings }, {
    for kind, bindings in local.project_binding_inputs : kind => {
      for key, binding in bindings : key => merge(binding, { project = coalesce(binding.project, var.project_id) })
    }
  })
  binding_role_problems = flatten([
    for kind, bindings in local.binding_inputs : [
      for key, binding in bindings : "${kind}_bindings[${key}] = ${binding.role}" if !(can(regex("^roles/[a-zA-Z0-9_.]+$", binding.role)) || can(regex(local.custom_role_id_pattern, binding.role))) ||
      can(regex("^roles/(owner|editor|viewer)$|^roles/iam[.]|^roles/resourcemanager[.]", binding.role))
    ]
  ])

  profile       = local.profiles[var.profile]
  title         = coalesce(local.profile.custom ? var.custom_profile_name : local.profile.title, var.name)
  permissions   = sort(distinct(concat(local.profile.permissions, local.capability_grants, var.custom_permissions)))
  generate_role = length(local.permissions) > 0

  scoped_permissions   = sort(distinct(local.profile.scoped_permissions))
  generate_scoped_role = length(local.scoped_permissions) > 0

  instance_name_prefixes = var.instance_name_prefixes != null ? var.instance_name_prefixes : local.profile.default_instance_name_prefixes
  # Fixed-length description: five 62-character prefixes could exceed the
  # 300-byte limit; the prefixes are in the expression.
  scoped_condition = {
    title       = "instance-name-prefix"
    description = "Restricted to ${length(local.instance_name_prefixes)} instance name prefix(es); see var.instance_name_prefixes."
    # resource.name is the full path (.../instances/<name>), so a name prefix
    # is matched with extract(), which returns "" when it is absent. Prefixes
    # are [a-z0-9-] only (variables.tf), so they need no escaping.
    expression = format(
      "resource.type == \"compute.googleapis.com/Instance\" && (%s)",
      join(" || ", [for prefix in local.instance_name_prefixes : "resource.name.extract(\"/instances/${prefix}{name}\") != \"\""]),
    )
  }

  # Injective: neither part can contain "." or "_" (variables.tf), so the
  # first "." splits deployment from name and "_" always came from "-".
  role_id        = "${replace(var.deployment_name, "-", "_")}.${replace(var.name, "-", "_")}"
  scoped_role_id = "${local.role_id}.scoped"

  # Everything this module grants to var.service_account_email, normalised;
  # every resource and the propagation wait read from it. This module creates no
  # service account.
  grants = {
    name               = var.name
    description        = local.profile.custom ? "Custom least-privilege permissions: ${local.title}." : local.profile.description
    project_roles      = local.profile.project_roles
    permissions        = local.permissions
    scoped_permissions = local.scoped_permissions
    scoped_condition   = local.generate_scoped_role ? local.scoped_condition : null
    stage              = var.custom_role_stage
    custom_roles       = var.custom_roles
    act_as             = var.act_as_service_accounts
    self_act_as        = local.profile.self_act_as
    bindings           = local.binding_inputs
  }

  # Keyed by role id, plus fixed keys for the generated roles, whose ids are
  # not known until they exist. The fixed keys can never be a custom role id.
  bound_roles = merge(
    { for role in local.grants.project_roles : "predefined_${role}" => "roles/${role}" },
    { for role in local.grants.custom_roles : role => role },
    local.generate_role ? { generated = google_project_iam_custom_role.this[0].id } : {},
  )

  email  = var.service_account_email
  member = "serviceAccount:${local.email}"

  # "projects/-/..." lets the API resolve each target's project from its
  # address: targets need not be in project_id, and the e-mail domain is not
  # always a project id (e.g. Google-managed agents).
  act_as = merge(
    { for account_email in local.grants.act_as : account_email => "projects/-/serviceAccounts/${account_email}" },
    local.grants.self_act_as ? { self = "projects/-/serviceAccounts/${local.email}" } : {},
  )
}

# Checks whose predicate is built from the inputs it checks cannot be variable
# validations (Terraform reports a dependency cycle), so they are preconditions:
# the plan fails, with nothing created, before any grant below is applied.
resource "terraform_data" "input_checks" {
  lifecycle {
    precondition {
      condition     = length(local.denied_custom_permissions) == 0
      error_message = "custom_permissions must not include permissions that let this account impersonate or mint credentials for service accounts, manage service-account keys or HMAC/API keys, change IAM policy on any resource or create or modify IAM roles, read secret payloads or decrypt with keys, log in to or change the metadata of other VMs, or run builds or deployments as another account. Denied: ${join(", ", local.denied_custom_permissions)}."
    }

    precondition {
      condition     = length(local.binding_role_problems) == 0
      error_message = "Every *_bindings role must be a role id (roles/<name>, projects/<project>/roles/<name> or organizations/<org>/roles/<name>) and must not be a basic role (owner, editor, viewer) or a roles/iam.* or roles/resourcemanager.* role, which reach beyond the named resource. Invalid: ${join(", ", local.binding_role_problems)}."
    }
  }
}

# role_id is deterministic; see custom_role_deletion_policy for soft-delete and
# id reuse.
resource "google_project_iam_custom_role" "this" {
  count = local.generate_role ? 1 : 0

  depends_on = [terraform_data.input_checks]

  project         = var.project_id
  role_id         = local.role_id
  title           = local.title
  description     = local.grants.description
  permissions     = local.grants.permissions
  stage           = local.grants.stage
  deletion_policy = var.custom_role_deletion_policy
}

# Holds only scoped_permissions; bound below with an IAM condition, never
# project-wide.
resource "google_project_iam_custom_role" "scoped" {
  count = local.generate_scoped_role ? 1 : 0

  depends_on = [terraform_data.input_checks]

  project         = var.project_id
  role_id         = local.scoped_role_id
  title           = "${local.title} (name-scoped)"
  description     = "${local.grants.description} ${local.scoped_condition.description}"
  permissions     = local.grants.scoped_permissions
  stage           = local.grants.stage
  deletion_policy = var.custom_role_deletion_policy
}

resource "google_project_iam_member" "project_role" {
  for_each = local.bound_roles

  depends_on = [terraform_data.input_checks]

  project = var.project_id
  role    = each.value
  member  = local.member
}

# A naming convention, not an ownership check: any VM in the project with a
# matching name is covered.
resource "google_project_iam_member" "scoped_role" {
  count = local.generate_scoped_role ? 1 : 0

  depends_on = [terraform_data.input_checks]

  project = var.project_id
  role    = google_project_iam_custom_role.scoped[0].id
  member  = local.member

  condition {
    title       = local.scoped_condition.title
    description = local.scoped_condition.description
    expression  = local.scoped_condition.expression
  }
}

resource "google_service_account_iam_member" "act_as" {
  for_each = local.act_as

  depends_on = [terraform_data.input_checks]

  service_account_id = each.value
  role               = "roles/iam.serviceAccountUser"
  member             = local.member
}

resource "google_storage_bucket_iam_member" "this" {
  for_each = local.grants.bindings.bucket

  depends_on = [terraform_data.input_checks]

  bucket = each.value.bucket
  role   = each.value.role
  member = local.member
}

resource "google_secret_manager_secret_iam_member" "this" {
  for_each = local.grants.bindings.secret

  depends_on = [terraform_data.input_checks]

  project   = each.value.project
  secret_id = each.value.secret_id
  role      = each.value.role
  member    = local.member
}

resource "google_artifact_registry_repository_iam_member" "this" {
  for_each = local.grants.bindings.artifact_registry

  depends_on = [terraform_data.input_checks]

  project    = each.value.project
  location   = each.value.location
  repository = each.value.repository_id
  role       = each.value.role
  member     = local.member
}

resource "google_bigquery_dataset_iam_member" "this" {
  for_each = local.grants.bindings.bigquery_dataset

  depends_on = [terraform_data.input_checks]

  project    = each.value.project
  dataset_id = each.value.dataset_id
  role       = each.value.role
  member     = local.member
}

resource "google_pubsub_topic_iam_member" "this" {
  for_each = local.grants.bindings.pubsub_topic

  depends_on = [terraform_data.input_checks]

  project = each.value.project
  topic   = each.value.topic
  role    = each.value.role
  member  = local.member
}

resource "google_pubsub_subscription_iam_member" "this" {
  for_each = local.grants.bindings.pubsub_subscription

  depends_on = [terraform_data.input_checks]

  project      = each.value.project
  subscription = each.value.subscription
  role         = each.value.role
  member       = local.member
}

# Shared VPC: instances this identity creates in a host-project subnetwork need
# compute.subnetworks.use there, which a role bound in the service project does
# not give.
resource "google_compute_subnetwork_iam_member" "this" {
  for_each = local.grants.bindings.subnetwork

  depends_on = [terraform_data.input_checks]

  project    = each.value.project
  region     = each.value.region
  subnetwork = each.value.subnetwork
  role       = each.value.role
  member     = local.member
}

# Best effort: IAM is eventually consistent (see iam_propagation_wait). Re-runs
# whenever what the account can do changes, including a DISABLED stage.
resource "time_sleep" "wait_for_iam" {
  create_duration = var.iam_propagation_wait

  triggers = {
    member     = local.member
    project_id = var.project_id
    grants     = jsonencode(local.grants)
  }

  depends_on = [
    google_project_iam_custom_role.this,
    google_project_iam_custom_role.scoped,
    google_project_iam_member.project_role,
    google_project_iam_member.scoped_role,
    google_service_account_iam_member.act_as,
    google_storage_bucket_iam_member.this,
    google_secret_manager_secret_iam_member.this,
    google_artifact_registry_repository_iam_member.this,
    google_bigquery_dataset_iam_member.this,
    google_pubsub_topic_iam_member.this,
    google_pubsub_subscription_iam_member.this,
    google_compute_subnetwork_iam_member.this,
  ]
}
