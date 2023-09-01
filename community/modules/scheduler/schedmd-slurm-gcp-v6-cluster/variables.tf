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


variable "project_id" {
  type        = string
  description = "Project ID to create resources in."
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming and slurm accounting."

  validation {
    condition     = can(regex("^[a-z](?:[a-z0-9]{0,9})$", var.slurm_cluster_name))
    error_message = "Variable 'slurm_cluster_name' must be a match of regex '^[a-z](?:[a-z0-9]{0,9})$'."
  }
}

variable "region" {
  type        = string
  description = "The default region to place resources in."
}

variable "create_bucket" {
  description = <<-EOD
    Create GCS bucket instead of using an existing one.
  EOD
  type        = bool
  default     = true
}

variable "bucket_name" {
  description = <<-EOD
    Name of GCS bucket.
    Ignored when 'create_bucket' is true.
  EOD
  type        = string
  default     = null
}

variable "bucket_dir" {
  description = "Bucket directory for cluster files to be put into. If not specified, then one will be chosen based on slurm_cluster_name."
  type        = string
  default     = null
}

variable "controller_instance_config" {
  description = <<EOD
Creates a controller instance with given configuration.
EOD
  type = object({
    additional_disks = optional(list(object({
      disk_name    = optional(string)
      device_name  = optional(string)
      disk_size_gb = optional(number)
      disk_type    = optional(string)
      disk_labels  = optional(map(string), {})
      auto_delete  = optional(bool, true)
      boot         = optional(bool, false)
    })), [])
    bandwidth_tier         = optional(string, "platform_default")
    can_ip_forward         = optional(bool, false)
    disable_smt            = optional(bool, false)
    disk_auto_delete       = optional(bool, true)
    disk_labels            = optional(map(string), {})
    disk_size_gb           = optional(number)
    disk_type              = optional(string, "n1-standard-1")
    enable_confidential_vm = optional(bool, false)
    enable_public_ip       = optional(bool, false)
    enable_oslogin         = optional(bool, true)
    enable_shielded_vm     = optional(bool, false)
    gpu = optional(object({
      count = number
      type  = string
    }))
    instance_template   = optional(string)
    labels              = optional(map(string), {})
    machine_type        = optional(string)
    metadata            = optional(map(string), {})
    min_cpu_platform    = optional(string)
    network_ip          = optional(string)
    network_tier        = optional(string, "STANDARD")
    on_host_maintenance = optional(string)
    preemptible         = optional(bool, false)
    region              = optional(string)
    service_account = optional(object({
      email  = optional(string)
      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])
    }))
    shielded_instance_config = optional(object({
      enable_integrity_monitoring = optional(bool, true)
      enable_secure_boot          = optional(bool, true)
      enable_vtpm                 = optional(bool, true)
    }))
    source_image_family  = optional(string)
    source_image_project = optional(string)
    source_image         = optional(string)
    spot                 = optional(bool, false)
    static_ip            = optional(string)
    subnetwork_project   = optional(string)
    subnetwork           = optional(string)
    tags                 = optional(list(string), [])
    termination_action   = optional(string)
    zone                 = optional(string)
  })
  default = {}
}

variable "enable_hybrid" {
  description = <<EOD
Enables use of hybrid controller mode. When true, controller_hybrid_config will
be used instead of controller_instance_config and will disable login instances.
EOD
  type        = bool
  default     = false
}

variable "controller_hybrid_config" {
  description = <<EOD
Creates a hybrid controller with given configuration.
See 'main.tf' for valid keys.
EOD
  type = object({
    google_app_cred_path    = optional(string)
    slurm_control_host      = optional(string)
    slurm_control_host_port = optional(string)
    slurm_control_addr      = optional(string)
    slurm_bin_dir           = optional(string)
    slurm_log_dir           = optional(string)
    output_dir              = optional(string)
    install_dir             = optional(string)
    munge_mount = optional(object({
      server_ip     = optional(string)
      remote_mount  = optional(string, "/etc/munge")
      fs_type       = optional(string, "nfs")
      mount_options = optional(string)
    }), {})
  })
  default = {}
}

variable "enable_login" {
  description = <<EOD
Enables the creation of login nodes and instance templates.
EOD
  type        = bool
  default     = true
}

