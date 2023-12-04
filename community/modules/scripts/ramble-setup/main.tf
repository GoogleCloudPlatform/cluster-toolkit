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
      chown_owner           = var.chown_owner == null ? "" : var.chown_owner
      chgrp_group           = var.chgrp_group == null ? "" : var.chgrp_group
      chmod_mode            = var.chmod_mode == null ? "" : var.chmod_mode
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

  bucket_md5  = substr(md5("${var.project_id}.${var.deployment_name}"), 0, 4)
  bucket_name = "ramble-scripts-${local.bucket_md5}"
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
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script?ref=50644b2"

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
