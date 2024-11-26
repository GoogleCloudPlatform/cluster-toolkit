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

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "deployment_name" {
  description = "Name of the HPC deployment, used to name GCS bucket for startup scripts."
  type        = string
}

variable "region" {
  description = "The region to deploy to"
  type        = string
}

variable "gcs_bucket_path" {
  description = "The GCS path for storage bucket and the object, starting with `gs://`."
  type        = string
  default     = null
}

variable "bucket_viewers" {
  description = "Additional service accounts or groups, users, and domains to which to grant read-only access to startup-script bucket (leave unset if using default Compute Engine service account)"
  type        = list(string)
  default     = []

  validation {
    condition = alltrue([
      for u in var.bucket_viewers : length(regexall("^(allUsers$|allAuthenticatedUsers$|user:|group:|serviceAccount:|domain:)", u)) > 0
    ])
    error_message = "Bucket viewer members must begin with user/group/serviceAccount/domain following https://cloud.google.com/iam/docs/reference/rest/v1/Policy#Binding"
  }
}

variable "debug_file" {
  description = "Path to an optional local to be written with 'startup_script'."
  type        = string
  default     = null
}

variable "labels" {
  description = "Labels for the created GCS bucket. Key-value pairs."
  type        = map(string)
}

variable "runners" {
  description = <<EOT
    List of runners to run on remote VM.
    Runners can be of type ansible-local, shell or data.
    A runner must specify one of 'source' or 'content'.
    All runners must specify 'destination'. If 'destination' does not include a
    path, it will be copied in a temporary folder and deleted after running.
    Runners may also pass 'args', which will be passed as argument to shell runners only.
EOT
  type        = list(map(string))
  validation {
    condition = alltrue([
      for r in var.runners : contains(keys(r), "type")
    ])
    error_message = "All runners must declare a type."
  }
  validation {
    condition = alltrue([
      for r in var.runners : contains(keys(r), "destination")
    ])
    error_message = "All runners must declare a destination name (even without a path)."
  }
  validation {
    condition     = length(distinct([for r in var.runners : r["destination"]])) == length(var.runners)
    error_message = "All startup-script runners must have a unique destination."
  }
  validation {
    condition = alltrue([
      for r in var.runners : r["type"] == "ansible-local" || r["type"] == "shell" || r["type"] == "data"
    ])
    error_message = "The 'type' must be 'ansible-local', 'shell' or 'data'."
  }
  # this validation tests that exactly 1 or other of source/content have been
  # set to anything (including null)
  validation {
    condition = alltrue([
      for r in var.runners :
      can(r["content"]) != can(r["source"])
    ])
    error_message = "A runner must specify either 'content' or 'source', but never both."
  }
  # this validation tests that at least 1 of source/content are non-null
  # can fail either by not having been set all or by being set to null
  validation {
    condition = alltrue([
      for r in var.runners :
      lookup(r, "content", lookup(r, "source", null)) != null
    ])
    error_message = "A runner must specify a non-null 'content' or 'source'."
  }
  default = []
}

variable "docker" {
  description = "Install and configure Docker"
  type = object({
    enabled        = optional(bool, false)
    world_writable = optional(bool, false)
    daemon_config  = optional(string, "")
  })
  default = {
    enabled = false
  }

  validation {
    condition     = !coalesce(var.docker.world_writable) || var.docker.enabled
    error_message = "var.docker.world_writable should only be set if var.docker.enabled is set to true"
  }

  validation {
    condition     = !can(coalesce(var.docker.daemon_config)) || var.docker.enabled
    error_message = "var.docker.daemon_config should only be set if var.docker.enabled is set to true"
  }

  validation {
    condition     = !can(coalesce(var.docker.daemon_config)) || can(jsondecode(var.docker.daemon_config))
    error_message = "var.docker.daemon_config should be set to a valid Docker daemon JSON configuration"
  }

}

# tflint-ignore: terraform_unused_declarations
variable "enable_docker_world_writable" {
  description = "DEPRECATED: use var.docker"
  type        = bool
  default     = null

  validation {
    condition     = var.enable_docker_world_writable == null
    error_message = "The variable enable_docker_world_writable has been removed. Use var.docker instead"
  }
}

