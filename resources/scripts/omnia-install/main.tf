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

resource "null_resource" "omnia_install" {
  depends_on = [var.depends]
  triggers = {
    "dependencies" = jsonencode(var.depends),
    "manager"      = var.manager_node
  }
  provisioner "local-exec" {
    command = "chmod +x ${path.module}/scripts/install_omnia.sh && ${path.module}/scripts/install_omnia.sh"
    environment = {
      DEPLOYMENT_NAME = var.deployment_name
      MANAGER_NODE    = var.manager_node
      ZONE            = var.zone
      PROJECT_ID      = var.project_id
    }
  }
}
