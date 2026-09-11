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

output "engine_id" {
  description = "The ID of the created Discovery Engine."
  value       = var.engine_id
  depends_on  = [terraform_data.engine]
}

output "assistant_id" {
  description = "The ID of the created Assistant."
  value       = var.assistant_id
  depends_on  = [terraform_data.assistant]
}

output "engine_name" {
  description = "The full resource name of the Discovery Engine."
  value       = "projects/${var.project_id}/locations/${var.location}/collections/${var.collection}/engines/${var.engine_id}"
  depends_on  = [terraform_data.engine]
}

output "assistant_name" {
  description = "The full resource name of the Assistant."
  value       = "projects/${var.project_id}/locations/${var.location}/collections/${var.collection}/engines/${var.engine_id}/assistants/${var.assistant_id}"
  depends_on  = [terraform_data.assistant]
}
