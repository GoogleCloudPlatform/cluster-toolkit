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
        "pubsub.admin"
    ]

    deploy_key1 = var.deployment_key != "" ? file(var.deployment_key) : ""

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
    names         = ["fe-service-account"]
    prefix        = var.deployment_name
    project_id    = var.project_id
    project_roles = [for role in local.sa_roles : "${var.project_id}=>roles/${role}"]
}

module "control_bucket" {
    source = "terraform-google-modules/cloud-storage/google"
    version = "~> 2.2"

    project_id = var.project_id
    names = ["storage"]
    prefix = var.deployment_name
    force_destroy = {
        storage = true
    }
    location = var.region
    storage_class = "STANDARD"
    set_admin_roles = true
    admins = ["serviceAccount:${module.service_account.email}"]
    set_storage_admin_roles = true
    storage_admins = ["serviceAccount:${module.service_account.email}"]
    labels = local.labels
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
    count = var.deployment_mode == "tarball" ? 1 : 0
    name = "webserver/deployment.tar.gz"
    bucket = module.control_bucket.bucket.name
    source = "deployment.tar.gz"
    metadata = { }
}

resource "google_storage_bucket_object" "config_file" {
    name = "webserver/config"
    bucket = module.control_bucket.bucket.name
    content = local.server_config_file
    metadata = { }
}



module "pubsub" {
    source = "terraform-google-modules/pubsub/google"
    version = "~> 1.8"

    topic = var.deployment_name
    project_id = var.project_id
    grant_token_creator = false
    topic_labels = local.labels
    subscription_labels = local.labels

    pull_subscriptions = [
        {
            name     = "${var.deployment_name}-c2resp"
            filter   = "NOT attributes:target"
        }
    ]
}


resource "google_compute_network" "hosting-vpc" {

    project = var.project_id
    name = "${var.deployment_name}-network"
    auto_create_subnetworks = false

}


resource "google_compute_subnetwork" "hosting-subnetwork" {

    name = "${var.deployment_name}-subnetwork"
    ip_cidr_range = "10.2.0.0/28"
    region = var.region
    network = google_compute_network.hosting-vpc.name

}


resource "google_compute_firewall" "allow-http-rule" {

  project     = var.project_id
  name        = "${var.deployment_name}-allow-http"
  network     = google_compute_network.hosting-vpc.name

  allow {
    protocol  = "tcp"
    ports     = ["80"]
  }

  source_tags = ["http-server"]
  source_ranges = ["0.0.0.0/0"]

}


resource "google_compute_firewall" "allow-https-rule" {

  project     = var.project_id
  name        = "${var.deployment_name}-allow-https"
  network     = google_compute_network.hosting-vpc.name

  allow {
    protocol  = "tcp"
    ports     = ["443"]
  }

  source_tags = ["https-server"]
  source_ranges = ["0.0.0.0/0"]

}


resource "google_compute_instance" "server_vm" {

    name = "${var.deployment_name}-server"
    machine_type = var.server_instance_type
    zone = var.zone

    hostname = var.webserver_hostname != "" ? var.webserver_hostname : null
    
    metadata = {
        startup-script-url = "${module.control_bucket.bucket.url}/webserver/startup.sh",
        webserver-config-bucket = module.control_bucket.bucket.name,
        ghpcfe-c2-topic = module.pubsub.topic,
        hostname = var.webserver_hostname
        deploy_mode = var.deployment_mode
        # TODO:  SSH Keys
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
    tags = ["http-server", "https-server"]

    boot_disk {
        initialize_params {
            image = "projects/rocky-linux-cloud/global/images/rocky-linux-8-v20220126"
            size = 30
            type = "pd-ssd"
        }
    }

    network_interface {
        subnetwork = google_compute_subnetwork.hosting-subnetwork.name
        access_config {
        }
    }

}
