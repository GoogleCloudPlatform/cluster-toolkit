<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.0, < 5.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | >= 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.0, < 5.0 |
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
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name for HTCondor pool | `string` | n/a | yes |
| <a name="input_pool_password"></a> [pool\_password](#input\_pool\_password) | HTCondor Pool Password | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which HTCondor pool will be created | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_access_point_runners"></a> [access\_point\_runners](#output\_access\_point\_runners) | Toolkit Runner to configure an HTCondor Access Point |
| <a name="output_access_point_service_account"></a> [access\_point\_service\_account](#output\_access\_point\_service\_account) | HTCondor Access Point Service Account (e-mail format) |
| <a name="output_central_manager_runners"></a> [central\_manager\_runners](#output\_central\_manager\_runners) | Toolkit Runner to configure an HTCondor Central Manager |
| <a name="output_central_manager_service_account"></a> [central\_manager\_service\_account](#output\_central\_manager\_service\_account) | HTCondor Central Manager Service Account (e-mail format) |
| <a name="output_pool_password_secret_id"></a> [pool\_password\_secret\_id](#output\_pool\_password\_secret\_id) | Google Cloud Secret Manager ID containing HTCondor Pool Password |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
