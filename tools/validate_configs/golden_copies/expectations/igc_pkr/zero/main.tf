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

module "network0" {
  source                   = "./modules/embedded/modules/network/vpc"
  deployment_name          = var.deployment_name
  enable_iap_rdp_ingress   = true
  enable_iap_winrm_ingress = true
  project_id               = var.project_id
  region                   = var.region
}

module "homefs" {
  source          = "./modules/embedded/modules/file-system/filestore"
  deployment_name = var.deployment_name
  labels          = var.labels
  local_mount     = "/home"
  network_id      = module.network0.network_id
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
}

module "projectsfs" {
  source          = "./modules/embedded/modules/file-system/filestore"
  deployment_name = var.deployment_name
  labels          = var.labels
  local_mount     = "/projects"
  network_id      = module.network0.network_id
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
}

module "script" {
  source          = "./modules/embedded/modules/scripts/startup-script"
  deployment_name = var.deployment_name
  labels          = var.labels
  project_id      = var.project_id
  region          = var.region
  runners = [{
    content     = "#!/bin/bash\necho \"Hello, World!\"\n"
    destination = "hello.sh"
    type        = "shell"
  }]
}

module "windows_startup" {
  source                = "./modules/embedded/community/modules/scripts/windows-startup-script"
  install_nvidia_driver = true
}
