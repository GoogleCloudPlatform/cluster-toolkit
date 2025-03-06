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

output "network_storage" {
  description = "Describes a remote network storage to be mounted by fs-tab."
  value = {
    server_ip             = var.server_ip
    remote_mount          = local.remote_mount
    local_mount           = var.local_mount
    fs_type               = var.fs_type
    mount_options         = var.mount_options
    client_install_runner = local.client_install_runner
    mount_runner          = local.mount_runner
  }
}

locals {
  # Update remote mount to include a slash if the fs_type requires one to exist
  remote_mount_with_slash = length(regexall("^/.*", var.remote_mount)) > 0 ? (
    var.remote_mount
  ) : format("/%s", var.remote_mount)
  remote_mount = contains(local.mount_vanilla_supported_fstype, var.fs_type) ? (
    local.remote_mount_with_slash
  ) : var.remote_mount

  # Client Install
  ddn_lustre_client_install_script = templatefile(
    "${path.module}/templates/ddn_exascaler_luster_client_install.tftpl",
    {
      server_ip    = split("@", var.server_ip)[0]
      remote_mount = local.remote_mount
      local_mount  = var.local_mount
    }
  )
  nfs_client_install_script  = file("${path.module}/scripts/install-nfs-client.sh")
  gcs_fuse_install_script    = file("${path.module}/scripts/install-gcs-fuse.sh")
  daos_client_install_script = file("${path.module}/scripts/install-daos-client.sh")

  install_scripts = {
    "lustre"  = local.ddn_lustre_client_install_script
    "nfs"     = local.nfs_client_install_script
    "gcsfuse" = local.gcs_fuse_install_script
    "daos"    = local.daos_client_install_script
  }

  client_install_runner = {
    "type"        = "shell"
    "content"     = lookup(local.install_scripts, var.fs_type, "echo 'skipping: client_install_runner not yet supported for ${var.fs_type}'")
    "destination" = "install_filesystem_client${replace(var.local_mount, "/", "_")}.sh"
    "args"        = ""
  }

  mount_vanilla_supported_fstype = ["lustre", "nfs"]
  mount_runner_vanilla = {
    "type"        = "shell"
    "destination" = "mount_filesystem${replace(var.local_mount, "/", "_")}.sh"
    "args"        = "\"${var.server_ip}\" \"${local.remote_mount}\" \"${var.local_mount}\" \"${var.fs_type}\" \"${var.mount_options}\""
    "content" = (
      contains(local.mount_vanilla_supported_fstype, var.fs_type) ?
      file("${path.module}/scripts/mount.sh") :
      "echo 'skipping: mount_runner not yet supported for ${var.fs_type}'"
    )
  }
  gcsbucket = trimprefix(var.remote_mount, "gs://")
  mount_runner_gcsfuse = {
    "type"        = "shell"
    "destination" = "mount_filesystem${replace(var.local_mount, "/", "_")}.sh"
    "args"        = "\"not-used\" \"${local.gcsbucket}\" \"${var.local_mount}\" \"${var.fs_type}\" \"${var.mount_options}\""
    "content"     = file("${path.module}/scripts/mount.sh")
  }

  mount_runner_daos = {
    "type" = "shell"
    "content" = templatefile("${path.module}/templates/mount-daos.sh.tftpl", {
      access_points     = var.remote_mount
      daos_agent_config = var.parallelstore_options.daos_agent_config
      dfuse_environment = var.parallelstore_options.dfuse_environment
      local_mount       = var.local_mount
      # avoid passing "--" as mount option to dfuse
      mount_options = length(var.mount_options) == 0 ? "" : join(" ", [for opt in split(",", var.mount_options) : "--${opt}"])
    })
    "destination" = "mount_filesystem${replace(var.local_mount, "/", "_")}.sh"
  }

  mount_scripts = {
    "lustre"  = local.mount_runner_vanilla
    "nfs"     = local.mount_runner_vanilla
    "gcsfuse" = local.mount_runner_gcsfuse
    "daos"    = local.mount_runner_daos
  }

  mount_runner = lookup(local.mount_scripts, var.fs_type, local.mount_runner_vanilla)
}

output "client_install_runner" {
  description = "Runner that performs client installation needed to use file system."
  value       = local.client_install_runner
}

output "mount_runner" {
  description = "Runner that mounts the file system."
  value       = local.mount_runner
}
