## Description

Creates the versioned, deletion-protected Cloud Storage bucket that holds the
Terraform state of everything the Open Front End provisions; `deploy.sh`
applies it before the front end (see "Terraform State" in
`docs/admin_guide.md`).

The Toolkit's `modules/file-system/cloud-storage-bucket` is not used because it
cannot carry `prevent_destroy` (Terraform allows no `lifecycle` block on a
module call), grants `roles/storage.objectUser` only per member rather than as an
authoritative binding for the writer list, and always creates an authoritative
`roles/storage.objectViewer` binding.

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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.4.0, < 9.0.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.4.0, < 9.0.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_storage_bucket.tfstate](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket) | resource |
| [google_storage_bucket_iam_binding.state_writers](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_iam_binding) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_default_writers"></a> [default\_writers](#input\_default\_writers) | Writers managed by deploy.sh: its provisioning service account, when that account exists. Regenerated on every deploy; do not set by hand. | `list(string)` | `[]` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Base "name" for the deployment. | `string` | n/a | yes |
| <a name="input_extra_labels"></a> [extra\_labels](#input\_extra\_labels) | Extra labels to apply to created GCP resources. | `map(any)` | `{}` | no |
| <a name="input_noncurrent_version_retention_days"></a> [noncurrent\_version\_retention\_days](#input\_noncurrent\_version\_retention\_days) | Days to retain non-current (superseded) state file versions before deletion. | `number` | `90` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP Project in which to create the Terraform state bucket. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | GCP Region (bucket location) for the Terraform state bucket. | `string` | n/a | yes |
| <a name="input_state_bucket_writers"></a> [state\_bucket\_writers](#input\_state\_bucket\_writers) | IAM members, in full "serviceAccount:name@project.iam.gserviceaccount.com"<br/>form, granted roles/storage.objectUser on the state bucket: the identities<br/>that run Terraform for the front end. Set in admin.auto.tfvars, which<br/>deploy.sh creates once and never overwrites. Add any provisioning<br/>credential from another project here, or it cannot read or write its<br/>resources' state. | `list(string)` | `[]` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_bucket_name"></a> [bucket\_name](#output\_bucket\_name) | Name of the Terraform state bucket. deploy.sh passes this to the ../ root as terraform\_state\_bucket. |
| <a name="output_bucket_url"></a> [bucket\_url](#output\_bucket\_url) | gs:// URL of the Terraform state bucket. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
