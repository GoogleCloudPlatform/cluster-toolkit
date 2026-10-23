/**
 * Copyright 2026 Google LLC
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
  description = "Project in which the LLDAP server and its secrets, bucket and firewall rule are created."
  type        = string
}

variable "deployment_name" {
  description = "Name of the deployment, used as a prefix for resource names."
  type        = string
}

variable "region" {
  description = "Region of the static internal IP address and the snapshot schedule. Must be the region of `subnetwork_self_link`."
  type        = string
}

variable "zone" {
  description = "Zone in which the LLDAP VM and its data disk are created."
  type        = string
}

variable "network_self_link" {
  description = "Self link of the VPC network for the LLDAP VM and the LDAPS firewall rule."
  type        = string
}

variable "subnetwork_self_link" {
  description = "Self link of the subnetwork for the LLDAP VM. Its primary range is the default for `allowed_client_cidrs`."
  type        = string
}

variable "labels" {
  description = "Labels to add to the resources created by this module. Key-value pairs."
  type        = map(string)
}

variable "machine_type" {
  description = "Machine type of the LLDAP VM."
  type        = string
  default     = "e2-small"
}

variable "lldap_image" {
  description = <<-EOT
    LLDAP container image. Pin it by digest. The default is the upstream
    v0.6.3 release (multi-arch index digest, identical on ghcr.io and Docker Hub).
    The image must provide /app/lldap_set_password.
    EOT
  type        = string
  default     = "ghcr.io/lldap/lldap:v0.6.3@sha256:2a8454b668c1aba7157e832eab0e242e1e7eb5fb7591d7e7774ba05286511ca8"
  validation {
    condition     = can(regex("^[A-Za-z0-9./:@_-]+$", var.lldap_image))
    error_message = "lldap_image must be a non-empty image reference using only letters, digits and . / : @ _ -."
  }
}

variable "base_dn" {
  description = "LDAP base DN, for example `dc=example,dc=com`. Only `dc=` components are accepted."
  type        = string
  validation {
    condition     = can(regex("^dc=[A-Za-z0-9-]+(,dc=[A-Za-z0-9-]+)*$", var.base_dn))
    error_message = "base_dn must be a comma-separated list of dc= components without spaces, for example \"dc=example,dc=com\"."
  }
}

variable "groups" {
  description = <<-EOT
    POSIX groups to create in LLDAP, as a map of group name to `{ gid }`.
    The access group (`access_group`) is created separately and must not be listed here.
    Names must match `^[a-z_][a-z0-9_-]{0,31}$`; GIDs must be unique integers >= 10000.
    EOT
  type = map(object({
    gid = number
  }))
  default = {}
  validation {
    condition     = alltrue([for name in keys(var.groups) : can(regex("^[a-z_][a-z0-9_-]{0,31}$", name))])
    error_message = "Group names must match ^[a-z_][a-z0-9_-]{0,31}$."
  }
  validation {
    condition     = alltrue([for name in keys(var.groups) : !contains(["lldap_admin", "lldap_password_manager", "lldap_strict_readonly"], name)])
    error_message = "Group names lldap_admin, lldap_password_manager and lldap_strict_readonly are reserved by LLDAP."
  }
  validation {
    condition     = alltrue([for g in values(var.groups) : g.gid >= 10000 && g.gid <= 2147483647 && floor(g.gid) == g.gid])
    error_message = "Group gid values must be integers between 10000 and 2147483647."
  }
  validation {
    condition     = length(distinct([for g in values(var.groups) : g.gid])) == length(var.groups)
    error_message = "Group gid values must be unique."
  }
  validation {
    condition     = !contains(keys(var.groups), var.access_group)
    error_message = "Do not list access_group in groups; it is created from access_group and access_group_gid."
  }
  validation {
    condition     = !contains([for g in values(var.groups) : g.gid], var.access_group_gid)
    error_message = "access_group_gid must differ from every gid in groups."
  }
}

variable "users" {
  description = <<-EOT
    LDAP users, as a map of username to settings:
    - `uid`: POSIX UID, a unique integer >= 10000.
    - `primary_group`: a key of `groups`, or `access_group`. Sets the user's gidNumber.
    - `groups`: extra groups (keys of `groups`, or `access_group`). Default `[]`.
    - `shell`: login shell. Default `/bin/bash`. Must be one of /bin/bash,
      /usr/bin/bash, /bin/sh, /usr/bin/sh, /bin/zsh, /usr/bin/zsh, /bin/tcsh,
      /usr/bin/tcsh, /bin/ksh, /usr/bin/ksh.
    - `email`: optional, at most 254 characters. Must be unique. When unset,
      `<username>@noemail.invalid` is stored, because LLDAP requires a unique email.
    - `display_name`: optional, at most 64 characters, no control characters or ':'.
    - `disabled`: when true, the user is kept but removed from `access_group`, so
      they still resolve but cannot log in. Default false.
    Every enabled user is a member of `access_group`. Passwords are never set here:
    a new user gets a random password in their own Secret Manager secret
    (see output `user_password_secret_ids`).
    EOT
  type = map(object({
    uid           = number
    primary_group = string
    groups        = optional(list(string), [])
    shell         = optional(string, "/bin/bash")
    email         = optional(string)
    display_name  = optional(string)
    disabled      = optional(bool, false)
  }))
  default = {}
  validation {
    condition     = alltrue([for name in keys(var.users) : can(regex("^[a-z_][a-z0-9_-]{0,31}$", name))])
    error_message = "User names must match ^[a-z_][a-z0-9_-]{0,31}$."
  }
  validation {
    condition     = alltrue([for name in keys(var.users) : !contains(["admin", "lldap_bind"], name)])
    error_message = "User names admin and lldap_bind are reserved for the LLDAP admin and the SSSD bind user."
  }
  validation {
    condition     = alltrue([for u in values(var.users) : u.uid >= 10000 && u.uid <= 2147483647 && floor(u.uid) == u.uid])
    error_message = "User uid values must be integers between 10000 and 2147483647."
  }
  validation {
    condition     = length(distinct([for u in values(var.users) : u.uid])) == length(var.users)
    error_message = "User uid values must be unique."
  }
  validation {
    condition     = alltrue([for u in values(var.users) : contains(concat(keys(var.groups), [var.access_group]), u.primary_group)])
    error_message = "Each user's primary_group must be a key of groups or equal to access_group."
  }
  validation {
    condition = alltrue(flatten([
      for u in values(var.users) : [for g in u.groups : contains(concat(keys(var.groups), [var.access_group]), g)]
    ]))
    error_message = "Each entry of a user's groups must be a key of groups or equal to access_group."
  }
  validation {
    condition = alltrue([for u in values(var.users) : contains([
      "/bin/bash", "/usr/bin/bash", "/bin/sh", "/usr/bin/sh", "/bin/zsh", "/usr/bin/zsh",
      "/bin/tcsh", "/usr/bin/tcsh", "/bin/ksh", "/usr/bin/ksh",
    ], u.shell)])
    error_message = "User shell must be one of /bin/bash, /usr/bin/bash, /bin/sh, /usr/bin/sh, /bin/zsh, /usr/bin/zsh, /bin/tcsh, /usr/bin/tcsh, /bin/ksh, /usr/bin/ksh."
  }
  validation {
    condition = alltrue([for u in values(var.users) :
      u.email == null ? true : (length(u.email) <= 254 && can(regex("^[^@[:space:]]+@[^@[:space:]]+\\.[^@[:space:]]+$", u.email)))
    ])
    error_message = "User email must be at most 254 characters and look like name@domain.tld."
  }
  validation {
    condition = (
      length(distinct([for u in values(var.users) : lower(u.email) if u.email != null])) ==
      length([for u in values(var.users) : u.email if u.email != null])
    )
    error_message = "User email values must be unique (case-insensitive); LLDAP enforces unique emails."
  }
  validation {
    condition = alltrue([for u in values(var.users) :
      u.display_name == null ? true : (length(u.display_name) <= 64 && !can(regex("[[:cntrl:]:]", u.display_name)))
    ])
    error_message = "User display_name must be at most 64 characters and contain no control characters (including newlines) or ':'."
  }
}

variable "access_group" {
  description = "LDAP group whose members may log in to the cluster (SSSD ldap_access_filter). Every enabled user is added to it."
  type        = string
  default     = "cluster_users"
  validation {
    condition     = can(regex("^[a-z_][a-z0-9_-]{0,31}$", var.access_group))
    error_message = "access_group must match ^[a-z_][a-z0-9_-]{0,31}$."
  }
  validation {
    condition     = !contains(["lldap_admin", "lldap_password_manager", "lldap_strict_readonly"], var.access_group)
    error_message = "access_group cannot be one of LLDAP's built-in groups."
  }
}

variable "access_group_gid" {
  description = "GID of `access_group`. An integer >= 10000, different from every gid in `groups`."
  type        = number
  default     = 10000
  validation {
    condition     = var.access_group_gid >= 10000 && var.access_group_gid <= 2147483647 && floor(var.access_group_gid) == var.access_group_gid
    error_message = "access_group_gid must be an integer between 10000 and 2147483647."
  }
}

variable "cleanup_removed_users" {
  description = <<-EOT
    When true, users and groups that exist in LLDAP but are not in `users`/`groups`
    are deleted on the next sync. The admin user, the bind user, members of
    lldap_admin and LLDAP's built-in groups are never deleted.
    When false (default), such users are only removed from `access_group`, so they can no longer log in.
    EOT
  type        = bool
  default     = false
}

variable "client_service_accounts" {
  description = <<-EOT
    Service account emails of every VM that uses this directory. Each gets
    secretAccessor on the CA certificate and bind password secrets only, which
    the client script reads at boot.
    - Not set (null): the project's default compute service account,
      `<project-number>-compute@developer.gserviceaccount.com`, which Slurm v6
      controller, login and compute nodes use when their
      `service_account_email` is not set.
    - Set: it replaces the default and must list the service account of every
      client: controller, login nodes and every nodeset/partition (one per
      partition is fine). A node whose account is missing cannot join LDAP.
    See "Granting nodes access" in the README.
    EOT
  type        = list(string)
  default     = null
  validation {
    condition     = var.client_service_accounts == null ? true : length(var.client_service_accounts) > 0
    error_message = "client_service_accounts must be null (use the default compute service account) or list the service account of every Slurm controller, login and compute node that uses this directory."
  }
  validation {
    condition     = var.client_service_accounts == null ? true : alltrue([for e in var.client_service_accounts : can(regex("^[a-z0-9._-]+@[a-z0-9.-]+\\.gserviceaccount\\.com$", e))])
    error_message = "client_service_accounts must contain service account emails (…@….gserviceaccount.com), not user or group emails."
  }
  validation {
    condition     = var.client_service_accounts == null ? true : length(distinct(var.client_service_accounts)) == length(var.client_service_accounts)
    error_message = "client_service_accounts must not contain duplicates; list each service account once even if several node roles share it."
  }
}

variable "allowed_client_cidrs" {
  description = "Source CIDR ranges allowed to reach LDAPS (tcp:6360). Defaults to the primary range of `subnetwork_self_link`."
  type        = list(string)
  default     = null
  validation {
    condition     = var.allowed_client_cidrs == null ? true : (length(var.allowed_client_cidrs) > 0 && alltrue([for c in var.allowed_client_cidrs : can(cidrnetmask(c))]))
    error_message = "allowed_client_cidrs must be null or a non-empty list of IPv4 CIDR ranges."
  }
}

variable "client_secret_wait_seconds" {
  description = <<-EOT
    How long the client script waits for the CA and bind password secrets to get
    their first version (written by the LLDAP VM on its first boot). The wait,
    the SSSD package install, and the lookup check (up to about 95 seconds: six
    5-second lookups and five sssd restarts with 10-second pauses) must together
    stay below the Slurm `*_startup_scripts_timeout` values (default 300
    seconds), or the node's Slurm setup fails. The default of 90 leaves about
    95 seconds for the install. For a first deployment, raise this and the
    Slurm timeouts together.
    EOT
  type        = number
  default     = 90
  validation {
    condition     = var.client_secret_wait_seconds >= 0
    error_message = "client_secret_wait_seconds must be >= 0."
  }
}

variable "deletion_protection" {
  description = "Enable deletion protection on the LLDAP VM. Set to false (and apply) before destroying the deployment."
  type        = bool
  default     = true
}

variable "data_disk_size_gb" {
  description = "Size in GB of the persistent data disk that holds the LLDAP database, TLS keys and server secrets."
  type        = number
  default     = 10
}

variable "data_disk_type" {
  description = "Type of the LLDAP data disk."
  type        = string
  default     = "pd-balanced"
}

variable "data_disk_deletion_policy" {
  description = <<-EOT
    Terraform deletion policy of the data disk (google_compute_disk.deletion_policy).
    "PREVENT" (default) makes any plan that would delete or replace the disk fail.
    Set to "DELETE" and apply before `gcluster destroy`, or "ABANDON" to keep the
    disk in GCP while removing it from Terraform state.
    EOT
  type        = string
  default     = "PREVENT"
  validation {
    condition     = contains(["PREVENT", "DELETE", "ABANDON"], var.data_disk_deletion_policy)
    error_message = "data_disk_deletion_policy must be PREVENT, DELETE or ABANDON."
  }
}

variable "data_disk_snapshot_before_destroy" {
  description = "Create a snapshot of the data disk when Terraform deletes it (google_compute_disk.create_snapshot_before_destroy). The snapshot is not managed by Terraform and must be deleted manually."
  type        = bool
  default     = true
}

variable "snapshot_retention_days" {
  description = "Days to keep the daily snapshots of the data disk."
  type        = number
  default     = 14
  validation {
    condition     = var.snapshot_retention_days >= 1
    error_message = "snapshot_retention_days must be >= 1."
  }
}

variable "enable_backups" {
  description = "Create a GCS bucket and a systemd timer that uploads an online SQLite backup of the LLDAP database, with its key seed and TLS material."
  type        = bool
  default     = true
}

variable "backup_schedule" {
  description = "systemd OnCalendar expression for the GCS backup timer (VM local time, UTC on Debian GCE images)."
  type        = string
  default     = "*-*-* 02:30:00"
  validation {
    condition     = can(regex("^[A-Za-z0-9*:, ./~-]+$", var.backup_schedule))
    error_message = "backup_schedule must be a systemd OnCalendar expression (letters, digits, spaces and * : , . / ~ -)."
  }
}

variable "backup_retention_days" {
  description = "Days to keep backup objects in the GCS bucket (lifecycle rule). Noncurrent versions are deleted after the same number of days."
  type        = number
  default     = 30
  validation {
    condition     = var.backup_retention_days >= 1
    error_message = "backup_retention_days must be >= 1."
  }
}
