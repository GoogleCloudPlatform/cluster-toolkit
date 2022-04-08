locals {

  partition_nodes = [
    {
      # Group Definition
      group_name    = "test"
      count_dynamic = var.count_dynamic
      count_static  = var.count_static
      node_conf     = {}

      # Template By Definition
      additional_disks       = []
      can_ip_forward         = false
      disable_smt            = false
      disk_auto_delete       = true
      disk_labels            = var.labels
      disk_size_gb           = var.disk_size_gb
      disk_type              = var.disk_type
      enable_confidential_vm = false
      enable_oslogin         = true
      enable_shielded_vm     = false
      gpu                    = var.gpu
      labels                 = {}
      machine_type           = var.machine_type
      metadata               = {}
      min_cpu_platform       = var.min_cpu_platform
      on_host_maintenance    = null
      preemptible            = var.preemptible
      service_account = {
        email = "default"
        scopes = [
          "https://www.googleapis.com/auth/cloud-platform",
        ]
      }
      shielded_instance_config = null
      source_image_family      = null
      source_image_project     = var.image_project
      source_image             = var.source_image
      tags                     = []

      # Template By Source
      instance_template = null
    },
  ]
}


module "slurm_partition" {
  source = "git::https://gitlab.com/SchedMD/slurm-gcp.git//terraform/modules/slurm_partition?ref=dev-v5"

  slurm_cluster_name      = var.slurm_cluster_name
  partition_nodes         = local.partition_nodes
  enable_job_exclusive    = var.exclusive
  enable_placement_groups = var.enable_placement
  network_storage         = var.network_storage
  partition_name          = var.partition_name
  project_id              = var.project_id
  region                  = var.region
  slurm_cluster_id        = "placeholder"
  subnetwork              = var.subnetwork_self_link
  partition_conf = {
    Default = "YES"
  }
}

