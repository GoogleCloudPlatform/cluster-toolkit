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

locals {
  ansible_installer = {
    type        = "shell"
    source      = "${path.module}/examples/install_ansible.sh"
    destination = "install_ansible_automatic.sh"
  }

  ansible_local_runners     = [for r in var.runners : r if r.type == "ansible-local"]
  prepend_ansible_installer = length(local.ansible_local_runners) > 0 && var.prepend_ansible_installer
  runners                   = local.prepend_ansible_installer ? concat([local.ansible_installer], var.runners) : var.runners

  storage_bucket = coalesce(one(module.config_storage_bucket), one(data.google_storage_bucket.existing_bucket))

  gcs_bucket_path_trimmed = var.gcs_bucket_path == null ? null : trimsuffix(var.gcs_bucket_path, "/")
  storage_folder_path = local.gcs_bucket_path_trimmed == null ? null : regex("^gs://([^/]*)/*(.*)", local.gcs_bucket_path_trimmed)[1]
  storage_folder_path_prefix = local.storage_folder_path == null || local.storage_folder_path == "" ? "" : "${local.storage_folder_path}/"

  load_runners = templatefile(
    "${path.module}/templates/startup-script-custom.tpl",
    {
      bucket = local.storage_bucket.name,
      runners = [
        for runner in local.runners : {
          object      = google_storage_bucket_object.scripts[basename(runner["destination"])].output_name
          type        = runner["type"]
          destination = runner["destination"]
          args        = contains(keys(runner), "args") ? runner["args"] : ""
        }
      ]
    }
  )

  stdlib_head     = file("${path.module}/files/startup-script-stdlib-head.sh")
  get_from_bucket = file("${path.module}/files/get_from_bucket.sh")
  stdlib_body     = file("${path.module}/files/startup-script-stdlib-body.sh")

  # List representing complete content, to be concatenated together.
  stdlib_list = [
    local.stdlib_head,
    local.get_from_bucket,
    local.load_runners,
    local.stdlib_body,
  ]

  # Final content output to the user
  stdlib = join("", local.stdlib_list)

  runners_map = { for runner in local.runners :
    basename(runner["destination"])
    => {
      content = lookup(runner, "content", null)
      source  = lookup(runner, "source", null)
    }
  }
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

module "config_storage_bucket" {
  count                       = var.gcs_bucket_path == null ? 1 : 0
  source                      = "terraform-google-modules/cloud-storage/google//modules/simple_bucket"
  version                     = "~> 3.4"

  project_id                  = var.project_id
  name                        = "${var.deployment_name}-startup-scripts-${random_id.resource_name_suffix.hex}"
  location                    = var.region
  storage_class               = "REGIONAL"
  labels                      = var.labels
}

data "google_storage_bucket" "existing_bucket" {
  count                       = var.gcs_bucket_path != null ? 1 : 0
  name                        = regex("^gs://([^/]*)/*(.*)", local.gcs_bucket_path_trimmed)[0]
}

resource "google_storage_bucket_object" "scripts" {
  # this writes all scripts exactly once into GCS
  for_each = local.runners_map
  name     = "${local.storage_folder_path_prefix}${each.key}"
  content  = each.value["content"]
  source   = each.value["source"]
  bucket   = local.storage_bucket.name
  timeouts {
    create = "10m"
    update = "10m"
  }
}

resource "local_file" "debug_file" {
  for_each = toset(var.debug_file != null ? [var.debug_file] : [])
  filename = var.debug_file
  content  = local.stdlib
}
