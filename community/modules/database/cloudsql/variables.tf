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

variable "database_version" {
  description = "The version of the database to be created, e.g. MYSQL_8_0 or POSTGRES_15."
  type        = string
  default     = "MYSQL_8_0"
  nullable    = false
  validation {
    condition     = can(regex("^(MYSQL|POSTGRES)_[A-Z0-9_]+$", var.database_version))
    error_message = "database_version must be a Cloud SQL MySQL or PostgreSQL version, such as MYSQL_8_0 or POSTGRES_15. SQL Server is not supported because this module does not set the root_password it requires."
  }
}

variable "data_cache_enabled" {
  description = "Whether data cache is enabled for the instance. Can be used with ENTERPRISE_PLUS edition."
  type        = bool
  default     = false
}

variable "database_flags" {
  description = "Database flags to set on instance."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "deployment_name" {
  description = "The name of the current deployment"
  type        = string
}

variable "disk_autoresize" {
  description = "Set to false to disable automatic disk grow."
  type        = bool
  default     = true
  nullable    = false
}

variable "disk_size_gb" {
  description = <<-EOT
    Size of the database disk in GiB. Raising it later resizes the disk in place. With
    disk_autoresize on, Cloud SQL may grow the disk beyond this size, and the provider then
    ignores the smaller configured value instead of shrinking or recreating the instance.
    EOT
  type        = number
  default     = null
}

variable "edition" {
  description = "The edition of the Cloud SQL instance. ENTERPRISE_PLUS is required for data cache."
  type        = string
  validation {
    condition     = contains(["ENTERPRISE", "ENTERPRISE_PLUS"], var.edition)
    error_message = "The database edition must be either ENTERPRISE or ENTERPRISE_PLUS"
  }
  default = "ENTERPRISE"
}

variable "enable_backups" {
  description = "Set true to enable automated backups."
  type        = bool
  default     = true
  nullable    = false
}

variable "point_in_time_recovery" {
  description = "Enable point-in-time recovery (binary logging on MySQL) when enable_backups is true."
  type        = bool
  default     = true
  nullable    = false
}

variable "backup_start_time" {
  description = "Start of the daily backup window, in UTC as HH:MM. Leave null for a time chosen by Cloud SQL."
  type        = string
  default     = null
  validation {
    condition     = var.backup_start_time == null || can(regex("^([01][0-9]|2[0-3]):[0-5][0-9]$", var.backup_start_time))
    error_message = "backup_start_time must be HH:MM in 24-hour UTC, for example \"03:00\"."
  }
}

variable "retained_backups" {
  description = "Number of automated backups to keep, at least 7 (the module keeps 7 days of transaction logs, and Cloud SQL requires at least as many backups). Leave null for the Cloud SQL default (7)."
  type        = number
  default     = null
  validation {
    condition     = var.retained_backups == null || try(var.retained_backups >= 7 && floor(var.retained_backups) == var.retained_backups, false)
    error_message = "retained_backups must be a whole number of at least 7, the module's transaction log retention in days."
  }
}

variable "retain_backups_on_delete" {
  description = "Keep the instance's automated backups after it is deleted (they remain billed until they expire). Set null to use the Cloud SQL default."
  type        = bool
  default     = true
}

variable "final_backup_retention_days" {
  description = "Take a final backup when the instance is deleted and keep it for this many days. Leave null for no final backup."
  type        = number
  default     = null
  validation {
    condition     = var.final_backup_retention_days == null || try(var.final_backup_retention_days >= 1 && floor(var.final_backup_retention_days) == var.final_backup_retention_days, false)
    error_message = "final_backup_retention_days must be a whole number of at least 1."
  }
}

variable "availability_type" {
  description = "ZONAL, or REGIONAL for a high-availability instance with a standby in another zone. Set null to use the Cloud SQL default (ZONAL)."
  type        = string
  default     = "ZONAL"
  validation {
    condition     = var.availability_type == null || contains(["ZONAL", "REGIONAL"], coalesce(var.availability_type, "ZONAL"))
    error_message = "availability_type must be ZONAL or REGIONAL."
  }
  validation {
    condition     = var.availability_type != "REGIONAL" || !startswith(var.database_version, "MYSQL") || (var.enable_backups && var.point_in_time_recovery)
    error_message = "A REGIONAL MySQL instance needs enable_backups and point_in_time_recovery (binary logging): Cloud SQL requires both for MySQL high availability."
  }
}

variable "ssl_mode" {
  description = "Connection encryption policy: ENCRYPTED_ONLY, TRUSTED_CLIENT_CERTIFICATE_REQUIRED or ALLOW_UNENCRYPTED_AND_ENCRYPTED. Set null to use the Cloud SQL default, which accepts unencrypted connections. TRUSTED_CLIENT_CERTIFICATE_REQUIRED also needs client certificates, which this module does not create."
  type        = string
  default     = "ENCRYPTED_ONLY"
  validation {
    condition     = var.ssl_mode == null || contains(["ENCRYPTED_ONLY", "TRUSTED_CLIENT_CERTIFICATE_REQUIRED", "ALLOW_UNENCRYPTED_AND_ENCRYPTED"], coalesce(var.ssl_mode, "ENCRYPTED_ONLY"))
    error_message = "ssl_mode must be ENCRYPTED_ONLY, TRUSTED_CLIENT_CERTIFICATE_REQUIRED or ALLOW_UNENCRYPTED_AND_ENCRYPTED."
  }
}

variable "set_root_password" {
  description = <<-EOT
    Set a random password, never stored in state or output, on the built-in
    root user (MySQL root, PostgreSQL postgres) of a created instance, so it
    cannot be used without one. Reset it with `gcloud sql users set-password`
    if you need root access. Only read when the instance is created; changing
    it later has no effect.
    EOT
  type        = bool
  default     = true
  nullable    = false
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "query_insights" {
  description = "Query insights configuration."
  nullable    = false
  default     = {}
  type = object({
    enabled                 = optional(bool, false)
    query_plans_per_minute  = optional(number)
    query_string_length     = optional(number)
    record_application_tags = optional(bool)
    record_client_address   = optional(bool)
  })
}

variable "region" {
  description = "The region where SQL instance will be configured"
  type        = string
}

variable "tier" {
  description = "The machine type to use for the SQL instance. Required unless existing_instance_name is set."
  type        = string
  default     = null
  validation {
    condition     = var.tier != null || var.existing_instance_name != null
    error_message = "tier is required when this module creates the instance (existing_instance_name is null)."
  }
}

variable "sql_instance_name" {
  description = "Name given to the SQL instance. If not supplied, a name is generated from deployment_name."
  type        = string
  default     = null
}

variable "deletion_protection" {
  description = "Prevent Terraform from destroying the instance. Set false, and apply, before destroying it."
  type        = bool
  default     = true
}

variable "api_deletion_protection" {
  description = "Also block deletion of the instance through the Cloud SQL API and console (deletion_protection_enabled). Set false, and apply, before destroying the instance."
  type        = bool
  default     = true
}

variable "labels" {
  description = "Labels to add to the instances. Key-value pairs."
  type        = map(string)
}

variable "sql_username" {
  description = <<-EOT
    Username for the SQL database, at most 32 characters for MySQL (16 for
    MYSQL_5_6). Module instances that share one Cloud SQL instance (for
    example a creating and an adopting module) must use different usernames:
    both the database user and the password secret ID are derived from it.
    EOT
  type        = string
  default     = "dbuser"
  validation {
    condition     = can(regex("^[a-zA-Z0-9_-]+$", var.sql_username))
    error_message = "sql_username must contain only letters, digits, underscores and hyphens, because it forms part of the Secret Manager secret ID."
  }
}

variable "sql_password" {
  description = <<-EOT
    Password for the SQL database. If not supplied, a random password is
    generated. It is written through write-only arguments, so changing it has
    no effect unless password_version is increased in the same apply. A value
    set here is written to the deployment folder and the saved plan.
    EOT
  type        = string
  default     = null
  sensitive   = true
}

variable "network_id" {
  description = <<-EOT
    The ID of the GCE VPC network to which the instance is going to be created in:
    `projects/<project_id>/global/networks/<network_name>`. Required unless
    use_psc_connection is true or existing_instance_name is set.
    EOT
  type        = string
  default     = null
  validation {
    condition     = var.network_id == null || can(regex("^projects/[^/]+/global/networks/[^/]+$", var.network_id))
    error_message = "The network id must be provided in the following format: projects/<project_id>/global/networks/<network_name>."
  }
  validation {
    condition     = var.network_id != null || var.use_psc_connection || var.existing_instance_name != null
    error_message = "network_id is required when this module creates the instance over Private Service Access."
  }
}

variable "private_vpc_connection_peering" {
  description = "The name of the VPC Network peering connection, used only as dependency for Cloud SQL creation."
  type        = string
  default     = null
}

variable "subnetwork_self_link" {
  description = "Self link of the network where Cloud SQL instance PSC endpoint will be created"
  type        = string
  default     = null
  validation {
    condition     = !var.use_psc_connection || var.subnetwork_self_link != null
    error_message = "subnetwork_self_link is required when use_psc_connection is true: the PSC endpoint address and forwarding rule are created in that subnetwork."
  }
}

variable "encryption_key_name" {
  description = <<-DOC
    The full resource name of a Cloud KMS CryptoKey used to encrypt the
    Cloud SQL instance data (CMEK). The Cloud SQL service identity must
    hold roles/cloudkms.cryptoKeyEncrypterDecrypter on the key, and the
    key location must match the instance region. Encryption mode is
    fixed at instance creation and cannot be changed in place.
  DOC
  type        = string
  default     = null
}

variable "user_managed_replication" {
  type = list(object({
    location     = string
    kms_key_name = optional(string)
  }))
  description = "Replication parameters that will be used for the generated password secret."
  default     = []
}

variable "use_psc_connection" {
  description = "Create Private Service Connection instead of using Private Service Access peering"
  type        = bool
  default     = false
  nullable    = false
}

variable "database_name" {
  description = "Name of the database to create on the instance."
  type        = string
  nullable    = false
}

variable "password_version" {
  description = <<-EOT
    Rotation counter for the database password. Increase it (for example
    from 1 to 2) to generate a new password: the module writes it to a new
    secret version, then sets it on the database user, in the same apply.
    EOT
  type        = number
  default     = 1
  nullable    = false
  validation {
    condition     = var.password_version >= 1 && floor(var.password_version) == var.password_version
    error_message = "password_version must be a whole number of at least 1."
  }
}

variable "password_secret_accessors" {
  description = <<-EOT
    IAM members granted `roles/secretmanager.secretAccessor` on the password
    secret, in IAM member form (e.g. `serviceAccount:sa@project.iam.gserviceaccount.com`).
    Values must be known at plan time, such as literal strings or the `service_account_iam_email`
    output of `modules/project/service-account` (the account may be created
    in the same apply).
    EOT
  type        = list(string)
  default     = []
  nullable    = false
}

variable "service_account_iam_email" {
  description = <<-EOT
    IAM members granted read access to the password secret, in addition to
    password_secret_accessors. Named to match the `service_account_iam_email`
    output of `modules/project/service-account`, so listing such modules in
    `use` fills it; several used modules are combined.
    EOT
  type        = list(string)
  default     = []
  nullable    = false
}

variable "maintenance_window" {
  description = "Maintenance window for the Cloud SQL instance: day 1 (Monday) to 7, hour 0 to 23 UTC, and update_track canary, stable or week5. Leave null to accept Cloud SQL defaults."
  type = object({
    day          = number
    hour         = number
    update_track = optional(string)
  })
  default = null
  validation {
    condition = var.maintenance_window == null || try(
      var.maintenance_window.day >= 1 && var.maintenance_window.day <= 7 && floor(var.maintenance_window.day) == var.maintenance_window.day &&
      var.maintenance_window.hour >= 0 && var.maintenance_window.hour <= 23 && floor(var.maintenance_window.hour) == var.maintenance_window.hour &&
      contains(["canary", "stable", "week5"], coalesce(var.maintenance_window.update_track, "stable")),
    false)
    error_message = "maintenance_window needs day 1-7, hour 0-23 and update_track canary, stable or week5."
  }
}

variable "psc_allowed_consumer_projects" {
  description = "Projects allowed to connect to the instance over Private Service Connect. Leave null to allow only project_id."
  type        = list(string)
  default     = null
  validation {
    condition     = var.psc_allowed_consumer_projects == null ? true : length(var.psc_allowed_consumer_projects) > 0
    error_message = "psc_allowed_consumer_projects must list at least one project, or be null to allow only project_id."
  }
}

variable "database_deletion_policy" {
  description = <<-EOT
    Deletion policy for the database. Leave null to use ABANDON when adopting
    an existing instance or while the created instance has deletion_protection
    or api_deletion_protection on, so destroying this module leaves the
    database and its data in place, and the provider default (drop the
    database) otherwise. Set to ABANDON to always leave it.
    EOT
  type        = string
  default     = null
  validation {
    condition     = var.database_deletion_policy == null || var.database_deletion_policy == "ABANDON"
    error_message = "database_deletion_policy must be null or \"ABANDON\"."
  }
}

variable "existing_instance_name" {
  description = <<-EOT
    Name of an existing Cloud SQL instance to adopt instead of creating a new
    one. The instance must have a private IP address. To adopt an instance
    created by another cloudsql module in the same apply, set that module's
    sql_instance_name and pass its instance_name output here; a literal name
    is read at plan time, before the instance exists, and fails. When set,
    the inputs that shape a new instance are ignored: api_deletion_protection,
    availability_type, backup_start_time, data_cache_enabled, database_flags,
    database_version, deletion_protection, disk_autoresize, disk_size_gb,
    edition, enable_backups, encryption_key_name,
    final_backup_retention_days, maintenance_window, network_id,
    point_in_time_recovery, private_vpc_connection_peering,
    psc_allowed_consumer_projects, query_insights, retain_backups_on_delete,
    retained_backups, set_root_password, sql_instance_name, ssl_mode and
    tier.
    EOT
  type        = string
  default     = null
  validation {
    condition     = var.existing_instance_name == null || !var.use_psc_connection
    error_message = "use_psc_connection is not supported together with existing_instance_name: PSC resources are only created alongside a new instance, so server_ip/private_ip_address would otherwise silently resolve to null or the adopted instance's non-PSC IP instead of a PSC endpoint."
  }
}

variable "user_deletion_policy" {
  description = <<-EOT
    Deletion policy for the database user. Leave null to use ABANDON for
    PostgreSQL, where a user that owns objects cannot be dropped and destroy
    would otherwise fail, and while the created instance has
    deletion_protection or api_deletion_protection on; otherwise the
    provider default (drop the user). Set to ABANDON to always leave the user
    in place on destroy.
    EOT
  type        = string
  default     = null
  validation {
    condition     = var.user_deletion_policy == null || var.user_deletion_policy == "ABANDON"
    error_message = "user_deletion_policy must be null or \"ABANDON\"."
  }
}
