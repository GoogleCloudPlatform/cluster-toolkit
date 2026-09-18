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

output "project_number" {
  description = "The GCP project number retrieved from the project ID"
  value       = data.google_project.this.number
}

output "project_id" {
  description = "The GCP project ID"
  value       = data.google_project.this.project_id
}

output "project_name" {
  description = "The display name of the GCP project"
  value       = data.google_project.this.name
}
