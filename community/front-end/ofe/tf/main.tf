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
  sa_roles = [
    "storage.objectAdmin",
    "logging.logWriter",
    "monitoring.metricWriter",
    "cloudtrace.agent",
    "pubsub.admin",
    "artifactregistry.admin",
    "secretmanager.admin"
  ]

  deploy_key1 = var.deployment_key != "" ? filebase64(var.deployment_key) : ""

  server_config_file = <<-EOT
django_username: "${var.django_su_username}"
django_password: "${var.django_su_password}"
django_email: "${var.django_su_email}"
deploy_key1: "${local.deploy_key1}"
git_branch: "${var.repo_branch}"
git_fork: "${var.repo_fork}"
google_client_id: PLACEHOLDER
google_client_secret: PLACEHOLDER
EOT

  default_labels = {
    ghpcfe_id = var.deployment_name,
  }
  labels = merge(var.extra_labels, local.default_labels)
}


module "service_account" {
  source  = "terraform-google-modules/service-accounts/google"
  version = "~> 4.1"

  description   = "Service Account for GHPC Open Frontend"
  names         = ["fe-sa"]
  prefix        = var.deployment_name
  project_id    = var.project_id
  project_roles = [for role in local.sa_roles : "${var.project_id}=>roles/${role}"]
}

module "control_bucket" {
  source  = "terraform-google-modules/cloud-storage/google"
  version = "~> 4.0"

  project_id       = var.project_id
  names            = ["storage"]
  prefix           = var.deployment_name
  randomize_suffix = true
  force_destroy = {
    storage = true
  }
  location                = var.region
  storage_class           = "STANDARD"
  set_admin_roles         = true
  admins                  = ["serviceAccount:${module.service_account.email}"]
  set_storage_admin_roles = true
  storage_admins          = ["serviceAccount:${module.service_account.email}"]
  labels                  = local.labels
}

resource "null_resource" "uploader" {
  depends_on = [module.control_bucket.bucket]
  # Upload files
  provisioner "local-exec" {
    command = "gsutil -m cp -r ../infrastructure_files/gcs_bucket/* ${module.control_bucket.bucket.url}/"
  }
}

# Also upload our deployment tarball
resource "google_storage_bucket_object" "deployment_file" {
  count    = var.deployment_mode == "tarball" ? 1 : 0
  name     = "webserver/deployment.tar.gz"
  bucket   = module.control_bucket.bucket.name
  source   = "deployment.tar.gz"
  metadata = {}
}

resource "google_storage_bucket_object" "config_file" {
  name     = "webserver/config"
  bucket   = module.control_bucket.bucket.name
  content  = local.server_config_file
  metadata = {}
}



module "pubsub" {
  source  = "terraform-google-modules/pubsub/google"
  version = "~> 5.0"

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
    startup-script-url      = "${module.control_bucket.bucket.url}/webserver/startup.sh",
    webserver-config-bucket = module.control_bucket.bucket.name,
    ghpcfe-c2-topic         = module.pubsub.topic,
    hostname                = var.webserver_hostname
    deploy_mode             = var.deployment_mode
    deployment_name         = var.deployment_name
  }

  service_account {
    email = module.service_account.email
    scopes = [
      "storage-full",
      "logging-write",
      "monitoring-write",
      "trace",
      "service-control",
      "service-management",
      "pubsub"
    ]
  }
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
