# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

locals {
  install_root_resolved = var.install_root != null ? var.install_root : var.network_storage[var.network_storage_index].local_mount
  install_root_clean    = local.install_root_resolved == "/" ? "" : trimsuffix(local.install_root_resolved, "/")

  sif_dir        = "${local.install_root_clean}/${var.sif_subdir}"
  bin_dir        = "${local.install_root_clean}/${var.bin_subdir}"
  modulefile_dir = "${local.install_root_clean}/${var.modulefile_subdir}"
  manifest_dir   = "${local.install_root_clean}/${var.manifest_subdir}"

  layout_runner = {
    type        = "shell"
    destination = "apptainer-runtime-layout.sh"
    content     = <<-EOT
      #!/bin/bash
      set -euo pipefail

      install -d -m 0755 \
        '${replace(local.sif_dir, "'", "'\\''")}' \
        '${replace(local.bin_dir, "'", "'\\''")}' \
        '${replace(local.modulefile_dir, "'", "'\\''")}' \
        '${replace(local.manifest_dir, "'", "'\\''")}'
    EOT
  }

  module_init_runner = {
    type        = "shell"
    destination = "apptainer-runtime-modulepath.sh"
    content     = <<-EOT
      #!/bin/bash
      set -euo pipefail

      module_init_path='${replace(var.module_init_path, "'", "'\\''")}'
      install -d -m 0755 "$(dirname "$module_init_path")"
      cat > "$module_init_path" <<'EOF_MODULE_INIT'
      #!/bin/bash
      module use '${replace(local.modulefile_dir, "'", "'\\''")}' 2>/dev/null || true
      EOF_MODULE_INIT
      chmod 0644 "$module_init_path"
    EOT
  }

  startup_runners = [
    local.layout_runner,
    local.module_init_runner,
  ]
}

resource "terraform_data" "module_ready" {
  input = {
    install_root   = local.install_root_resolved
    sif_dir        = local.sif_dir
    bin_dir        = local.bin_dir
    modulefile_dir = local.modulefile_dir
    manifest_dir   = local.manifest_dir
  }
}
