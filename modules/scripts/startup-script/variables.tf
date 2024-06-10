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
  type = list(object({
    type        = string,
    destination = string,
    source      = optional(string, null),
    content     = optional(string, null),
    args        = optional(string, null)
  }))


  validation {
    condition     = length(distinct(var.runners[*].destination)) == length(var.runners)
    error_message = "All startup-script runners must have a unique destination."
  }

  validation {
    condition = alltrue([
      for r in var.runners : contains(["ansible-local", "shell", "data"], r.type)
    ])
    error_message = "The 'type' must be 'ansible-local', 'shell' or 'data'."
  }

  validation {
    condition = alltrue([
      for r in var.runners :
      (r.content == null) != can(r.source == null)
    ])
    error_message = "A runner must specify exactly one of 'content' or 'source'"
  }
  default = []
}

variable "enable_docker_world_writable" {
  description = "Configure Docker daemon to be writable by all users (if var.install_docker is set to true)."
  type        = bool
  default     = false
  nullable    = false
}

variable "install_docker" {
  description = "Install Docker command line tool and daemon."
  type        = bool
  default     = false
  nullable    = false
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
