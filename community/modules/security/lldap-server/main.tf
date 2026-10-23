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

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "lldap-server", ghpc_role = "security" })
}

locals {
  # Truncate so that "<name>-<suffix>" stays within the 63 character GCE limit.
  name = "${substr(var.deployment_name, 0, 40)}-lldap"

  # Service account IDs must match ^[a-z][-a-z0-9]{4,28}[a-z0-9]$. Keep up to 16
  # characters of the deployment name so the account can be traced to its
  # deployment; "-ldap-" plus the 8 hex suffix completes it.
  sa_base   = replace(lower(var.deployment_name), "/[^-a-z0-9]/", "-")
  sa_prefix = replace(substr(can(regex("^[a-z]", local.sa_base)) ? local.sa_base : "d${local.sa_base}", 0, 16), "/-+$/", "")

  # Fixed names. Validations on var.users reserve them.
  admin_user = "admin"
  bind_user  = "lldap_bind"
  bind_dn    = "uid=${local.bind_user},ou=people,${var.base_dn}"

  data_device_name = "lldap-data"
  static_ip        = google_compute_address.lldap.address
  ldap_uri         = "ldaps://${local.static_ip}:6360"

  allowed_client_cidrs = var.allowed_client_cidrs != null ? var.allowed_client_cidrs : [data.google_compute_subnetwork.lldap.ip_cidr_range]

  sa_member = "serviceAccount:${google_service_account.lldap.email}"
  # Keyed by list index: emails may be unknown until apply (e.g. created in the same deployment).
  # Without an explicit list, clients are Slurm nodes on the default compute service account.
  client_service_accounts = var.client_service_accounts != null ? var.client_service_accounts : ["${data.google_project.lldap.number}-compute@developer.gserviceaccount.com"]
  client_members          = { for i, email in local.client_service_accounts : tostring(i) => "serviceAccount:${email}" }

  # Users and groups as delivered to the VM. Contains no passwords.
  lldap_config = jsonencode({
    access_group     = var.access_group
    access_group_gid = var.access_group_gid
    cleanup          = var.cleanup_removed_users
    groups           = { for name, g in var.groups : name => { gid = g.gid } }
    users = { for name, u in var.users : name => {
      uid          = u.uid
      gid          = u.primary_group == var.access_group ? var.access_group_gid : var.groups[u.primary_group].gid
      groups       = distinct(u.disabled ? [for g in u.groups : g if g != var.access_group] : concat(u.groups, [var.access_group]))
      shell        = u.shell
      email        = coalesce(u.email, "${name}@noemail.invalid")
      display_name = coalesce(u.display_name, name)
      disabled     = u.disabled
      secret       = google_secret_manager_secret.user_password[name].secret_id
    } }
  })

  server_startup_script = templatefile("${path.module}/templates/server-startup.sh.tftpl", {
    project_id         = var.project_id
    base_dn            = var.base_dn
    lldap_image        = var.lldap_image
    admin_user         = local.admin_user
    bind_user          = local.bind_user
    admin_secret       = google_secret_manager_secret.admin_password.secret_id
    bind_secret        = google_secret_manager_secret.bind_password.secret_id
    ca_secret          = google_secret_manager_secret.ca_cert.secret_id
    static_ip          = local.static_ip
    data_device_name   = local.data_device_name
    enable_backups     = var.enable_backups
    backup_bucket      = var.enable_backups ? google_storage_bucket.backup[0].name : ""
    backup_schedule    = var.backup_schedule
    gcp_helper_py      = file("${path.module}/files/lldap_gcp.py")
    sync_py            = file("${path.module}/files/lldap_sync.py")
    backup_sh          = file("${path.module}/files/lldap-backup.sh")
    lldap_service_unit = file("${path.module}/files/lldap.service")
  })

  client_script = templatefile("${path.module}/templates/client-setup.sh.tftpl", {
    project_id          = var.project_id
    ldap_uri            = local.ldap_uri
    base_dn             = var.base_dn
    bind_dn             = local.bind_dn
    access_group        = var.access_group
    ca_secret           = google_secret_manager_secret.ca_cert.secret_id
    bind_secret         = google_secret_manager_secret.bind_password.secret_id
    secret_wait_seconds = var.client_secret_wait_seconds
  })
}

