locals {

  partition_nodes = [
    {
      # Group Definition
      group_name    = "test"
      count_dynamic = 10
      count_static  = 0
      node_conf     = {}

      # Template By Definition
      additional_disks       = []
      can_ip_forward         = false
      disable_smt            = false
      disk_auto_delete       = true
      disk_labels            = {}
      disk_size_gb           = 32
      disk_type              = "pd-standard"
      enable_confidential_vm = false
      enable_oslogin         = true
      enable_shielded_vm     = false
      gpu = {
        count = 1
        type  = "nvidia-tesla-v100"
      }
      labels              = {}
      machine_type        = "c2-standard-4"
      metadata            = {}
      min_cpu_platform    = null
      on_host_maintenance = null
      preemptible         = false
      service_account = {
        email = "default"
        scopes = [
          "https://www.googleapis.com/auth/cloud-platform",
        ]
      }
      shielded_instance_config = null
      source_image_family      = null
      source_image_project     = "hpc-toolkit-dev"
      source_image             = "schedmd-v5-slurm-21-08-4-hpc-centos-7-1648163377"
      tags                     = []

      # Template By Source
      instance_template = null
    },
  ]



}


module "slurm_partition" {
  source = "git::https://gitlab.com/SchedMD/slurm-gcp.git//terraform/modules/slurm_partition?ref=dev-v5"

  # TODO: this next one does not like '-'
  slurm_cluster_name      = var.deployment_name
  partition_nodes         = local.partition_nodes
  enable_job_exclusive    = var.exclusive
  enable_placement_groups = var.enable_placement
  network_storage         = var.network_storage
  partition_name          = var.partition_name
  project_id              = var.project_id
  region                  = var.region
  slurm_cluster_id        = "placeholder"
  subnetwork              = "default"
}

