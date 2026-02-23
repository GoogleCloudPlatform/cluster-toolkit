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
locals {
  retries = var.timeout / 30
}
resource "null_resource" "wait_for_startup" {
  provisioner "local-exec" {
    command = "/bin/bash ${path.module}/scripts/wait-for-startup-status.sh"
    environment = {
      INSTANCE_NAME = var.instance_name
      ZONE          = var.zone
      PROJECT_ID    = var.project_id
      RETRIES       = local.retries
    }
  }
}
