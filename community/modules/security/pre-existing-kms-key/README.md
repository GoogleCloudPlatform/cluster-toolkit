## Description

Looks up a Cloud KMS CryptoKey that already exists and publishes its id, for
blueprints that must use a key someone else owns — a key provisioned by a
security team, held in a dedicated key project, or created outside Terraform
entirely.

Nothing here is created, so `terraform destroy` leaves the key completely
untouched: it is never in Terraform state to begin with. That is a stronger
guarantee than [kms-key] offers, where the key is created and then deliberately
abandoned rather than deleted.

This module does not grant anyone access to the key. Pass `crypto_key_id` to a
[kms-key-iam] module, and have CMEK consumers `use` that.

## Example usage

```yaml
- id: existing_key
  source: community/modules/security/pre-existing-kms-key
  settings:
    project_id: my-key-project
    location: us-central1
    key_ring_name: security-team-ring
    key_name: hpc-cmek

- id: kms_key_iam
  source: community/modules/security/kms-key-iam
  use: [existing_key]
  settings:
    service_agent_principals:
    - "serviceAccount:service-1234567890@compute-system.iam.gserviceaccount.com"
```

`project_id` is the *key* project, which need not be the project the encrypted
resources live in. Granting a service agent from another project is exactly how
the dedicated-key-project topology works.

## Requirements

The key must be a symmetric `ENCRYPT_DECRYPT` CryptoKey; the module fails at
plan time otherwise, rather than when the first resource that depends on it is
created — by which point the error names the consumer rather than the key.

The identity running Terraform needs `cloudkms.cryptoKeys.get` on the key (for
example `roles/cloudkms.viewer` on the key or its ring), and — if a
[kms-key-iam] module is granting on it — `cloudkms.cryptoKeys.setIamPolicy`.

A key can only encrypt resources in its own location, unless it is `global`.

[kms-key]: ../kms-key/README.md
[kms-key-iam]: ../kms-key-iam/README.md

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
| [google_kms_crypto_key.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/kms_crypto_key) | data source |
| [google_kms_key_ring.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/kms_key_ring) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_key_name"></a> [key\_name](#input\_key\_name) | The name of the existing symmetric CryptoKey. | `string` | n/a | yes |
| <a name="input_key_ring_name"></a> [key\_ring\_name](#input\_key\_ring\_name) | The name of the existing Cloud KMS key ring holding the CryptoKey. | `string` | n/a | yes |
| <a name="input_location"></a> [location](#input\_location) | The Cloud KMS location of the key ring, e.g. "us-central1" or "global". A key can only encrypt resources in its own location, except for "global" keys. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project holding the Cloud KMS key ring. This is the key project, which need not be the project the encrypted resources live in. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_crypto_key_id"></a> [crypto\_key\_id](#output\_crypto\_key\_id) | The canonical resource ID of the adopted CryptoKey. Pass this to a kms-key-iam module; CMEK consumers should `use` that module rather than this one, so they are ordered behind the grants. |
| <a name="output_key_ring_id"></a> [key\_ring\_id](#output\_key\_ring\_id) | The canonical resource ID of the key ring holding the CryptoKey. |
| <a name="output_primary_crypto_key_version_id"></a> [primary\_crypto\_key\_version\_id](#output\_primary\_crypto\_key\_version\_id) | The resource name of the CryptoKey's current primary CryptoKeyVersion. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->