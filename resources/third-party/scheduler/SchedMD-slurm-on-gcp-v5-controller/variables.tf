#
# Copyright 2019 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

###########
# GENERAL #
###########

variable "project_id" {
  type        = string
  description = "Project ID to create resources in."
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming and slurm accounting."

  validation {
    condition     = can(regex("(^[a-z][a-z0-9]*$)", var.slurm_cluster_name))
    error_message = "Variable 'slurm_cluster_name' must be a match of regex '(^[a-z][a-z0-9]*$)'."
  }
}

variable "on_host_maintenance" {
  type        = string
  description = "Instance availability Policy"
  default     = "MIGRATE"
}

variable "labels" {
  type        = map(string)
  description = "Labels, provided as a map"
  default     = {}
}

variable "enable_oslogin" {
  type        = bool
  description = <<EOD
Enables Google Cloud os-login for user login and authentication for VMs.
See https://cloud.google.com/compute/docs/oslogin
EOD
  default     = true
}


###########
# NETWORK #
###########
variable "can_ip_forward" {
  type        = bool
  description = "Enable IP forwarding, for NAT instances for example."
  default     = false
}


variable "network" {
  type        = string
  description = "Network to deploy to. Only one of network or subnetwork should be specified."
  default     = ""
}

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to. Only one of network or subnetwork should be specified."
  default     = ""
}

variable "subnetwork_project" {
  type        = string
  description = "The project that subnetwork belongs to."
  default     = ""
}

variable "region" {
  type        = string
  description = "Region where the instances should be created."
  default     = null
}

variable "network_ip" {
  type        = string
  description = "Private IP address to assign to the instance if desired."
  default     = ""
}



############
# INSTANCE #
############

variable "static_ips" {
  type        = list(string)
  description = "List of static IPs for VM instances."
  default     = []
}

variable "access_config" {
  description = "Access configurations, i.e. IPs via which the VM instance can be accessed via the Internet."
  type = list(object({
    nat_ip       = string
    network_tier = string
  }))
  default = []
}

variable "zone" {
  type        = string
  description = <<EOD
Zone where the instances should be created. If not specified, instances will be
spread across available zones in the region.
EOD
  default     = null
}

variable "metadata" {
  type        = map(string)
  description = "Metadata, provided as a map"
  default     = {}
}


variable "tags" {
  type        = list(string)
  description = "Network tag list."
  default     = []
}


#########
# SLURM #
#########

variable "disable_smt" {
  type        = bool
  description = "Disables Simultaneous Multi-Threading (SMT) on instance."
  default     = false
}


variable "slurm_cluster_id" {
  type        = string
  description = "The Cluster ID to use. If 'null' or '', then an ID will be generated."
  default     = null
}

variable "enable_devel" {
  type        = bool
  description = "Enables development mode. Not for production use."
  default     = false
}

variable "enable_bigquery_load" {
  description = "Enable loading of cluster job usage into big query"
  type        = bool
  default     = false
}

variable "slurmdbd_conf_tpl" {
  type        = string
  description = "Slurm slurmdbd.conf template file path."
  default     = null
}

variable "slurm_conf_tpl" {
  type        = string
  description = "Slurm slurm.conf template file path."
  default     = null
}

variable "cgroup_conf_tpl" {
  type        = string
  description = "Slurm cgroup.conf template file path."
  default     = null
}

variable "cloudsql" {
  description = <<EOD
Use this database instead of the one on the controller.
* server_ip : Address of the database server.
* user      : The user to access the database as.
* password  : The password, given the user, to access the given database. (sensitive)
* db_name   : The database to access.
EOD
  type = object({
    server_ip = string
    user      = string
    password  = string # sensitive
    db_name   = string
  })
  default   = null
  sensitive = true
}

variable "controller_d" {
  description = "List of scripts to be ran on controller VM startup."
  type = list(object({
    filename = string
    content  = string
  }))
  default = []
}

variable "compute_d" {
  description = "List of scripts to be ran on compute VM startup."
  type = list(object({
    filename = string
    content  = string
  }))
  default = []
}

variable "prolog_d" {
  description = <<EOD
List of scripts to be used for Prolog. Programs for the slurmd to execute
whenever it is asked to run a job step from a new job allocation.
See https://slurm.schedmd.com/slurm.conf.html#OPT_Prolog.
EOD
  type = list(object({
    filename = string
    content  = string
  }))
  default = []
}

variable "epilog_d" {
  description = <<EOD
List of scripts to be used for Epilog. Programs for the slurmd to execute
on every node when a user's job completes.
See https://slurm.schedmd.com/slurm.conf.html#OPT_Epilog.
EOD
  type = list(object({
    filename = string
    content  = string
  }))
  default = []
}

variable "network_storage" {
  description = <<EOD
Storage to mounted on all instances.
* server_ip     : Address of the storage server.
* remote_mount  : The location in the remote instance filesystem to mount from.
* local_mount   : The location on the instance filesystem to mount to.
* fs_type       : Filesystem type (e.g. "nfs").
* mount_options : Options to mount with.
EOD
  type = list(object({
    server_ip     = string
    remote_mount  = string
    local_mount   = string
    fs_type       = string
    mount_options = string
  }))
  default = []
}

