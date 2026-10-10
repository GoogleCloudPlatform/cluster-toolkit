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

variable "project_id" {
  description = "Project in which Google Cloud resources will be created."
  type        = string
}

variable "deployment_name" {
  description = "Cluster Toolkit deployment name."
  type        = string
}

variable "region" {
  description = "Default region for creating resources."
  type        = string
}

variable "zone" {
  description = "Default zone for creating resources."
  type        = string
}

variable "instance_count" {
  description = "Number of instances."
  type        = number
  default     = 1
}

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

variable "instance_image" {
  description = <<-EOD
    Image used to build the noVNC desktop node.

    Expected fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.
    EOD
  type        = map(string)
  default = {
    project = "debian-cloud"
    name    = "debian-12-bookworm-v20250610"
  }
}

variable "disk_size_gb" {
  description = "Size of disk for instances."
  type        = number
  default     = 100
}

variable "disk_type" {
  description = "Disk type for instances."
  type        = string
  default     = "pd-balanced"
}

variable "auto_delete_boot_disk" {
  description = "Controls if boot disk should be auto-deleted when instance is deleted."
  type        = bool
  default     = true
}

variable "name_prefix" {
  description = "Optional name prefix for VM resources."
  type        = string
  default     = "desktop"
}

variable "add_deployment_name_before_prefix" {
  description = "If true, names are prefixed with deployment_name for uniqueness."
  type        = bool
  default     = true
}

variable "enable_public_ips" {
  description = "If true, instances receive public IPs."
  type        = bool
  default     = false
}

variable "machine_type" {
  description = "Machine type to use for desktop instance creation."
  type        = string
  default     = "e2-standard-4"
}

variable "labels" {
  description = "Labels to add to the instances."
  type        = map(string)
  default     = {}
}

variable "service_account_email" {
  description = "Service account e-mail address to attach to the VM."
  type        = string
  default     = null
}

variable "service_account_scopes" {
  description = "Scopes to attach to the VM service account."
  type        = set(string)
  default = [
    "https://www.googleapis.com/auth/cloud-platform",
  ]
}

variable "network_self_link" {
  description = "The self link of the network to attach the VM."
  type        = string
  default     = "default"
}

variable "subnetwork_self_link" {
  description = "The self link of the subnetwork to attach the VM."
  type        = string
  default     = null
}

variable "network_interfaces" {
  description = "Explicit network interfaces. If set, network_self_link and subnetwork_self_link are ignored by the VM module."
  type = list(object({
    network            = string,
    subnetwork         = string,
    subnetwork_project = string,
    network_ip         = string,
    nic_type           = string,
    stack_type         = string,
    queue_count        = number,
    access_config = list(object({
      nat_ip                 = string,
      public_ptr_domain_name = string,
      network_tier           = string
    })),
    ipv6_access_config = list(object({
      public_ptr_domain_name = string,
      network_tier           = string
    })),
    alias_ip_range = list(object({
      ip_cidr_range         = string,
      subnetwork_range_name = string
    }))
  }))
  default = []
}

variable "metadata" {
  description = "Metadata provided as a map."
  type        = map(string)
  default     = {}
}

variable "startup_script" {
  description = "Optional additional startup script prepended before the desktop runtime setup."
  type        = string
  default     = null
}

variable "slurm_auth_mode" {
  description = "Optional Slurm authentication mode forwarded to novnc-runtime: none, munge, native, or auto."
  type        = string
  default     = "none"
}

variable "guest_accelerator" {
  description = "List of the type and count of accelerator cards attached to the instance."
  type = list(object({
    type  = string
    count = number
  }))
  default = []
}

variable "bandwidth_tier" {
  description = "Bandwidth tier to use for the instance."
  type        = string
  default     = "not_enabled"
}

variable "threads_per_core" {
  description = "Sets the number of threads per physical core."
  type        = number
  default     = 2
}

variable "on_host_maintenance" {
  description = "Describes maintenance behavior for the instance."
  type        = string
  default     = "MIGRATE"
}

