variable "project_id" {
  description = "ID of project in which NetApp volumes will be created."
  type        = string
}

variable "deployment_name" {
  description = "Name of the HPC deployment, used as name of the storage pool and volume if no name is specified."
  type        = string
}

variable "region" {
  description = "Location for NetApp volumes at Enterprise tier."
  type        = string
}

variable "network_name" {
  description = "VPC network name with format: projects/{{project}}/global/networks/{{network}}"
  type        = string
}

variable "network_id" {
  description = <<-EOT
    The ID of the GCE VPC network to which the instance is connected given in the format:
    `projects/<project_id>/global/networks/<network_name>`"
    EOT
  type        = string
  validation {
    condition     = length(split("/", var.network_id)) == 5
    error_message = "The network id must be provided in the following format: projects/<project_id>/global/networks/<network_name>."
  }
}

variable "labels" {
  description = "Labels to add to supporting resources. Key-value pairs."
  type        = map(string)
}

################ Storage Pool Variables ########################
variable "storage_pool_name" {
  description = "The resource name of the storage pool."
  type        = string
  default     = null
}

variable "storage_pool_service_level" {
  description = "Service level of the storage pool. Possible values are: PREMIUM, EXTREME, STANDARD, FLEX."
  type        = string
  default     = "PREMIUM"
  nullable    = false
  validation {
    condition     = contains(["PREMIUM", "EXTREME", "STANDARD", "FLEX"], var.storage_pool_service_level)
    error_message = "ERROR: Enter valid value for service level. Valid values are 'PREMIUM', 'EXTREME', 'STANDARD', 'FLEX'."
  }
}

variable "storage_pool_size" {
  description = "Capacity of the storage pool (in GiB)"
  type        = number
  default     = 2048
}

variable "storage_pool_ldap_enabled" {
  description = <<-EOD
  When enabled, the volumes uses Active Directory as LDAP name service for UID/GID lookups. 
  Required to enable extended group support for NFSv3, using security identifiers for 
  NFSv4.1 or principal names for kerberized NFSv4.1.
  EOD
  type        = bool
  default     = false
}

variable "storage_pool_active_directory" {
  description = <<-EOD
  Specifies the Active Directory policy to be used. 
  The policy needs to be in the same location as the storage pool.
  EOD  
  type        = string
  default     = null
}

variable "storage_pool_allow_auto_tiering" {
  description = "True if the storage pool supports Auto Tiering enabled volumes. "
  type        = string
  default     = false
}
################################################################

################ Storage Volume Variables ########################
variable "volume_name" {
  description = "The name of the volume. Needs to be unique per location."
  type        = string
  default     = "hpc-volume1"
}

variable "volume_capacity_gib" {
  description = "Capacity of the volume (in GiB)."
  type        = string
  default     = "100"
}

variable "volume_share_name" {
  description = "Share name (SMB) or export path (NFS) of the volume. Needs to be unique per location."
  type        = string
  default     = "hpc-share"
}

variable "volume_protocol" {
  description = <<-EOD
  The protocol of the volume. Allowed combinations are 
  ['NFSV3'], ['NFSV4'], ['SMB'], ['NFSV3', 'NFSV4'], ['SMB', 'NFSV3'] and ['SMB', 'NFSV4']. 
  Each value may be one of: NFSV3, NFSV4, SMB.
   EOD
  type        = list(string)
  default = [ "NFSV3" ]
  validation {
    condition = can(
      contains(["NFSV3"], var.volume_protocol) ||
      contains(["NFSV4"], var.volume_protocol) ||
      contains(["SMB"], var.volume_protocol) ||
      contains(["NFSV3", "NFSV4"], var.volume_protocol) ||
      contains(["SMB", "NFSV3"], var.volume_protocol) ||
      contains(["SMB", "NFSV4"], var.volume_protocol)
    )
    error_message = "Allowed combinations are ['NFSV3'], ['NFSV4'], ['SMB'], ['NFSV3', 'NFSV4'], ['SMB', 'NFSV3'], and ['SMB', 'NFSV4']."
  }
}

variable "volume_deletion_policy" {
  description = "Policy to determine if the volume should be deleted forcefully."
  type        = string
  default     = "FORCE"
  nullable    = false
  validation {
    condition     = contains(["DEFAULT", "FORCE"], var.volume_deletion_policy)
    error_message = "ERROR: Valid values are 'DEFAULT' and 'FORCE'."
  }
}

variable "snapshot_policy_enabled" {
  description = "Enable or disable the snapshot policy"
  type        = bool
  default     = true
}

variable "daily_schedule" {
  description = "Daily schedule for snapshots"
  type = object({
    snapshots_to_keep = number
    minute            = number
    hour              = number
  })
  #default = null
  default = {
    snapshots_to_keep = 1
    minute            = 0
    hour              = 0
  }
}

variable "export_policy_rules" {
  description = "Export policy rules"
  type = map(object({
    allowed_clients = optional(string)
    access_type     = optional(string)
    nfsv3           = optional(bool)
    nfsv4           = optional(bool)
    has_root_access = optional(bool)
  }))
  default = {
    rules = {
      allowed_clients = "0.0.0.0/0"
      access_type     = "READ_WRITE"
      nfsv3           = true
      nfsv4           = false
      has_root_access = true
    }
  }

  validation {
    condition = alltrue([
      for rule in var.export_policy_rules : contains(["READ_ONLY", "READ_WRITE", "READ_NONE"], rule.access_type)
    ])
    error_message = "Access type must be one of 'READ_ONLY', 'READ_WRITE', or 'READ_NONE'."
  }
}

variable "mount_options" {
  description = "Options describing various aspects of the file system. Consider adding setting to 'defaults,_netdev,implicit_dirs' when using gcsfuse."
  type        = string
  default     = "defaults,_netdev"
}

variable "volume_scheduled_backup_enabled_flag" {
  description = "When set to true, scheduled backup is enabled on the volume. Omit if no backup_policy is specified."
  type        = bool
  default     = false
}

variable "volume_large_capacity_flag" {
  description = "This flag indicates if the volume is of large capacity or not. Default is false."
  type        = bool
  default     = false
}

variable "volume_snapshot_directory_flag" {
  description = "If enabled, a NFS volume will contain a read-only snapshot directory which provides access to each of the volume's snapshots. Will enable 'Previous Versions' support for SMB."
  type        = bool
  default     = false
}
################################################################




#variable "local_mount" {
#  description = "Mountpoint for this NFS compute instance"
#  type        = string
#  default     = "/mnt"
#}

