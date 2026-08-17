## Description

Grants `roles/cloudkms.cryptoKeyEncrypterDecrypter` on an existing CryptoKey to
the service agents that encrypt with it, and re-exports the key id ordered
behind those grants.

**CMEK consumers should `use` this module, not the module that created the
key.** The key id is available as soon as the key exists, which is before any
service agent can use it. A consumer wired to the creating module can therefore
be created first and fail with a KMS `PERMISSION_DENIED` that depends only on
how Terraform happened to schedule the two. Every output here carries a
`depends_on` on the grants, which also reverses the order on destroy so the
encrypted resource is removed before the grant it relies on.

## Example usage

Pairs with either [kms-key] (create a key) or [pre-existing-kms-key] (adopt
one) — both publish `crypto_key_id`, so `use` connects them:

```yaml
- id: kms_key
  source: community/modules/security/kms-key
  settings:
    project_id: $(vars.project_id)
    location: $(vars.region)
    key_ring_name: my-keyring
    key_name: my-key

- id: kms_key_iam
  source: community/modules/security/kms-key-iam
  use: [kms_key]
  settings:
    service_agents: [compute, filestore]
    # custom_service_accounts: ["my-sa@my-project.iam.gserviceaccount.com"]

- id: homefs
  source: modules/file-system/filestore
  use: [network, kms_key_iam]
  settings:
    local_mount: /home
    filestore_tier: ZONAL   # CMEK needs ZONAL, REGIONAL or ENTERPRISE
    size_gb: 1024
```

## Which principal needs the grant

Grant the identity that actually performs the encryption, which is not always
the one you would expect:

| Resource | Principal |
| -------- | --------- |
| Compute Engine disks | `service-PROJECT_NUMBER@compute-system.iam.gserviceaccount.com` — unless a module's `disk_encryption_key_service_account` names another identity |
| Cloud Storage buckets | `service-PROJECT_NUMBER@gs-project-accounts.iam.gserviceaccount.com` |
| Filestore instances | `service-PROJECT_NUMBER@cloud-filer.iam.gserviceaccount.com` |

These are per-service *service agents*, not the service account a VM runs as.
Some do not exist until provisioned:

```shell
gcloud storage service-agent --project=PROJECT_ID
gcloud beta services identity create --service=file.googleapis.com --project=PROJECT_ID
```

## Outputs and `use`

Cluster Toolkit's `use` matches an output name against the using module's input
variable name, so this module publishes the key id under each consumer's own
input name:

| Output | Consumer input it satisfies |
| ------ | --------------------------- |
| `kms_key_name` | `modules/file-system/filestore` |
| `disk_encryption_key` | `schedmd-slurm-gcp-v6-controller`, `-login`, `-nodeset` |
| `slurm_bucket_kms_key` | `schedmd-slurm-gcp-v6-controller` |
| `image_encryption_key` | `modules/packer/custom-image` |
| `repository_kms_key_name` | `community/modules/container/artifact-registry` |
| `encryption_key_name` | `community/modules/database/slurm-cloudsql-federation` |
| `crypto_key_id` | anything whose input is named otherwise |

All are the same key id; the separate names exist only so `use` matches.

## Out-of-band permissions

Set `skip_iam_role_grants: true` when a key's IAM is managed by someone
else -- for example a security team granting
`roles/cloudkms.cryptoKeyEncrypterDecrypter` directly on a key adopted with
[pre-existing-kms-key], where the identity running this module has no
`cloudkms.admin`/`setIamPolicy` on that key. Without this, Terraform
attempts the grant anyway and fails with a 403, even though the caller only
wanted this module's `use`-wiring convenience, not for it to manage IAM.

`service_agents` and `custom_service_accounts` must both be empty when this
is set -- `terraform plan` fails otherwise, since a non-empty value here
would otherwise look like a grant request that silently does nothing.

## Adding a new consumer

The output-aliasing table above is what makes `use:` wire a CryptoKey id
into a consumer's own input without the blueprint author naming it by hand
-- but that means this module has to already know the exact input name
every CMEK-capable module expects. It does not discover that automatically.

Giving Cluster Toolkit CMEK support for a module that does not appear in
the table above requires updating `outputs.tf` in this module at the same
time, not just the new module's own `variables.tf`. The pattern is the
same for every existing entry:

* Add an output whose name is exactly the new consumer's input variable
  name for its CMEK key setting (e.g. a module with
  `variable "my_encryption_key"` needs an output named `my_encryption_key`
  here).
* Give it `depends_on = [google_kms_crypto_key_iam_member.this]`, the same
  as every other output, so `use` orders the consumer behind the grant on
  `apply` and ahead of it on `destroy`.
