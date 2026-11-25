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

# Create Service Account
# Create bucket
# Upload files

# Create PubSub topic
# Create VPC and subnet
# Create VM instance

locals {
  server_config_file = <<-EOT
django_username: "${var.django_su_username}"
django_password: "${var.django_su_password}"
django_email: "${var.django_su_email}"
git_branch: "${var.repo_branch}"
git_fork: "${var.repo_fork}"
google_client_id: PLACEHOLDER
google_client_secret: PLACEHOLDER
EOT

  default_labels = {
    ghpcfe_id = var.deployment_name,
  }
  labels = merge(var.extra_labels, local.default_labels)

  # Content hash over everything staged into the control bucket. Clusters pull
  # their bootstrap/ansible files from the bucket at boot, so the staged copy
  # must be re-uploaded whenever any source file changes. Local build artifacts
  # (__pycache__, .DS_Store) are excluded here and from the upload below.
  gcs_bucket_root = "${path.module}/../infrastructure_files/gcs_bucket"
  gcs_bucket_files = [
    for f in fileset(local.gcs_bucket_root, "**") : f
    if !can(regex("(^|/)(__pycache__|\\.DS_Store)(/|$)", f))
  ]
  gcs_bucket_hash = sha256(join(",", [for f in local.gcs_bucket_files : filesha256("${local.gcs_bucket_root}/${f}")]))
}


# -- Web server identity
#
# The account comes from the Cluster Toolkit service-account module and its
# permissions from the service-account-profiles module, both sourced from this
# checkout. The account keeps its id (<deployment_name>-fe-sa) and only the
# logging and monitoring project roles; everything else is granted on the
# named resources below. tf/README.md lists each grant and why it is needed.
module "fe_sa" {
  source = "../../../../modules/project/service-account"

  project_id      = var.project_id
  deployment_name = var.deployment_name
  name            = "fe-sa"
  description     = "Service Account for GHPC Open Frontend"
  display_name    = "Open Front End web server"
  grant_xpn_roles = false
  # The baseline roles are granted here rather than by fe_profile
  # (grant_baseline_roles = false), so each binding has a single owner when an
  # existing deployment is upgraded. monitoring.viewer: Grafana's Cloud
  # Monitoring datasource reads metrics as this account, with no key.
  project_roles = ["logging.logWriter", "monitoring.metricWriter", "monitoring.viewer"]
}

# A new suffix per deployment (it lives in state, so it changes only after a
# teardown), so a redeploy under the same deployment_name never collides with
# the custom role id Google holds for up to 44 days after a teardown.
resource "random_id" "fe_role_suffix" {
  byte_length = 3
}

module "fe_profile" {
  source = "../../../../community/modules/iam/service-account-profiles"

  project_id            = var.project_id
  deployment_name       = var.deployment_name
  name                  = "fe-sa"
  role_id_suffix        = random_id.fe_role_suffix.hex
  service_account_email = module.fe_sa.service_account_email
  profile               = "custom"
  custom_profile_name   = "Open Front End"
  grant_baseline_roles  = false

  # compute.machineTypes.get: gcluster create looks up each machine type in a
  # blueprint with Application Default Credentials, which on the server are
  # this account, not the impersonated provisioning account. It reads public
  # machine-type specifications only.
  # The pubsub permissions: the server creates, reads and deletes one
  # subscription per cluster at run time and grants the cluster's account on
  # it, so these cannot be bound on a named subscription in advance.
  custom_permissions = [
    "compute.machineTypes.get",
    "pubsub.subscriptions.create",
    "pubsub.subscriptions.delete",
    "pubsub.subscriptions.get",
    "pubsub.subscriptions.getIamPolicy",
    "pubsub.subscriptions.setIamPolicy",
  ]

  bucket_bindings = {
    control = {
      bucket = module.control_storage.name
      role   = "roles/storage.admin"
    }
  }

  pubsub_topic_bindings = {
    c2 = {
      topic = module.pubsub.topic
      role  = "roles/pubsub.admin"
    }
  }

