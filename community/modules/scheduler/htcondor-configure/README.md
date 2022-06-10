## Description

**THIS MODULE IS PRE-RELEASE AND DOES NOT YET SUPPORT A FULLY FUNCTIONAL
HTCONDOR POOL**

This module performs the following tasks:

- store an HTCondor Pool password in Google Cloud Secret Manager
  - will generate a new password if one is not supplied
- create service accounts for an HTCondor Access Point and Central Manager
- create a Toolkit runner for an Access Point
- create a Toolkit runner for a Central Manager

[htcrole]: https://htcondor.readthedocs.io/en/latest/getting-htcondor/admin-quick-start.html#what-get-htcondor-does-to-configure-a-role

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2022 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_random"></a> [random](#requirement\_random) | >= 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_random"></a> [random](#provider\_random) | >= 3.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_access_point_service_account"></a> [access\_point\_service\_account](#module\_access\_point\_service\_account) | terraform-google-modules/service-accounts/google | ~> 4.1 |
| <a name="module_central_manager_service_account"></a> [central\_manager\_service\_account](#module\_central\_manager\_service\_account) | terraform-google-modules/service-accounts/google | ~> 4.1 |

## Resources

| Name | Type |
|------|------|
| [google_secret_manager_secret.pool_password](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret) | resource |
| [google_secret_manager_secret_iam_member.access_point](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_iam_member) | resource |
| [google_secret_manager_secret_iam_member.central_manager](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_iam_member) | resource |
| [google_secret_manager_secret_version.pool_password](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_version) | resource |
| [random_password.pool](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/password) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_access_point_roles"></a> [access\_point\_roles](#input\_access\_point\_roles) | Project-wide roles for HTCondor Access Point service account | `list(string)` | <pre>[<br>  "roles/monitoring.metricWriter",<br>  "roles/logging.logWriter",<br>  "roles/storage.objectViewer"<br>]</pre> | no |
| <a name="input_central_manager_roles"></a> [central\_manager\_roles](#input\_central\_manager\_roles) | Project-wide roles for HTCondor Central Manager service account | `list(string)` | <pre>[<br>  "roles/compute.instanceAdmin",<br>  "roles/monitoring.metricWriter",<br>  "roles/logging.logWriter",<br>  "roles/storage.objectViewer"<br>]</pre> | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. HTCondor cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_pool_password"></a> [pool\_password](#input\_pool\_password) | HTCondor Pool Password | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which HTCondor pool will be created | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_access_point_runner"></a> [access\_point\_runner](#output\_access\_point\_runner) | Toolkit Runner to configure an HTCondor Access Point |
| <a name="output_access_point_service_account"></a> [access\_point\_service\_account](#output\_access\_point\_service\_account) | HTCondor Access Point Service Account (e-mail format) |
| <a name="output_central_manager_runner"></a> [central\_manager\_runner](#output\_central\_manager\_runner) | Toolkit Runner to configure an HTCondor Central Manager |
| <a name="output_central_manager_service_account"></a> [central\_manager\_service\_account](#output\_central\_manager\_service\_account) | HTCondor Central Manager Service Account (e-mail format) |
| <a name="output_pool_password_secret_id"></a> [pool\_password\_secret\_id](#output\_pool\_password\_secret\_id) | Google Cloud Secret Manager ID containing HTCondor Pool Password |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
