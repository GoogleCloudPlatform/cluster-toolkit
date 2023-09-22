/**
 * Copyright 2023 Google LLC
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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "startup-script", ghpc_role = "scripts" })
}

locals {
  ops_agent_installer = var.install_cloud_ops_agent ? [{
    type        = "shell"
    source      = "${path.module}/files/install_cloud_ops_agent.sh"
    destination = "install_cloud_ops_agent_automatic.sh"
  }] : []

  warnings = [
    {
      type        = "shell"
      content     = "chmod -R o+r /var/log/journal"
      destination = "access_joural.sh"
    },
    {
      type        = "data"
      content     = file("${path.module}/files/running-script-warning.sh")
      destination = "/etc/profile.d/20-running-script-warning.sh"
    }
  ]

  configure_ssh = length(var.configure_ssh_host_patterns) > 0
  host_args = {
    host_name_prefix = var.configure_ssh_host_patterns
  }

  prefix_file = "/tmp/prefix_file.json"

  configure_ssh_runners = local.configure_ssh ? [
    {
      type        = "data"
      source      = "${path.module}/files/setup-ssh-keys.sh"
      destination = "/usr/local/ghpc/setup-ssh-keys.sh"
    },
    {
      type        = "data"
      source      = "${path.module}/files/setup-ssh-keys.yml"
      destination = "/usr/local/ghpc/setup-ssh-keys.yml"
    },
    {
      type        = "data"
      content     = jsonencode(local.host_args)
      destination = local.prefix_file
    },
    {
      type        = "ansible-local"
      content     = file("${path.module}/files/configure-ssh.yml")
      destination = "configure-ssh.yml"
      args        = "-e  @${local.prefix_file}"
    }
  ] : []

  has_ansible_runners = anytrue([for r in var.runners : r.type == "ansible-local"]) || local.configure_ssh
  install_ansible     = var.install_ansible == null ? local.has_ansible_runners : var.install_ansible
  ansible_installer = local.install_ansible ? [{
    type        = "shell"
    source      = "${path.module}/files/install_ansible.sh"
    destination = "install_ansible_automatic.sh"
    args        = var.ansible_virtualenv_path
  }] : []

  runners = concat(local.warnings, local.ops_agent_installer, local.ansible_installer, local.configure_ssh_runners, var.runners)

  bucket_regex               = "^gs://([^/]*)/*(.*)"
  gcs_bucket_path_trimmed    = var.gcs_bucket_path == null ? null : trimsuffix(var.gcs_bucket_path, "/")
  storage_folder_path        = local.gcs_bucket_path_trimmed == null ? null : regex(local.bucket_regex, local.gcs_bucket_path_trimmed)[1]
  storage_folder_path_prefix = local.storage_folder_path == null || local.storage_folder_path == "" ? "" : "${local.storage_folder_path}/"

  user_provided_bucket_name = try(regex(local.bucket_regex, local.gcs_bucket_path_trimmed)[0], null)
  storage_bucket_name       = coalesce(one(google_storage_bucket.configs_bucket[*].name), local.user_provided_bucket_name)

  load_runners = templatefile(
    "${path.module}/templates/startup-script-custom.tpl",
    {
      bucket = local.storage_bucket_name,
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
    basename(runner["destination"]) => {
      content = lookup(runner, "content", null)
      source  = lookup(runner, "source", null)
    }
  }
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_storage_bucket" "configs_bucket" {
  count                       = var.gcs_bucket_path == null ? 1 : 0
  project                     = var.project_id
  name                        = "${var.deployment_name}-startup-scripts-${random_id.resource_name_suffix.hex}"
  uniform_bucket_level_access = true
  location                    = var.region
  storage_class               = "REGIONAL"
  labels                      = local.labels
}

resource "google_storage_bucket_iam_binding" "viewers" {
  bucket  = local.storage_bucket_name
  role    = "roles/storage.objectViewer"
  members = var.bucket_viewers
}

resource "google_storage_bucket_object" "scripts" {
  # this writes all scripts exactly once into GCS
  for_each = local.runners_map
  name     = "${local.storage_folder_path_prefix}${each.key}-${substr(try(md5(each.value.content), filemd5(each.value.source)), 0, 4)}"
  content  = each.value.content
  source   = each.value.source
  bucket   = local.storage_bucket_name
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
