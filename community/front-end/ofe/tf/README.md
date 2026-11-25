## Description

This Terraform deploys the Open Front End web server: its service account,
control bucket, Pub/Sub topic and subscriptions, network (unless an existing
subnet is given) and VM. `deploy.sh` and `teardown.sh` run it; run them rather
than Terraform directly.

### Web server identity

The server runs as `<deployment_name>-fe-sa`, created by the Cluster Toolkit
`modules/project/service-account` module. Its permissions come from the
`community/modules/iam/service-account-profiles` module with
`profile: custom`. Both are sourced from this checkout by relative path, so the
server always uses the module code that ships with this Open Front End.

The account holds only these grants. Every other Google Cloud call the server
makes (listing machine types, regions, subnets, Filestore instances, images,
Artifact Registry repositories and Cloud Build builds, reading secrets, and
running Terraform, Packer and the Cluster Toolkit) uses the credential
registered in the Front End: by default the provisioning account below, which
the server impersonates.

| Grant | Where | Why |
| ----- | ----- | --- |
| `roles/logging.logWriter`, `roles/monitoring.metricWriter` | project | Server logs and metrics. |
| `roles/monitoring.viewer` | project | Grafana's Cloud Monitoring datasource reads cluster metrics as this account, with no key. |
| `roles/storage.admin` | control bucket only | The startup script reads its files there (and, for a tarball deployment, the code), and the server uploads cluster files. For clusters created before service-account profiles, the server also grants the cluster's account read and write access to the bucket; a newer cluster's blueprint grants its controller read and write access and its login and compute nodes read access, as the cluster's credential. |
| `roles/pubsub.admin` | command-and-control topic only | The server publishes commands to clusters. For clusters created before service-account profiles, it also grants the cluster's account permission to publish replies; a newer cluster's blueprint makes that grant. |
| `roles/pubsub.subscriber` | the `<deployment_name>-c2resp` and `<deployment_name>-build-logs-sub` subscriptions only | The server reads cluster replies and Cloud Build log messages. |
| `compute.machineTypes.get` (custom role) | project | `gcluster create` looks up each machine type in a blueprint with Application Default Credentials, which on the server are this account, not the provisioning account the server impersonates. It reads public machine-type specifications only. |
| `pubsub.subscriptions.create`, `.delete`, `.get`, `.getIamPolicy`, `.setIamPolicy` (custom role) | project | The server creates and deletes one subscription per cluster at run time and, for clusters created before service-account profiles, grants the cluster's account access to it, so these cannot be granted on a named subscription in advance. |
| `roles/secretmanager.secretAccessor`, `roles/secretmanager.secretVersionAdder` | the `<deployment_name>-db-key` secret only | The server stores its database encryption key there on first boot and reads it on every start. |
| `roles/secretmanager.secretAccessor` | the `<deployment_name>-server-config` secret only | The startup script reads the server's configuration (the admin user's name, password and e-mail address and the other first-boot settings) there on first boot. It is a secret rather than a control-bucket object so that cluster accounts with access to the bucket cannot read it. Principals with Secret Manager roles on the whole project can still read it, including project owners, the provisioning account and the shared account of any cluster created before an upgrade, until that cluster is recreated, but only while a version is enabled. |
| `roles/secretmanager.secretVersionManager` | the `<deployment_name>-server-config` secret only | Once the first boot has installed and started the server, the startup script disables the version it read. A first boot whose fetch of the TKFE code fails or is interrupted leaves it enabled, so a restart tries again; one that fails or stops later never writes its completion marker, stops the web server, and it and every later boot leave the version enabled, and the server VM must be recreated. If the disable fails, the script logs an error and the server disables the version at its next start. While a version is disabled no one can read it, so principals with project-wide Secret Manager access (such as the shared account of a cluster created before an upgrade) can no longer read the admin password; the server's own account, which holds both roles on this secret, could enable it again and read it, as could anyone who takes control of the server VM. An installed server disables any version that is still enabled each time it starts. The role lists, enables and disables versions; it cannot read them. The secret version's `enabled` attribute is in `ignore_changes`, so a later apply does not enable it again. |
| `roles/iam.serviceAccountTokenCreator` | the provisioning account only | The server creates clusters and other resources by impersonating that account, with no key. Granted only when `provisioning_service_account` is set. |

Servers deployed with earlier versions also held these project roles. They
are no longer granted:

- `storage.objectAdmin`, `pubsub.admin`: replaced by the grants on the
  control bucket, topic and subscriptions above.
- `artifactregistry.admin`, `secretmanager.admin`: the server reaches
  Artifact Registry and Secret Manager only with the registered credential,
  apart from its own database-key and configuration secrets above.
- `cloudtrace.agent`: the server exports no traces.

The server VM uses the `cloud-platform` scope; access is limited by the grants
above, not by scopes. The VM is created only after the grants have been made
and given time to propagate, the impersonation grant exists, its configuration
secret has its version, and the files it reads from the control bucket at boot
(the startup script and, for a tarball deployment, the code) have been
uploaded. It finds the configuration secret's id in the instance metadata key
`ofe-server-config-secret`.