variable "login_nodes" {
  description = "List of slurm login instance definitions."
  type = list(object({
    additional_disks = optional(list(object({
      disk_name    = optional(string)
      device_name  = optional(string)
      disk_size_gb = optional(number)
      disk_type    = optional(string)
      disk_labels  = optional(map(string), {})
      auto_delete  = optional(bool, true)
      boot         = optional(bool, false)
    })), [])
    bandwidth_tier         = optional(string, "platform_default")
    can_ip_forward         = optional(bool, false)
    disable_smt            = optional(bool, false)
    disk_auto_delete       = optional(bool, true)
    disk_labels            = optional(map(string), {})
    disk_size_gb           = optional(number)
    disk_type              = optional(string, "n1-standard-1")
    enable_confidential_vm = optional(bool, false)
    enable_public_ip       = optional(bool, false)
    enable_oslogin         = optional(bool, true)
    enable_shielded_vm     = optional(bool, false)
    gpu = optional(object({
      count = number
      type  = string
    }))
    group_name          = string
    instance_template   = optional(string)
    labels              = optional(map(string), {})
    machine_type        = optional(string)
    metadata            = optional(map(string), {})
    min_cpu_platform    = optional(string)
    network_tier        = optional(string, "STANDARD")
    num_instances       = optional(number, 1)
    on_host_maintenance = optional(string)
    preemptible         = optional(bool, false)
    region              = optional(string)
    service_account = optional(object({
      email  = optional(string)
      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])
    }))
    shielded_instance_config = optional(object({
      enable_integrity_monitoring = optional(bool, true)
      enable_secure_boot          = optional(bool, true)
      enable_vtpm                 = optional(bool, true)
    }))
    source_image_family  = optional(string)
    source_image_project = optional(string)
    source_image         = optional(string)
    static_ips           = optional(list(string), [])
    subnetwork_project   = optional(string)
    subnetwork           = optional(string)
    spot                 = optional(bool, false)
    tags                 = optional(list(string), [])
    zone                 = optional(string)
    termination_action   = optional(string)
  }))
  default = []
}

variable "nodeset" {
  description = "Define nodesets, as a list."
  type = list(object({
    node_count_static      = optional(number, 0)
    node_count_dynamic_max = optional(number, 1)
    node_conf              = optional(map(string), {})
    nodeset_name           = string
    additional_disks = optional(list(object({
      disk_name    = optional(string)
      device_name  = optional(string)
      disk_size_gb = optional(number)
      disk_type    = optional(string)
      disk_labels  = optional(map(string), {})
      auto_delete  = optional(bool, true)
      boot         = optional(bool, false)
    })), [])
    bandwidth_tier         = optional(string, "platform_default")
    can_ip_forward         = optional(bool, false)
    disable_smt            = optional(bool, false)
    disk_auto_delete       = optional(bool, true)
    disk_labels            = optional(map(string), {})
    disk_size_gb           = optional(number)
    disk_type              = optional(string)
    enable_confidential_vm = optional(bool, false)
    enable_placement       = optional(bool, false)
    enable_public_ip       = optional(bool, false)
    enable_oslogin         = optional(bool, true)
    enable_shielded_vm     = optional(bool, false)
    gpu = optional(object({
      count = number
      type  = string
    }))
    instance_template   = optional(string)
    labels              = optional(map(string), {})
    machine_type        = optional(string)
    metadata            = optional(map(string), {})
    min_cpu_platform    = optional(string)
    network_tier        = optional(string, "STANDARD")
    on_host_maintenance = optional(string)
    preemptible         = optional(bool, false)
    region              = optional(string)
    service_account = optional(object({
      email  = optional(string)
      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])
    }))
    shielded_instance_config = optional(object({
      enable_integrity_monitoring = optional(bool, true)
      enable_secure_boot          = optional(bool, true)
      enable_vtpm                 = optional(bool, true)
    }))
    source_image_family  = optional(string)
    source_image_project = optional(string)
    source_image         = optional(string)
    subnetwork_project   = optional(string)
    subnetwork           = optional(string)
    spot                 = optional(bool, false)
    tags                 = optional(list(string), [])
    termination_action   = optional(string)
    zones                = optional(list(string), [])
    zone_target_shape    = optional(string, "ANY_SINGLE_ZONE")
  }))
  default = []

  validation {
    condition     = length(distinct([for x in var.nodeset : x.nodeset_name])) == length(var.nodeset)
    error_message = "All nodesets must have a unique name."
  }
}

variable "nodeset_dyn" {
  description = "Defines nodesets (dynamic), as a list."
  type = list(object({
    nodeset_name    = string
    nodeset_feature = string
  }))
  default = []

  validation {
    condition     = length(distinct([for x in var.nodeset_dyn : x.nodeset_name])) == length(var.nodeset_dyn)
    error_message = "All nodesets must have a unique name."
  }
}

variable "nodeset_tpu" {
  description = "Define TPU nodesets, as a list."
  type = list(object({
    node_count_static      = optional(number, 0)
    node_count_dynamic_max = optional(number, 1)
    nodeset_name           = string
    enable_public_ip       = optional(bool, false)
    node_type              = string
    accelerator_config = optional(object({
      topology = string
      version  = string
      }), {
      topology = ""
      version  = ""
    })
    tf_version   = string
    preemptible  = optional(bool, false)
    preserve_tpu = optional(bool, true)
    zone         = string
    data_disks   = optional(list(string), [])
    docker_image = optional(string, "")
    subnetwork   = optional(string, "")
    service_account = optional(object({
      email  = optional(string)
      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])
    }))
  }))
  default = []

  validation {
    condition     = length(distinct([for x in var.nodeset_tpu : x.nodeset_name])) == length(var.nodeset_tpu)
    error_message = "All TPU nodesets must have a unique name."
  }
}

