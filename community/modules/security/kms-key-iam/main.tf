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

locals {
  # Per-service Google service agents, keyed by the short name callers use
  # in var.service_agents. `%s` is the project number, which is why this
  # needs the data source below: callers know their project id, not its
  # number.
  service_agent_formats = {
    compute          = "service-%s@compute-system.iam.gserviceaccount.com"
    storage          = "service-%s@gs-project-accounts.iam.gserviceaccount.com"
    filestore        = "service-%s@cloud-filer.iam.gserviceaccount.com"
    cloudsql         = "service-%s@gcp-sa-cloud-sql.iam.gserviceaccount.com"
    artifactregistry = "service-%s@gcp-sa-artifactregistry.iam.gserviceaccount.com"
    secretmanager    = "service-%s@gcp-sa-secretmanager.iam.gserviceaccount.com"
    pubsub           = "service-%s@gcp-sa-pubsub.iam.gserviceaccount.com"
    notebooks        = "service-%s@gcp-sa-notebooks.iam.gserviceaccount.com"
  }

  derived = toset([
    for agent in var.service_agents :
    "serviceAccount:${format(local.service_agent_formats[agent], data.google_project.this.number)}"
  ])

  # Union rather than either/or: only agents belonging to var.project_id can
  # be derived, so a key in one project granting agents from another needs
  # the explicit list as well.
  principals = setunion(local.derived, var.service_agent_principals)
}

# Resolves the project number behind the service-agent addresses, so callers
# do not have to look it up and paste it into a blueprint.
data "google_project" "this" {
  project_id = var.project_id
}

resource "google_kms_crypto_key_iam_member" "this" {
  for_each = local.principals

  crypto_key_id = var.crypto_key_id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = each.value
}
