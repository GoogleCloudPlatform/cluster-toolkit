## Description

Creates a symmetric Cloud KMS CryptoKey, in a key ring it creates
(`key_ring_name`) or one that already exists (`key_ring_id`). It supports both
co-located keys (pass the workload project as `project_id`) and centralized keys
(pass a dedicated key project). The CryptoKey uses
`deletion_policy = "DELETE"` by default, so an ordinary `terraform destroy`
destroys the CryptoKey's key version(s) along with it, rendering data
encrypted with the key permanently unrecoverable. Set `deletion_policy =
"ABANDON"` for a key whose data must outlive the deployment. Neither setting
frees the key ring or the CryptoKey name -- see "Lifecycle and naming" below.

This module creates the key and nothing else. Two companions complete the set:

* [kms-key-iam] grants service agents on a key, and is what CMEK consumers
  should `use` — consuming this module directly is a race, see below.
* [pre-existing-kms-key] adopts a key someone else owns, and is
  interchangeable with this module downstream.

## Example usage

```yaml
- id: kms_key
  source: community/modules/security/kms-key
  settings:
    project_id: my-project
    location: us-central1
    key_ring_name: my-keyring
    key_name: my-key

- id: kms_key_iam
  source: community/modules/security/kms-key-iam
  use: [kms_key]
  settings:
    service_agent_principals:
    - "serviceAccount:service-PROJECT_NUMBER@cloud-filer.iam.gserviceaccount.com"
    - "serviceAccount:service-PROJECT_NUMBER@compute-system.iam.gserviceaccount.com"

- id: homefs
  source: modules/file-system/filestore
  use: [network, kms_key_iam]
  settings:
    local_mount: /home
    filestore_tier: ZONAL   # CMEK needs ZONAL, REGIONAL or ENTERPRISE
    size_gb: 1024
```

### Consuming the key

**`use` the [kms-key-iam] module, not this one.** This module's
`crypto_key_id` is available as soon as the key exists, which is before any
service agent has been granted on it. A consumer wired directly to it can be
created first and fail with a KMS `PERMISSION_DENIED` that depends only on how
Terraform happened to schedule the two. kms-key-iam re-exports the same id
ordered behind its grants, under a name matching each consumer's own input.

## Lifecycle and naming

Cloud KMS makes several of this module's inputs permanent, so they deserve
care before the first apply:

* **Key rings can never be deleted, and CryptoKey names can never be
  reused.** Pick `key_ring_name` and `key_name` deliberately.
* **Create a ring, or reuse one — supply exactly one of `key_ring_name` or
  `key_ring_id`.** `key_ring_name` creates a new ring. `key_ring_id` points at
  an existing ring and creates only the CryptoKey inside it, which is how many
  CryptoKeys can share a single long-lived ring instead of each deployment
  stranding another permanent one. The ring may live in any project, including
  a dedicated key project, but `project_id` and `location` must then name that
  ring's project and location.
* **Re-applying a destroyed deployment needs one of those two paths.** Teardown
  retains the key ring and CryptoKey, so re-running with the same
  `key_ring_name` fails with `Error 409: ... already exists`. Either pass the
  retained ring as `key_ring_id` together with a fresh `key_name`, or choose new
  names (for example derived from `deployment_name`). Nothing is silently
  reused and nothing is lost, but a redeploy is never fully automatic.
* **Teardown destroys key material by default.** `terraform destroy` drops
  the CryptoKey from Terraform state and schedules every key version for
  destruction, after which anything encrypted with them is permanently
  unrecoverable -- the key ring and CryptoKey name are still retained (Cloud
  KMS never frees either), but the data they protected is not. Set
  `deletion_policy = "ABANDON"` for a key whose data must outlive the
  deployment: teardown then leaves every key version intact and enabled, and
  data encrypted with the key stays decryptable after the deployment is
  gone. Unlike `protection_level` and `destroy_scheduled_duration`,
  `deletion_policy` is an in-place update, so it can be changed on an
  existing key by re-applying -- including switching an existing key to
  ABANDON before a teardown you want it to survive.
