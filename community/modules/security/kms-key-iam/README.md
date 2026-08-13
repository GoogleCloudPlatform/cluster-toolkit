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
    service_agent_principals:
    - "serviceAccount:service-PROJECT_NUMBER@compute-system.iam.gserviceaccount.com"
    - "serviceAccount:service-PROJECT_NUMBER@cloud-filer.iam.gserviceaccount.com"

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

## Testing

`terraform validate` passes on this module in isolation.

Live-verified against a real GCP project, granting `compute`, `storage` and
`filestore` on both a generated key ([kms-key]) and an adopted key
([pre-existing-kms-key]):

* Grants land on the correct project service agents in both cases --
  confirmed with `gcloud kms keys get-iam-policy`.
* Ordering behind the grants works as designed: every consumer (Filestore,
  Slurm boot disks, the controller's config bucket) deployed successfully
  with no `PERMISSION_DENIED` races, across multiple full-cluster deploys.
* Output aliasing confirmed correct by inspecting the generated Terraform:
  `kms_key_name` reached `modules/file-system/filestore`, `disk_encryption_key`
  reached the Slurm boot disk settings, and `slurm_bucket_kms_key` reached the
  controller's Slurm bucket setting, all via `use` with no explicit wiring.
* On destroy, the encrypted resources were removed before the grants they
  depended on, with no ordering errors, across every teardown performed.

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
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project whose service agents are granted on the key. This is the project holding the resources being encrypted, which need not be the project holding the key. | `string` | n/a | yes |
| <a name="input_service_agent_principals"></a> [service\_agent\_principals](#input\_service\_agent\_principals) | Fully qualified principal strings granted roles/cloudkms.cryptoKeyEncrypterDecrypter on the CryptoKey, e.g. "serviceAccount:service-PROJECT\_NUMBER@compute-system.iam.gserviceaccount.com". Use this for principals service\_agents cannot derive: agents belonging to a different project, or a user-managed service account. Unioned with service\_agents; each principal must already exist. | `set(string)` | `[]` | no |
| <a name="input_service_agents"></a> [service\_agents](#input\_service\_agents) | Short names of the Google service agents to grant on the key. Their<br/>addresses are derived from project\_id, so the project number does not<br/>have to be looked up and pasted in. One of:<br/><br/>  compute            Compute Engine disks, images and snapshots<br/>  storage            Cloud Storage buckets and objects<br/>  filestore          Filestore instances<br/>  cloudsql           Cloud SQL instances<br/>  artifactregistry   Artifact Registry repositories<br/>  secretmanager      Secret Manager secrets<br/>  pubsub             Pub/Sub topics<br/>  notebooks          Vertex AI Workbench instances<br/><br/>Grant `compute` for Slurm boot and additional disks: with<br/>disk\_encryption\_key\_service\_account left unset, Compute Engine<br/>encrypts as its own service agent rather than as the instance's<br/>service account. | `set(string)` | `[]` | no |

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