  pubsub_subscription_bindings = {
    c2resp = {
      subscription = one(module.pubsub.subscription_names)
      role         = "roles/pubsub.subscriber"
    }
    build_logs = {
      subscription = google_pubsub_subscription.cloud_build_logs_sub.name
      role         = "roles/pubsub.subscriber"
    }
  }

  # The server creates the database encryption key on first boot, stores it as
  # the secret's first version, and reads it on every start. The startup
  # script reads the server configuration once, on first boot, and then
  # disables the version it read (and, on every later start, any version
  # still enabled). While a version is disabled no one can read it, so
  # principals with project-wide Secret Manager access can no longer read the
  # admin password; this account, which holds both roles below on the secret,
  # could enable it again and read it. Secret Version Manager lets it list,
  # enable and disable versions, not read them.
  secret_bindings = {
    db_key_read = {
      secret_id = google_secret_manager_secret.db_key.secret_id
      role      = "roles/secretmanager.secretAccessor"
    }
    db_key_add = {
      secret_id = google_secret_manager_secret.db_key.secret_id
      role      = "roles/secretmanager.secretVersionAdder"
    }
    server_config_read = {
      secret_id = google_secret_manager_secret.server_config.secret_id
      role      = "roles/secretmanager.secretAccessor"
    }
    server_config_disable = {
      secret_id = google_secret_manager_secret.server_config.secret_id
      role      = "roles/secretmanager.secretVersionManager"
    }
  }
}

# -- Database encryption key
#
# The secret only: Terraform creates no version, so the key never appears in
# Terraform state or in this directory. The server generates the key on first
# boot and adds it as a version (fe_profile grants that on this secret only).
# terraform destroy deletes the secret and every version with it.
resource "google_secret_manager_secret" "db_key" {
  project   = var.project_id
  secret_id = "${var.deployment_name}-db-key"
  labels    = local.labels

  replication {
    auto {}
  }
}

