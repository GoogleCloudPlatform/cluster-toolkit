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

module "network1" {
  source          = "./modules/embedded/modules/network/vpc"
  deployment_name = var.deployment_name
  extra_iap_ports = [8080]
  firewall_rules = [{
    allow = [{
      ports    = ["8080"]
      protocol = "tcp"
    }]
    description = "Allow external ingress to Guacamole on TCP port 8080"
    direction   = "INGRESS"
    name        = "allow-guacamole-8080-ext"
    ranges      = ["0.0.0.0/0"]
  }]
  labels     = var.labels
  project_id = var.project_id
  region     = var.region
}

module "enable-apis" {
  source           = "./modules/embedded/community/modules/project/service-enablement"
  gcp_service_list = ["secretmanager.googleapis.com", "storage.googleapis.com", "compute.googleapis.com"]
  project_id       = var.project_id
}

module "vdi-setup" {
  source          = "./modules/embedded/community/modules/scripts/vdi-setup"
  deployment_name = var.deployment_name
  labels          = var.labels
  project_id      = var.project_id
  region          = var.region
  user_provision  = "local_users"
  vdi_resolution  = "1920x1080"
  vdi_tool        = "guacamole"
  vdi_user_group  = "vdiusers"
  vdi_users = [{
    port     = 5901
    username = "alice"
    }, {
    port        = 5902
    secret_name = "a-password-for-bob"
    username    = "bob"
  }]
  vnc_flavor = "tigervnc"
}

module "machine" {
  source             = "./modules/embedded/modules/compute/vm-instance"
  deployment_name    = var.deployment_name
  disable_public_ips = false
  instance_image = {
    family  = "debian-11"
    project = "debian-cloud"
  }
  labels               = var.labels
  machine_type         = "g2-standard-8"
  network_self_link    = module.network1.network_self_link
  project_id           = var.project_id
  region               = var.region
  startup_script       = module.vdi-setup.startup_script
  subnetwork_self_link = module.network1.subnetwork_self_link
  tags                 = ["guacamole"]
  zone                 = var.zone
}