variable "partitions" {
  description = "Cluster partitions as a list."
  type = list(object({
    default              = optional(bool, false)
    enable_job_exclusive = optional(bool, false)
    network_storage = optional(list(object({
      server_ip     = string
      remote_mount  = string
      local_mount   = string
      fs_type       = string
      mount_options = string
    })), [])
    partition_conf        = optional(map(string), {})
    partition_name        = string
    partition_nodeset     = optional(list(string), [])
    partition_nodeset_dyn = optional(list(string), [])
    partition_nodeset_tpu = optional(list(string), [])
    resume_timeout        = optional(number)
    suspend_time          = optional(number, 300)
    suspend_timeout       = optional(number)
  }))

  validation {
    condition     = length(var.partitions) > 0
    error_message = "Partitions cannot be empty."
  }
}

variable "enable_devel" {
  type        = bool
  description = "Enables development mode. Not for production use."
  default     = false
}

variable "enable_cleanup_compute" {
  description = <<EOD
Enables automatic cleanup of compute nodes and resource policies (e.g.
placement groups) managed by this module, when cluster is destroyed.

NOTE: Requires Python and script dependencies.

*WARNING*: Toggling this may impact the running workload. Deployed compute nodes
may be destroyed and their jobs will be requeued.
EOD
  type        = bool
  default     = false
}

variable "enable_bigquery_load" {
  description = <<EOD
Enables loading of cluster job usage into big query.

NOTE: Requires Google Bigquery API.
EOD
  type        = bool
  default     = false
}

variable "cloud_parameters" {
  description = "cloud.conf options."
  type = object({
    no_comma_params = optional(bool, false)
    resume_rate     = optional(number, 0)
    resume_timeout  = optional(number, 300)
    suspend_rate    = optional(number, 0)
    suspend_timeout = optional(number, 300)
  })
  default = {}
}

variable "disable_default_mounts" {
  description = <<-EOD
    Disable default global network storage from the controller
    * /usr/local/etc/slurm
    * /etc/munge
    * /home
    * /apps
    If these are disabled, the slurm etc and munge dirs must be added manually,
    or some other mechanism must be used to synchronize the slurm conf files
    and the munge key across the cluster.
    EOD
  type        = bool
  default     = false
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured on all instances."
  type = list(object({
    server_ip             = string
    remote_mount          = string
    local_mount           = string
    fs_type               = string
    mount_options         = string
    client_install_runner = map(string)
    mount_runner          = map(string)

  }))
  default = []
}

variable "login_network_storage" {
  description = "Storage to mounted on login and controller instances."
  type = list(object({
    server_ip     = string
    remote_mount  = string
    local_mount   = string
    fs_type       = string
    mount_options = string
  }))
  default = []
}

variable "slurmdbd_conf_tpl" {
  description = "Slurm slurmdbd.conf template file path."
  type        = string
  default     = null
}

variable "slurm_conf_tpl" {
  description = "Slurm slurm.conf template file path."
  type        = string
  default     = null
}

variable "cgroup_conf_tpl" {
  description = "Slurm cgroup.conf template file path."
  type        = string
  default     = null
}

variable "controller_startup_scripts" {
  description = "List of scripts to be ran on controller VM startup."
  type = list(object({
    filename = string
    content  = string
  }))
  default = []
}

variable "controller_startup_scripts_timeout" {
  description = <<EOD
The timeout (seconds) applied to each script in controller_startup_scripts. If
any script exceeds this timeout, then the instance setup process is considered
failed and handled accordingly.

NOTE: When set to 0, the timeout is considered infinite and thus disabled.
EOD
  type        = number
  default     = 300
}

variable "login_startup_scripts" {
  description = "List of scripts to be ran on login VM startup."
  type = list(object({
    filename = string
    content  = string
  }))
  default = []
}

variable "login_startup_scripts_timeout" {
  description = <<EOD
The timeout (seconds) applied to each script in login_startup_scripts. If
any script exceeds this timeout, then the instance setup process is considered
failed and handled accordingly.

NOTE: When set to 0, the timeout is considered infinite and thus disabled.
EOD
  type        = number
  default     = 300
}

variable "compute_startup_scripts" {
  description = "List of scripts to be ran on compute VM startup."
  type = list(object({
    filename = string
    content  = string
  }))
  default = []
}

variable "compute_startup_scripts_timeout" {
  description = <<EOD
The timeout (seconds) applied to each script in compute_startup_scripts. If
any script exceeds this timeout, then the instance setup process is considered
failed and handled accordingly.

NOTE: When set to 0, the timeout is considered infinite and thus disabled.
EOD
  type        = number
  default     = 300
}

variable "prolog_scripts" {
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

variable "epilog_scripts" {
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

variable "cloudsql" {
  description = "Use this database instead of one on controller."
  type = object({
    server_ip = string
    user      = string
    password  = string # sensitive
    db_name   = string
  })
  default   = null
  sensitive = true
}
