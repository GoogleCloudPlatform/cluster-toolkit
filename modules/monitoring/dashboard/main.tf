/**
 * Copyright 2022 Google LLC
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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "dashboard", ghpc_role = "monitoring" })
}

locals {
  dash_path = "${path.module}/dashboards/${var.base_dashboard}.json.tpl"
}

resource "google_monitoring_dashboard" "dashboard" {
  dashboard_json = templatefile(local.dash_path, {
    widgets         = var.widgets
    deployment_name = var.deployment_name
    title           = var.title
    labels          = local.labels
    }
  )
  project = var.project_id
}
