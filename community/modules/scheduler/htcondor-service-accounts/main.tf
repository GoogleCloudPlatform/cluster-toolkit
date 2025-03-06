/**
 * Copyright 2024 Google LLC
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

# NB: the community/modules/project/service-account module will not output the
# service account e-mail address until all IAM bindings have been created; if
# underlying implementation changes, this module should declare explicit
# depends_on the IAM bindings to prevent race conditions for services that
# require them

module "access_point_service_account" {
  source = "../../../../community/modules/project/service-account"

  project_id      = var.project_id
  display_name    = "HTCondor Access Point"
  deployment_name = var.deployment_name
  name            = "access"
  project_roles   = var.access_point_roles
}

module "execute_point_service_account" {
  source = "../../../../community/modules/project/service-account"

  project_id      = var.project_id
  display_name    = "HTCondor Execute Point"
  deployment_name = var.deployment_name
  name            = "execute"
  project_roles   = var.execute_point_roles
}

module "central_manager_service_account" {
  source = "../../../../community/modules/project/service-account"

  project_id      = var.project_id
  display_name    = "HTCondor Central Manager"
  deployment_name = var.deployment_name
  name            = "cm"
  project_roles   = var.central_manager_roles
}
