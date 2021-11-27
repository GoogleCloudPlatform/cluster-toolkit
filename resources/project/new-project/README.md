<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2021 Google LLC

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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 3.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="project-factory"></a> [project\-factory](#module\_project\-factory) | terraform-google-modules/project-factory/google | ~> 10.1 |

## Inputs

| Name | Description | Type | Default |
|------|-------------|------|---------|
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project id with which project will be created                | `string` | n/a     |
| <a name="input_folder_id"></a> [folder\_id](#input\_folder\_id) | Folder id in which Project will be created                   | `string` | n/a     |
| <a name="input_billing_account"></a> [billing\_account](#input\_billing\_account) | Billing account against which Project and its resources will be created | `string` | n/a     |
| <a name="input_default_service_account"></a> [default\_service\_account](#input\_default\_service\_account) | Project default service account setting: can be one of `delete`, `deprivilege`, `disable`, or `keep`." | `string` | `keep`. |

<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->