variable "tags" {
  description = "Network tags applied to the VM."
  type        = list(string)
  default     = []
}

variable "spot" {
  description = "Provision VMs using discounted Spot pricing."
  type        = bool
  default     = false
}

variable "enable_oslogin" {
  description = "Enable or Disable OS Login with ENABLE or DISABLE."
  type        = string
  default     = "ENABLE"
}

variable "allowed_ingress_cidrs" {
  description = "Private CIDR ranges allowed to reach the noVNC desktop broker listener."
  type        = list(string)
  default = [
    "10.0.0.0/8",
    "172.16.0.0/12",
    "192.168.0.0/16",
  ]
}

variable "install_root" {
  description = "Directory under which noVNC assets are installed."
  type        = string
  default     = "/opt/ghpc-remote-desktop"
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
}

variable "vnc_display_number" {
  description = "First X display number reserved for per-user VNC sessions."
  type        = number
  default     = 1
}

variable "novnc_listen_port" {
  description = "Port exposed by the desktop broker for browser access to noVNC."
  type        = number
  default     = 6080
}

variable "novnc_version" {
  description = "noVNC release version to install."
  type        = string
  default     = "1.7.0"
}

variable "secret_project_id" {
  description = "Project holding the Secret Manager secrets below. Defaults to the deployment project."
  type        = string
  default     = null
}

variable "novnc_proxy_secret" {
  description = <<-EOT
    Shared secret the desktop broker requires in the X-Cluster-Desktop-Secret
    header, supplied directly. Mutually exclusive with novnc_proxy_secret_id.

    Use this where the caller already owns the value and must hold the plaintext
    anyway - a front end that sends the header on every request gains nothing
    from a round trip through Secret Manager. Otherwise prefer the _id form: a
    literal is carried in the startup script staged in Cloud Storage and appears
    in Terraform state.
    EOT
  type        = string
  sensitive   = true
  default     = null
}

variable "novnc_proxy_secret_id" {
  description = <<-EOT
    Secret Manager secret ID holding the shared secret the desktop broker
    requires in the X-Cluster-Desktop-Secret header. Fetched on the instance at
    boot, so the value never enters Terraform state.

    Create it with:
      openssl rand -base64 48 | gcloud secrets create SECRET_ID --data-file=-

    The instance service account needs roles/secretmanager.secretAccessor.
    Mutually exclusive with novnc_proxy_secret.
    EOT
  type        = string
  default     = null
}

variable "novnc_proxy_secret_version" {
  description = "Version of novnc_proxy_secret_id to read."
  type        = string
  default     = "latest"
}

variable "novnc_identity_mode" {
  description = <<-EOT
    How the desktop broker establishes which user a request belongs to. Only
    "trusted_proxy" is supported: the identity is taken from request headers
    with no verification, so it is only safe where an authenticating proxy is
    the sole route to the broker.
    EOT
  type        = string
  default     = "trusted_proxy"

  validation {
    condition     = contains(["trusted_proxy"], lower(trimspace(var.novnc_identity_mode)))
    error_message = "novnc_identity_mode must be trusted_proxy."
  }
}

variable "desktop_endpoint_dir" {
  description = "Optional directory where the runtime publishes DESKTOP_* endpoint metadata for service discovery."
  type        = string
  default     = null
}

variable "desktop_endpoint_name" {
  description = "Endpoint metadata file name used when desktop_endpoint_dir is set."
  type        = string
  default     = "desktop"
}

variable "max_user_sessions" {
  description = "Maximum number of concurrent per-user desktop sessions the desktop host will support."
  type        = number
  default     = 32
}

variable "session_idle_timeout_seconds" {
  description = "Idle timeout after which a per-user desktop session is cleaned up. Set to 0 to disable cleanup."
  type        = number
  default     = 43200
}

variable "enable_gpu_acceleration" {
  description = <<-EOT
    Use hardware OpenGL for desktop sessions. Requires an instance_image with an
    NVIDIA driver and a GPU on the machine type. See the novnc-runtime module.
    EOT
  type        = bool
  default     = false
}
