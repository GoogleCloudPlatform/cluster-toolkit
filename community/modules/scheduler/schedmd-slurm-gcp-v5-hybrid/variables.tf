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

variable "deployment_name" {
  description = "Name of the deployment."
  type        = string
}

variable "slurm_cluster_name" {
  type        = string
  description = <<-EOD
    Cluster name, used for resource naming and slurm accounting. If not provided
    it will default to the first 8 characters of the deployment name (removing
    any invalid characters).
    EOD
  default     = null
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
    NOTE: Requires Python and script dependencies.
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
    NOTE: Requires Python and script dependencies.
    *WARNING*: Toggling this may temporarily impact var.enable_reconfigure behavior.
    EOD
  type        = bool
  default     = false
}

variable "enable_reconfigure" {
  description = <<-EOD
    Enables automatic Slurm reconfigure on when Slurm configuration changes (e.g.
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
  description = <<-EOD
    Enables loading of cluster job usage into big query.
    NOTE: Requires Google Bigquery API.
    EOD
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

variable "slurm_control_host" {
  type        = string
  description = <<EOD
The short, or long, hostname of the machine where Slurm control daemon is
executed (i.e. the name returned by the command "hostname -s").
This value is passed to slurm.conf such that:
SlurmctldHost={var.slurm_control_host}\({var.slurm_control_addr}\)
See https://slurm.schedmd.com/slurm.conf.html#OPT_SlurmctldHost
EOD

  validation {
    condition     = (var.slurm_control_host != null && var.slurm_control_host != "")
    error_message = "Variable 'slurm_control_host' cannot be empty (\"\") or omitted (null)."
  }
}

variable "slurm_control_host_port" {
  type        = string
  description = <<EOD
The port number that the Slurm controller, slurmctld, listens to for work.
See https://slurm.schedmd.com/slurm.conf.html#OPT_SlurmctldPort
EOD
  default     = null

  validation {
    condition     = var.slurm_control_host_port != ""
    error_message = "Variable 'slurm_control_host_port' cannot be empty (\"\")."
  }
}

variable "slurm_control_addr" {
  type        = string
  description = <<EOD
The IP address or a name by which the address can be identified.
This value is passed to slurm.conf such that:
SlurmctldHost={var.slurm_control_host}\({var.slurm_control_addr}\)
See https://slurm.schedmd.com/slurm.conf.html#OPT_SlurmctldHost
EOD
  default     = null

  validation {
    condition     = var.slurm_control_addr != ""
    error_message = "Variable 'slurm_control_addr' cannot be empty (\"\")."
  }
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

variable "prolog_scripts" {
  description = <<-EOD
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
  description = <<-EOD
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

variable "disable_default_mounts" {
  description = <<-EOD
    Disable default global network storage from the controller: /usr/local/etc/slurm,
    /etc/munge, /home, /apps.
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
        bandwidth_tier         = string
        node_count_dynamic_max = number
        node_count_static      = number
        enable_spot_vm         = bool
        group_name             = string
        instance_template      = string
        node_conf              = map(string)
        access_config = list(object({
          nat_ip       = string
          network_tier = string
        }))
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

variable "google_app_cred_path" {
  type        = string
  description = "Path to Google Application Credentials."
  default     = null
}

variable "slurm_bin_dir" {
  type        = string
  description = <<-EOD
    Path to directory of Slurm binary commands (e.g. scontrol, sinfo). If 'null',
    then it will be assumed that binaries are in $PATH.
    EOD
  default     = null
}

variable "slurm_log_dir" {
  type        = string
  description = "Directory where Slurm logs to."
  default     = "/var/log/slurm"
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

variable "output_dir" {
  type        = string
  description = <<-EOD
    Directory where this module will write its files to. These files include:
    cloud.conf; cloud_gres.conf; config.yaml; resume.py; suspend.py; and util.py.
    If not specified explicitly, this will also be used as the default value
    for the `install_dir` variable.
    EOD
  default     = null
}

variable "install_dir" {
  description = <<-EOD
    Directory where the hybrid configuration directory will be installed on the
    on-premise controller. This updates the prefix path for the resume and
    suspend scripts in the generated `cloud.conf` file. The value defaults to
    output_dir if not specified.
    EOD
  type        = string
  default     = null
}

variable "munge_mount" {
  description = <<-EOD
  Remote munge mount for compute and login nodes to acquire the munge.key.

  By default, the munge mount server will be assumed to be the
  `var.slurm_control_host` (or `var.slurm_control_addr` if non-null) when
  `server_ip=null`.
  EOD
  type = object({
    server_ip     = string
    remote_mount  = string
    fs_type       = string
    mount_options = string
  })
  default = {
    server_ip     = null
    remote_mount  = "/etc/munge/"
    fs_type       = "nfs"
    mount_options = ""
  }
}
