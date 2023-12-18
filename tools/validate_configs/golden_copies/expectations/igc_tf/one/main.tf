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

module "homefs" {
  source          = "./modules/embedded/modules/file-system/filestore"
  deployment_name = var.deployment_name
  labels          = var.labels
  local_mount     = "/home"
  name            = var.subnetwork_name_network0
  network_id      = var.network_id_network0
  project_id      = var.project_id
  region          = var.region
  zone            = var.zone
}
