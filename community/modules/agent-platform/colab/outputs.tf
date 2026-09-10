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

output "id" {
  description = "The unique resource name of the Colab runtime template."
  value       = google_colab_runtime_template.template.id
}

output "template_id" {
  description = "The generated ID of the Colab runtime template."
  value       = google_colab_runtime_template.template.name
}

output "runtime_id" {
  description = "The generated ID of the Colab runtime."
  value       = google_colab_runtime.runtime.name
}