### Provisioning account

`deploy.sh` creates `<deployment_name>-prov-sa`, with no key, through
`script/service_account.sh create`, and sets `provisioning_service_account` to
its e-mail address. That script lists the account's project roles, each with
the resources that need it. The server reads the address from the instance
metadata key `ofe-provisioning-service-account`.

The account keeps project-wide IAM administration roles
(`resourcemanager.projectIamAdmin`, `iam.serviceAccountAdmin`,
`iam.serviceAccountUser`, `iam.roleAdmin`) without IAM conditions: clusters
and images need accounts, custom roles and grants of their own, and an IAM
condition cannot limit those roles to the ones OFE creates. Only the web
server's account is granted permission to impersonate it, and every change it
makes is in the project's audit logs, but whoever controls the server controls
this account. Run the Front End in a project of its own. OFE never grants the
Owner or Editor role, but whoever controls this account could; an organization
policy set outside OFE can refuse such grants, as described in
[Hardening the provisioning account with an organization policy](../docs/AdvancedAdmin.md#hardening-the-provisioning-account-with-an-organization-policy).

### Database encryption key

Terraform creates the Secret Manager secret `<deployment_name>-db-key`, and no
version of it, so the key is never in Terraform state. The server generates the
key on first boot, adds it as the secret's first version and reads it on every
start; it finds the secret id in the instance metadata key
`ofe-db-key-secret`. Destroying the deployment deletes the secret, and the key
with it, together with the server.

The custom role id ends in a random suffix kept in Terraform state, so a
redeploy under the same deployment name after a teardown does not collide with
the role id Google holds for up to 44 days after the teardown.

### Upgrading an existing deployment

Run `deploy.sh --config` with this version from the same directory, with the
deployment's existing values (the interactive mode stops when it finds
`tf/.tkfe.lock`). With `--config`, `deploy.sh` enables any API this version
needs that the project does not have yet, such as the Secret Manager and IAM
Service Account Credentials APIs, without asking. The administrator's guide,
[Upgrading an existing TKFE](../docs/admin_guide.md#upgrading-an-existing-tkfe),
has the whole procedure, including the steps on the server. The upgrade:

- runs `terraform init -upgrade`, which moves the lock file to the
  `hashicorp/google` 7.x providers. If you use a provider mirror, it must hold
  `hashicorp/google` and `hashicorp/google-beta` `>= 7.33.0, < 8.0.0`,
  `hashicorp/time`, `hashicorp/random` and `hashicorp/null` first;
- keeps the service account and the control bucket (`moved` blocks);
- keeps the control bucket in its existing location: `deploy.sh` reads it
  from Terraform state and sets `control_bucket_location`, because a new
  location would replace the bucket and delete its files. Servers that
  earlier versions deployed with `--config` have their bucket in the `US`
  multi-region, not in the deployment's region. If `deploy.sh` cannot read
  the location, it stops before the apply;
- stops managing the old project-role grants and the bucket's old IAM
  bindings without deleting them (`removed` blocks), so the server and
  existing clusters keep their access throughout;
- after a successful apply, removes the project roles listed above as no
  longer granted from the server's account. If that fails, `deploy.sh` prints
  the `gcloud` commands to run by hand;
- stops and starts the server VM once, to change its scope. The installed
  server is not reinstalled;
- creates the provisioning account (or reuses it), grants the server's
  account permission to impersonate it, creates the database-key secret and
  adds both to the server's instance metadata;
- stores the server's configuration, which holds the admin password, in the
  secret `<deployment_name>-server-config`, and deletes the copy that earlier
  versions uploaded to the control bucket as `webserver/config`. An installed
  server does not read the secret again; it disables the secret's enabled
  versions the next time it starts.

`teardown.sh` with this version also tears down a server deployed with an
earlier version that was never upgraded. Terraform forgets that server's old
project-role grants instead of destroying them, so before the destroy
`teardown.sh` removes them with `gcloud`; if it cannot, it destroys nothing
and prints the commands to run by hand.

The account stays in the bucket's old `roles/storage.objectAdmin` binding,
which also holds the accounts of clusters created before the upgrade. Remove
it by hand once none of those clusters remain:

```shell
gcloud storage buckets remove-iam-policy-binding gs://<control bucket> \
  --member=serviceAccount:<deployment_name>-fe-sa@<project>.iam.gserviceaccount.com \
  --role=roles/storage.objectAdmin
```

### A service account changed outside Terraform

If the server's account was deleted, or deleted and created again under the
same name, after the last deploy, `deploy.sh` notices (its unique id in
Terraform state no longer matches the live one) and makes the apply wait again
for IAM after the account and its grants are created, before they are used.

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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.33.0, < 8.0.0 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 7.33.0, < 8.0.0 |
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.33.0, < 8.0.0 |
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

| Name | Source | Version |
| ---- | ------ | ------- |
| <a name="module_control_storage"></a> [control\_storage](#module\_control\_storage) | terraform-google-modules/cloud-storage/google | ~> 11.1 |
| <a name="module_fe_profile"></a> [fe\_profile](#module\_fe\_profile) | ../../../../community/modules/iam/service-account-profiles | n/a |
| <a name="module_fe_sa"></a> [fe\_sa](#module\_fe\_sa) | ../../../../modules/project/service-account | n/a |
| <a name="module_network"></a> [network](#module\_network) | ./network | n/a |
| <a name="module_pubsub"></a> [pubsub](#module\_pubsub) | terraform-google-modules/pubsub/google | ~> 8.3 |

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_instance.server_vm](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance) | resource |
| [google_logging_project_sink.build_sink](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/logging_project_sink) | resource |
| [google_pubsub_subscription.cloud_build_logs_sub](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/pubsub_subscription) | resource |
| [google_pubsub_topic.cloud_build_logs](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/pubsub_topic) | resource |
| [google_pubsub_topic_iam_member.build_sink_pub](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/pubsub_topic_iam_member) | resource |
| [google_secret_manager_secret.db_key](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret) | resource |
| [google_secret_manager_secret.server_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret) | resource |
| [google_secret_manager_secret_version.server_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_version) | resource |
| [google_service_account_iam_member.fe_impersonates_provisioning](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_account_iam_member) | resource |
| [google_storage_bucket_object.deployment_file](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [null_resource.uploader](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [random_id.fe_role_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_control_bucket_location"></a> [control\_bucket\_location](#input\_control\_bucket\_location) | Location of the control bucket. Empty: the bucket is created in region.<br/>deploy.sh sets it to an existing bucket's location when it is run again<br/>on a deployment, because Terraform replaces a bucket whose location<br/>changes, deleting its files. | `string` | `""` | no |
| <a name="input_deployment_mode"></a> [deployment\_mode](#input\_deployment\_mode) | Use a tarball of this directory, or download from git to deploy the server. Must be either 'tarball' or 'git' | `string` | `"tarball"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Base "name" for the deployment. | `string` | n/a | yes |
| <a name="input_django_su_email"></a> [django\_su\_email](#input\_django\_su\_email) | DJango Admin SuperUser email | `string` | n/a | yes |
| <a name="input_django_su_password"></a> [django\_su\_password](#input\_django\_su\_password) | DJango Admin SuperUser password | `string` | n/a | yes |
| <a name="input_django_su_username"></a> [django\_su\_username](#input\_django\_su\_username) | DJango Admin SuperUser username | `string` | `"admin"` | no |
| <a name="input_extra_labels"></a> [extra\_labels](#input\_extra\_labels) | Extra labels to apply to created GCP resources. | `map(any)` | `{}` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP Project in which to deploy the HPC Frontend. | `string` | n/a | yes |
| <a name="input_provisioning_service_account"></a> [provisioning\_service\_account](#input\_provisioning\_service\_account) | E-mail address of the keyless service account, in project\_id, that the web<br/>server acts as to create clusters and other resources (deploy.sh creates<br/><deployment\_name>-prov-sa). The web server's account is granted the<br/>Service Account Token Creator role on this account only, and the server<br/>reads the address from its instance metadata. Empty: no account is set and<br/>no grant is made. | `string` | `""` | no |
| <a name="input_region"></a> [region](#input\_region) | GCP Region for HPC Frontend deployment. | `string` | n/a | yes |
| <a name="input_repo_branch"></a> [repo\_branch](#input\_repo\_branch) | git branch to checkout when deploying the HPC Frontend | `string` | `"main"` | no |
| <a name="input_repo_fork"></a> [repo\_fork](#input\_repo\_fork) | GitHub repository name in which to find the cluster-toolkit repo | `string` | `"GoogleCloudPlatform"` | no |
| <a name="input_server_instance_type"></a> [server\_instance\_type](#input\_server\_instance\_type) | Instance size to use from HPC Frontend webserver | `string` | `"n2d-standard-2"` | no |
| <a name="input_static_ip"></a> [static\_ip](#input\_static\_ip) | Optional pre-configured static IP for HPC Frontend. | `string` | `""` | no |
| <a name="input_subnet"></a> [subnet](#input\_subnet) | Subnet in which to deploy HPC Frontend. | `string` | `""` | no |
| <a name="input_terraform_state_bucket"></a> [terraform\_state\_bucket](#input\_terraform\_state\_bucket) | Name of the GCS bucket holding Terraform state for resources the front end<br/>provisions, created by the separate tf/state\_bucket root and passed in by<br/>deploy.sh. No default: silently falling back to local state would bring<br/>back the risk this bucket exists to remove. | `string` | n/a | yes |
| <a name="input_webserver_hostname"></a> [webserver\_hostname](#input\_webserver\_hostname) | DNS Hostname for the webserver | `string` | `""` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | GCP Zone for HPC Frontend deployment. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_server_ip"></a> [server\_ip](#output\_server\_ip) | Webserver IP Address |
| <a name="output_server_service_account_email"></a> [server\_service\_account\_email](#output\_server\_service\_account\_email) | E-mail address of the web server's service account, available once its grants have propagated. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
