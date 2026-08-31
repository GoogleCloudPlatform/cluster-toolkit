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
  install_root_resolved    = var.install_root != null ? var.install_root : try(var.network_storage[var.network_storage_index].local_mount, "")
  install_root_clean       = trimsuffix(local.install_root_resolved, "/")
  module_name_resolved     = var.module_name != null ? var.module_name : var.app_id
  module_version_resolved  = var.module_version != null ? trimspace(var.module_version) : ""
  modulefile_relative_path = local.module_version_resolved != "" ? "${local.module_name_resolved}/${local.module_version_resolved}" : local.module_name_resolved
  registry_host            = split("/", var.image_ref)[0]

  sif_path        = "${local.install_root_clean}/${var.sif_subdir}/${var.app_id}.sif"
  wrapper_path    = "${local.install_root_clean}/${var.bin_subdir}/${var.app_id}"
  modulefile_path = "${local.install_root_clean}/${var.modulefile_subdir}/${local.modulefile_relative_path}"
  manifest_path   = "${local.install_root_clean}/${var.manifest_subdir}/${var.app_id}.yaml"
  wrapper_dir     = "${local.install_root_clean}/${var.bin_subdir}"

  app_env_name = "GHPC_APPTAINER_${upper(replace(replace(var.app_id, "-", "_"), ".", "_"))}_SIF"

  wrapper_env_exports     = [for key, value in var.env : "export ${key}='${replace(value, "'", "'\\''")}'"]
  wrapper_bind_arg_lines  = [for bind in var.bind_paths : "bind_args+=(--bind '${replace(bind, "'", "'\\''")}')"]
  wrapper_run_arg_lines   = [for arg in var.run_args : "run_args+=('${replace(arg, "'", "'\\''")}')"]
  wrapper_entry_arg_lines = [for arg in var.entrypoint_args : "entrypoint_args+=('${replace(arg, "'", "'\\''")}')"]

  wrapper_script = join("\n", concat(
    [
      "#!/bin/bash",
      "set -euo pipefail",
      "",
      "bind_args=()",
      "run_args=()",
      "entrypoint_args=()",
    ],
    local.wrapper_bind_arg_lines,
    local.wrapper_run_arg_lines,
    local.wrapper_entry_arg_lines,
    local.wrapper_env_exports,
    [
      "",
      "cmd=(apptainer exec)",
      "cmd+=(\"$${run_args[@]}\")",
      "cmd+=(\"$${bind_args[@]}\")",
      "cmd+=('${replace(local.sif_path, "'", "'\\''")}')",
      "if [[ -n '${replace(var.entrypoint, "'", "'\\''")}' ]]; then",
      "  cmd+=('${replace(var.entrypoint, "'", "'\\''")}')",
      "  cmd+=(\"$${entrypoint_args[@]}\")",
      "fi",
      "cmd+=(\"$@\")",
      "exec \"$${cmd[@]}\"",
      "",
    ]
  ))

  modulefile_content = join("\n", [
    "#%Module1.0",
    "proc ModulesHelp { } {",
    "  puts stderr {Adds ${var.display_name} wrapper commands to PATH.}",
    "}",
    "module-whatis {${var.display_name}}",
    "prepend-path PATH {${local.wrapper_dir}}",
    "setenv ${local.app_env_name} {${local.sif_path}}",
    "",
  ])

  manifest_content = chomp(yamlencode({
    app_id          = var.app_id
    display_name    = var.display_name
    image_ref       = var.image_ref
    project_id      = var.project_id
    deployment_name = var.deployment_name
    region          = var.region
    install_root    = local.install_root_resolved
    sif_path        = local.sif_path
    wrapper_path    = local.wrapper_path
    modulefile_path = local.modulefile_path
    manifest_path   = local.manifest_path
    module_name     = local.module_name_resolved
    module_version  = local.module_version_resolved != "" ? local.module_version_resolved : null
    bind_paths      = var.bind_paths
    env             = var.env
    run_args        = var.run_args
    entrypoint      = var.entrypoint != "" ? var.entrypoint : null
    entrypoint_args = var.entrypoint_args
    generated_by    = "community/modules/container/apptainer-app"
  }))

  install_runner = {
    type        = "shell"
    destination = "apptainer-install-${var.app_id}.sh"
    content     = <<-EOT
      #!/bin/bash
      set -euo pipefail

      if [[ ${var.install_apptainer ? "\"true\"" : "\"false\""} != "true" ]]; then
        echo "install_apptainer=false; skipping Apptainer installation for ${var.app_id}"
        exit 0
      fi

      if command -v apptainer >/dev/null 2>&1; then
        echo "Apptainer already present; skipping installation for ${var.app_id}"
        exit 0
      fi

      package_name='${replace(var.apptainer_package, "'", "'\\''")}'

      if command -v dnf >/dev/null 2>&1; then
        dnf install -y "$package_name"
      elif command -v yum >/dev/null 2>&1; then
        yum install -y "$package_name"
      elif command -v apt-get >/dev/null 2>&1; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get update
        apt-get install -y "$package_name"
      else
        echo "No supported package manager found while attempting to install Apptainer" >&2
        exit 1
      fi

      command -v apptainer >/dev/null 2>&1
    EOT
  }

  auth_runner = {
    type        = "shell"
    destination = "apptainer-auth-${var.app_id}.sh"
    content     = <<-EOT
      #!/bin/bash
      set -euo pipefail

      if ! command -v apptainer >/dev/null 2>&1; then
        echo "Apptainer is required before authenticating to Artifact Registry" >&2
        exit 1
      fi

      if ! command -v gcloud >/dev/null 2>&1; then
        echo "gcloud is required before authenticating to Artifact Registry" >&2
        exit 1
      fi

      registry_host='${replace(local.registry_host, "'", "'\\''")}'
      access_token="$(gcloud auth print-access-token)"

      if [[ -z "$access_token" ]]; then
        echo "Failed to obtain an access token from gcloud" >&2
        exit 1
      fi

      install -d -m 0700 "$${HOME:-/root}/.apptainer"
      apptainer registry login --username=oauth2accesstoken --password="$access_token" "docker://$registry_host"
    EOT
  }

  stage_runner = {
    type        = "shell"
    destination = "apptainer-stage-${var.app_id}.sh"
    content     = <<-EOT
      #!/bin/bash
      set -euo pipefail

      if ! command -v apptainer >/dev/null 2>&1; then
        echo "Apptainer is required before staging ${var.app_id}" >&2
        exit 1
      fi

      output_path='${replace(local.sif_path, "'", "'\\''")}'
      image_ref='${replace(var.image_ref, "'", "'\\''")}'
      pull_policy='${replace(var.pull_policy, "'", "'\\''")}'
      install -d -m 0755 "$(dirname "$output_path")"

      if [[ "$pull_policy" != "always" && -f "$output_path" ]]; then
        echo "SIF already exists for ${var.app_id}: $output_path"
        exit 0
      fi

      tmp_path="$${output_path}.tmp.$$$$"
      trap 'rm -f "$tmp_path"' EXIT

      apptainer pull "$tmp_path" "docker://$image_ref"
      chmod 0644 "$tmp_path"
      mv -f "$tmp_path" "$output_path"
      trap - EXIT
    EOT
  }

  wrapper_runner = {
    type        = "shell"
    destination = "apptainer-wrapper-${var.app_id}.sh"
    content     = <<-EOT
      #!/bin/bash
      set -euo pipefail

      wrapper_path='${replace(local.wrapper_path, "'", "'\\''")}'
      install -d -m 0755 "$(dirname "$wrapper_path")"
      cat > "$wrapper_path" <<'EOF_WRAPPER'
      ${local.wrapper_script}
      EOF_WRAPPER
      chmod 0755 "$wrapper_path"
    EOT
  }

  modulefile_runner = {
    type        = "shell"
    destination = "apptainer-modulefile-${var.app_id}.sh"
    content     = <<-EOT
      #!/bin/bash
      set -euo pipefail

      modulefile_path='${replace(local.modulefile_path, "'", "'\\''")}'
      install -d -m 0755 "$(dirname "$modulefile_path")"
      cat > "$modulefile_path" <<'EOF_MODULEFILE'
      ${local.modulefile_content}
      EOF_MODULEFILE
      chmod 0644 "$modulefile_path"
    EOT
  }

  manifest_runner = {
    type        = "shell"
    destination = "apptainer-manifest-${var.app_id}.sh"
    content     = <<-EOT
      #!/bin/bash
      set -euo pipefail

      manifest_path='${replace(local.manifest_path, "'", "'\\''")}'
      install -d -m 0755 "$(dirname "$manifest_path")"
      cat > "$manifest_path" <<'EOF_MANIFEST'
      ${local.manifest_content}
      EOF_MANIFEST
      printf 'generated_at: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$manifest_path"
      chmod 0644 "$manifest_path"
    EOT
  }

  startup_runners = [
    local.install_runner,
    local.auth_runner,
    local.stage_runner,
    local.wrapper_runner,
    local.modulefile_runner,
    local.manifest_runner,
  ]
}

resource "terraform_data" "module_ready" {
  input = {
    app_id          = var.app_id
    image_ref       = var.image_ref
    install_root    = local.install_root_resolved
    modulefile_path = local.modulefile_path
    sif_path        = local.sif_path
  }
}
