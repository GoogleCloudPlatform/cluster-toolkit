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

locals {
  load_runners = templatefile(
    "${path.module}/templates/startup-script-custom.tpl",
    {
      bucket = google_storage_bucket.configs_bucket.name,
      runners = [
        for runner in var.runners : {
          object      = basename(runner["destination"]),
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

  runners_dic = { for runner in var.runners :
    basename(runner["destination"])
    => {
      content = contains(keys(runner), "content") ? runner["content"] : null
      source  = contains(keys(runner), "source") ? runner["source"] : null
    }
  }
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_storage_bucket" "configs_bucket" {
  project                     = var.project_id
  name                        = "${var.deployment_name}-startup-scripts-${random_id.resource_name_suffix.hex}"
  uniform_bucket_level_access = true
  location                    = var.region
  storage_class               = "REGIONAL"
  labels                      = var.labels
}

resource "google_storage_bucket_object" "scripts" {
  # this writes all scripts exactly once into GCS
  for_each = local.runners_dic
  name     = each.key
  content  = each.value["content"]
  source   = each.value["source"]
  bucket   = google_storage_bucket.configs_bucket.name
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
