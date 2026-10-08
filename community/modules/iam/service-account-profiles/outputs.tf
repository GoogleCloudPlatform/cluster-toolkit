# Copyright 2026 "Google LLC"
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

output "service_account_email" {
  description = "The service account e-mail address passed in as var.service_account_email, available once this module's IAM grants have propagated."
  value       = local.email
  depends_on  = [time_sleep.wait_for_iam]
}

output "custom_role_id" {
  description = "Id of the custom role this module generated for the account, or null if it generated none."
  value       = one(google_project_iam_custom_role.this[*].id)
}

output "scoped_role_id" {
  description = "Id of the name-scoped custom role this module generated (compute.instances.setMetadata, bound only for instances named by instance_name_prefixes), or null if it generated none."
  value       = one(google_project_iam_custom_role.scoped[*].id)
}
