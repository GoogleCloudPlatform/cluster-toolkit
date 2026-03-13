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

output "deployment_id" {
  description = "RADLab Module Deployment ID"
  value       = local.random_id
}

output "project_radlab_ds_analytics_id" {
  description = "Analytics Project ID"
  value       = local.project.project_id
}

output "notebook_instance_name" {
  description = "Notebook Instance Names"
  value       = google_notebooks_instance.ai_notebook.name
}

output "notebook_proxy_uri" {
  description = "Notebook Proxy URIs"
  value       = google_notebooks_instance.ai_notebook.proxy_uri
}