# -- Server configuration
#
# The admin user's name, password and e-mail address and the server's other
# first-boot settings, which the startup script reads once. Only the web
# server's account may read it (fe_profile grants that on this secret only), so
# cluster accounts that can read the control bucket cannot. The content is in
# Terraform state, as it was when it was a control-bucket object. Earlier
# versions uploaded it as the bucket object webserver/config; that resource is
# gone, so an upgrade apply deletes any copy of it left in the bucket.
# The startup script disables the version once it has read it, and an
# installed server disables any enabled version each time it starts.
# terraform destroy deletes the secret and its version.
resource "google_secret_manager_secret" "server_config" {
  project   = var.project_id
  secret_id = "${var.deployment_name}-server-config"
  labels    = local.labels

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "server_config" {
  secret      = google_secret_manager_secret.server_config.id
  secret_data = local.server_config_file

  # The server disables the version after reading it; a later apply must not
  # enable it again. Changed content still creates a new, enabled version.
  lifecycle {
    ignore_changes = [enabled]
  }
}

# -- Provisioning account
#
# The web server creates clusters and other resources as the keyless
# provisioning account deploy.sh creates, by impersonating it. Only the web
# server's account is granted that, and only on this one account. The
# profiles module refuses roles/iam.* bindings, so this is a plain grant.
resource "google_service_account_iam_member" "fe_impersonates_provisioning" {
  count = var.provisioning_service_account != "" ? 1 : 0

  service_account_id = "projects/${var.project_id}/serviceAccounts/${var.provisioning_service_account}"
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${module.fe_sa.service_account_email}"
}

module "control_storage" {
  source  = "terraform-google-modules/cloud-storage/google"
  version = "~> 11.1"

  project_id       = var.project_id
  names            = ["storage"]
  prefix           = var.deployment_name
  randomize_suffix = true
  force_destroy = {
    storage = true
  }
  # Uniform bucket-level access, keyed by the name in names. It pins the
  # module's own default (true), which every earlier version also used, so an
  # existing deployment's plan shows no change. The prefix-limited grants of
  # cluster controllers are IAM conditions, which Google accepts only on a
  # bucket with uniform access, so a module default change must never reach it.
  bucket_policy_only = {
    storage = true
  }
  # A bucket cannot move: a new location replaces it and deletes its files.
  # deploy.sh sets control_bucket_location to an existing bucket's location,
  # which differs from region for servers that earlier versions deployed with
  # --config (no region was set, so their bucket is in the US multi-region).
  location      = var.control_bucket_location != "" ? var.control_bucket_location : var.region
  storage_class = "STANDARD"
  labels        = local.labels
  # The bucket holds the server's boot files and configuration, which no one
  # outside the project reads, so it can never be made public. An existing
  # bucket is updated in place.
  public_access_prevention = "enforced"
  # No admins or storage_admins: the web server's access is granted by
  # fe_profile's bucket_bindings, which wait for the account to exist.
}

# -- Upgrading a deployment made before the move to the profile modules
#
# The account keeps its address inside the Cluster Toolkit module, so it is
# not replaced.
moved {
  from = module.service_account.google_service_account.service_accounts["fe-sa"]
  to   = module.fe_sa.module.service_account.google_service_account.service_accounts["fe-sa"]
}

# The old project-role grants are forgotten, not destroyed, so the server never
# loses access during the upgrade. deploy.sh removes the superseded ones after
# a successful apply; logging.logWriter and monitoring.metricWriter are granted
# again by fe_sa.
removed {
  from = module.service_account.google_project_iam_member.project-roles

  lifecycle {
    destroy = false
  }
}

# The bucket module call was renamed so that its old authoritative IAM
# bindings, still declared (with no instances) inside the module, can be
# forgotten below. The bucket and its name suffix keep their state.
moved {
  from = module.control_bucket.google_storage_bucket.buckets["storage"]
  to   = module.control_storage.google_storage_bucket.buckets["storage"]
}

moved {
  from = module.control_bucket.random_id.bucket_suffix[0]
  to   = module.control_storage.random_id.bucket_suffix[0]
}

# Forgotten, not destroyed: destroying the authoritative objectAdmin binding
# would also remove the bucket access that existing clusters' accounts were
# given through it.
removed {
  from = module.control_bucket.google_storage_bucket_iam_binding.admins

  lifecycle {
    destroy = false
  }
}

removed {
  from = module.control_bucket.google_storage_bucket_iam_binding.storage_admins

  lifecycle {
    destroy = false
  }
}

resource "null_resource" "uploader" {
  depends_on = [module.control_storage.bucket]
  # Re-run the upload whenever any staged source file changes, so clusters
  # never boot from a stale copy of the bootstrap/ansible files.
  triggers = {
    gcs_bucket_hash = local.gcs_bucket_hash
  }
  # Upload files
  provisioner "local-exec" {
    command = "gcloud storage rsync --recursive --exclude='(^|.*/)(__pycache__/.*|\\.DS_Store)$' ../infrastructure_files/gcs_bucket ${module.control_storage.bucket.url}"
  }
}

# Also upload our deployment tarball
resource "google_storage_bucket_object" "deployment_file" {
  count    = var.deployment_mode == "tarball" ? 1 : 0
  name     = "webserver/deployment.tar.gz"
  bucket   = module.control_storage.bucket.name
  source   = "deployment.tar.gz"
  metadata = {}
}



module "pubsub" {
  source  = "terraform-google-modules/pubsub/google"
  version = "~> 8.3"

  topic               = var.deployment_name
  project_id          = var.project_id
  grant_token_creator = false
  topic_labels        = local.labels
  subscription_labels = local.labels

  pull_subscriptions = [
    {
      name   = "${var.deployment_name}-c2resp"
      filter = "NOT attributes:target"
    }
  ]
}


module "network" {
  count           = length(trimspace(var.subnet)) > 0 ? 0 : 1
  source          = "./network"
  project_id      = var.project_id
  region          = var.region
  deployment_name = var.deployment_name
}


resource "google_compute_instance" "server_vm" {

  name         = "${var.deployment_name}-server"
  machine_type = var.server_instance_type
  zone         = var.zone

  hostname = length(trimspace(var.webserver_hostname)) > 0 ? var.webserver_hostname : null

  metadata = {
    startup-script-url      = "${module.control_storage.bucket.url}/webserver/startup.sh",
    webserver-config-bucket = module.control_storage.bucket.name,
    terraform-state-bucket  = var.terraform_state_bucket,
    ghpcfe-c2-topic         = module.pubsub.topic,
    hostname                = var.webserver_hostname
    deploy_mode             = var.deployment_mode
    deployment_name         = var.deployment_name
    # Read by the server: the account it impersonates (empty when none is
    # set), the secret holding its database encryption key and the secret
    # holding its configuration.
    ofe-provisioning-service-account = var.provisioning_service_account
    ofe-db-key-secret                = google_secret_manager_secret.db_key.secret_id
    ofe-server-config-secret         = google_secret_manager_secret.server_config.secret_id
  }

  # The profile's email output is available only after its grants have been
  # made and have had time to propagate, so the server starts with its access
  # in place. Access is limited by IAM, not by the scope.
  service_account {
    email  = module.fe_profile.service_account_email
    scopes = ["cloud-platform"]
  }
  # Ordering only (depends_on never replaces an existing VM): the server boots
  # after its configuration's secret version exists, and after the grant that
  # lets it impersonate the provisioning account. Its access to the
  # configuration secret is a fe_profile grant, in place before the
  # service_account email above is available.
  depends_on = [
    google_secret_manager_secret_version.server_config,
    google_service_account_iam_member.fe_impersonates_provisioning,
  ]
  # Changing the scopes of an existing server requires a stop and start; the
  # startup script exits early on a server that is already installed.
  allow_stopping_for_update = true

  scheduling {
    on_host_maintenance = "MIGRATE"
  }

  labels = local.labels
  tags   = ["http-server", "https-server", "ssh-server"]

  boot_disk {
    initialize_params {
      image = "projects/rocky-linux-cloud/global/images/rocky-linux-8-v20220126"
      size  = 30
      type  = "pd-ssd"
    }
  }

  network_interface {
    subnetwork = length(trimspace(var.subnet)) > 0 ? var.subnet : module.network[0].subnet_name
    access_config {
      nat_ip = length(trimspace(var.static_ip)) > 0 ? var.static_ip : null
    }
  }

}


resource "google_pubsub_topic" "cloud_build_logs" {
  name    = "${var.deployment_name}-build-logs"
  project = var.project_id

  labels = {
    environment = "dev"
  }
}


resource "google_logging_project_sink" "build_sink" {
  name    = "${var.deployment_name}-build-sink"
  project = var.project_id
  # Filter for "resource.type=build" ensures only Cloud Build logs match.
  filter = "resource.type=\"build\""
  # Pub/Sub topic you just created
  destination = "pubsub.googleapis.com/projects/${var.project_id}/topics/${google_pubsub_topic.cloud_build_logs.name}"
}


# 3) Grant sink “writer identity” permission to publish to your new topic
resource "google_pubsub_topic_iam_member" "build_sink_pub" {
  project = var.project_id
  topic   = google_pubsub_topic.cloud_build_logs.name
  role    = "roles/pubsub.publisher"

  # The sink's writer_identity is a service account that Google auto-creates
  member = google_logging_project_sink.build_sink.writer_identity
}


# 4) Create a new pull subscription for your Django app
resource "google_pubsub_subscription" "cloud_build_logs_sub" {
  name    = "${var.deployment_name}-build-logs-sub"
  topic   = google_pubsub_topic.cloud_build_logs.name
  project = var.project_id

  ack_deadline_seconds = 30
  expiration_policy {
    ttl = "" # Messages never expire
  }
  # Retain messages for up to 7 days (example)
  message_retention_duration = "604800s"

  labels = {
    environment = "dev"
  }
}
