locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "nfs", ghpc_role = "gcnv" })
}

resource "random_id" "resource_name_suffix" {
  byte_length = 3
}

locals {
  # id format: https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_network#id
  split_network_id = split("/", var.network_id)
  network_name     = local.split_network_id[4]
  network_project  = local.split_network_id[1]
  shared_vpc       = local.network_project != var.project_id

  export_policy_rules = {
    rules = {
      allowed_clients = "0.0.0.0/0"
      access_type     = "READ_WRITE"
      nfsv3           = true
      nfsv4           = false
      has_root_access = true
    }
  }
}

module "netapp_volumes" {
  source  = "GoogleCloudPlatform/netapp-volumes/google"
  version = "~> 2.0"

  project_id = var.project_id
  location   = var.region

  storage_pool = {
    create_pool        = true
    name               = var.storage_pool_name != null ? var.storage_pool_name : "${var.deployment_name}-${random_id.resource_name_suffix.hex}"
    size               = var.storage_pool_size
    service_level      = var.storage_pool_service_level
    ldap_enabled       = var.storage_pool_ldap_enabled
    network_name       = local.shared_vpc ? var.network_id : local.network_name
    allow_auto_tiering = var.storage_pool_allow_auto_tiering
    labels             = local.labels
  }

  storage_volumes = [
    # 1st volume
    {
      #name            = var.volume_name != null ? var.volume_name : "${var.deployment_name}-${random_id.resource_name_suffix.hex}-volume"
      name                     = var.volume_name
      share_name               = var.volume_share_name
      size                     = var.volume_capacity_gib
      protocols                = var.volume_protocol
      large_capacity           = var.volume_large_capacity_flag
      deletion_policy          = var.volume_deletion_policy
      scheduled_backup_enabled = var.volume_scheduled_backup_enabled_flag
      snapshot_directory       = var.volume_snapshot_directory_flag
      snapshot_policy = {
        enabled = var.snapshot_policy_enabled
        daily_schedule = {
          snapshots_to_keep = var.daily_schedule.snapshots_to_keep
          minute            = var.daily_schedule.minute
          hour              = var.daily_schedule.hour
        }
      }
      export_policy_rules     = local.export_policy_rules

    },
  ]
}