* **`protection_level` and `destroy_scheduled_duration` are chosen at creation,
  not changed later.** Cloud KMS cannot alter either on an existing CryptoKey,
  so Terraform would have to replace the CryptoKey — which fails with
  `Error 409: ... already exists` because the retained key still holds the
  name. Use a new `key_name` to move to a different protection level or
  destroy-scheduled duration. `rotation_period` and `labels`, by contrast, are
  updated in place.
* **Recovering a CryptoKey that is missing from state** (after the above, or
  after state loss) is done by importing it rather than renaming it:

  ```shell
  terraform import module.<id>.google_kms_crypto_key.this <crypto_key_id>
  ```

Automatic rotation creates a new primary version but does not re-encrypt
existing data or retire old versions, so previous versions remain required
for previously encrypted data. This module never disables or destroys a key
version outside of `terraform destroy`, whose effect on existing versions is
controlled entirely by `deletion_policy`.

## Testing

`terraform validate` passes on this module in isolation.

Live-verified against a real GCP project, using both a full trimmed Slurm
cluster and a minimal standalone deployment:

* A generated key comes up `ENABLED`/`ENCRYPT_DECRYPT`, and `kms-key-iam`
  grants land on the expected service agents (compute, storage, filestore) --
  confirmed with `gcloud kms keys describe` / `get-iam-policy`.
* Consumers actually use the key, not a Google-managed one: a controller boot
  disk and a Filestore instance both confirmed via
  `gcloud ... describe --format="value(...kmsKeyName)"` pointing at the
  generated key.
* A real Slurm job (`srun`) completed successfully on a CMEK-encrypted
  compute node.
* Disabling the key's version produces lockout without deleting anything: a
  Compute Engine instance start is rejected with an explicit `DISABLED`-state
  error (and a `kmsKeyError` system event forcing the instance to
  `TERMINATED`), the Filestore instance transitions to `SUSPENDED`, and Cloud
  Storage reads fail with `KEY_DISABLED` -- all three resources remain
  listed, only access is blocked. Re-enabling the version restores all three
  without redeploying.
* `deletion_policy = "DELETE"` (the default): `terraform destroy` schedules
  the key's version for destruction (`DESTROY_SCHEDULED`, with `destroyTime`
  set `destroy_scheduled_duration` out) -- confirmed on a live key,
  independently reproduced twice.
* `deletion_policy = "ABANDON"`: `terraform destroy` leaves the key ring,
  CryptoKey and every version intact and `ENABLED`.
