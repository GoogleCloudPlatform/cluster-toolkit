output "gcnv_storage_pool_id" {
  description = "Storage Pool ID for Google Cloud NetApp Volume"
  value       = module.netapp_volumes.storage_pool.id
}

output "gcnv_volume" {
  description = "Volume ID for Google Cloud NetApp Volume"
  value       =  module.netapp_volumes.storage_volumes
}

output "gcnv_volume_server_ip" {
  description = "Server IP of the volume for Google Cloud NetApp Volume"
  value       = local.server_ip
}

output "gcnv_volume_mount_points" {
  value = local.mount_points
}

output "capacity_gb" {
  description = "File share capacity in GiB."
  value       = var.volume_capacity_gib
}


output "network_storage" {
  description = "Describes a remote network storage to be mounted."
  value = {
    server_ip             = local.server_ip
    remote_mount          = local.remote_mount
    local_mount           = var.volume_share_name
    fs_type               = local.fs_type
    mount_options         = var.mount_options
    client_install_runner = local.install_nfs_client_runner
    mount_runner          = local.mount_runner
  }
}

locals {
  remote_mount              = local.mount_points[0].export
  #mount_points              = module.netapp_volumes.mount_options[0]
  mount_points              = [for volume in module.netapp_volumes.storage_volumes : volume.mount_options[0]]
  mount_path                = split("/", local.mount_points[0].export_full)
  #mount_path                = split("/", local.mount_points)
  server_ip                 = local.mount_path[0]
  fs_type                   = "nfs"
  script_path               = substr(path.module, 0, length(path.module) - 4)
  nfs_client_install_script = file("${local.script_path}/scripts/install-nfs-client.sh")

  install_nfs_client_runner = {
    "type"        = "shell"
    "content"     = local.nfs_client_install_script
    #"destination" = "install_filesystem_client${replace(var.volume_share_name, "/", "_")}.sh"
    "destination" = "install_filesystem_client_${var.volume_share_name}.sh"
  }

  mount_vanilla_supported_fstype = ["nfs"]
  install_scripts = {
    "nfs" = local.nfs_client_install_script
  }

  mount_runner_vanilla = {
    "type"        = "shell"
    #"destination" = "mount_filesystem${replace(var.volume_share_name, "/", "_")}.sh"
    "destination" = "mount_filesystem_${var.volume_share_name}.sh"
    "args"        = "\"${local.server_ip}\" \"${local.remote_mount}\" \"${var.volume_share_name}\" \"${local.fs_type}\" \"${var.mount_options}\""
    "content" = (
      contains(local.mount_vanilla_supported_fstype, local.fs_type) ?
      file("${local.script_path}/scripts/mount.sh") :
      "echo 'skipping: mount_runner not yet supported for ${local.fs_type}'"
    )
  }

  mount_runner = lookup(local.mount_scripts, local.fs_type, local.mount_runner_vanilla)

  mount_scripts = {
    "nfs" = local.mount_runner_vanilla
  }
}

output "install_nfs_client" {
  description = "Script for installing NFS client"
  value       = file("${local.script_path}/scripts/install-nfs-client.sh")
}

output "install_nfs_client_runner" {
  description = "Runner that performs client installation needed to use file system."
  value       = local.install_nfs_client_runner
}

output "mount_runner" {
  description = "Runner that mounts the file system."
  value       = local.mount_runner
}