* Have it return `var.crypto_key_id` -- every output here is the same key
  id, just under a different name.
* Add a row to the table above and to this module's `Inputs`/`Outputs`
  docs (regenerate with `terraform-docs`, or by hand if it is unavailable).

Without this, a new module can still be granted access to a key by passing
`crypto_key_id` explicitly instead of relying on `use:` -- but that is
exactly the ordering hazard this module exists to avoid (see "Description"
above), so a permanent CMEK integration for a new module should add the
named output rather than route around it.

## Testing

`terraform validate` passes on this module in isolation.

Granting `compute`, `storage` and `filestore` on a [kms-key]-generated key:

```console
$ gcloud kms keys get-iam-policy KEY --keyring=KEYRING --location=LOCATION --project=PROJECT_ID
bindings:
- members:
  - serviceAccount:service-PROJECT_NUMBER@cloud-filer.iam.gserviceaccount.com
  - serviceAccount:service-PROJECT_NUMBER@compute-system.iam.gserviceaccount.com
  - serviceAccount:service-PROJECT_NUMBER@gs-project-accounts.iam.gserviceaccount.com
  role: roles/cloudkms.cryptoKeyEncrypterDecrypter
```

`custom_service_accounts`, granting a single user-managed SA with no
`service_agents` at all, to prove the custom-SA path works standalone:

```console
$ ./ghpc deploy DEPLOYMENT_DIR --auto-approve
...
Apply complete! Resources: 38 added, 0 changed, 0 destroyed.

$ gcloud kms keys get-iam-policy KEY --keyring=KEYRING --location=LOCATION --project=PROJECT_ID
bindings:
- members:
  - serviceAccount:CUSTOM_SA@PROJECT_ID.iam.gserviceaccount.com
  role: roles/cloudkms.cryptoKeyEncrypterDecrypter

$ gcloud compute disks describe CONTROLLER_DISK --zone=ZONE --project=PROJECT_ID \
    --format="value(diskEncryptionKey.kmsKeyServiceAccount)"
CUSTOM_SA@PROJECT_ID.iam.gserviceaccount.com

$ gcloud compute disks describe LOGIN_DISK --zone=ZONE --project=PROJECT_ID \
    --format="value(diskEncryptionKey.kmsKeyServiceAccount)"
CUSTOM_SA@PROJECT_ID.iam.gserviceaccount.com

$ gcloud compute ssh LOGIN_INSTANCE --zone=ZONE --project=PROJECT_ID --tunnel-through-iap \
    --command="srun -p debug -N1 hostname; echo EXIT_CODE:\$?"
NODESET_INSTANCE
EXIT_CODE:0

$ gcloud compute disks describe NODESET_DISK --zone=ZONE --project=PROJECT_ID \
    --format="value(diskEncryptionKey.kmsKeyServiceAccount)"
CUSTOM_SA@PROJECT_ID.iam.gserviceaccount.com
```

`kmsKeyServiceAccount` on every disk confirms Compute Engine actually used
the custom SA to encrypt, not a fallback to its own service agent -- on the
controller and login boot disks, and on the dynamically-provisioned compute
node that ran the job.

`custom_service_accounts` also accepts more than one entry, each
independently grantable and independently usable -- granting two SAs on one
key, then pointing two different resources at two different SAs:

```console
$ ./ghpc deploy DEPLOYMENT_DIR --auto-approve
...
Apply complete! Resources: 39 added, 0 changed, 0 destroyed.

$ gcloud kms keys get-iam-policy KEY --keyring=KEYRING --location=LOCATION --project=PROJECT_ID
bindings:
- members:
  - serviceAccount:CUSTOM_SA_ONE@PROJECT_ID.iam.gserviceaccount.com
  - serviceAccount:CUSTOM_SA_TWO@PROJECT_ID.iam.gserviceaccount.com
  role: roles/cloudkms.cryptoKeyEncrypterDecrypter

$ gcloud compute disks describe CONTROLLER_DISK --zone=ZONE --project=PROJECT_ID \
    --format="value(diskEncryptionKey.kmsKeyServiceAccount)"
CUSTOM_SA_ONE@PROJECT_ID.iam.gserviceaccount.com

$ gcloud compute disks describe LOGIN_DISK --zone=ZONE --project=PROJECT_ID \
    --format="value(diskEncryptionKey.kmsKeyServiceAccount)"
CUSTOM_SA_ONE@PROJECT_ID.iam.gserviceaccount.com

$ gcloud compute ssh LOGIN_INSTANCE --zone=ZONE --project=PROJECT_ID --tunnel-through-iap \
    --command="srun -p debug -N1 hostname; echo EXIT_CODE:\$?"
NODESET_INSTANCE
EXIT_CODE:0

$ gcloud compute disks describe NODESET_DISK --zone=ZONE --project=PROJECT_ID \
    --format="value(diskEncryptionKey.kmsKeyServiceAccount)"
CUSTOM_SA_TWO@PROJECT_ID.iam.gserviceaccount.com
```

