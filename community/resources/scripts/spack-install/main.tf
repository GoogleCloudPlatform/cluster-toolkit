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
  script_content = templatefile(
    "${path.module}/templates/install_spack.tpl",
    {
      ZONE               = var.zone
      PROJECT_ID         = var.project_id
      INSTALL_DIR        = var.install_dir
      SPACK_URL          = var.spack_url
      SPACK_REF          = var.spack_ref
      COMPILERS          = var.compilers == null ? [] : var.compilers
      CONFIGS            = var.configs == null ? [] : var.configs
      LICENSES           = var.licenses == null ? [] : var.licenses
      PACKAGES           = var.packages == null ? [] : var.packages
      ENVIRONMENTS       = var.environments == null ? [] : var.environments
      MIRRORS            = var.spack_cache_url == null ? [] : var.spack_cache_url
      GPG_KEYS           = var.gpg_keys == null ? [] : var.gpg_keys
      CACHES_TO_POPULATE = var.caches_to_populate == null ? [] : var.caches_to_populate
      LOG_FILE           = var.log_file == null ? "/dev/null" : var.log_file
    }
  )
  install_spack_deps_runner = {
    "type"        = "ansible-local"
    "source"      = "${path.module}/scripts/install_spack_deps.yml"
    "destination" = "install_spack_deps.yml"
  }
  install_spack_runner = {
    "type"        = "shell"
    "content"     = local.script_content
    "destination" = "install_spack.sh"
  }
}
