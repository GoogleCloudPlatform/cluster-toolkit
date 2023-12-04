/**
 * Copyright 2021 Google LLC
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
  random_id = var.random_id != null ? var.random_id : random_id.default.hex
  project = (var.create_project
    ? try(module.project_radlab_ds_analytics[0], null)
    : try(data.google_project.existing_project[0], null)
  )
  region = join("-", [split("-", var.zone)[0], split("-", var.zone)[1]])

  network = (
    var.create_network
    ? try(module.vpc_ai_notebook[0].network.network, null)
    : try(data.google_compute_network.default[0], null)
  )

  subnet = (
    var.create_network
    ? try(module.vpc_ai_notebook[0].subnets["${local.region}/${var.subnet_name}"], null)
    : try(data.google_compute_subnetwork.default[0], null)
  )

  notebook_sa_project_roles = [
    "roles/compute.instanceAdmin",
    "roles/notebooks.admin",
    "roles/bigquery.user",
    "roles/storage.objectViewer"
  ]

  project_services = var.enable_services ? [
    "compute.googleapis.com",
    "bigquery.googleapis.com",
    "notebooks.googleapis.com",
    "bigquerystorage.googleapis.com"
  ] : []
}

resource "random_id" "default" {
  byte_length = 2
}

resource "google_storage_bucket_object" "startup_script" {
  name   = var.wb_startup_script_name
  source = "../../startup_script.sh"
  bucket = var.wb_startup_script_bucket
}

#####################
# ANALYTICS PROJECT #
#####################

data "google_project" "existing_project" {
  count      = var.create_project ? 0 : 1
  project_id = var.project_name
}

module "project_radlab_ds_analytics" {
  count   = var.create_project ? 1 : 0
  source  = "terraform-google-modules/project-factory/google"
  version = "~> 11.0"

  name              = format("%s-%s", var.project_name, local.random_id)
  random_project_id = false
  folder_id         = var.folder_id
  billing_account   = var.billing_account_id
  org_id            = var.organization_id

  activate_apis = []
}

resource "google_project_service" "enabled_services" {
  for_each                   = toset(local.project_services)
  project                    = local.project.project_id
  service                    = each.value
  disable_dependent_services = true
  disable_on_destroy         = true

  depends_on = [
    module.project_radlab_ds_analytics
  ]
}

data "google_compute_network" "default" {
  count   = var.create_network ? 0 : 1
  project = local.project.project_id
  name    = var.network_name
}

data "google_compute_subnetwork" "default" {
  count   = var.create_network ? 0 : 1
  project = local.project.project_id
  name    = var.subnet_name
  region  = local.region
}

module "vpc_ai_notebook" {
  count   = var.create_network ? 1 : 0
  source  = "terraform-google-modules/network/google"
  version = "~> 3.0"

  project_id   = local.project.project_id
  network_name = var.network_name
  routing_mode = "GLOBAL"
  description  = "VPC Network created via Terraform"

  subnets = [
    {
      subnet_name           = var.subnet_name
      subnet_ip             = var.ip_cidr_range
      subnet_region         = local.region
      description           = "Subnetwork inside *vpc-analytics* VPC network, created via Terraform"
      subnet_private_access = true
    }
  ]

  firewall_rules = [
    {
      name        = "fw-ai-notebook-allow-internal"
      description = "Firewall rule to allow traffic on all ports inside *vpc-analytics* VPC network."
      priority    = 65534
      ranges      = ["10.0.0.0/8"]
      direction   = "INGRESS"

      allow = [{
        protocol = "tcp"
        ports    = ["0-65535"]
      }]
    }
  ]

  depends_on = [
    google_project_service.enabled_services
  ]
}

resource "google_service_account" "sa_p_notebook" {
  project      = local.project.project_id
  account_id   = format("sa-p-notebook-%s", local.random_id)
  display_name = "Notebooks in trusted environment"
}

resource "google_project_iam_member" "sa_p_notebook_permissions" {
  for_each = toset(local.notebook_sa_project_roles)
  project  = local.project.project_id
  member   = "serviceAccount:${google_service_account.sa_p_notebook.email}"
  role     = each.value
}

resource "google_service_account_iam_member" "sa_ai_notebook_user_iam" {
  role               = "roles/iam.serviceAccountUser"
  service_account_id = google_service_account.sa_p_notebook.id
  member             = "user:${var.trusted_user}"
}

resource "google_notebooks_instance" "ai_notebook" {
  project      = local.project.project_id
  name         = "notebooks-instance-${local.random_id}"
  location     = var.zone
  machine_type = var.machine_type

  vm_image {
    image_family = try(var.instance_image.family, null)
    image_name   = try(var.instance_image.name, null)
    project      = var.instance_image.project
  }

  instance_owners = var.owner_id

  install_gpu_driver = false
  boot_disk_type     = var.boot_disk_type
  boot_disk_size_gb  = var.boot_disk_size_gb

  no_public_ip    = false
  no_proxy_access = false

  network = local.network.self_link
  subnet  = local.subnet.self_link

  post_startup_script = format("gs://%s/%s", var.wb_startup_script_bucket, var.wb_startup_script_name)

  labels = {
    module = "data-science"
  }

  metadata = {
    terraform  = "true"
    proxy-mode = "mail"
  }

}

resource "google_compute_instance_iam_member" "oslogin_permissions" {
  project       = google_notebooks_instance.ai_notebook.project
  zone          = google_notebooks_instance.ai_notebook.location
  instance_name = google_notebooks_instance.ai_notebook.name
  role          = "roles/compute.osLogin"
  member        = "user:${var.trusted_user}"
}

module "waitforstartup" {

  source = "./wait-for-startup"

  instance_name = google_notebooks_instance.ai_notebook.name
  zone          = google_notebooks_instance.ai_notebook.location
  project_id    = google_notebooks_instance.ai_notebook.project
  timeout       = 1200
}
