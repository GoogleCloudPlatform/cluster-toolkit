# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

output "repo_url" {
  description = "The URL of the artifact registry repo."
  value       = local.repo_url
}

output "registry_url" {
  description = "The URL of the artifact registry repo."
  # Technically, this is the repo URL, not the registry URL
  # It is preserved for backward compatibility
  value = local.repo_url
}

output "repository_id" {
  description = "The ID of the created artifact registry repository."
  value       = local.repository_name
}

output "repository_name" {
  description = "The name (ID) of the created artifact registry repository."
  value       = local.repository_name
}

output "repository_resource_name" {
  description = "The full resource name of the repository."
  value       = google_artifact_registry_repository.artifact_registry.name
}
