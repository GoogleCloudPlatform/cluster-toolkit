# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured."
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

variable "startup_script" {
  description = "Optional additional startup script prepended before the desktop runtime setup."
  type        = string
  default     = null
}

variable "slurm_auth_mode" {
  description = <<-EOD
    Optional Slurm authentication mode for hosts that also participate in a Slurm cluster.

    Supported values:
    - none:   no Slurm-specific handling
    - munge:  host is expected to use MUNGE authentication
    - native: host is expected to use Slurm Native Authentication and the runtime disables stale munge.service state
    - auto:   detect AuthType from slurm.conf at startup and apply the Native Auth cleanup only when needed
    EOD
  type        = string
  default     = "none"

  validation {
    condition     = contains(["none", "munge", "native", "auto"], lower(trimspace(var.slurm_auth_mode)))
    error_message = "slurm_auth_mode must be one of: none, munge, native, auto."
  }
}

variable "install_root" {
  description = "Directory under which noVNC assets are installed."
  type        = string
  default     = "/opt/ghpc-remote-desktop"

  validation {
    condition     = trimspace(var.install_root) != "" && can(regex("^/", trimspace(var.install_root)))
    error_message = "install_root must be a non-empty absolute path."
  }
}

variable "vnc_backend" {
  description = "VNC server backend used for per-user desktop sessions: tigervnc or turbovnc."
  type        = string
  default     = "tigervnc"

  validation {
    condition     = contains(["tigervnc", "turbovnc"], lower(trimspace(var.vnc_backend)))
    error_message = "vnc_backend must be one of: tigervnc, turbovnc."
  }
}

variable "session_resolution" {
  description = "Desktop resolution used for each per-user XFCE session."
  type        = string
  default     = "1920x1080"

  validation {
    condition     = can(regex("^[1-9][0-9]*x[1-9][0-9]*$", var.session_resolution))
    error_message = "session_resolution must be in WIDTHxHEIGHT form."
  }
}

variable "vnc_display_number" {
  description = "First X display number reserved for per-user VNC sessions. Starts high to stay clear of users' own X servers on a shared host."
  type        = number
  default     = 50

  validation {
    condition     = var.vnc_display_number >= 1
    error_message = "vnc_display_number must be greater than or equal to 1."
  }
}

variable "novnc_version" {
  description = "noVNC release version to install."
  type        = string
  default     = "1.7.0"

  validation {
    condition     = trimspace(var.novnc_version) != ""
    error_message = "novnc_version must be a non-empty string."
  }
}

variable "max_user_sessions" {
  description = "Maximum number of concurrent per-user desktop sessions the host VM will support."
  type        = number
  default     = 32

  validation {
    condition     = var.max_user_sessions > 0
    error_message = "max_user_sessions must be greater than zero."
  }
}

variable "session_idle_timeout_seconds" {
  description = "Idle timeout after which a per-user desktop session is cleaned up. Set to 0 to keep idle sessions; a session whose desktop has exited is still cleaned up."
  type        = number
  default     = 43200

  validation {
    condition     = var.session_idle_timeout_seconds >= 0
    error_message = "session_idle_timeout_seconds must be zero or greater."
  }
}

variable "enable_gpu_acceleration" {
  description = <<-EOT
    Use hardware OpenGL for desktop sessions. Requires a GPU on the host and an
    image whose NVIDIA driver has graphics (not merely compute) support; falls
    back to software rendering otherwise. Requires vnc_backend = "turbovnc".
    See community/examples/remote-desktop/README.md for which image to use.
    EOT
  type        = bool
  default     = false
}
