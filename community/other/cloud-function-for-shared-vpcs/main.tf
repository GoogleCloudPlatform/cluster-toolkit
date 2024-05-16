locals {
    host_project = "host-project-id"
    service_project = "service-project-id"
}

provider "google" {}



resource "google_compute_network" "vpc" {
  name                    = "vpc2"
  project = local.host_project
  auto_create_subnetworks = false
}

resource "google_compute_shared_vpc_service_project" "shared_vpc" {
  host_project    = local.host_project
  service_project = local.service_project
}

resource "google_compute_subnetwork" "hpc" {
  name          = "hpc"
  project = local.host_project
  ip_cidr_range = "10.1.3.0/24"
  region        = "europe-west4"
  network       = google_compute_network.vpc.id
}


resource "google_project_iam_custom_role" "subnet_iampolicy_role" {
  project = local.host_project
  role_id     = "subnetIamPolicyRole"
  title       = "Subnet IAM Policy Role"
  description = "This role is used for giving access to control iam policy for specific subnet"
  permissions = ["compute.subnetworks.getIamPolicy", "compute.subnetworks.setIamPolicy"]
}


resource "google_service_account" "cloud_function_service_account" {
  account_id   = "subnet-iam-assigner"
  project = local.service_project
  display_name = "For runninng Cloud Function, that controls iam permissions in host project for subnet."
}


resource "google_project_iam_binding" "subnet_iam_policy_binding" {
  project = local.host_project
  role    = google_project_iam_custom_role.subnet_iampolicy_role.id
  condition {
    expression = "resource.name == \"${google_compute_subnetwork.hpc.id}\""
    title = "Only access to ${google_compute_subnetwork.hpc.id}"
    description = "Restrict permissions to single subnet"
  }
  members = [
    "serviceAccount:${google_service_account.cloud_function_service_account.email}"
  ]
}

resource "google_pubsub_topic" "log-sink-topic" {
  name = "service-account-auditlogs"
  project = local.service_project
  message_retention_duration = "86600s"
}

resource "google_logging_project_sink" "logs_sink" {
  name = "service-account-audit-logs"
  project = local.service_project
  # Can export to pubsub, cloud storage, bigquery, log bucket, or another project
  destination = "pubsub.googleapis.com/projects/${google_pubsub_topic.log-sink-topic.project}/topics/${google_pubsub_topic.log-sink-topic.name}"

  # Log all WARN or higher severity messages relating to instances
  filter = "protoPayload.methodName=\"google.iam.admin.v1.DeleteServiceAccount\" OR protoPayload.methodName=\"google.iam.admin.v1.CreateServiceAccount\""

  # Use a unique writer (creates a unique service account used for writing)
  unique_writer_identity = true
}

resource "google_pubsub_topic_iam_binding" "log-sink-topic-binding" {
  project = google_pubsub_topic.log-sink-topic.project
  topic = google_pubsub_topic.log-sink-topic.name
  role = "roles/pubsub.publisher"
  members = [
    google_logging_project_sink.logs_sink.writer_identity
  ]
}



resource "google_storage_bucket" "cf_source_bucket" {
  name     = "${local.service_project}-service-account-auditlog-gcf-source"  # Every bucket name must be globally unique
  project = local.service_project
  location = "europe-west1"
  uniform_bucket_level_access = true
}

data "archive_file" "cf_source" {
  type        = "zip"
  source_dir = "./cloudfunction/"
  output_path = "function-source.zip"
  excludes = ["venv"]
}

resource "google_storage_bucket_object" "object" {
  name   = "function-source-${data.archive_file.cf_source.output_sha256}.zip"
  bucket = google_storage_bucket.cf_source_bucket.name
  source = "function-source.zip"
}



resource "google_cloudfunctions2_function" "serviceaccount_audit_logs_watcher" {
  name = "serviceaccount-audit-log-watcher"
  location = "europe-west1"
  project = local.service_project
  description = "Parse service account audit logs"

  build_config {
    runtime = "python312"
    entry_point = "process_log_entry"  # Set the entry point 
    source {
      storage_source {
        bucket = google_storage_bucket.cf_source_bucket.name
        object = google_storage_bucket_object.object.name
      }
    }
  }

  service_config {
    max_instance_count  = 1
    min_instance_count = 0
    available_memory    = "256Mi"
    timeout_seconds     = 60
    max_instance_request_concurrency = 1
    environment_variables = {
        HOST_PROJECT = google_compute_subnetwork.hpc.project
        SUBNET_REGION = google_compute_subnetwork.hpc.region
        SUBNET_NAME = google_compute_subnetwork.hpc.name
    }
    service_account_email = google_service_account.cloud_function_service_account.email
  }

  event_trigger {
    trigger_region = "europe-west4"
    event_type = "google.cloud.pubsub.topic.v1.messagePublished"
    pubsub_topic = google_pubsub_topic.log-sink-topic.id
    retry_policy = "RETRY_POLICY_RETRY"
  }

}



