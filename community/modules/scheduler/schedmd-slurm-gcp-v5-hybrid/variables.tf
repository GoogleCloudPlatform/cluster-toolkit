
variable "project_id" {
  type        = string
  description = "Project ID to create resources in."
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming and slurm accounting."

  validation {
    condition     = can(regex("(^[a-z][a-z0-9]*$)", var.slurm_cluster_name))
    error_message = "Variable 'slurm_cluster_name' must be composed of only alphanumeric values and begin with a leter. regex: '(^[a-z][a-z0-9]*$)'."
  }
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

variable "disable_default_mounts" {
  description = <<-EOD
    Disable default global network storage from the controller
    - /usr/local/etc/slurm
    - /etc/munge
    - /home
    - /apps
    If these are disabled, the slurm etc and munge dirs must be added manually,
    or some other mechanism must be used to synchronize the slurm conf files
    and the munge key across the cluster.
    EOD
  type        = bool
  default     = false
}

variable "slurm_control_host" {
  type        = string
  description = <<-EOD
    The short, or long, hostname of the machine where Slurm control daemon is
    executed (i.e. the name returned by the command "hostname -s").
    See https://slurm.schedmd.com/slurm.conf.html#OPT_SlurmctldHost
    EOD
  default     = null
}

variable "compute_startup_script" {
  description = "Startup script used by the compute VMs."
  type        = string
  default     = ""
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

variable "network_storage" {
  description = <<-EOD
    Storage to mounted on all instances.
    - server_ip     : Address of the storage server.
    - remote_mount  : The location in the remote instance filesystem to mount from.
    - local_mount   : The location on the instance filesystem to mount to.
    - fs_type       : Filesystem type (e.g. "nfs").
    - mount_options : Options to mount with.
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
  description = <<-EOD
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
        node_count_dynamic_max = number
        node_count_static      = number
        enable_spot_vm         = bool
        group_name             = string
        instance_template      = string
        node_conf              = map(string)
        spot_instance_config = object({
          termination_action = string
        })
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
    error_message = "Item 'partition_name' must be alphanumeric and begin with a letter. regex: '(^[a-z][a-z0-9]*$)'."
  }
}

variable "google_app_cred_path" {
  type        = string
  description = "Path to Google Applicaiton Credentials."
  default     = null
}

variable "slurm_bin_dir" {
  type        = string
  description = <<-EOD
    Path to directroy of Slurm binary commands (e.g. scontrol, sinfo). If 'null',
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
    EOD
  default     = null
}

variable "slurm_depends_on" {
  description = <<-EOD
    Custom terraform dependencies without replacement on delta. This is useful to
    ensure order of resource creation.
    NOTE: Also see terraform meta-argument 'depends_on'.
    EOD
  type        = list(string)
  default     = []
}