variable "login_network_storage" {
  description = <<EOD
Storage to mounted on login and controller instances
* server_ip     : Address of the storage server.
* remote_mount  : The location in the remote instance filesystem to mount from.
* local_mount   : The location on the instance filesystem to mount to.
* fs_type       : Filesystem type (e.g. "nfs").
* mount_options : Options to mount with.
EOD
  type = list(object({
    server_ip     = string
    remote_mount  = string
    local_mount   = string
    fs_type       = string
    mount_options = string
  }))
  default = []
}

variable "partition" {
  description = "Cluster partitions as a list."
  type = list(object({
    compute_list = list(string)
    partition = object({
      enable_job_exclusive    = bool
      enable_placement_groups = bool
      network_storage = list(object({
        server_ip     = string
        remote_mount  = string
        local_mount   = string
        fs_type       = string
        mount_options = string
      }))
      partition_conf = map(string)
      partition_name = string
      partition_nodes = map(object({
        count_dynamic     = number
        count_static      = number
        group_name        = string
        instance_template = string
        node_conf         = map(string)
      }))
      subnetwork        = string
      zone_policy_allow = list(string)
      zone_policy_deny  = list(string)
    })
  }))
  default = []

  validation {
    condition = alltrue([
      for x in var.partition[*].partition : can(regex("(^[a-z][a-z0-9]*$)", x.partition_name))
    ])
    error_message = "Items 'partition_name' must be a match of regex '(^[a-z][a-z0-9]*$)'."
  }
}

variable "cloud_parameters" {
  description = "cloud.conf options."
  type = object({
    resume_rate     = number
    resume_timeout  = number
    suspend_rate    = number
    suspend_timeout = number
  })
  default = {
    resume_rate     = 0
    resume_timeout  = 300
    suspend_rate    = 0
    suspend_timeout = 300
  }
}

################
# SOURCE IMAGE #
################

variable "source_image_project" {
  type        = string
  description = "Project where the source image comes from. If it is not provided, the provider project is used."
  default     = ""
}

variable "source_image_family" {
  type        = string
  description = "Source image family."
  default     = ""
}

variable "source_image" {
  type        = string
  description = "Source disk image."
  default     = ""
}

############
# INSTANCE #
############

variable "machine_type" {
  type        = string
  description = "Machine type to create."
  default     = "n1-standard-1"
}

variable "min_cpu_platform" {
  type        = string
  description = <<EOD
Specifies a minimum CPU platform. Applicable values are the friendly names of
CPU platforms, such as Intel Haswell or Intel Skylake. See the complete list:
https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
EOD
  default     = null
}

variable "gpu" {
  type = object({
    type  = string
    count = number
  })
  description = <<EOD
GPU information. Type and count of GPU to attach to the instance template. See
https://cloud.google.com/compute/docs/gpus more details.
* type : the GPU type
* count : number of GPUs
EOD
  default     = null
}

variable "service_account" {
  type = object({
    email  = string
    scopes = set(string)
  })
  description = <<EOD
Service account to attach to the instances. See
'main.tf:local.service_account' for the default.
EOD
  default     = null
}

variable "shielded_instance_config" {
  type = object({
    enable_integrity_monitoring = bool
    enable_secure_boot          = bool
    enable_vtpm                 = bool
  })
  description = <<EOD
Shielded VM configuration for the instance. Note: not used unless
enable_shielded_vm is 'true'.
* enable_integrity_monitoring : Compare the most recent boot measurements to the
  integrity policy baseline and return a pair of pass/fail results depending on
  whether they match or not.
* enable_secure_boot : Verify the digital signature of all boot components, and
  halt the boot process if signature verification fails.
* enable_vtpm : Use a virtualized trusted platform module, which is a
  specialized computer chip you can use to encrypt objects like keys and
  certificates.
EOD
  default = {
    enable_integrity_monitoring = true
    enable_secure_boot          = true
    enable_vtpm                 = true
  }
}

variable "enable_confidential_vm" {
  type        = bool
  description = "Enable the Confidential VM configuration. Note: the instance image must support option."
  default     = false
}

variable "enable_shielded_vm" {
  type        = bool
  description = "Enable the Shielded VM configuration. Note: the instance image must support option."
  default     = false
}

variable "preemptible" {
  type        = bool
  description = "Allow the instance to be preempted."
  default     = false
}

########
# DISK #
########

variable "disk_type" {
  type        = string
  description = "Boot disk type, can be either pd-ssd, local-ssd, or pd-standard."
  default     = "pd-standard"
}

variable "disk_size_gb" {
  type        = number
  description = "Boot disk size in GB."
  default     = 100
}

variable "disk_labels" {
  type        = map(string)
  description = "Labels to be assigned to boot disk, provided as a map."
  default     = {}
}

variable "disk_auto_delete" {
  type        = bool
  description = "Whether or not the boot disk should be auto-deleted."
  default     = true
}

variable "additional_disks" {
  type = list(object({
    disk_name    = string
    device_name  = string
    disk_type    = string
    disk_size_gb = number
    disk_labels  = map(string)
    auto_delete  = bool
    boot         = bool
  }))
  description = "List of maps of disks."
  default     = []
}


