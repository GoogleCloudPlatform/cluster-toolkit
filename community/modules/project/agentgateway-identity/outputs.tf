# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

output "node_service_account_email" {
  description = "Node service account email, created or supplied."
  value       = var.create_service_accounts ? google_service_account.node[0].email : var.node_service_account_email
  depends_on  = [time_sleep.iam_ready]
}

output "proxy_service_account_email" {
  description = "Proxy service account email, created or supplied."
  value       = var.create_service_accounts ? google_service_account.proxy[0].email : var.proxy_service_account_email
  depends_on  = [time_sleep.iam_ready]
}
