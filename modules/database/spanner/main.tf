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


resource "google_spanner_instance" "main" {
  project          = var.project_id
  name             = var.instance_name
  config           = var.config
  display_name     = var.display_name
  processing_units = var.processing_units
  labels           = local.labels
  edition          = var.edition
}

resource "google_spanner_database" "db" {
  for_each            = var.databases
  project             = var.project_id
  instance            = google_spanner_instance.main.name
  name                = each.value.name
  deletion_protection = each.value.deletion_protection
}

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "spanner", ghpc_role = "database" })
}

locals {
  # Flatten IAM members
  db_iam_members = flatten([
    for db_key, db in var.databases : [
      for iam in db.iam_members : {
        database = db_key
        role     = iam.role
        member   = iam.member
        key      = "${db_key}-${iam.role}-${iam.member}"
      }
    ]
  ])
}

resource "google_spanner_database_iam_member" "member" {
  for_each = { for iam in local.db_iam_members : iam.key => iam }
  project  = var.project_id
  instance = google_spanner_instance.main.name
  database = google_spanner_database.db[each.value.database].name
  role     = each.value.role
  member   = each.value.member
}
