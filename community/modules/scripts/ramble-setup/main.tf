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
  labels = merge(var.labels, { ghpc_module = "ramble-setup", ghpc_role = "scripts" })
}

locals {
  profile_script = <<-EOF
    if [ -f ${var.install_dir}/share/ramble/setup-env.sh ]; then
          test -t 1 && echo "** Ramble's python virtualenv (/usr/local/ramble-python) is activated. Call 'deactivate' to deactivate."
          VIRTUAL_ENV_DISABLE_PROMPT=1 . ${var.ramble_virtualenv_path}/bin/activate
          . ${var.install_dir}/share/ramble/setup-env.sh
    fi
  EOF

  script_content = templatefile(
    "${path.module}/templates/ramble_setup.yml.tftpl",
    {
      sw_name               = "ramble"
      profile_script        = indent(4, yamlencode(local.profile_script))
      install_dir           = var.install_dir
      git_url               = var.ramble_url
      git_ref               = var.ramble_ref
      chmod_mode            = var.chmod_mode
      system_user_name      = var.system_user_name
      system_user_uid       = var.system_user_uid
      system_user_gid       = var.system_user_gid
      finalize_setup_script = "echo 'no finalize setup script'"
      profile_script_path   = var.ramble_profile_script_path
    }
  )

  install_ramble_deps_runner = {
    "type"        = "ansible-local"
    "source"      = "${path.module}/scripts/install_ramble_deps.yml"
    "destination" = "install_ramble_deps.yml"
    "args"        = "-e virtualenv_path=${var.ramble_virtualenv_path}"
  }

  python_reqs_content = templatefile(
    "${path.module}/templates/install_ramble_python_deps.yml.tftpl",
    {
      install_dir     = var.install_dir
      virtualenv_path = var.ramble_virtualenv_path
    }
  )

  python_reqs_runner = {
    "type"        = "ansible-local"
    "content"     = local.python_reqs_content
    "destination" = "install_ramble_reqs.yml"
  }

  install_ramble_runner = {
    "type"        = "ansible-local"
    "content"     = local.script_content
    "destination" = "install_ramble.yml"
  }

  bucket_md5 = substr(md5("${var.project_id}.${var.deployment_name}"), 0, 8)
  # Max bucket name length is 63, so truncate deployment_name if necessary.
  #   The string "-ramble-scripts-" is 16 characters and bucket_md5 is 8 characters,
  #   leaving 63-16-8=39 chars for deployment_name.
  bucket_name = "${substr(var.deployment_name, 0, 39)}-ramble-scripts-${local.bucket_md5}"
  runners     = [local.install_ramble_deps_runner, local.install_ramble_runner, local.python_reqs_runner]

  combined_runner = {
    "type"        = "shell"
    "content"     = module.startup_script.startup_script
    "destination" = "ramble-install-and-setup.sh"
  }

}

resource "google_storage_bucket" "bucket" {
  project                     = var.project_id
  name                        = local.bucket_name
  uniform_bucket_level_access = true
  location                    = var.region
  storage_class               = "REGIONAL"
  labels                      = local.labels
}

module "startup_script" {
  source = "../../../../modules/scripts/startup-script"

  labels          = local.labels
  project_id      = var.project_id
  deployment_name = var.deployment_name
  region          = var.region
  runners         = local.runners
  gcs_bucket_path = "gs://${google_storage_bucket.bucket.name}"
}

resource "local_file" "debug_file_shell_install" {
  content  = local.script_content
  filename = "${path.module}/debug_install.yml"
}
