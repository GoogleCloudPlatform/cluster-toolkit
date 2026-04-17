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

output "key" {
  description = "Service account key (if creation was requested)"
  value       = module.service_account.key
}

output "service_account_email" {
  description = "Service account e-mail address"
  value       = module.service_account.email
  depends_on = [
    module.service_account,
  ]
}

output "service_account_iam_email" {
  description = "Service account IAM binding format (serviceAccount:name@example.com)"
  value       = module.service_account.iam_email
  depends_on = [
    module.service_account,
  ]
}
