## Description

This module creates a [Cloud SQL](https://cloud.google.com/sql/) instance
(MySQL or PostgreSQL), a database and a user, reachable only over private
networking. It can also adopt an existing instance.

The generated password is stored in a Secret Manager secret, never in
Terraform state, a blueprint or a startup script, and consumers read it at run
time.

### Example

```yaml
- id: network
  source: modules/network/vpc

- id: private_service_access
  source: modules/network/private-service-access
  use: [network]
  settings:
    deletion_policy: ABANDON

- id: app_sa
  source: modules/project/service-account
  settings:
    name: app
    project_roles:
    - logging.logWriter

- id: sql
  source: community/modules/database/cloudsql
  use: [network, private_service_access, app_sa]
  settings:
    database_name: appdb
    tier: db-f1-micro
```

This creates a private instance, a database, a user and a secret holding the
generated password. Using `app_sa` grants that service account read access to
the secret; the account can be created in the same deployment.

### Connectivity

The instance never has a public IP. Choose one of:

- **Private Service Access** (default): set `network_id` and
  `private_vpc_connection_peering`, typically through `use` of the `vpc` and
  `private-service-access` modules.
- **Private Service Connect**: set `use_psc_connection = true` and
  `subnetwork_self_link` (required, checked at plan time); the module creates
  the PSC endpoint address and forwarding rule in that subnetwork.
  `network_id` is not needed. `psc_allowed_consumer_projects` lists the
  projects allowed to connect (default: `project_id` only).

Set `deletion_policy: ABANDON` on `private-service-access`, as above.
Otherwise `terraform destroy` can fail with "Producer services ... are still
using this connection" for some time after the instance is deleted. To unblock
an existing deployment, run
`gcloud compute networks peerings delete servicenetworking-googleapis-com --network=<NETWORK> --project=<PROJECT>`
and destroy again.

### Production defaults

A created instance defaults to:

- `deletion_protection = true` and `api_deletion_protection = true`: the
  first stops Terraform destroying the instance, the second blocks deletion
  through the Cloud SQL API and console. Set both to `false` and apply before
  destroying the instance. Both are checked only when the instance itself is
  deleted, after Terraform has removed the database, user, secret and PSC
  endpoint, so while either is on the module also fails a destroy before it
  removes anything. That check runs as a destroy-time `local-exec`, so the
  machine running Terraform needs `sh`. The database and user also default to
  `ABANDON` while either is on.
- `enable_backups = true` with `point_in_time_recovery = true`, and
  `retain_backups_on_delete = true`, so automated backups outlive the instance
  (they stay billed until they expire). `backup_start_time`,
  `retained_backups` and `final_backup_retention_days` tune them.
- `ssl_mode = "ENCRYPTED_ONLY"`: clients must connect over TLS, for example
  `mysql --ssl` or `psql "sslmode=require"`. The module does not create
  client certificates, so with `TRUSTED_CLIENT_CERTIFICATE_REQUIRED` issue
  them yourself, for example with `gcloud sql ssl client-certs create`.
- `set_root_password = true`: the built-in `root` (MySQL) or `postgres`
  (PostgreSQL) user gets a random password that is never stored, so it cannot
  be used without one. Reset it with `gcloud sql users set-password` if needed.
  It is only read at creation, so turning it off later leaves the password set.
- `availability_type = "ZONAL"`: set `REGIONAL` for a high-availability
  instance with a standby in another zone (about twice the cost). A REGIONAL
  MySQL instance needs `enable_backups` and `point_in_time_recovery`.

### Adopting an existing instance

Set `existing_instance_name` to create only the database, user and secret on
an existing instance. Inputs that shape a new instance, such as `tier`,
`database_version` and `network_id`, are ignored (the variable's description
lists them all). The instance must have a private IP address, and adoption
cannot be combined with `use_psc_connection`; both are checked at plan time.
Destroying the module leaves the adopted instance's database in place
(`database_deletion_policy` defaults to `ABANDON` when adopting).

To adopt an instance created by another `cloudsql` module in the same
deployment, set that module's `sql_instance_name` and pass its
`instance_name` output. A literal name does not work in that case: it is read
at plan time, before the instance exists. Modules sharing an instance must use
different `sql_username` values, because the user and the secret ID
(`<instance>-<sql_username>-password`) are derived from it.

### Credentials

- The password is generated ephemerally and written only to the secret and the
  database user, through write-only arguments. It is not stored in state, plan
  files or outputs.
- The user's password is read back from the secret, so the two always match,
  including after an interrupted apply or a replaced secret.
- Accounts in `password_secret_accessors`, or from `use` of
  `modules/project/service-account` (the `service_account_iam_email` input),
  get `roles/secretmanager.secretAccessor` on that secret only. That role is
  all a consumer needs to read it, for example with
  `gcloud secrets versions access latest --secret=<SECRET_ID>`.
- The identity running Terraform needs `secretmanager.secrets.setIamPolicy`,
  and `secretmanager.versions.access` because every plan and apply reads the
  secret version back. With data-access audit logs enabled, each plan writes an
  audit entry.
- A `sql_password` you set yourself is stored in your blueprint, the
  deployment folder and the saved plan file, and changing it has no effect
  until you also increase `password_version`. Leave it unset to keep the
  password off disk.

PostgreSQL users, on created or adopted instances, default to
`user_deletion_policy = ABANDON`, because a user that owns objects cannot be
dropped and destroy would fail. For an adopted instance the engine is read from
the instance, not from `database_version`.

#### Rotating the password

Increase `password_version` and apply. The module replaces the secret version
(the old one is disabled, not deleted) and updates the user in the same apply.
Until the apply finishes, clients may briefly be unable to read the secret, or
read a password the user does not have yet, and should retry. An interrupted
rotation completes on the next apply. Changing `user_managed_replication`
replaces the secret, which also generates a new password.

Each rotation leaves the previous version disabled, so it can be re-enabled if
an apply fails partway. Disabled versions are still billed. Once the new
password works, destroy them with
`gcloud secrets versions destroy <VERSION> --secret=<SECRET_ID>`; Terraform
only tracks the current version.

#### Recovering from an unreadable secret version

A plan fails if the current version was disabled or destroyed outside
Terraform, or if the Terraform identity lost `secretmanager.versions.access`.
Restore the access, or run
`gcloud secrets versions enable <VERSION> --secret=<SECRET_ID>`. If the version
cannot be restored, increase `password_version`: the plan then replaces the
version instead of reading it.

### Customer-managed encryption keys

Set `encryption_key_name` to encrypt the instance, and `kms_key_name` in
`user_managed_replication` to encrypt the secret; `encryption_key_name` does
not apply to the secret, and an automatically replicated secret uses
Google-managed encryption. Grant the key to the Cloud
SQL and Secret Manager service agents, for example with `kms-key-iam`'s
`service_agents: [cloudsql, secretmanager]`. In a project that has never used
these services, create the agents first:

```shell
gcloud beta services identity create --service=sqladmin.googleapis.com --project=PROJECT_ID
gcloud beta services identity create --service=secretmanager.googleapis.com --project=PROJECT_ID
```

### Required APIs

The metadata lists `sqladmin`, `servicenetworking` (Private Service Access),
`secretmanager` (the secret) and `compute` (the PSC endpoint). The
`test_apis_enabled` validator requires all four, even for a deployment that
does not call some of them (PSC does not use `servicenetworking`, Private
Service Access does not use `compute`); enable them, or skip that validator.

### Testing

Static coverage is in the `cloudsql-*.yaml` blueprints described in the
[test configs README](../../../../tools/validate_configs/test_configs/README.md).
The daily integration test
[cloudsql-database](../../../../tools/cloud-build/daily-tests/tests/cloudsql-database.yml)
deploys a MySQL instance over Private Service Access, a second module
adopting it, and a PostgreSQL instance over Private Service Connect. A client
VM reads each secret with its own service account and logs in over TLS. The
test also checks that unencrypted and root logins are refused and that no
password is in Terraform state, then rotates the MySQL password by
re-applying with a higher `password_version`.

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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.13.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | >= 3.7.1, < 4.0.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.13.0 |
| <a name="provider_random"></a> [random](#provider\_random) | >= 3.7.1, < 4.0.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_address.psc](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_address) | resource |
| [google_compute_forwarding_rule.psc_consumer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_forwarding_rule) | resource |
| [google_secret_manager_secret.password](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret) | resource |
| [google_secret_manager_secret_iam_member.password_accessor](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_iam_member) | resource |
| [google_secret_manager_secret_version.password](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_version) | resource |
| [google_sql_database.database](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_database) | resource |
| [google_sql_database_instance.instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_database_instance) | resource |
| [google_sql_user.users](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_user) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [terraform_data.destroy_guard](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [google_sql_database_instance.existing](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/sql_database_instance) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_api_deletion_protection"></a> [api\_deletion\_protection](#input\_api\_deletion\_protection) | Also block deletion of the instance through the Cloud SQL API and console (deletion\_protection\_enabled). Set false, and apply, before destroying the instance. | `bool` | `true` | no |
| <a name="input_availability_type"></a> [availability\_type](#input\_availability\_type) | ZONAL, or REGIONAL for a high-availability instance with a standby in another zone. Set null to use the Cloud SQL default (ZONAL). | `string` | `"ZONAL"` | no |
| <a name="input_backup_start_time"></a> [backup\_start\_time](#input\_backup\_start\_time) | Start of the daily backup window, in UTC as HH:MM. Leave null for a time chosen by Cloud SQL. | `string` | `null` | no |
| <a name="input_data_cache_enabled"></a> [data\_cache\_enabled](#input\_data\_cache\_enabled) | Whether data cache is enabled for the instance. Can be used with ENTERPRISE\_PLUS edition. | `bool` | `false` | no |
| <a name="input_database_deletion_policy"></a> [database\_deletion\_policy](#input\_database\_deletion\_policy) | Deletion policy for the database. Leave null to use ABANDON when adopting<br/>an existing instance or while the created instance has deletion\_protection<br/>or api\_deletion\_protection on, so destroying this module leaves the<br/>database and its data in place, and the provider default (drop the<br/>database) otherwise. Set to ABANDON to always leave it. | `string` | `null` | no |
| <a name="input_database_flags"></a> [database\_flags](#input\_database\_flags) | Database flags to set on instance. | `map(string)` | `{}` | no |
| <a name="input_database_name"></a> [database\_name](#input\_database\_name) | Name of the database to create on the instance. | `string` | n/a | yes |
| <a name="input_database_version"></a> [database\_version](#input\_database\_version) | The version of the database to be created, e.g. MYSQL\_8\_0 or POSTGRES\_15. | `string` | `"MYSQL_8_0"` | no |
| <a name="input_deletion_protection"></a> [deletion\_protection](#input\_deletion\_protection) | Prevent Terraform from destroying the instance. Set false, and apply, before destroying it. | `bool` | `true` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment | `string` | n/a | yes |
| <a name="input_disk_autoresize"></a> [disk\_autoresize](#input\_disk\_autoresize) | Set to false to disable automatic disk grow. | `bool` | `true` | no |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of the database disk in GiB. Raising it later resizes the disk in place. With<br/>disk\_autoresize on, Cloud SQL may grow the disk beyond this size, and the provider then<br/>ignores the smaller configured value instead of shrinking or recreating the instance. | `number` | `null` | no |
| <a name="input_edition"></a> [edition](#input\_edition) | The edition of the Cloud SQL instance. ENTERPRISE\_PLUS is required for data cache. | `string` | `"ENTERPRISE"` | no |
| <a name="input_enable_backups"></a> [enable\_backups](#input\_enable\_backups) | Set true to enable automated backups. | `bool` | `true` | no |
| <a name="input_encryption_key_name"></a> [encryption\_key\_name](#input\_encryption\_key\_name) | The full resource name of a Cloud KMS CryptoKey used to encrypt the<br/>Cloud SQL instance data (CMEK). The Cloud SQL service identity must<br/>hold roles/cloudkms.cryptoKeyEncrypterDecrypter on the key, and the<br/>key location must match the instance region. Encryption mode is<br/>fixed at instance creation and cannot be changed in place. | `string` | `null` | no |
| <a name="input_existing_instance_name"></a> [existing\_instance\_name](#input\_existing\_instance\_name) | Name of an existing Cloud SQL instance to adopt instead of creating a new<br/>one. The instance must have a private IP address. To adopt an instance<br/>created by another cloudsql module in the same apply, set that module's<br/>sql\_instance\_name and pass its instance\_name output here; a literal name<br/>is read at plan time, before the instance exists, and fails. When set,<br/>the inputs that shape a new instance are ignored: api\_deletion\_protection,<br/>availability\_type, backup\_start\_time, data\_cache\_enabled, database\_flags,<br/>database\_version, deletion\_protection, disk\_autoresize, disk\_size\_gb,<br/>edition, enable\_backups, encryption\_key\_name,<br/>final\_backup\_retention\_days, maintenance\_window, network\_id,<br/>point\_in\_time\_recovery, private\_vpc\_connection\_peering,<br/>psc\_allowed\_consumer\_projects, query\_insights, retain\_backups\_on\_delete,<br/>retained\_backups, set\_root\_password, sql\_instance\_name, ssl\_mode and<br/>tier. | `string` | `null` | no |
| <a name="input_final_backup_retention_days"></a> [final\_backup\_retention\_days](#input\_final\_backup\_retention\_days) | Take a final backup when the instance is deleted and keep it for this many days. Leave null for no final backup. | `number` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_maintenance_window"></a> [maintenance\_window](#input\_maintenance\_window) | Maintenance window for the Cloud SQL instance: day 1 (Monday) to 7, hour 0 to 23 UTC, and update\_track canary, stable or week5. Leave null to accept Cloud SQL defaults. | <pre>object({<br/>    day          = number<br/>    hour         = number<br/>    update_track = optional(string)<br/>  })</pre> | `null` | no |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the instance is going to be created in:<br/>`projects/<project_id>/global/networks/<network_name>`. Required unless<br/>use\_psc\_connection is true or existing\_instance\_name is set. | `string` | `null` | no |
| <a name="input_password_secret_accessors"></a> [password\_secret\_accessors](#input\_password\_secret\_accessors) | IAM members granted `roles/secretmanager.secretAccessor` on the password<br/>secret, in IAM member form (e.g. `serviceAccount:sa@project.iam.gserviceaccount.com`).<br/>Values must be known at plan time, such as literal strings or the `service_account_iam_email`<br/>output of `modules/project/service-account` (the account may be created<br/>in the same apply). | `list(string)` | `[]` | no |
| <a name="input_password_version"></a> [password\_version](#input\_password\_version) | Rotation counter for the database password. Increase it (for example<br/>from 1 to 2) to generate a new password: the module writes it to a new<br/>secret version, then sets it on the database user, in the same apply. | `number` | `1` | no |
| <a name="input_point_in_time_recovery"></a> [point\_in\_time\_recovery](#input\_point\_in\_time\_recovery) | Enable point-in-time recovery (binary logging on MySQL) when enable\_backups is true. | `bool` | `true` | no |
| <a name="input_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#input\_private\_vpc\_connection\_peering) | The name of the VPC Network peering connection, used only as dependency for Cloud SQL creation. | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_psc_allowed_consumer_projects"></a> [psc\_allowed\_consumer\_projects](#input\_psc\_allowed\_consumer\_projects) | Projects allowed to connect to the instance over Private Service Connect. Leave null to allow only project\_id. | `list(string)` | `null` | no |
| <a name="input_query_insights"></a> [query\_insights](#input\_query\_insights) | Query insights configuration. | <pre>object({<br/>    enabled                 = optional(bool, false)<br/>    query_plans_per_minute  = optional(number)<br/>    query_string_length     = optional(number)<br/>    record_application_tags = optional(bool)<br/>    record_client_address   = optional(bool)<br/>  })</pre> | `{}` | no |
| <a name="input_region"></a> [region](#input\_region) | The region where SQL instance will be configured | `string` | n/a | yes |
| <a name="input_retain_backups_on_delete"></a> [retain\_backups\_on\_delete](#input\_retain\_backups\_on\_delete) | Keep the instance's automated backups after it is deleted (they remain billed until they expire). Set null to use the Cloud SQL default. | `bool` | `true` | no |
| <a name="input_retained_backups"></a> [retained\_backups](#input\_retained\_backups) | Number of automated backups to keep, at least 7 (the module keeps 7 days of transaction logs, and Cloud SQL requires at least as many backups). Leave null for the Cloud SQL default (7). | `number` | `null` | no |
| <a name="input_service_account_iam_email"></a> [service\_account\_iam\_email](#input\_service\_account\_iam\_email) | IAM members granted read access to the password secret, in addition to<br/>password\_secret\_accessors. Named to match the `service_account_iam_email`<br/>output of `modules/project/service-account`, so listing such modules in<br/>`use` fills it; several used modules are combined. | `list(string)` | `[]` | no |
| <a name="input_set_root_password"></a> [set\_root\_password](#input\_set\_root\_password) | Set a random password, never stored in state or output, on the built-in<br/>root user (MySQL root, PostgreSQL postgres) of a created instance, so it<br/>cannot be used without one. Reset it with `gcloud sql users set-password`<br/>if you need root access. Only read when the instance is created; changing<br/>it later has no effect. | `bool` | `true` | no |
| <a name="input_sql_instance_name"></a> [sql\_instance\_name](#input\_sql\_instance\_name) | Name given to the SQL instance. If not supplied, a name is generated from deployment\_name. | `string` | `null` | no |
| <a name="input_sql_password"></a> [sql\_password](#input\_sql\_password) | Password for the SQL database. If not supplied, a random password is<br/>generated. It is written through write-only arguments, so changing it has<br/>no effect unless password\_version is increased in the same apply. A value<br/>set here is written to the deployment folder and the saved plan. | `string` | `null` | no |
| <a name="input_sql_username"></a> [sql\_username](#input\_sql\_username) | Username for the SQL database, at most 32 characters for MySQL (16 for<br/>MYSQL\_5\_6). Module instances that share one Cloud SQL instance (for<br/>example a creating and an adopting module) must use different usernames:<br/>both the database user and the password secret ID are derived from it. | `string` | `"dbuser"` | no |
| <a name="input_ssl_mode"></a> [ssl\_mode](#input\_ssl\_mode) | Connection encryption policy: ENCRYPTED\_ONLY, TRUSTED\_CLIENT\_CERTIFICATE\_REQUIRED or ALLOW\_UNENCRYPTED\_AND\_ENCRYPTED. Set null to use the Cloud SQL default, which accepts unencrypted connections. TRUSTED\_CLIENT\_CERTIFICATE\_REQUIRED also needs client certificates, which this module does not create. | `string` | `"ENCRYPTED_ONLY"` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | Self link of the network where Cloud SQL instance PSC endpoint will be created | `string` | `null` | no |
| <a name="input_tier"></a> [tier](#input\_tier) | The machine type to use for the SQL instance. Required unless existing\_instance\_name is set. | `string` | `null` | no |
| <a name="input_use_psc_connection"></a> [use\_psc\_connection](#input\_use\_psc\_connection) | Create Private Service Connection instead of using Private Service Access peering | `bool` | `false` | no |
| <a name="input_user_deletion_policy"></a> [user\_deletion\_policy](#input\_user\_deletion\_policy) | Deletion policy for the database user. Leave null to use ABANDON for<br/>PostgreSQL, where a user that owns objects cannot be dropped and destroy<br/>would otherwise fail, and while the created instance has<br/>deletion\_protection or api\_deletion\_protection on; otherwise the<br/>provider default (drop the user). Set to ABANDON to always leave the user<br/>in place on destroy. | `string` | `null` | no |
| <a name="input_user_managed_replication"></a> [user\_managed\_replication](#input\_user\_managed\_replication) | Replication parameters that will be used for the generated password secret. | <pre>list(object({<br/>    location     = string<br/>    kms_key_name = optional(string)<br/>  }))</pre> | `[]` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_connection_name"></a> [connection\_name](#output\_connection\_name) | The connection name of the Cloud SQL instance, used by the Cloud SQL Auth Proxy and BigQuery federation. |
| <a name="output_database_name"></a> [database\_name](#output\_database\_name) | The name of the database created on the instance. |
| <a name="output_instance_name"></a> [instance\_name](#output\_instance\_name) | The name of the Cloud SQL instance, whether created or adopted. |
| <a name="output_password_secret_id"></a> [password\_secret\_id](#output\_password\_secret\_id) | The Secret Manager secret ID holding the database password. |
| <a name="output_password_secret_version"></a> [password\_secret\_version](#output\_password\_secret\_version) | The Secret Manager secret version resource name holding the database password. |
| <a name="output_private_ip_address"></a> [private\_ip\_address](#output\_private\_ip\_address) | The private IP address of the Cloud SQL instance, or its PSC endpoint address when use\_psc\_connection is true. |
| <a name="output_self_link"></a> [self\_link](#output\_self\_link) | The URI of the Cloud SQL instance. |
| <a name="output_username"></a> [username](#output\_username) | The username of the database user. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