Controller and login both used SA one; the nodeset used SA two -- each disk
used the SA it was actually assigned, not just whichever came first in the
list, confirming the grants are independent rather than a single catch-all.
The SA-two-encrypted compute node also ran the job successfully.

`skip_iam_role_grants`, against a [pre-existing-kms-key] key whose grant was
applied entirely by hand (`gcloud kms keys add-iam-policy-binding`),
simulating a security team managing permissions out-of-band:

```console
$ ./ghpc deploy DEPLOYMENT_DIR --auto-approve
...
Apply complete! Resources: 28 added, 0 changed, 0 destroyed.

$ terraform state list | grep -i kms
module.imported_key.data.google_kms_crypto_key.this
module.imported_key.data.google_kms_key_ring.this
module.kms_key_iam.data.google_project.this

$ gcloud compute disks describe CONTROLLER_DISK --zone=ZONE --project=PROJECT_ID \
    --format="value(diskEncryptionKey.kmsKeyName)"
projects/PROJECT_ID/locations/LOCATION/keyRings/KEYRING/cryptoKeys/KEY

$ gcloud compute disks describe LOGIN_DISK --zone=ZONE --project=PROJECT_ID \
    --format="value(diskEncryptionKey.kmsKeyName)"
projects/PROJECT_ID/locations/LOCATION/keyRings/KEYRING/cryptoKeys/KEY

$ gcloud compute ssh LOGIN_INSTANCE --zone=ZONE --project=PROJECT_ID --tunnel-through-iap \
    --command="uptime; echo EXIT_CODE:\$?"
 13:38:07 up 5 min,  2 users,  load average: 0.00, 0.03, 0.00
EXIT_CODE:0
```

No `google_kms_crypto_key_iam_member` resource exists in state, yet both
disks correctly resolved and used the manually-granted key, and the login
instance booted and stayed reachable -- confirming this module's grant
resource is genuinely skipped, not merely hidden, while its output-aliasing
convenience still works. Setting `skip_iam_role_grants: true` together with
a non-empty `service_agents` fails `terraform plan` immediately with the
cross-variable validation error, before any resource is created:

```console
$ ./ghpc deploy DEPLOYMENT_DIR --auto-approve
...
Error: Invalid value for variable

  on main.tf line 32, in module "kms_key_iam":
  32:   skip_iam_role_grants = true
    |-----------------
    | var.service_agents is set of string with 1 element
    | var.skip_iam_role_grants is true

service_agents and custom_service_accounts must be empty when
skip_iam_role_grants is true -- there is nothing to grant when this module
isn't managing IAM.
```