data "google_project" "lldap" {
  project_id = var.project_id
}

data "google_compute_subnetwork" "lldap" {
  self_link = var.subnetwork_self_link
}

resource "random_id" "suffix" {
  byte_length = 4
}

resource "google_service_account" "lldap" {
  project      = var.project_id
  account_id   = "${local.sa_prefix}-ldap-${random_id.suffix.hex}"
  display_name = "LLDAP server for ${var.deployment_name}"
}

resource "google_compute_address" "lldap" {
  project      = var.project_id
  name         = "${local.name}-ip"
  region       = var.region
  subnetwork   = var.subnetwork_self_link
  address_type = "INTERNAL"
  labels       = local.labels

  lifecycle {
    precondition {
      condition     = data.google_compute_subnetwork.lldap.region == var.region
      error_message = "var.region must be the region of subnetwork_self_link."
    }
  }
}

# Secret containers only. Versions are written by the LLDAP VM, so no secret
# value is ever stored in Terraform state.
resource "google_secret_manager_secret" "admin_password" {
  project   = var.project_id
  secret_id = "${local.name}-admin-password"
  labels    = local.labels
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "bind_password" {
  project   = var.project_id
  secret_id = "${local.name}-bind-password"
  labels    = local.labels
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "ca_cert" {
  project   = var.project_id
  secret_id = "${local.name}-ca-cert"
  labels    = local.labels
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "user_password" {
  for_each  = var.users
  project   = var.project_id
  secret_id = "${local.name}-user-${each.key}"
  labels    = local.labels
  replication {
    auto {}
  }
}

# The VM adds versions to every secret.
resource "google_secret_manager_secret_iam_member" "vm_adder" {
  for_each = merge(
    {
      admin = google_secret_manager_secret.admin_password.id
      bind  = google_secret_manager_secret.bind_password.id
      ca    = google_secret_manager_secret.ca_cert.id
    },
    { for name, s in google_secret_manager_secret.user_password : "user-${name}" => s.id },
  )
  project   = var.project_id
  secret_id = each.value
  role      = "roles/secretmanager.secretVersionAdder"
  member    = local.sa_member
}

# The VM reads back the admin and bind passwords (generated once, reused on
# every boot), and the CA certificate (to publish it only when it changed).
resource "google_secret_manager_secret_iam_member" "vm_accessor" {
  for_each = {
    admin = google_secret_manager_secret.admin_password.id
    bind  = google_secret_manager_secret.bind_password.id
    ca    = google_secret_manager_secret.ca_cert.id
  }
  project   = var.project_id
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = local.sa_member
}

# The VM lists (but cannot read) versions of user secrets, to warn when an
# existing user's secret has no password version.
resource "google_secret_manager_secret_iam_member" "vm_user_viewer" {
  for_each  = google_secret_manager_secret.user_password
  project   = var.project_id
  secret_id = each.value.id
  role      = "roles/secretmanager.viewer"
  member    = local.sa_member
}

# Clients read the CA certificate and the bind password only.
resource "google_secret_manager_secret_iam_member" "client_ca" {
  for_each  = local.client_members
  project   = var.project_id
  secret_id = google_secret_manager_secret.ca_cert.id
  role      = "roles/secretmanager.secretAccessor"
  member    = each.value
}

resource "google_secret_manager_secret_iam_member" "client_bind" {
  for_each  = local.client_members
  project   = var.project_id
  secret_id = google_secret_manager_secret.bind_password.id
  role      = "roles/secretmanager.secretAccessor"
  member    = each.value
}

resource "google_storage_bucket" "backup" {
  count                       = var.enable_backups ? 1 : 0
  project                     = var.project_id
  name                        = lower("${local.name}-backup-${random_id.suffix.hex}")
  location                    = var.region
  labels                      = local.labels
  force_destroy               = false
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      age = var.backup_retention_days
    }
    action {
      type = "Delete"
    }
  }

  # Objects deleted by the rule above become noncurrent in a versioned bucket.
  lifecycle_rule {
    condition {
      days_since_noncurrent_time = var.backup_retention_days
    }
    action {
      type = "Delete"
    }
  }
}

resource "google_storage_bucket_iam_member" "backup_writer" {
  count  = var.enable_backups ? 1 : 0
  bucket = google_storage_bucket.backup[0].name
  role   = "roles/storage.objectCreator"
  member = local.sa_member
}

resource "google_compute_disk" "data" {
  project = var.project_id
  name    = "${local.name}-data"
  zone    = var.zone
  size    = var.data_disk_size_gb
  type    = var.data_disk_type
  labels  = local.labels

  # Protects the database from deletion or replacement by Terraform.
  deletion_policy                = var.data_disk_deletion_policy
  create_snapshot_before_destroy = var.data_disk_snapshot_before_destroy
}

resource "google_compute_resource_policy" "data_snapshots" {
  project = var.project_id
  name    = "${local.name}-snapshots"
  region  = var.region

  snapshot_schedule_policy {
    schedule {
      daily_schedule {
        days_in_cycle = 1
        start_time    = "03:00"
      }
    }
    retention_policy {
      max_retention_days    = var.snapshot_retention_days
      on_source_disk_delete = "KEEP_AUTO_SNAPSHOTS"
    }
    snapshot_properties {
      labels = local.labels
    }
  }
}

resource "google_compute_disk_resource_policy_attachment" "data_snapshots" {
  project = var.project_id
  name    = google_compute_resource_policy.data_snapshots.name
  disk    = google_compute_disk.data.name
  zone    = var.zone
}

resource "google_compute_instance" "lldap" {
  project      = var.project_id
  name         = local.name
  zone         = var.zone
  machine_type = var.machine_type
  labels       = local.labels

  deletion_protection       = var.deletion_protection
  allow_stopping_for_update = true

  boot_disk {
    auto_delete = true
    initialize_params {
      image  = "debian-cloud/debian-12"
      size   = 20
      type   = "pd-balanced"
      labels = local.labels
    }
  }

  # Not auto-deleted: the disk outlives the VM and is managed above.
  attached_disk {
    source      = google_compute_disk.data.id
    device_name = local.data_device_name
  }

  network_interface {
    subnetwork = var.subnetwork_self_link
    network_ip = local.static_ip
    # No access_config: no external IP.
  }

  service_account {
    email  = google_service_account.lldap.email
    scopes = ["https://www.googleapis.com/auth/cloud-platform"]
  }

  shielded_instance_config {
    enable_secure_boot          = true
    enable_vtpm                 = true
    enable_integrity_monitoring = true
  }

  # The "startup-script" and "lldap-config" metadata keys are updated in place.
  # metadata_startup_script is not used because changing it replaces the VM.
  metadata = {
    enable-oslogin = "TRUE"
    startup-script = local.server_startup_script
    lldap-config   = local.lldap_config
  }

  lifecycle {
    # A newer debian-12 image must not replace the VM.
    ignore_changes = [boot_disk[0].initialize_params[0].image]
  }

  depends_on = [
    google_secret_manager_secret_iam_member.vm_adder,
    google_secret_manager_secret_iam_member.vm_accessor,
    google_storage_bucket_iam_member.backup_writer,
  ]
}

resource "google_compute_firewall" "ldaps" {
  project                 = var.project_id
  name                    = "${local.name}-ldaps"
  network                 = var.network_self_link
  direction               = "INGRESS"
  source_ranges           = local.allowed_client_cidrs
  target_service_accounts = [google_service_account.lldap.email]

  allow {
    protocol = "tcp"
    ports    = ["6360"]
  }
}