* Redeploying under a fresh `key_ring_id`/`key_name` after a teardown works
  as documented; redeploying under the same retained ring/key name fails
  with `Error 409: ... already exists`, as expected.

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

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.33.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_kms_crypto_key.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/kms_crypto_key) | resource |
| [google_kms_key_ring.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/kms_key_ring) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_deletion_policy"></a> [deletion\_policy](#input\_deletion\_policy) | What `terraform destroy` does with the CryptoKey this module created.<br/><br/>  DELETE   destroy all key versions, rendering data encrypted with<br/>           them permanently unrecoverable<br/>  ABANDON  drop it from Terraform state, leaving the CryptoKey and<br/>           every key version intact and enabled in Cloud KMS<br/><br/>DELETE is the default, matching the provider's own default for this<br/>resource: a key this deployment created is this deployment's to destroy,<br/>so tearing the deployment down destroys its key material rather than<br/>leaving it enabled forever. Set ABANDON when data encrypted with this<br/>key must outlive the deployment that created it -- for example a<br/>Filestore instance or bucket meant to survive `terraform destroy`.<br/><br/>Neither setting frees the CryptoKey name: Cloud KMS never deletes a<br/>CryptoKey resource itself, only DELETE additionally destroys its<br/>version(s). A key adopted with the pre-existing-kms-key module is never<br/>affected by this variable, since that module never creates a<br/>google\_kms\_crypto\_key resource for `terraform destroy` to act on.<br/><br/>Changing this is an in-place update, so it can be set on an existing<br/>key by re-applying -- unlike protection\_level and<br/>destroy\_scheduled\_duration, which are fixed at creation. | `string` | `"DELETE"` | no |
| <a name="input_destroy_scheduled_duration"></a> [destroy\_scheduled\_duration](#input\_destroy\_scheduled\_duration) | The period a CryptoKeyVersion spends in DESTROY\_SCHEDULED before transitioning to DESTROYED, expressed as a duration string ending in "s" (seconds), e.g. "2592000s" for 30 days. Chosen at creation and immutable afterwards; use a new key\_name to change it. See the module README. | `string` | `"2592000s"` | no |
| <a name="input_key_name"></a> [key\_name](#input\_key\_name) | The permanent name of the symmetric CryptoKey. Cloud KMS CryptoKey names cannot be renamed and cannot be reused once destroyed. | `string` | n/a | yes |
| <a name="input_key_ring_id"></a> [key\_ring\_id](#input\_key\_ring\_id) | The id of an existing Cloud KMS key ring to create the CryptoKey in, for<br/>example "projects/my-project/locations/us-central1/keyRings/my-keyring".<br/>Set this instead of key\_ring\_name to reuse a key ring rather than create<br/>one, which is what makes it possible to hold many CryptoKeys in a single<br/>long-lived ring and to redeploy after a teardown that retained the ring.<br/>Exactly one of key\_ring\_name or key\_ring\_id must be supplied. | `string` | `null` | no |
| <a name="input_key_ring_name"></a> [key\_ring\_name](#input\_key\_ring\_name) | The permanent name of a Cloud KMS key ring to create. Cloud KMS key ring names cannot be changed or reused once created. Leave null when adopting an existing ring with key\_ring\_id. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the CryptoKey. Key-value pairs. Cloud KMS key rings and IAM members do not support labels. | `map(string)` | `{}` | no |
| <a name="input_location"></a> [location](#input\_location) | The Cloud KMS location (region or multi-region) in which to create the key ring, e.g. "us-central1" or "us". Must be a location that can serve the resources being encrypted; Cloud KMS validates it. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project in which to create the Cloud KMS key ring and CryptoKey. Pass the workload project for co-located keys, or a dedicated key project for centralized keys. | `string` | n/a | yes |
| <a name="input_protection_level"></a> [protection\_level](#input\_protection\_level) | The protection level for new CryptoKeyVersions, either "SOFTWARE" or "HSM". Chosen at creation and effectively immutable afterwards; use a new key\_name to change it. See the module README. | `string` | `"SOFTWARE"` | no |
| <a name="input_rotation_period"></a> [rotation\_period](#input\_rotation\_period) | The interval between automatic CryptoKeyVersion rotations, expressed as a duration string ending in "s" (seconds), e.g. "7776000s" for 90 days. Must be greater than one day (86400s). Cannot currently be disabled. | `string` | `"7776000s"` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_crypto_key_id"></a> [crypto\_key\_id](#output\_crypto\_key\_id) | The canonical resource ID of the CryptoKey. Pass this to a kms-key-iam module; CMEK consumers should `use` that module rather than this one, so they are ordered behind the grants. |
| <a name="output_key_ring_id"></a> [key\_ring\_id](#output\_key\_ring\_id) | The canonical resource ID of the Cloud KMS key ring holding the CryptoKey, whether this module created it or adopted an existing one. |
| <a name="output_primary_crypto_key_version_id"></a> [primary\_crypto\_key\_version\_id](#output\_primary\_crypto\_key\_version\_id) | The resource name of the CryptoKey's current primary CryptoKeyVersion. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->

[kms-key-iam]: ../kms-key-iam/README.md
[pre-existing-kms-key]: ../pre-existing-kms-key/README.md