Also deployed as part of a full Slurm cluster: every consumer (Filestore,
Slurm boot disks, the controller's config bucket) deployed successfully with
no `PERMISSION_DENIED` races, and the generated Terraform confirmed `use`
correctly matched `kms_key_name`, `disk_encryption_key` and
`slurm_bucket_kms_key` to their respective consumers with no explicit
wiring. On destroy, encrypted resources were removed before the grants they
depended on.

[kms-key]: ../kms-key/README.md
[pre-existing-kms-key]: ../pre-existing-kms-key/README.md

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

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.33.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_kms_crypto_key_iam_member.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/kms_crypto_key_iam_member) | resource |
| [google_project.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/project) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_crypto_key_id"></a> [crypto\_key\_id](#input\_crypto\_key\_id) | The id of the CryptoKey to grant on, of the form projects/PROJECT/locations/LOCATION/keyRings/RING/cryptoKeys/KEY. Take this from a kms-key or pre-existing-kms-key module rather than writing it out, so the grants are ordered against the key. | `string` | n/a | yes |
| <a name="input_custom_service_accounts"></a> [custom\_service\_accounts](#input\_custom\_service\_accounts) | Bare email addresses of user-managed service accounts to grant roles/cloudkms.cryptoKeyEncrypterDecrypter on the CryptoKey, e.g. "my-sa@my-project.iam.gserviceaccount.com". Use this for identities service\_agents cannot derive: a custom disk\_encryption\_key\_service\_account, or an agent belonging to a different project. Do not include the "serviceAccount:" prefix; the module adds it. Unioned with service\_agents; each account must already exist. | `list(string)` | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project whose service agents are granted on the key. This is the project holding the resources being encrypted, which need not be the project holding the key. | `string` | n/a | yes |
| <a name="input_service_agents"></a> [service\_agents](#input\_service\_agents) | Short names of the Google service agents to grant on the key. Their<br/>addresses are derived from project\_id, so the project number does not<br/>have to be looked up and pasted in. One of:<br/><br/>  compute            Compute Engine disks, images and snapshots<br/>  storage            Cloud Storage buckets and objects<br/>  filestore          Filestore instances<br/>  cloudsql           Cloud SQL instances<br/>  artifactregistry   Artifact Registry repositories<br/>  secretmanager      Secret Manager secrets<br/>  pubsub             Pub/Sub topics<br/>  notebooks          Vertex AI Workbench instances<br/><br/>Grant `compute` for Slurm boot and additional disks: with<br/>disk\_encryption\_key\_service\_account left unset, Compute Engine<br/>encrypts as its own service agent rather than as the instance's<br/>service account. | `set(string)` | `[]` | no |
| <a name="input_skip_iam_role_grants"></a> [skip\_iam\_role\_grants](#input\_skip\_iam\_role\_grants) | Skip creating the IAM grants this module normally creates, while every<br/>output still resolves crypto\_key\_id as usual. Set this when permissions<br/>on the key are managed out-of-band by someone else -- for example a<br/>security team granting roles/cloudkms.cryptoKeyEncrypterDecrypter on a<br/>pre-existing-kms-key key directly -- and the identity running this<br/>module lacks cloudkms.admin/setIamPolicy on it. Without this, Terraform<br/>would attempt the grant anyway and fail with a 403, even though the<br/>caller only wanted this module's `use`-wiring convenience.<br/><br/>service\_agents and custom\_service\_accounts must both be empty when this<br/>is true. Setting either alongside skip\_iam\_role\_grants would otherwise<br/>look like a grant request that silently does nothing, which is exactly<br/>the kind of surprising behavior this variable exists to prevent. | `bool` | `false` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_crypto_key_id"></a> [crypto\_key\_id](#output\_crypto\_key\_id) | The CryptoKey id, ordered after the encrypt/decrypt grants. This is the output to pass to any CMEK-consuming module whose input is not named below. |
| <a name="output_disk_encryption_key"></a> [disk\_encryption\_key](#output\_disk\_encryption\_key) | The CryptoKey id under the name used by Slurm instance modules, so `use` wires it automatically. Consumed by the schedmd-slurm-gcp-v6 controller, login and nodeset modules to encrypt boot disks. Leave their disk\_encryption\_key\_service\_account unset, so Compute Engine encrypts as the project's Compute Engine service agent -- that agent, not the instances' own service account, is the principal that needs the grant. |
| <a name="output_encryption_key_name"></a> [encryption\_key\_name](#output\_encryption\_key\_name) | The CryptoKey id under the name used by the Cloud SQL federation module, so `use` wires it automatically. Consumed by community/modules/database/slurm-cloudsql-federation's encryption\_key\_name. Grant the `cloudsql` service agent. |
| <a name="output_image_encryption_key"></a> [image\_encryption\_key](#output\_image\_encryption\_key) | The CryptoKey id under the name used by the custom-image Packer module, so `use` wires it automatically. Consumed by modules/packer/custom-image's image\_encryption\_key. Grant the `compute` service agent: a Packer build creates a Compute Engine disk and then an image from it. |
| <a name="output_kms_key_name"></a> [kms\_key\_name](#output\_kms\_key\_name) | The CryptoKey id under the name used by CMEK-capable storage modules, so `use` wires it automatically. Consumed by modules/file-system/filestore's kms\_key\_name. |
| <a name="output_repository_kms_key_name"></a> [repository\_kms\_key\_name](#output\_repository\_kms\_key\_name) | The CryptoKey id under the name used by the Artifact Registry module, so `use` wires it automatically. Consumed by community/modules/container/artifact-registry's repository\_kms\_key\_name. Grant the `artifactregistry` service agent, and note the key location must match the repository location. |
| <a name="output_slurm_bucket_kms_key"></a> [slurm\_bucket\_kms\_key](#output\_slurm\_bucket\_kms\_key) | The CryptoKey id under the name used by the Slurm controller for its configuration bucket, so `use` wires it automatically. Consumed by schedmd-slurm-gcp-v6-controller's slurm\_bucket\_kms\_key. The bucket is written by Cloud Storage, so that project's Cloud Storage service agent needs the grant. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
