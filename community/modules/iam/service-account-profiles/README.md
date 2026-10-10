## Description

This module applies a named least-privilege permission profile to an existing, keyless
service account. It does not create the account: create it with
[modules/project/service-account](../../../../modules/project/service-account/README.md) and
connect the two with `use: [<account module id>]`, which supplies the account's
`service_account_email`. For an account created some other way, set `service_account_email`
directly instead. The module grants:

- the profile's predefined roles, project-wide (logging and monitoring for every profile);
- a generated [custom IAM role](https://cloud.google.com/iam/docs/creating-custom-roles)
  holding only the permissions the profile, its capability flags and any custom permissions
  need;
- for `slurm-controller` and `image-builder`, a second custom role holding
  `compute.instances.setMetadata`, bound only to instances named by `instance_name_prefixes`
  (see [Name-scoped setMetadata](#name-scoped-setmetadata));
- `roles/iam.serviceAccountUser` on individual accounts, never on the project: from the Slurm
  controller to the compute accounts it attaches to nodes, and from the image builder to
  itself;
- resource-scoped grants on the buckets, secrets, repositories, datasets, topics,
  subscriptions and subnetworks you name.

The `service_account_email` output returns the account's address, once the grants have
propagated, for `use:` by the Slurm v6 controller, login and nodeset modules and by
`modules/packer/custom-image`.

## Profiles

| Profile | Project roles | Custom role | Name-scoped role | Also accepts |
| --- | --- | --- | --- | --- |
| `slurm-controller` | `logging.logWriter`, `monitoring.metricWriter` | Compute Engine permissions Slurm needs to create, poll, suspend and delete nodes | `compute.instances.setMetadata` | capability flags, `act_as_service_accounts` |
| `slurm-login` | `logging.logWriter`, `monitoring.metricWriter` | none | none | resource-scoped bindings only |
| `slurm-compute` | `logging.logWriter`, `monitoring.metricWriter` | none | none | resource-scoped bindings only |
| `image-builder` | `logging.logWriter`, `monitoring.metricWriter` | `compute.instances.get`, plus actAs on itself | `compute.instances.setMetadata` | resource-scoped bindings only |
| `custom` | `logging.logWriter`, `monitoring.metricWriter` | `custom_permissions` | none | `custom_profile_name` (required), `custom_permissions`, `custom_roles`, capability flags except `enable_mig`, `enable_mig_flex` and `enable_tpu_gce`, `*_bindings` |

Every profile accepts the resource-scoped `*_bindings` variables. The exact permission lists
are in the `profiles` catalogue in `main.tf`, and inputs a profile does not accept are rejected
at plan time. A new profile needs a catalogue entry and its name in `metadata.yaml`'s
`allowed_enum` validator.

Capability flags, for `slurm-controller` and (except the MIG and Compute Engine TPU flags)
`custom`. Set them in the module's `settings`, not as deployment variables: a deployment
variable reaches every module with an input of the same name, and the login and compute
profiles reject capability flags.

- `enable_placement_policies`: create and use placement policies. Needed whenever a nodeset
  keeps `schedmd-slurm-gcp-v6-nodeset`'s default `enable_placement: true`, or uses a
  reservation with its own attached resource policies.
- `enable_public_ips`: resume nodes that get an external IP (a nodeset with
  `enable_public_ips` or an `access_config`).
- `enable_mig`: manage nodesets backed by a managed instance group that Terraform creates
  (`provisioning_engine = "MIG"`). See [MIG capabilities](#mig-capabilities).
- `enable_mig_flex`: manage DWS Flex nodesets (a nodeset with `dws_flex` enabled), which the
  controller provisions through a regional managed instance group per job. Includes
  everything `enable_mig` grants. See [MIG capabilities](#mig-capabilities).
- `enable_bigquery_load`: the controller's `enable_bigquery_load` job accounting.
- `enable_tpu`: manage Cloud TPU VM nodesets (`schedmd-slurm-gcp-v6-nodeset-tpu`).
- `enable_tpu_gce`: manage TPU machine types on Compute Engine (`ct*`/`tpu*` in a regular
  nodeset), which the controller provisions through a MIG per slice. Includes everything
  `enable_mig_flex` grants. See [MIG capabilities](#mig-capabilities).

The config-bucket read access that the controller, login and compute nodes need, and the
controller's read of its Cloud SQL secret, are granted by `schedmd-slurm-gcp-v6-controller`,
not by this module.

No profile or flag covers the Slurm repair flow (`compute.instances.update`), opportunistic
maintenance on compute nodes (`compute.instances.performMaintenance`), or
`controller_network_attachment` (the controller writes its address to the config bucket).

### MIG capabilities

**`enable_mig`, `enable_mig_flex` and `enable_tpu_gce` grant `compute.instances.setMetadata` on
every instance in the project, not only this deployment's.** Google requires it for instance
group manager patch and insert, and IAM Conditions cannot scope an instance group manager, so
this grant cannot be name-scoped. `custom` refuses all three flags for the same reason it
refuses `setMetadata` in `custom_permissions`. Enable them only on `slurm-controller`, and only
for a cluster that uses MIG-backed, DWS Flex or Compute Engine TPU nodesets.

Verified live (2026-10-06): with `enable_mig`, a static MIG nodeset (`provisioning_engine =
"MIG"`) is resumed into its Terraform-created group and runs jobs; without it, the controller's
calls to list the group and `deleteInstances` fail with a 403, leaving the instances in the group
while Slurm marks them powered down. With `enable_mig_flex`, a DWS Flex job runs in a per-job group
that the controller creates and then deletes.

`enable_tpu_gce` is verified live up to the TPU capacity request: with a `ct5lp-hightpu-4t`
nodeset, the controller creates the slice's workload policy, copies its instance template, creates
the MIG and submits its resize request. End-to-end validation is blocked on availability: booting
Compute Engine TPU nodes needs a slurm-gcp image with Slurm 26.05 or later (current images ship
25.11), and TPU capacity.

### Name-scoped setMetadata

`compute.instances.setMetadata` can add an SSH key to an instance, so `custom_permissions`
refuses it, and the controller and image builder get it only through a second custom role
(`<deployment_name>.<name>.scoped`) bound with this condition:

    resource.type == "compute.googleapis.com/Instance" &&
      (resource.name.extract("/instances/<prefix>{name}") != "" || ...)

| Profile | Default `instance_name_prefixes` | Why |
| --- | --- | --- |
| `slurm-controller` | `<first 10 characters of deployment_name, lowercase, letters and digits only>-` | Slurm names every VM `<slurm_cluster_name>-...`, and the controller module's default `slurm_cluster_name` is that expression. |
| `image-builder` | `packer-` | `modules/packer/custom-image` always names the build VM `packer-<6 hex>`. |

If the controller sets its own `slurm_cluster_name`, pass the same value to this module's
`slurm_cluster_name` (as `community/examples/hpc-slurm-ha.yaml` does) rather than writing
`instance_name_prefixes` by hand. The controller's `bulkInsert` needs this permission even
though it sets no metadata itself, whether or not its instance template carries metadata.

Verified live (2026-10-05), as a `slurm-controller` account:

- `setMetadata` on an instance whose name matches the prefix succeeds, and on one whose name
  does not is denied with a 403.
- `bulkInsert` of an instance with a non-matching name is denied with a 403 for
  `compute.instances.setMetadata`, from a template with or without metadata; with a matching
  name it succeeds. A Slurm cluster whose condition names the wrong prefix cannot create
  nodes, and creates them again once the prefix is corrected.
- The image builder's completion signal on its `packer-*` VM succeeds.

The scope is a naming convention, not ownership: another VM in the project whose name matches
(for example another user's `packer-*` build) is covered too.

## Example usage

Each identity is two module blocks: `modules/project/service-account` creates the keyless
account, and `service-account-profiles` applies the profile to it, connected with
`use: [<account module id>]`. Give both blocks the same `name`, and leave `project_roles` empty;
the baseline roles come from the profile.

```yaml
- id: compute_account
  source: modules/project/service-account
  settings:
    name: compute
    project_roles: []

- id: compute_sa
  source: community/modules/iam/service-account-profiles
  use: [compute_account]
  settings:
    name: compute
    profile: slurm-compute

- id: login_account
  source: modules/project/service-account
  settings:
    name: login
    project_roles: []

- id: login_sa
  source: community/modules/iam/service-account-profiles
  use: [login_account]
  settings:
    name: login
    profile: slurm-login

- id: controller_account
  source: modules/project/service-account
  settings:
    name: controller
    project_roles: []

- id: controller_sa
  source: community/modules/iam/service-account-profiles
  use: [controller_account]
  settings:
    name: controller
    profile: slurm-controller
    enable_placement_policies: true
    # Only the accounts the controller attaches to nodes it creates. Login VMs are
    # created by Terraform, so no actAs on login_sa is needed.
    act_as_service_accounts:
    - $(compute_sa.service_account_email)

- id: n2_nodeset
  source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
  use: [network, compute_sa]
  settings:
    node_count_dynamic_max: 4
    machine_type: n2-standard-2

- id: n2_partition
  source: community/modules/compute/schedmd-slurm-gcp-v6-partition
  use: [n2_nodeset]
  settings:
    partition_name: n2
    is_default: true

- id: slurm_login
  source: community/modules/scheduler/schedmd-slurm-gcp-v6-login
  use: [network, login_sa]

- id: slurm_controller
  source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
  use: [network, controller_sa, slurm_login, n2_partition]
```

The remaining examples show only the `service-account-profiles` block; each also needs the
`modules/project/service-account` block its `use:` names, as above.

To give a nodeset its own compute account, for example one with access to a results bucket,
add another `slurm-compute` identity, use it in that nodeset, and add it to the controller's
`act_as_service_accounts`. A TPU nodeset needs the same. With `enable_backup_controller`, both
HA controllers share `controller_sa`; HA needs no additional permissions.

### Resource-scoped access

Each `*_bindings` variable is a map, keyed arbitrarily, that grants `role` on one named
resource. `project` defaults to `project_id` (buckets have none). For Shared VPC, grant
`roles/compute.networkUser` on the host-project subnetwork the identity creates instances in:

```yaml
- id: compute_sa
  source: community/modules/iam/service-account-profiles
  use: [compute_account]
  settings:
    name: compute
    profile: slurm-compute
    bucket_bindings:
      results:
        bucket: $(vars.deployment_name)-results
        role: roles/storage.objectUser

- id: controller_sa
  source: community/modules/iam/service-account-profiles
  use: [controller_account]
  settings:
    name: controller
    profile: slurm-controller
    subnetwork_bindings:
      shared:
        project: <host_project_id>
        region: us-central1
        subnetwork: <shared_subnetwork>
        role: roles/compute.networkUser
```

### Custom profile

`profile: custom` takes a `custom_profile_name`, which names the generated custom role, and at
least one of `custom_permissions` (put into the generated role), `custom_roles` (ids of
existing custom roles to bind), a capability flag other than `enable_mig`, `enable_mig_flex`
and `enable_tpu_gce`, or a `*_bindings` entry. Predefined roles beyond the logging and
monitoring baseline are not accepted. The Open Front End web identity, for example:

```yaml
- id: frontend_sa
  source: community/modules/iam/service-account-profiles
  use: [frontend_account]
  settings:
    name: fe
    profile: custom
    custom_profile_name: Open Front End
    # The web server creates one Pub/Sub subscription per cluster at run time,
    # so these cannot be granted on individual subscriptions in advance.
    custom_permissions:
    - pubsub.subscriptions.create
    - pubsub.subscriptions.delete
    - pubsub.subscriptions.get
    - pubsub.subscriptions.getIamPolicy
    - pubsub.subscriptions.setIamPolicy
    bucket_bindings:
      control:
        bucket: $(vars.deployment_name)-storage
        role: roles/storage.admin
    pubsub_topic_bindings:
      c2:
        topic: $(vars.deployment_name)
        role: roles/pubsub.admin
    pubsub_subscription_bindings:
      c2resp:
        subscription: $(vars.deployment_name)-c2resp
        role: roles/pubsub.subscriber
```

### Input checks

- `custom_permissions` rejects permissions that would let the account impersonate or mint
  credentials for other service accounts, manage service-account, HMAC or API keys, edit IAM
  roles, read secret payloads or decrypt with Cloud KMS keys, log in to or change the metadata
  of other VMs, or run builds and deployments as another account. It also rejects every
  permission ending in `.setIamPolicy` except `pubsub.subscriptions.setIamPolicy`, which the
  Open Front End identity above needs. The list is best effort; built-in profiles are
  reviewed by hand instead.
- Every `*_bindings` role must be a role id (`roles/<name>`, `projects/<p>/roles/<name>` or
  `organizations/<o>/roles/<name>`) and must not be a basic role (`owner`, `editor`,
  `viewer`) or a `roles/iam.*` or `roles/resourcemanager.*` role. Resource-level `*.admin`
  roles such as `roles/storage.admin` are accepted, because they act only on the bound
  resource, but they can change that resource's IAM policy; prefer a narrower role.
- `act_as_service_accounts` accepts only accounts in `project_id`
  (`<name>@<project_id>.iam.gserviceaccount.com`). Accounts in other projects, Google-managed
  service agents, and the default Compute Engine, App Engine and Google APIs accounts are
  refused. Review each address as you would any `roles/iam.serviceAccountUser` grant.
- `custom_roles` are bound as they are: the module cannot check what an existing role
  contains, now or after it is edited.

### Image-builder service account

`profile: image-builder` gives the temporary Packer build VM its own identity instead of the
default Compute Engine service account:

```yaml
- id: image_builder_sa
  source: community/modules/iam/service-account-profiles
  use: [image_builder_account]
  settings:
    name: builder
    profile: image-builder

- id: my-image
  source: modules/packer/custom-image
  kind: packer
  use: [network, image_builder_sa]
  settings:
    ...
```

The account id is `<deployment_name>-<name>` and is limited to 30 characters (enforced by
`modules/project/service-account`), so keep `name` short.

The profile grants no project-wide storage access. A build whose startup script comes from
`modules/scripts/startup-script` downloads its runners from that module's staging bucket, so
give the build account read access there, or the build fails with a 403 from
`gcloud storage cp`:

```yaml
- id: scripts_for_image
  source: modules/scripts/startup-script
  settings:
    bucket_viewers:
    - serviceAccount:$(image_builder_sa.service_account_email)
    runners: ...
```

Grant any other bucket the build reads through `bucket_bindings`.

To run a cluster on the built image, give the cluster its own `slurm-controller`,
`slurm-login` and `slurm-compute` identities, as `examples/image-builder.yaml` does. The
controller profile includes `compute.images.useReadOnly` for images in `project_id`; for an
image in another project, grant the controller account `roles/compute.imageUser` there.

A Packer build uses two identities: the build VM's service account, which this profile
covers, and the principal that runs `packer build` (the Packer caller), which this module
grants nothing. The caller's permissions below are derived from the
`packer-plugin-googlecompute` v1.2.7 source and Google's per-method permission pages, and are
not yet verified with a caller restricted to exactly these permissions.

| Step | Performed by | Calls and permissions |
| --- | --- | --- |
| 1. Create or prepare the instance | Packer caller | Pre-flight: `compute.images.get`, `compute.images.getFromFamily` (source image family; on the source image's project), `compute.zones.get`, `compute.machineTypes.get`, `compute.projects.get` (non-fatal). Create (`instances.insert`): `compute.instances.create`, `compute.disks.create`, `compute.images.useReadOnly`, `compute.subnetworks.use` (`compute.subnetworks.useExternalIp` only with `omit_external_ip: false`), `compute.instances.setMetadata` and `compute.instances.setLabels` (the module always sets metadata and labels), `compute.instances.setTags` (only if `tags` is set), `compute.instances.setServiceAccount` and `iam.serviceAccounts.actAs` on the image-builder account. Operations: `compute.zoneOperations.get`, `compute.globalOperations.get` |
| 2. Access required resources | Build VM account | Read access on each bucket the build reads, granted per bucket: `bucket_viewers` on `modules/scripts/startup-script` for its staging bucket, `bucket_bindings` for others |
| 3. Run the build and signal completion | Build VM account, with the caller polling | Build VM account: `gcloud compute instances add-metadata` on itself needs `compute.instances.setMetadata` (Google) and, in live testing, `compute.instances.get` and actAs on itself (this profile grants all three; `setMetadata` only on `packer-*`). Caller: `compute.instances.get` (waits for the VM and for the `startup-script-status` metadata); with `use_iap: true` (the module default) also `iap.tunnelInstances.accessViaIAP` and `compute.instances.list` (IAP TCP forwarding; not re-verified here); with `use_os_login: true`, OS Login permissions (not verified) |
| 4. Delete the temporary instance | Packer caller | `compute.instances.delete`, `compute.disks.delete` (the boot disk is not auto-deleted), `compute.instances.getSerialPortOutput` (non-fatal), zone-operation read |
| 5. Create the final image | Packer caller | `compute.images.create`, `compute.disks.useReadOnly` (source disk), `compute.images.get`, `compute.images.deprecate` (always called), `compute.images.setLabels` (labels), global-operation read; `compute.images.delete` with `packer build -force` over an existing image (unverified) |

Omitting `service_account_email` on the Packer template attaches the default Compute Engine
service account instead, which the caller also needs actAs on; this profile exists to avoid
that.

## Troubleshooting a `PERMISSION_DENIED`

The generated role's id is in the `custom_role_id` output, and the name-scoped role's in
`scoped_role_id`. Compare a role's permissions with the denied call:

```bash
gcloud iam roles describe "$(terraform output -raw custom_role_id)" \
  --format="value(includedPermissions)"
```

- A denial right after apply may be IAM propagation, which Google documents as eventually
  consistent; retry, or raise `iam_propagation_wait`.
- A `compute.instances.setMetadata` denial on an instance whose name does not start with any
  of `instance_name_prefixes` is the name-scoped condition working as intended. On an
  instance that should match, the prefix is wrong: check `instance_name_prefixes` (or
  `slurm_cluster_name`) against the instance name.
- Slurm controller denials appear in `/var/log/slurm/resume.log` or `slurmsync.log` on the
  controller. A missing `enable_placement_policies` shows as a `CRITICAL: failed to create
  placement policies` line followed by a `bulkInsert` 404 for the policy; the 404 is what
  Slurm reports on the job and node.
- Denials for resource-scoped bindings come back from `terraform apply`. Denials for
  capability permissions appear in the calling script's own log.
- A disabled API fails as `SERVICE_DISABLED`: at apply time for a binding on that service, at
  run time for a capability. This module requires only `iam.googleapis.com` and
  `cloudresourcemanager.googleapis.com`; enable the APIs of the services you bind to.

If a profile always needs a permission, add it to that profile's catalogue entry and to the
daily tests' allowlists in
[test-service-account-profiles.yml](../../../../tools/cloud-build/daily-tests/ansible_playbooks/test-validation/test-service-account-profiles.yml).
If only your deployment needs it, use a resource-scoped binding or the custom profile instead.

## Custom role lifecycle

The generated role's id is `<deployment_name>.<name>`, each part with dashes replaced by
underscores (for example `my_dep.my_name`); the name-scoped role adds `.scoped`. Neither part
can contain a period, so two different inputs never produce the same id. Ids are limited to 64
bytes, so `deployment_name` plus `name` can be at most 56 characters. What destroy and
recreate do depends on `custom_role_deletion_policy` (see Inputs): with the default, recreate
the same deployment within 7 days or use a new `deployment_name`. After an undelete, a second
`apply` may be needed for a non-default `custom_role_stage` to take effect.

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

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.39.0 |
| <a name="requirement_time"></a> [time](#requirement\_time) | >= 0.9 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.39.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |
| <a name="provider_time"></a> [time](#provider\_time) | >= 0.9 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_artifact_registry_repository_iam_member.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/artifact_registry_repository_iam_member) | resource |
| [google_bigquery_dataset_iam_member.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/bigquery_dataset_iam_member) | resource |
| [google_compute_subnetwork_iam_member.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_subnetwork_iam_member) | resource |
| [google_project_iam_custom_role.scoped](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_custom_role) | resource |
| [google_project_iam_custom_role.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_custom_role) | resource |
| [google_project_iam_member.project_role](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.scoped_role](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_pubsub_subscription_iam_member.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/pubsub_subscription_iam_member) | resource |
| [google_pubsub_topic_iam_member.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/pubsub_topic_iam_member) | resource |
| [google_secret_manager_secret_iam_member.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_iam_member) | resource |
| [google_service_account_iam_member.act_as](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_account_iam_member) | resource |
| [google_storage_bucket_iam_member.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_iam_member) | resource |
| [terraform_data.input_checks](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [time_sleep.wait_for_iam](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/sleep) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_act_as_service_accounts"></a> [act\_as\_service\_accounts](#input\_act\_as\_service\_accounts) | For the Slurm controller profile: e-mail addresses of compute service accounts in<br/>project\_id (name@<project\_id>.iam.gserviceaccount.com) that the controller attaches to<br/>the nodes it creates. Grants "roles/iam.serviceAccountUser" on each target account<br/>individually, never project-wide. The default Compute Engine, App Engine and Google APIs<br/>accounts, accounts in another project, and Google-managed service agents are all refused:<br/>a designated compute account this controller creates and attaches, and nothing else. | `list(string)` | `[]` | no |
| <a name="input_artifact_registry_bindings"></a> [artifact\_registry\_bindings](#input\_artifact\_registry\_bindings) | Roles to grant on individual Artifact Registry repositories: a map of { project (default project\_id), location, repository\_id, role }. | <pre>map(object({<br/>    project       = optional(string)<br/>    location      = string<br/>    repository_id = string<br/>    role          = string<br/>  }))</pre> | `{}` | no |
| <a name="input_bigquery_dataset_bindings"></a> [bigquery\_dataset\_bindings](#input\_bigquery\_dataset\_bindings) | Roles to grant on individual BigQuery datasets: a map of { project (default project\_id), dataset\_id, role }. | <pre>map(object({<br/>    project    = optional(string)<br/>    dataset_id = string<br/>    role       = string<br/>  }))</pre> | `{}` | no |
| <a name="input_bucket_bindings"></a> [bucket\_bindings](#input\_bucket\_bindings) | Roles to grant on individual Cloud Storage buckets: a map, keyed arbitrarily, of { bucket, role }. | <pre>map(object({<br/>    bucket = string<br/>    role   = string<br/>  }))</pre> | `{}` | no |
| <a name="input_custom_permissions"></a> [custom\_permissions](#input\_custom\_permissions) | For the custom profile: IAM permissions (e.g. "compute.instances.get") for this account's<br/>generated custom role. | `list(string)` | `[]` | no |
| <a name="input_custom_profile_name"></a> [custom\_profile\_name](#input\_custom\_profile\_name) | For the custom profile: a short name for what this identity is for, used as the custom role's title. | `string` | `null` | no |
| <a name="input_custom_role_deletion_policy"></a> [custom\_role\_deletion\_policy](#input\_custom\_role\_deletion\_policy) | Deletion policy for the generated custom role. Unset means the provider default,<br/>"DELETE": destroy soft-deletes the role, a later apply within 7 days undeletes it, and its<br/>id cannot be reused until Google's 44-day deletion process ends. "ABANDON" leaves the role<br/>active, so a later apply fails until the role is imported or removed. "PREVENT" blocks<br/>deletion. | `string` | `null` | no |
| <a name="input_custom_role_stage"></a> [custom\_role\_stage](#input\_custom\_role\_stage) | Launch stage of the generated custom role: "ALPHA", "BETA", "GA", "DEPRECATED",<br/>"DISABLED" or "EAP". Unset means the provider default, "GA". A "DISABLED" role stays<br/>bound but grants nothing. Only valid when the account gets a generated custom role. | `string` | `null` | no |
| <a name="input_custom_roles"></a> [custom\_roles](#input\_custom\_roles) | For the custom profile: ids of existing custom roles to bind to this account, e.g.<br/>"projects/<project\_id>/roles/<role\_id>" or "organizations/<org\_id>/roles/<role\_id>". Can be combined<br/>with custom\_permissions. Predefined roles are not accepted. | `list(string)` | `[]` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment. With name, it builds the generated custom role ids. Use the deployment\_name given to whatever created the account. | `string` | n/a | yes |
| <a name="input_enable_bigquery_load"></a> [enable\_bigquery\_load](#input\_enable\_bigquery\_load) | Capability: add the BigQuery permissions for the Slurm controller's enable\_bigquery\_load job accounting. | `bool` | `false` | no |
| <a name="input_enable_mig"></a> [enable\_mig](#input\_enable\_mig) | Capability: add the permissions to manage nodesets backed by a managed instance group that<br/>Terraform creates (provisioning\_engine = "MIG"). Includes compute.instances.setMetadata<br/>without a name condition, because Google requires it for instance group manager patch,<br/>whose target is a group rather than an instance. | `bool` | `false` | no |
| <a name="input_enable_mig_flex"></a> [enable\_mig\_flex](#input\_enable\_mig\_flex) | Capability: add the regional instance-group permissions needed to manage DWS Flex nodesets<br/>(a nodeset with dws\_flex enabled). Includes everything enable\_mig adds, including the<br/>unscoped compute.instances.setMetadata. | `bool` | `false` | no |
| <a name="input_enable_placement_policies"></a> [enable\_placement\_policies](#input\_enable\_placement\_policies) | Capability: add the permissions to create and use placement policies. | `bool` | `false` | no |
| <a name="input_enable_public_ips"></a> [enable\_public\_ips](#input\_enable\_public\_ips) | Capability: add compute.subnetworks.useExternalIp, needed to resume nodes that get an<br/>external IP (a nodeset with enable\_public\_ips or an access\_config). | `bool` | `false` | no |
| <a name="input_enable_tpu"></a> [enable\_tpu](#input\_enable\_tpu) | Capability: add the Cloud TPU API permissions needed to manage TPU VM nodesets (schedmd-slurm-gcp-v6-nodeset-tpu). | `bool` | `false` | no |
| <a name="input_enable_tpu_gce"></a> [enable\_tpu\_gce](#input\_enable\_tpu\_gce) | Capability: add the permissions to manage TPU machine types on Compute Engine (a regular<br/>nodeset whose machine type is in a ct or tpu family), which the controller provisions through<br/>a MIG per slice. Includes everything enable\_mig\_flex adds, including the unscoped<br/>compute.instances.setMetadata. | `bool` | `false` | no |
| <a name="input_iam_propagation_wait"></a> [iam\_propagation\_wait](#input\_iam\_propagation\_wait) | How long the service\_account\_email output waits after this module's grants are created, as a Go duration<br/>("30s", "2m"). Best effort: Google documents IAM changes as eventually consistent, "typically<br/>2 minutes, potentially 7 minutes or longer", so no value guarantees the grants are effective.<br/>Raise it if the first use of the account fails with PERMISSION\_DENIED right after apply. | `string` | `"30s"` | no |
| <a name="input_instance_name_prefixes"></a> [instance\_name\_prefixes](#input\_instance\_name\_prefixes) | For profiles that get a name-scoped role (slurm-controller, image-builder): prefixes of the<br/>Compute Engine instance names that role covers, 4-62 characters each. Unset uses the<br/>profile default: slurm-controller "<slurm\_cluster\_name, if set, or the first 10 characters<br/>of deployment\_name, lowercase, letters and digits only>-"; image-builder "packer-". For the<br/>controller, prefer setting slurm\_cluster\_name over this variable when the controller's own<br/>slurm\_cluster\_name is set: it takes the exact value, with no risk of re-deriving it<br/>differently here. Scoping is by name only: any other VM in the project with a matching name<br/>is covered too; a short prefix makes an accidental match more likely, hence the 4-character<br/>floor. | `list(string)` | `null` | no |
| <a name="input_name"></a> [name](#input\_name) | Short name for this identity. With deployment\_name, it builds the generated custom role ids. Use the name given to whatever created the account. | `string` | n/a | yes |
| <a name="input_profile"></a> [profile](#input\_profile) | Permission profile for this account, from the module's catalogue: the Slurm controller,<br/>login and compute profiles, the Packer image builder profile, or the custom profile. See<br/>the README for what each grants. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of the project the service account and its generated roles live in. | `string` | n/a | yes |
| <a name="input_pubsub_subscription_bindings"></a> [pubsub\_subscription\_bindings](#input\_pubsub\_subscription\_bindings) | Roles to grant on individual Pub/Sub subscriptions: a map of { project (default project\_id), subscription, role }. | <pre>map(object({<br/>    project      = optional(string)<br/>    subscription = string<br/>    role         = string<br/>  }))</pre> | `{}` | no |
| <a name="input_pubsub_topic_bindings"></a> [pubsub\_topic\_bindings](#input\_pubsub\_topic\_bindings) | Roles to grant on individual Pub/Sub topics: a map of { project (default project\_id), topic, role }. | <pre>map(object({<br/>    project = optional(string)<br/>    topic   = string<br/>    role    = string<br/>  }))</pre> | `{}` | no |
| <a name="input_secret_bindings"></a> [secret\_bindings](#input\_secret\_bindings) | Roles to grant on individual Secret Manager secrets: a map of { project (default project\_id), secret\_id, role }. | <pre>map(object({<br/>    project   = optional(string)<br/>    secret_id = string<br/>    role      = string<br/>  }))</pre> | `{}` | no |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | E-mail address of the existing service account to grant this profile's permissions to.<br/>Usually set by use: [<account module id>] on a modules/project/service-account block, which<br/>supplies its service\_account\_email output; can also be set directly to any existing account's<br/>e-mail. This module does not create service accounts. | `string` | n/a | yes |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | For the Slurm controller profile: the same value given to<br/>schedmd-slurm-gcp-v6-controller's slurm\_cluster\_name input, if set. Slurm names every<br/>VM "<slurm\_cluster\_name>-...", and the controller does not re-normalise an explicit<br/>value, so passing the identical value here keeps the name-scoped role's instance-name<br/>condition correct without hand-deriving instance\_name\_prefixes. Unset mirrors the<br/>controller's own derived default (the first 10 characters of deployment\_name,<br/>lowercase, letters and digits only). Ignored if instance\_name\_prefixes is also set. | `string` | `null` | no |
| <a name="input_subnetwork_bindings"></a> [subnetwork\_bindings](#input\_subnetwork\_bindings) | Roles to grant on individual Compute Engine subnetworks: a map of { project (default project\_id; the host project for Shared VPC), region, subnetwork, role }. | <pre>map(object({<br/>    project    = optional(string)<br/>    region     = string<br/>    subnetwork = string<br/>    role       = string<br/>  }))</pre> | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_custom_role_id"></a> [custom\_role\_id](#output\_custom\_role\_id) | Id of the custom role this module generated for the account, or null if it generated none. |
| <a name="output_scoped_role_id"></a> [scoped\_role\_id](#output\_scoped\_role\_id) | Id of the name-scoped custom role this module generated (compute.instances.setMetadata, bound only for instances named by instance\_name\_prefixes), or null if it generated none. |
| <a name="output_service_account_email"></a> [service\_account\_email](#output\_service\_account\_email) | The service account e-mail address passed in as var.service\_account\_email, available once this module's IAM grants have propagated. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
