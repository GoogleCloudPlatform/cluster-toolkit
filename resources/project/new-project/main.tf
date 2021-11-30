/**
 * Copyright 2021 Google LLC
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

module "project_factory" {
  source  = "terraform-google-modules/project-factory/google"
  version = "~> 10.1"

  name                    = var.project_id
  random_project_id       = true
  folder_id               = var.folder_id
  org_id                  = var.org_id
  billing_account         = var.billing_account
  default_service_account = var.default_service_account
}
