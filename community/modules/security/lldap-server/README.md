## Description

This module runs an [LLDAP] server on a small Compute Engine VM and gives
Slurm v6 clusters LDAP users and groups through SSSD. Users and groups are
declared in the blueprint; passwords never pass through Terraform.

It creates:

- a private VM (Debian 12, shielded, no external IP) that runs the LLDAP
  container with podman under systemd, with its own service account and a
  reserved static internal IP address;
- a separate persistent data disk for the LLDAP database, the TLS keys and the
  server secrets, with a daily snapshot schedule;
- Secret Manager secrets (containers only) for the admin password, the
  read-only bind password, the CA certificate, and one secret per user;
- a firewall rule that allows LDAPS (tcp:6360) to the VM from
  `allowed_client_cidrs` (default: the subnetwork's primary range). Firewall
  rules only add access: other allow rules in the VPC, such as the `vpc`
  module's default rule that allows all internal traffic, still apply to the
  VM;
- optionally (`enable_backups`, default true) a versioned GCS bucket that
  receives a daily online backup of the database.

It outputs an LDAP client setup script for Slurm v6 controller, login and
compute nodes, as `runners` and as plain strings.

[LLDAP]: https://github.com/lldap/lldap

### Example

```yaml
  - id: lldap
    source: community/modules/security/lldap-server
    use: [network]
    settings:
      base_dn: dc=example,dc=com
      groups:
        research: { gid: 20001 }
      users:
        alice:
          uid: 30001
          primary_group: research
          email: alice@example.com
          display_name: Alice Example
        bob:
          uid: 30002
          primary_group: research
          disabled: true
      client_service_accounts:
      - $(compute_sa.service_account_email)
      - $(login_sa.service_account_email)
      - $(controller_sa.service_account_email)
```

A complete blueprint is
[community/examples/hpc-slurm-lldap.yaml](../../../examples/hpc-slurm-lldap.yaml).

### Connecting Slurm nodes

The client script must run on the controller, the login nodes and every
nodeset. There are two ways to wire it.

**1. `use` the module directly (simplest).** The module outputs
`startup_script` (an input of the login and nodeset modules) and
`controller_startup_script` (an input of the controller module):

```yaml
  - id: compute_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network, lldap]
  - id: slurm_login
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-login
    use: [network, lldap]
  - id: slurm_controller
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
    use: [network, compute_partition, slurm_login, lldap]
    settings:
      # Waiting for the LLDAP VM and installing SSSD can exceed the 300 second
      # default. Keep lldap's client_secret_wait_seconds well below these.
      controller_startup_scripts_timeout: 900
      login_startup_scripts_timeout: 900
      compute_startup_scripts_timeout: 900
```

and, on the `lldap` module, `client_secret_wait_seconds: 600` (the wait plus
about 200 seconds for the install and the lookup check must stay below 900).

These inputs are strings. When a module `use`s several modules that each
provide the same string input, only the first one in the `use` list is applied
(lists are merged, strings are not). An explicit `startup_script` setting
always wins over `use`. If a node needs other startup scripts too, use the
second pattern.

**2. Combine runners with the `startup-script` module.** The `runners` output
is a list with one shell runner; list inputs from `use` are merged:

```yaml
  - id: node_startup
    source: modules/scripts/startup-script
    use: [lldap]            # adds lldap's runner
    settings:
      runners:
      - type: shell
        destination: other.sh
        content: echo other setup
  - id: compute_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network]
    settings:
      startup_script: $(node_startup.startup_script)
```

Do the same for the login node (`startup_script`) and the controller
(`controller_startup_script`).

The node service accounts must be listed in `client_service_accounts`, so they
can read the CA certificate and the bind password. See
[Granting nodes access](#granting-nodes-access).

### Granting nodes access

Every node that uses the directory reads two secrets at boot with its own
service account: the LLDAP CA certificate (to verify LDAPS) and the read-only
bind password (for SSSD lookups). The module grants
`roles/secretmanager.secretAccessor` on exactly these two secrets, and nothing
else, to the client service accounts. It cannot discover the Slurm nodes'
service accounts by itself, so pick one of two modes:

| Your Slurm modules | `client_service_accounts` |
|---|---|
| No `service_account_email` on the controller, login or any nodeset: all nodes run as the project's default compute service account | leave unset. The module grants `<project-number>-compute@developer.gserviceaccount.com`. |
| `service_account_email` set on any of them (for example one per role, or one per partition) | set it, and list **every** client service account: the controller, every login group and every nodeset/partition. Setting it replaces the default, so a node still on the default account must be listed too. |

```yaml
# Mode 2 example: one account per role and per partition.
client_service_accounts:
- $(controller_sa.service_account_email)
- $(login_sa.service_account_email)
- $(compute_sa.service_account_email)   # partition "compute"
- $(gpu_sa.service_account_email)       # partition "gpu"
```

- List each distinct account once. If several roles share one account, list it
  once.
- Compute nodes need access too: jobs run as LDAP users, so compute nodes must
  resolve them.
- When set, the list must not be empty and must hold service account emails
  (`…@….gserviceaccount.com`); the module rejects anything else at plan time.
  Terraform cannot check that the list matches the nodes' actual accounts.
- A node whose account is missing waits `client_secret_wait_seconds` and then
  fails its setup with: `could not read secrets … Check that the LLDAP VM
  finished its first boot and that this VM's service account is in
  client_service_accounts.` Add the account and apply. Then re-run the client
  setup on that node: Slurm-GCP runs custom scripts only on a node's first
  boot (a reboot or `instances reset` skips them with `Slurm was previously
  configured, quitting`), so either run the installed script as root, for
  example `/slurm/custom_scripts/nodeset.d/<nodeset>/ghpc_nodeset_startup.sh`
  on a compute node (`login.d` and `controller.d` for the other roles), or
  recreate the node. Slurm replaces dynamic compute nodes on their own.
- The per-secret binding is the only access the module grants. If the client
  account also has project-level roles that include Secret Manager access
  (`roles/owner`, `roles/editor`, `roles/secretmanager.*`), it can read every
  secret in the project, including the LLDAP admin and user password secrets.
  The default compute service account often holds `roles/editor` or more, so
  for real deployments prefer dedicated node service accounts without such
  roles (as the example does).
- Clients can never read the admin password or user password secrets.

The client script supports Rocky Linux 8 and 9, Debian 12, and Ubuntu 22.04
and 24.04. On any other OS it exits with an error. On every boot it:

- waits up to `client_secret_wait_seconds` for the CA and bind password
  secrets. The LLDAP VM publishes the CA certificate early, and the bind
  password only once the bind user exists, is in `lldap_strict_readonly` and
  can log in with it, so a client never gets a bind password that does not
  work yet;
- installs SSSD (`dnf` or `apt-get`) and configures PAM with
  `authinfo_unavail=ignore` on the `pam_sss` account line (see
  [Troubleshooting: sssd down](#troubleshooting-sssd-down)): on Rocky through
  a custom authselect profile `custom/lldap` selected with `with-mkhomedir`; on
  Debian and Ubuntu through the pam-auth-update profiles `lldap-sss` and
  `lldap-mkhomedir`, which replace `sss` and `mkhomedir`;
- creates home directories at first login with mode **0700** on every
  supported OS: on Debian and Ubuntu, `lldap-mkhomedir` runs
  `pam_mkhomedir.so umask=0077`; on Rocky, oddjob-mkhomedir uses `HOME_MODE`
  from `/etc/login.defs`, which is 0700 on Rocky 8 and 9 (the script warns if
  it is not). Existing home directories are not changed;
- keeps every existing source on the `passwd` and `group` lines of
  `/etc/nsswitch.conf` (for example `slurm`, `cache_oslogin`, `oslogin`) and
  adds `sss`. On Rocky, `authselect select` rewrites the file, so the script
  records the sources before and restores them after, and fails if any is
  missing. `authselect check` then reports the file as modified; this is
  expected;
- installs the CA in the OS trust store and writes `/etc/sssd/sssd.conf` with
  `access_provider = ldap` and
  `ldap_access_filter = memberOf=cn=<access_group>,ou=groups,<base_dn>`;
- checks that LDAP lookups work (`getent group <access_group>`). If they do
  not, it restarts `sssd` and checks again, up to 5 restarts 10 seconds apart.
  If lookups still fail, the script exits with "LDAP lookups through sssd
  failed after 5 restarts; the cluster's LLDAP integration is not working",
  and Slurm reports the node's startup script as failed;
- on login nodes only (`slurm_instance_role` metadata is `login`), enables
  `PasswordAuthentication` for SSH through `/etc/ssh/sshd_config.d/00-lldap.conf`,
  or at the top of `/etc/ssh/sshd_config` when that file has no early
  `Include` of the drop-in directory (Rocky 8).

Slurm runs each startup script with a timeout of 300 seconds by default; if the
client script takes longer, the node's Slurm setup fails. The script's worst
case is roughly:

| Step | Time |
| ---- | ---- |
| wait for the secrets (`client_secret_wait_seconds`, first deployment only) | 90 s (default) |
| lookup check: six lookups of up to 5 s, five sssd restarts with 10 s pauses | about 95 s |
| PAM, CA and sssd configuration | about 15 s |
| SSSD package install (`dnf` or `apt-get`), first boot only | the rest, about 95 s |

On a first deployment the LLDAP VM usually needs longer than 90 seconds to
publish its secrets. Raise `controller_startup_scripts_timeout`,
`login_startup_scripts_timeout` and `compute_startup_scripts_timeout` on the
controller and `client_secret_wait_seconds` together, keeping the wait plus
about 200 seconds below the timeouts. The example blueprint uses timeouts of
900 seconds and `client_secret_wait_seconds: 600`.

### Network requirements

The LLDAP VM has no external IP. It needs outbound access to install podman
and sqlite3 from the Debian mirrors and to pull `lldap_image` (ghcr.io by
default), for example through Cloud NAT (the `vpc` module creates one by
default), and access to Secret Manager and Cloud Storage APIs. Clients need
the same for SSSD packages and Secret Manager.

### Security model

- **Logins.** Only members of `access_group` (default `cluster_users`) can log
  in as LDAP users. Every enabled user is added to it; `disabled = true`
  removes the user from it. Disabled users still resolve with `id` and
  `getent`, so file ownership stays readable.
- **Other accounts.** SSSD's access filter applies to LDAP users only. OS
  Login, metadata SSH keys and local accounts are not affected, also while
  `sssd` is down (see below). UIDs and GIDs must be >= 10000 and SSSD ignores
  lower IDs (`min_id = 10000`); keep them clear of OS Login UIDs.
- **`authinfo_unavail=ignore` and `memcache_timeout = 0`.** By default the
  `pam_sss` account line is
  `account [default=bad success=ok user_unknown=ignore] pam_sss.so`. When
  `sssd` is stopped, `pam_sss` can return `PAM_AUTHINFO_UNAVAIL`, which
  `default=bad` turns into a denial for every user that reaches that line,
  including OS Login admins. The module adds `authinfo_unavail=ignore`, so that
  result is skipped. To keep this from letting LDAP users in while `sssd` is
  down (for example a disabled user with their own `~/.ssh/authorized_keys`),
  `sssd.conf` sets `memcache_timeout = 0` in `[nss]`: without SSSD's in-memory
  cache, LDAP users stop resolving as soon as `sssd` stops, and sshd refuses
  unknown users. Upstream sssd.conf(5) warns that disabling the in-memory cache
  "will have significant negative impact on SSSD's performance"; lookups then
  always go to the `sssd` daemon (which keeps its own cache).
- **LDAPS only.** Clients use LDAPS on port 6360 with the module's private CA
  (`ldap_tls_reqcert = demand`). Plain LDAP (3890) and the web UI (17170)
  listen on the VM's loopback interface only. The module's firewall rule adds
  access to 6360 from `allowed_client_cidrs`; it does not block other sources
  that other allow rules in the VPC already admit.
- **Shell access to the LLDAP VM.** Grant OS Login access to the LLDAP VM
  (for example `roles/compute.osAdminLogin`) only to LDAP administrators.
  While the sync service sets a new user's password, the admin JWT is briefly
  on the command line of a process inside the container, and `/proc` on the VM
  is not mounted with `hidepid`, so any local user could read it.
- **Bind user.** SSSD binds as `lldap_bind`, a member of LLDAP's
  `lldap_strict_readonly` group. Its password is readable by
  `client_service_accounts` and stored on clients in `/etc/sssd/sssd.conf`
  (mode 0600).
- **No secrets in Terraform state.** Terraform creates empty secrets. The VM
  generates the admin and bind passwords, the CA and the server certificate on
  first boot and adds them as secret versions. The VM's service account can
  add versions to every secret, read back the admin, bind and CA secrets, and
  list (but not read) the versions of user secrets.
  The `lldap-config` metadata contains no passwords.
- **Admin password.** The LLDAP admin (`admin`) password is reset to the
  Secret Manager value every time LLDAP starts.
- **On the VM**, `/var/lib/lldap` is mode 0700. It holds the database, the
  LLDAP key seed and JWT secret, and the CA private key.

### Password handover

A new user gets a random 32-character password. It is stored in that user's
secret only after the account exists, has that password and has its group
memberships, so a password in the secret always works. Send it to the user through your usual channel:

```shell
gcloud secrets versions access latest \
  --secret=<deployment_name>-lldap-user-<username> --project=<project>
```

(The deployment name is truncated to 40 characters in resource names.)

The output `user_password_secret_ids` maps usernames to secret IDs. Users can
change their password with `passwd` on a login node (SSSD `chpass_provider =
ldap`). The module never resets the password of an existing user, so their own
change is kept, but the secret keeps showing the initial password.

The admin password for the LLDAP web UI is in the secret
`admin_password_secret_id`. The UI listens on the VM's loopback interface; reach
it through an IAP tunnel, for example
`gcloud compute ssh <vm> --tunnel-through-iap -- -L 17170:127.0.0.1:17170`.

### Managing users

Users and groups are applied by the `lldap-sync` service on the VM. It watches
the `lldap-config` metadata key and applies changes within seconds, without
recreating the VM. It also re-applies the configuration every hour, which
undoes manual changes to the groups it manages.

- **Add a user:** add them to `users` and redeploy. Other users' passwords are
  not changed.
- **Disable a user:** set `disabled = true`. They are removed from
  `access_group`. Running jobs are not cancelled.
- **Remove a user:** delete them from `users`. With
  `cleanup_removed_users = false` (default) the account stays in LLDAP but is
  removed from `access_group`. With `cleanup_removed_users = true`, users and
  groups not in the blueprint are deleted. The admin user, the bind user,
  members of `lldap_admin` and LLDAP's built-in groups are never deleted.
  Removing a user also deletes their password secret.
- Group memberships are only managed for the groups declared in the
  blueprint and `access_group`; memberships in other groups, for example ones
  added in the web UI, are left alone.
- When `email` is not set, `<username>@noemail.invalid` is stored, because
  LLDAP requires a unique email address for every user.

- If a user is removed from the blueprint and added again while the account
  stays in LLDAP (`cleanup_removed_users = false`), Terraform creates a new,
  empty password secret, but the existing account keeps its old password,
  which is never reset. The sync service logs a warning for such users. Reset
  the password in the web UI or with `passwd` if needed.
- An error in one user or group is logged and the others are still applied;
  the sync then retries every 60 seconds until all entries succeed.

Sync logs: `sudo journalctl -u lldap-sync` on the VM.

Changes to `users`, `groups`, `access_group`, `access_group_gid` and
`cleanup_removed_users` apply live. Changes to `lldap_image`, `backup_schedule`
or `enable_backups` (and module upgrades that change the startup script) take
effect only after the VM reboots or after
`sudo google_metadata_script_runner startup` on the VM.

### Data safety and destroying the deployment

- The VM has `deletion_protection = true` by default.
- The data disk has `deletion_policy = "PREVENT"` by default
  (`data_disk_deletion_policy`): any plan that would delete or replace it fails.
  This needs google provider 7.33.0 or later. With
  `data_disk_snapshot_before_destroy = true` (default) the provider also takes
  a snapshot of the disk if it is deleted.
- A snapshot schedule takes daily snapshots of the data disk and keeps them for
  `snapshot_retention_days`. Snapshots are kept when the disk is deleted.
- Changing users, groups or the startup script updates the VM metadata in
  place. A new `debian-12` image does not replace the VM.

To destroy the deployment:

1. Set `deletion_protection: false` and `data_disk_deletion_policy: DELETE`
   and run `gcluster deploy`.
2. Empty the backup bucket (versioned):
   `gcloud storage rm --recursive --all-versions gs://<bucket>/**`.
3. Run `gcluster destroy`.
4. Delete the remaining disk snapshots if you no longer need them.

### Backup and restore

With `enable_backups = true`, the `lldap-backup.timer` systemd timer runs on
`backup_schedule`. It takes an online SQLite backup of the database, checks it
with `PRAGMA integrity_check`, and uploads a `tar.gz` with the database, the
key seed, the JWT secret and the TLS directory to
`gs://<bucket>/lldap/lldap-<timestamp>.tar.gz`. Objects are deleted after
`backup_retention_days`. The archive contains secret material: limit access to
the bucket. The VM's service account can only create objects in it.

To restore from a backup (an operator with read access to the bucket):

1. Copy the archive to the VM, for example with `gcloud storage cp` and
   `gcloud compute scp --tunnel-through-iap`.
2. On the VM, stop the services and restore the files:

   ```shell
   sudo systemctl stop lldap-sync lldap
   mkdir /tmp/restore && tar -xzf lldap-<timestamp>.tar.gz -C /tmp/restore
   sudo rm -f /var/lib/lldap/data/users.db-wal /var/lib/lldap/data/users.db-shm
   sudo install -m 0600 /tmp/restore/users.db /var/lib/lldap/data/users.db
   sudo install -m 0600 /tmp/restore/key_seed /tmp/restore/jwt_secret /var/lib/lldap/secrets/
   sudo cp -a /tmp/restore/tls/. /var/lib/lldap/tls/
   sudo google_metadata_script_runner startup
   ```

The startup script restarts LLDAP, resets the admin password to the Secret
Manager value, publishes the restored CA certificate if it differs, and the
sync service re-applies the blueprint. User passwords are restored with the
database. To restore from a snapshot instead, create a disk from the snapshot,
attach it to the VM, and copy the same files from it.

### Troubleshooting: sssd down

While `sssd` is stopped or failing on a node:

- OS Login users (including OS Login admins) can still SSH in and use `sudo`.
  The `pam_sss` account check is skipped for them (`authinfo_unavail=ignore`).
- LDAP users cannot log in and do not resolve with `id` or `getent`: there is
  no in-memory cache (`memcache_timeout = 0`), and password authentication
  needs `sssd`. Files they own show numeric UIDs.

To recover, log in as an OS Login admin and run
`sudo systemctl restart sssd`; check `sudo journalctl -u sssd` and
`sudo sssctl config-check`.

### PAM maintenance notes

- **Rocky:** the script creates the custom authselect profile
  `/etc/authselect/custom/lldap` once (`authselect create-profile lldap -b
  sssd`), patches the `pam_sss` account line in its `system-auth`,
  `password-auth`, `smartcard-auth` and `fingerprint-auth`, and selects
  `custom/lldap` with the features of the previous `sssd` or `custom/lldap`
  selection plus `with-mkhomedir`. A custom profile is a copy: authselect
  package updates to the `sssd` profile are not picked up. To refresh it,
  delete `/etc/authselect/custom/lldap` and re-run the startup scripts; the
  script recreates and reselects it.
- **Debian and Ubuntu:** the script writes `/usr/share/pam-configs/lldap-sss`,
  a copy of libpam-sss's `sss` profile with a different `Name:`, the extended
  account control and `Conflicts: sss`, and runs
  `pam-auth-update --disable sss mkhomedir --enable lldap-sss lldap-mkhomedir`.
  `/usr/share/pam-configs/lldap-mkhomedir` is a copy of the stock `mkhomedir`
  profile with `pam_mkhomedir.so umask=0077` and `Conflicts: mkhomedir`.
  Both copies are rebuilt from the stock profiles on every boot, so package
  updates are picked up then. pam-auth-update records `sss` and `mkhomedir` as
  seen and does not re-enable them on package upgrades. pam-auth-update does not overwrite locally modified
  `/etc/pam.d/common-*` files; the script then fails with an error.
- On both, the script fails if the resulting PAM files do not contain exactly
  the expected `pam_sss` account line.

### Known gaps

- Users are not forced to change their initial password at first login.
- Disabling a user does not cancel their running or queued jobs.
- A user's `shell` must be installed on the nodes; it is not installed for
  them.
- The bind password is reset to the Secret Manager value whenever the sync
  service finds that it no longer works.
- During the first deployment, nodes can boot before the LLDAP VM is ready;
  they wait for `client_secret_wait_seconds`, and their startup script fails
  if LDAP lookups do not work after 5 sssd restarts.

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.33.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | >= 3.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.33.0 |
| <a name="provider_random"></a> [random](#provider\_random) | >= 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_address.lldap](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_address) | resource |
| [google_compute_disk.data](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk) | resource |
| [google_compute_disk_resource_policy_attachment.data_snapshots](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk_resource_policy_attachment) | resource |
| [google_compute_firewall.ldaps](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_firewall) | resource |
| [google_compute_instance.lldap](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance) | resource |
| [google_compute_resource_policy.data_snapshots](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_resource_policy) | resource |
| [google_secret_manager_secret.admin_password](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret) | resource |
| [google_secret_manager_secret.bind_password](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret) | resource |
| [google_secret_manager_secret.ca_cert](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret) | resource |
| [google_secret_manager_secret.user_password](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret) | resource |
| [google_secret_manager_secret_iam_member.client_bind](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_iam_member) | resource |
| [google_secret_manager_secret_iam_member.client_ca](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_iam_member) | resource |
| [google_secret_manager_secret_iam_member.vm_accessor](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_iam_member) | resource |
| [google_secret_manager_secret_iam_member.vm_adder](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_iam_member) | resource |
| [google_secret_manager_secret_iam_member.vm_user_viewer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_iam_member) | resource |
| [google_service_account.lldap](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_account) | resource |
| [google_storage_bucket.backup](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket) | resource |
| [google_storage_bucket_iam_member.backup_writer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_iam_member) | resource |
| [random_id.suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [google_compute_subnetwork.lldap](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork) | data source |
| [google_project.lldap](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/project) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_access_group"></a> [access\_group](#input\_access\_group) | LDAP group whose members may log in to the cluster (SSSD ldap\_access\_filter). Every enabled user is added to it. | `string` | `"cluster_users"` | no |
| <a name="input_access_group_gid"></a> [access\_group\_gid](#input\_access\_group\_gid) | GID of `access_group`. An integer >= 10000, different from every gid in `groups`. | `number` | `10000` | no |
| <a name="input_allowed_client_cidrs"></a> [allowed\_client\_cidrs](#input\_allowed\_client\_cidrs) | Source CIDR ranges allowed to reach LDAPS (tcp:6360). Defaults to the primary range of `subnetwork_self_link`. | `list(string)` | `null` | no |
| <a name="input_backup_retention_days"></a> [backup\_retention\_days](#input\_backup\_retention\_days) | Days to keep backup objects in the GCS bucket (lifecycle rule). Noncurrent versions are deleted after the same number of days. | `number` | `30` | no |
| <a name="input_backup_schedule"></a> [backup\_schedule](#input\_backup\_schedule) | systemd OnCalendar expression for the GCS backup timer (VM local time, UTC on Debian GCE images). | `string` | `"*-*-* 02:30:00"` | no |
| <a name="input_base_dn"></a> [base\_dn](#input\_base\_dn) | LDAP base DN, for example `dc=example,dc=com`. Only `dc=` components are accepted. | `string` | n/a | yes |
| <a name="input_cleanup_removed_users"></a> [cleanup\_removed\_users](#input\_cleanup\_removed\_users) | When true, users and groups that exist in LLDAP but are not in `users`/`groups`<br/>are deleted on the next sync. The admin user, the bind user, members of<br/>lldap\_admin and LLDAP's built-in groups are never deleted.<br/>When false (default), such users are only removed from `access_group`, so they can no longer log in. | `bool` | `false` | no |
| <a name="input_client_secret_wait_seconds"></a> [client\_secret\_wait\_seconds](#input\_client\_secret\_wait\_seconds) | How long the client script waits for the CA and bind password secrets to get<br/>their first version (written by the LLDAP VM on its first boot). The wait,<br/>the SSSD package install, and the lookup check (up to about 95 seconds: six<br/>5-second lookups and five sssd restarts with 10-second pauses) must together<br/>stay below the Slurm `*_startup_scripts_timeout` values (default 300<br/>seconds), or the node's Slurm setup fails. The default of 90 leaves about<br/>95 seconds for the install. For a first deployment, raise this and the<br/>Slurm timeouts together. | `number` | `90` | no |
| <a name="input_client_service_accounts"></a> [client\_service\_accounts](#input\_client\_service\_accounts) | Service account emails of every VM that uses this directory. Each gets<br/>secretAccessor on the CA certificate and bind password secrets only, which<br/>the client script reads at boot.<br/>- Not set (null): the project's default compute service account,<br/>  `<project-number>-compute@developer.gserviceaccount.com`, which Slurm v6<br/>  controller, login and compute nodes use when their<br/>  `service_account_email` is not set.<br/>- Set: it replaces the default and must list the service account of every<br/>  client: controller, login nodes and every nodeset/partition (one per<br/>  partition is fine). A node whose account is missing cannot join LDAP.<br/>See "Granting nodes access" in the README. | `list(string)` | `null` | no |
| <a name="input_data_disk_deletion_policy"></a> [data\_disk\_deletion\_policy](#input\_data\_disk\_deletion\_policy) | Terraform deletion policy of the data disk (google\_compute\_disk.deletion\_policy).<br/>"PREVENT" (default) makes any plan that would delete or replace the disk fail.<br/>Set to "DELETE" and apply before `gcluster destroy`, or "ABANDON" to keep the<br/>disk in GCP while removing it from Terraform state. | `string` | `"PREVENT"` | no |
| <a name="input_data_disk_size_gb"></a> [data\_disk\_size\_gb](#input\_data\_disk\_size\_gb) | Size in GB of the persistent data disk that holds the LLDAP database, TLS keys and server secrets. | `number` | `10` | no |
| <a name="input_data_disk_snapshot_before_destroy"></a> [data\_disk\_snapshot\_before\_destroy](#input\_data\_disk\_snapshot\_before\_destroy) | Create a snapshot of the data disk when Terraform deletes it (google\_compute\_disk.create\_snapshot\_before\_destroy). The snapshot is not managed by Terraform and must be deleted manually. | `bool` | `true` | no |
| <a name="input_data_disk_type"></a> [data\_disk\_type](#input\_data\_disk\_type) | Type of the LLDAP data disk. | `string` | `"pd-balanced"` | no |
| <a name="input_deletion_protection"></a> [deletion\_protection](#input\_deletion\_protection) | Enable deletion protection on the LLDAP VM. Set to false (and apply) before destroying the deployment. | `bool` | `true` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, used as a prefix for resource names. | `string` | n/a | yes |
| <a name="input_enable_backups"></a> [enable\_backups](#input\_enable\_backups) | Create a GCS bucket and a systemd timer that uploads an online SQLite backup of the LLDAP database, with its key seed and TLS material. | `bool` | `true` | no |
| <a name="input_groups"></a> [groups](#input\_groups) | POSIX groups to create in LLDAP, as a map of group name to `{ gid }`.<br/>The access group (`access_group`) is created separately and must not be listed here.<br/>Names must match `^[a-z_][a-z0-9_-]{0,31}$`; GIDs must be unique integers >= 10000. | <pre>map(object({<br/>    gid = number<br/>  }))</pre> | `{}` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the resources created by this module. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_lldap_image"></a> [lldap\_image](#input\_lldap\_image) | LLDAP container image. Pin it by digest. The default is the upstream<br/>v0.6.3 release (multi-arch index digest, identical on ghcr.io and Docker Hub).<br/>The image must provide /app/lldap\_set\_password. | `string` | `"ghcr.io/lldap/lldap:v0.6.3@sha256:2a8454b668c1aba7157e832eab0e242e1e7eb5fb7591d7e7774ba05286511ca8"` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type of the LLDAP VM. | `string` | `"e2-small"` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | Self link of the VPC network for the LLDAP VM and the LDAPS firewall rule. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the LLDAP server and its secrets, bucket and firewall rule are created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region of the static internal IP address and the snapshot schedule. Must be the region of `subnetwork_self_link`. | `string` | n/a | yes |
| <a name="input_snapshot_retention_days"></a> [snapshot\_retention\_days](#input\_snapshot\_retention\_days) | Days to keep the daily snapshots of the data disk. | `number` | `14` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | Self link of the subnetwork for the LLDAP VM. Its primary range is the default for `allowed_client_cidrs`. | `string` | n/a | yes |
| <a name="input_users"></a> [users](#input\_users) | LDAP users, as a map of username to settings:<br/>- `uid`: POSIX UID, a unique integer >= 10000.<br/>- `primary_group`: a key of `groups`, or `access_group`. Sets the user's gidNumber.<br/>- `groups`: extra groups (keys of `groups`, or `access_group`). Default `[]`.<br/>- `shell`: login shell. Default `/bin/bash`. Must be one of /bin/bash,<br/>  /usr/bin/bash, /bin/sh, /usr/bin/sh, /bin/zsh, /usr/bin/zsh, /bin/tcsh,<br/>  /usr/bin/tcsh, /bin/ksh, /usr/bin/ksh.<br/>- `email`: optional, at most 254 characters. Must be unique. When unset,<br/>  `<username>@noemail.invalid` is stored, because LLDAP requires a unique email.<br/>- `display_name`: optional, at most 64 characters, no control characters or ':'.<br/>- `disabled`: when true, the user is kept but removed from `access_group`, so<br/>  they still resolve but cannot log in. Default false.<br/>Every enabled user is a member of `access_group`. Passwords are never set here:<br/>a new user gets a random password in their own Secret Manager secret<br/>(see output `user_password_secret_ids`). | <pre>map(object({<br/>    uid           = number<br/>    primary_group = string<br/>    groups        = optional(list(string), [])<br/>    shell         = optional(string, "/bin/bash")<br/>    email         = optional(string)<br/>    display_name  = optional(string)<br/>    disabled      = optional(bool, false)<br/>  }))</pre> | `{}` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone in which the LLDAP VM and its data disk are created. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_admin_password_secret_id"></a> [admin\_password\_secret\_id](#output\_admin\_password\_secret\_id) | Secret Manager secret holding the LLDAP admin password (user `admin`). |
| <a name="output_base_dn"></a> [base\_dn](#output\_base\_dn) | LDAP base DN. |
| <a name="output_bind_dn"></a> [bind\_dn](#output\_bind\_dn) | DN of the read-only bind user used by SSSD. |
| <a name="output_bind_password_secret_id"></a> [bind\_password\_secret\_id](#output\_bind\_password\_secret\_id) | Secret Manager secret holding the bind user's password. |
| <a name="output_ca_cert_secret_id"></a> [ca\_cert\_secret\_id](#output\_ca\_cert\_secret\_id) | Secret Manager secret holding the PEM CA certificate of the LDAPS server. |
| <a name="output_controller_startup_script"></a> [controller\_startup\_script](#output\_controller\_startup\_script) | The LDAP client setup script as a string, for the `controller_startup_script` input of the Slurm v6 controller module. |
| <a name="output_ldap_uri"></a> [ldap\_uri](#output\_ldap\_uri) | LDAPS URI of the LLDAP server (static internal IP, port 6360). |
| <a name="output_lldap_backup_bucket"></a> [lldap\_backup\_bucket](#output\_lldap\_backup\_bucket) | Name of the GCS bucket holding database backups, or null when backups are disabled. |
| <a name="output_lldap_instance_name"></a> [lldap\_instance\_name](#output\_lldap\_instance\_name) | Name of the LLDAP VM. |
| <a name="output_lldap_service_account_email"></a> [lldap\_service\_account\_email](#output\_lldap\_service\_account\_email) | Email of the service account created for the LLDAP VM. |
| <a name="output_runners"></a> [runners](#output\_runners) | A list with one shell runner that configures an LDAP client (SSSD). Use it with the startup-script module. |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | The LDAP client setup script as a string, for the `startup_script` input of the Slurm v6 login and nodeset modules. |
| <a name="output_user_password_secret_ids"></a> [user\_password\_secret\_ids](#output\_user\_password\_secret\_ids) | Map of username to the Secret Manager secret holding that user's initial password. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
