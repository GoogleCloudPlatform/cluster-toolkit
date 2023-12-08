/**
 * Copyright 2023 Google LLC
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

# Most variables have been sourced and modified from the SchedMD/slurm-gcp
# github repository: https://github.com/SchedMD/slurm-gcp/tree/5.9.1

variable "access_config" {
  description = "Access configurations, i.e. IPs via which the VM instance can be accessed via the Internet."
  type = list(object({
    nat_ip       = string
    network_tier = string
  }))
  default = []
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

variable "can_ip_forward" {
  type        = bool
  description = "Enable IP forwarding, for NAT instances for example."
  default     = false
}

variable "cloud_parameters" {
  description = "cloud.conf options."
  type = object({
    no_comma_params = bool
    resume_rate     = number
    resume_timeout  = number
    suspend_rate    = number
    suspend_timeout = number
  })
  default = {
    no_comma_params = false
    resume_rate     = 0
    resume_timeout  = 300
    suspend_rate    = 0
    suspend_timeout = 300
  }
}

variable "cloudsql" {
  description = <<EOD
Use this database instead of the one on the controller.
  server_ip : Address of the database server.
  user      : The user to access the database as.
  password  : The password, given the user, to access the given database. (sensitive)
  db_name   : The database to access.
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

variable "compute_startup_script" {
  description = "Startup script used by the compute VMs."
  type        = string
  default     = ""
}

variable "compute_startup_scripts_timeout" {
  description = <<-EOD
    The timeout (seconds) applied to the compute_startup_script. If
    any script exceeds this timeout, then the instance setup process is considered
    failed and handled accordingly.

    NOTE: When set to 0, the timeout is considered infinite and thus disabled.
    EOD
  type        = number
  default     = 300
}

variable "controller_startup_script" {
  description = "Startup script used by the controller VM."
  type        = string
  default     = ""
}

variable "controller_startup_scripts_timeout" {
  description = <<-EOD
    The timeout (seconds) applied to the controller_startup_script. If
    any script exceeds this timeout, then the instance setup process is considered
    failed and handled accordingly.

    NOTE: When set to 0, the timeout is considered infinite and thus disabled.
    EOD
  type        = number
  default     = 300
}

variable "login_startup_scripts_timeout" {
  description = <<-EOD
    The timeout (seconds) applied to the login startup script. If
    any script exceeds this timeout, then the instance setup process is considered
    failed and handled accordingly.

    NOTE: When set to 0, the timeout is considered infinite and thus disabled.
    EOD
  type        = number
  default     = 300
}

variable "cgroup_conf_tpl" {
  type        = string
  description = "Slurm cgroup.conf template file path."
  default     = null
}

variable "deployment_name" {
  description = "Name of the deployment."
  type        = string
}

variable "disable_controller_public_ips" {
  description = "If set to false. The controller will have a random public IP assigned to it. Ignored if access_config is set."
  type        = bool
  default     = true
}

variable "disable_default_mounts" {
  description = <<-EOD
    Disable default global network storage from the controller
    - /usr/local/etc/slurm
    - /etc/munge
    - /home
    - /apps
    Warning: If these are disabled, the slurm etc and munge dirs must be added
    manually, or some other mechanism must be used to synchronize the slurm conf
    files and the munge key across the cluster.
    EOD
  type        = bool
  default     = false
}

variable "disable_smt" {
  type        = bool
  description = "Disables Simultaneous Multi-Threading (SMT) on instance."
  default     = true
}

variable "disk_type" {
  type        = string
  description = "Boot disk type, can be either pd-ssd, pd-standard, pd-balanced, or pd-extreme."
  default     = "pd-ssd"

  validation {
    condition     = contains(["pd-ssd", "pd-standard", "pd-balanced", "pd-extreme"], var.disk_type)
    error_message = "Variable disk_type must be one of pd-ssd, pd-standard, pd-balanced, or pd-extreme."
  }
}

variable "disk_size_gb" {
  type        = number
  description = "Boot disk size in GB."
  default     = 50
}

variable "disk_auto_delete" {
  type        = bool
  description = "Whether or not the boot disk should be auto-deleted."
  default     = true
}

variable "disk_labels" {
  description = "Labels specific to the boot disk. These will be merged with var.labels."
  type        = map(string)
  default     = {}
}

variable "enable_devel" {
  type        = bool
  description = "Enables development mode. Not for production use."
  default     = false
}

variable "enable_cleanup_compute" {
  description = <<-EOD
    Enables automatic cleanup of compute nodes and resource policies (e.g.
    placement groups) managed by this module, when cluster is destroyed.

    NOTE: Requires Python and pip packages listed at the following link:
    https://github.com/SchedMD/slurm-gcp/blob/3979e81fc5e4f021b5533a23baa474490f4f3614/scripts/requirements.txt

    *WARNING*: Toggling this may impact the running workload. Deployed compute nodes
    may be destroyed and their jobs will be requeued.
    EOD
  type        = bool
  default     = false
}

variable "enable_cleanup_subscriptions" {
  description = <<-EOD
    Enables automatic cleanup of pub/sub subscriptions managed by this module, when
    cluster is destroyed.

    NOTE: Requires Python and pip packages listed at the following link:
    https://github.com/SchedMD/slurm-gcp/blob/3979e81fc5e4f021b5533a23baa474490f4f3614/scripts/requirements.txt

    *WARNING*: Toggling this may temporarily impact var.enable_reconfigure behavior.
    EOD
  type        = bool
  default     = false
}

variable "enable_reconfigure" {
  description = <<-EOD
    Enables automatic Slurm reconfiguration when Slurm configuration changes (e.g.
    slurm.conf.tpl, partition details). Compute instances and resource policies
    (e.g. placement groups) will be destroyed to align with new configuration.
    NOTE: Requires Python and Google Pub/Sub API.
    *WARNING*: Toggling this will impact the running workload. Deployed compute nodes
    will be destroyed and their jobs will be requeued.
    EOD
  type        = bool
  default     = false
}

variable "enable_bigquery_load" {
  description = "Enable loading of cluster job usage into big query."
  type        = bool
  default     = false
}

variable "enable_slurm_gcp_plugins" {
  description = <<EOD
Enables calling hooks in scripts/slurm_gcp_plugins during cluster resume and suspend.
EOD
  type        = bool
  default     = false
}

variable "enable_oslogin" {
  type        = bool
  description = <<-EOD
    Enables Google Cloud os-login for user login and authentication for VMs.
    See https://cloud.google.com/compute/docs/oslogin
    EOD
  default     = true
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

variable "gpu" {
  type = object({
    type  = string
    count = number
  })
  description = "DEPRECATED: use var.guest_accelerator"
  default     = null
  validation {
    condition     = var.gpu == null
    error_message = "var.gpu is deprecated. Use var.guest_accelerator."
  }
}

variable "guest_accelerator" {
  description = "List of the type and count of accelerator cards attached to the instance."
  type = list(object({
    type  = string,
    count = number
  }))
  default  = []
  nullable = false

  validation {
    condition     = length(var.guest_accelerator) <= 1
    error_message = "The Slurm modules supports 0 or 1 models of accelerator card on each node."
  }
}

variable "labels" {
  type        = map(string)
  description = "Labels, provided as a map."
  default     = {}
}

variable "machine_type" {
  type        = string
  description = "Machine type to create."
  default     = "c2-standard-4"
}

variable "metadata" {
  type        = map(string)
  description = "Metadata, provided as a map."
  default     = {}
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

variable "network_ip" {
  type        = string
  description = "DEPRECATED: Use `static_ips` variable to assign an internal static ip address."
  default     = null
  validation {
    condition     = var.network_ip == null
    error_message = "network_ip is deprecated. Use static_ips to assign an internal static ip address."
  }
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured on all instances."
  type = list(object({
    server_ip             = string,
    remote_mount          = string,
    local_mount           = string,
    fs_type               = string,
    mount_options         = string,
    client_install_runner = map(string)
    mount_runner          = map(string)
  }))
  default = []
}

variable "on_host_maintenance" {
  type        = string
  description = "Instance availability Policy."
  default     = "MIGRATE"
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
      partition_conf    = map(string)
      partition_feature = string
      partition_name    = string
      partition_nodes = map(object({
        access_config = list(object({
          network_tier = string
        }))
        bandwidth_tier         = string
        node_count_dynamic_max = number
        node_count_static      = number
        enable_spot_vm         = bool
        group_name             = string
        instance_template      = string
        node_conf              = map(string)
        reservation_name       = string
        spot_instance_config = object({
          termination_action = string
        })
      }))
      partition_startup_scripts_timeout = number
      subnetwork                        = string
      zone_policy_allow                 = list(string)
      zone_policy_deny                  = list(string)
      zone_target_shape                 = string
    })
  }))
  default = []

  validation {
    condition = alltrue([
      for x in var.partition[*].partition : can(regex("(^[a-z][a-z0-9]*$)", x.partition_name))
    ])
    error_message = "Item 'partition_name' must be alphanumeric and begin with a letter. regex: '(^[a-z][a-z0-9]*$)'."
  }
}

variable "preemptible" {
  type        = bool
  description = "Allow the instance to be preempted."
  default     = false
}

variable "project_id" {
  type        = string
  description = "Project ID to create resources in."
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

variable "region" {
  type        = string
  description = "Region where the instances should be created."
  default     = null
}

variable "service_account" {
  type = object({
    email  = string
    scopes = set(string)
  })
  description = <<-EOD
    Service account to attach to the controller instance. If not set, the
    default compute service account for the given project will be used with the
    "https://www.googleapis.com/auth/cloud-platform" scope.
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
  enable_integrity_monitoring : Compare the most recent boot measurements to the
  integrity policy baseline and return a pair of pass/fail results depending on
  whether they match or not.
  enable_secure_boot : Verify the digital signature of all boot components, and
  halt the boot process if signature verification fails.
  enable_vtpm : Use a virtualized trusted platform module, which is a
  specialized computer chip you can use to encrypt objects like keys and
  certificates.
EOD
  default = {
    enable_integrity_monitoring = true
    enable_secure_boot          = true
    enable_vtpm                 = true
  }
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming and slurm accounting. If not provided it will default to the first 8 characters of the deployment name (removing any invalid characters)."
  default     = null
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


variable "instance_template" {
  description = <<-EOD
    Self link to a custom instance template. If set, other VM definition
    variables such as machine_type and instance_image will be ignored in favor
    of the provided instance template.

    For more information on creating custom images for the instance template
    that comply with Slurm on GCP see the "Slurm on GCP Custom Images" section
    in docs/vm-images.md.
    EOD
  type        = string
  default     = null
}

variable "instance_image" {
  description = <<-EOD
    Defines the image that will be used in the Slurm controller VM instance.

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.

    For more information on creating custom images that comply with Slurm on GCP
    see the "Slurm on GCP Custom Images" section in docs/vm-images.md.
    EOD
  type        = map(string)
  default = {
    project = "schedmd-slurm-public"
    family  = "slurm-gcp-5-9-hpc-centos-7"
  }

  validation {
    condition     = can(coalesce(var.instance_image.project))
    error_message = "In var.instance_image, the \"project\" field must be a string set to the Cloud project ID."
  }

  validation {
    condition     = can(coalesce(var.instance_image.name)) != can(coalesce(var.instance_image.family))
    error_message = "In var.instance_image, exactly one of \"family\" or \"name\" fields must be set to desired image family or name."
  }
}

variable "instance_image_custom" {
  description = <<-EOD
    A flag that designates that the user is aware that they are requesting
    to use a custom and potentially incompatible image for this Slurm on
    GCP module.

    If the field is set to false, only the compatible families and project
    names will be accepted.  The deployment will fail with any other image
    family or name.  If set to true, no checks will be done.

    See: https://goo.gle/hpc-slurm-images
    EOD
  type        = bool
  default     = false
}

variable "source_image_project" {
  type        = string
  description = "DEPRECATED: Use `instance_image` instead."
  default     = null
  validation {
    condition     = var.source_image_project == null
    error_message = "Variable `source_image_project` is deprecated. Use `instance_image` instead."
  }
}

variable "source_image_family" {
  type        = string
  description = "DEPRECATED: Use `instance_image` instead."
  default     = null
  validation {
    condition     = var.source_image_family == null
    error_message = "Variable `source_image_family` is deprecated. Use `instance_image` instead."
  }
}

variable "source_image" {
  type        = string
  description = "DEPRECATED: Use `instance_image` instead."
  default     = null
  validation {
    condition     = var.source_image == null
    error_message = "Variable `source_image` is deprecated. Use `instance_image` instead."
  }
}

variable "static_ips" {
  type        = list(string)
  description = "List of static IPs for VM instances."
  default     = []
}

variable "network_self_link" {
  type        = string
  description = "Network to deploy to. Either network_self_link or subnetwork_self_link must be specified."
  default     = null
}

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to. Either network_self_link or subnetwork_self_link must be specified."
  default     = null
}

variable "subnetwork_project" {
  type        = string
  description = "The project that subnetwork belongs to."
  default     = null
}

variable "tags" {
  type        = list(string)
  description = "Network tag list."
  default     = []
}

variable "zone" {
  type        = string
  description = <<EOD
Zone where the instances should be created. If not specified, instances will be
spread across available zones in the region.
EOD
  default     = null
}
