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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "spack-setup", ghpc_role = "scripts" })
}

locals {
  profile_script = <<-EOF
    SPACK_PYTHON=${var.spack_virtualenv_path}/bin/python3
    if [ -f ${var.install_dir}/share/spack/setup-env.sh ]; then
          test -t 1 && echo "Running Spack setup, this may take a moment on first login."
          . ${var.install_dir}/share/spack/setup-env.sh
    fi
  EOF

  supported_cache_versions = ["v0.19.0", "v0.20.0"]
  cache_version            = contains(local.supported_cache_versions, var.spack_ref) ? var.spack_ref : "latest"
  add_google_mirror_script = !var.configure_for_google ? "" : <<-EOF
    if ! spack mirror list | grep -q google_binary_cache; then
      spack mirror add --scope site google_binary_cache gs://spack/${local.cache_version}
      spack buildcache keys --install --trust
    fi
  EOF

  finalize_setup_script = <<-EOF
    set -e
    . ${var.spack_profile_script_path}
    spack config --scope site add 'packages:all:permissions:read:world'
    spack gpg init
    spack compiler find --scope site
    ${local.add_google_mirror_script}
  EOF

  script_content = templatefile(
    "${path.module}/templates/spack_setup.yml.tftpl",
    {
      sw_name               = "spack"
      profile_script        = indent(4, yamlencode(local.profile_script))
      install_dir           = var.install_dir
      git_url               = var.spack_url
      git_ref               = var.spack_ref
      chown_owner           = var.chown_owner == null ? "" : var.chown_owner
      chgrp_group           = var.chgrp_group == null ? "" : var.chgrp_group
      chmod_mode            = var.chmod_mode == null ? "" : var.chmod_mode
      finalize_setup_script = indent(4, yamlencode(local.finalize_setup_script))
      profile_script_path   = var.spack_profile_script_path
    }
  )

  install_spack_deps_runner = {
    "type"        = "ansible-local"
    "source"      = "${path.module}/scripts/install_spack_deps.yml"
    "destination" = "install_spack_deps.yml"
    "args"        = "-e virtualenv_path=${var.spack_virtualenv_path}"
  }
  install_spack_runner = {
    "type"        = "ansible-local"
    "content"     = local.script_content
    "destination" = "install_spack.yml"
  }

  bucket_md5  = substr(md5("${var.project_id}.${var.deployment_name}"), 0, 4)
  bucket_name = "spack-scripts-${local.bucket_md5}"
  runners     = [local.install_spack_deps_runner, local.install_spack_runner]

  combined_runner = {
    "type"        = "shell"
    "content"     = module.startup_script.startup_script
    "destination" = "spack-install-and-setup.sh"
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