# tflint-ignore: terraform_unused_declarations
variable "install_docker" {
  description = "DEPRECATED: use var.docker."
  type        = bool
  default     = null

  validation {
    condition     = var.install_docker == null
    error_message = "The variable install_docker has been removed. Use var.docker instead"
  }
}

variable "local_ssd_filesystem" {
  description = "Create and mount a filesystem from local SSD disks (data will be lost if VMs are powered down without enabling migration); enable by setting mountpoint field to a valid directory path."
  type = object({
    fs_type     = optional(string, "ext4")
    mountpoint  = optional(string, "")
    permissions = optional(string, "0755")
  })

  validation {
    condition     = can(coalesce(var.local_ssd_filesystem.fs_type))
    error_message = "var.local_ssd_filesystem.fs_type must be set to a filesystem supported by the Linux distribution."
  }

  validation {
    condition     = var.local_ssd_filesystem.mountpoint == "" || startswith(var.local_ssd_filesystem.mountpoint, "/")
    error_message = "To enable local SSD filesystems, var.local_ssd_filesystem.mountpoint must be set to an absolute path to a mountpoint."
  }

  validation {
    condition     = length(regexall("^[0-7]{3,4}$", var.local_ssd_filesystem.permissions)) > 0
    error_message = "The POSIX permissions for the mountpoint must be represented as a 3 or 4-digit octal"
  }

  default = {
    fs_type     = "ext4"
    mountpoint  = ""
    permissions = "0755"
  }

  nullable = false
}

variable "install_cloud_ops_agent" {
  description = "Warning: Consider using `install_stackdriver_agent` for better performance. Run Google Ops Agent installation script if set to true."
  type        = bool
  default     = false
}

variable "install_stackdriver_agent" {
  description = "Run Google Stackdriver Agent installation script if set to true. Preferred over ops agent for performance."
  type        = bool
  default     = false
}

variable "install_ansible" {
  description = "Run Ansible installation script if either set to true or unset and runner of type 'ansible-local' are used."
  type        = bool
  default     = null
}

variable "configure_ssh_host_patterns" {
  description = <<EOT
  If specified, it will automate ssh configuration by:
  - Defining a Host block for every element of this variable and setting StrictHostKeyChecking to 'No'.
  Ex: "hpc*", "hpc01*", "ml*"
  - The first time users log-in, it will create ssh keys that are added to the authorized keys list
  This requires a shared /home filesystem and relies on specifying the right prefix.
  EOT
  type        = list(string)
  default     = []
}

# tflint-ignore: terraform_unused_declarations
variable "prepend_ansible_installer" {
  description = <<EOT
  DEPRECATED. Use `install_ansible=false` to prevent ansible installation.
  EOT
  type        = bool
  default     = null
  validation {
    condition     = var.prepend_ansible_installer == null
    error_message = "The variable prepend_ansible_installer has been removed. Use install_ansible instead"
  }
}

variable "ansible_virtualenv_path" {
  description = "Virtual environment path in which to install Ansible"
  type        = string
  default     = "/usr/local/ghpc-venv"
  validation {
    condition     = can(regex("^(/[\\w-]+)+$", var.ansible_virtualenv_path))
    error_message = "var.ansible_virtualenv_path must be an absolute path to a directory without spaces or special characters"
  }
}

variable "http_proxy" {
  description = "Web (http and https) proxy configuration for pip, apt, and yum/dnf and interactive shells"
  type        = string
  default     = ""
  nullable    = false
}

variable "http_no_proxy" {
  description = "Domains for which to disable http_proxy behavior. Honored only if var.http_proxy is set"
  type        = string
  default     = ".google.com,.googleapis.com,metadata.google.internal,localhost,127.0.0.1"
  nullable    = false
}

variable "install_cloud_rdma_drivers" {
  description = "If true, will install and reload Cloud RDMA drivers. Currently only supported on Rocky Linux 8."
  type        = bool
  default     = false
}
