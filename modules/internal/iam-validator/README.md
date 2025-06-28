<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |
| <a name="requirement_external"></a> [external](#requirement\_external) | ~> 2.3 |
| <a name="requirement_null"></a> [null](#requirement\_null) | >= 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_external"></a> [external](#provider\_external) | ~> 2.3 |
| <a name="provider_null"></a> [null](#provider\_null) | >= 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [null_resource.permission_validator](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [external_external.gcp_permission_check](https://registry.terraform.io/providers/hashicorp/external/latest/docs/data-sources/external) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The GCP project ID to check permissions against. | `string` | n/a | yes |
| <a name="input_required_permissions"></a> [required\_permissions](#input\_required\_permissions) | A set of IAM permissions that the user must have on the project. | `set(string)` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_missing_permissions"></a> [missing\_permissions](#output\_missing\_permissions) | Missing permissions for principal on project. |
| <a name="output_suggested_roles"></a> [suggested\_roles](#output\_suggested\_roles) | Suggested roles for principal on project. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
