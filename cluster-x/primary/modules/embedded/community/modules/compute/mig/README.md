<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |
| <a name="requirement_google"></a> [google](#requirement\_google) | > 5.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | > 5.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_compute_instance_group_manager.mig](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance_group_manager) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_base_instance_name"></a> [base\_instance\_name](#input\_base\_instance\_name) | Base name for the instances in the MIG | `string` | `null` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, will be used to name MIG if `var.name` is not provided | `string` | n/a | yes |
| <a name="input_ghpc_module_id"></a> [ghpc\_module\_id](#input\_ghpc\_module\_id) | Internal GHPC field, do not set this value | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the MIG | `map(string)` | n/a | yes |
| <a name="input_name"></a> [name](#input\_name) | Name of the MIG. If not provided, will be generated from `var.deployment_name` | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the MIG will be created | `string` | n/a | yes |
| <a name="input_target_size"></a> [target\_size](#input\_target\_size) | Target number of instances in the MIG | `number` | `0` | no |
| <a name="input_versions"></a> [versions](#input\_versions) | Application versions managed by this instance group. Each version deals with a specific instance template | <pre>list(object({<br/>    name              = string<br/>    instance_template = string<br/>    target_size = optional(object({<br/>      fixed   = optional(number)<br/>      percent = optional(number)<br/>    }))<br/>  }))</pre> | n/a | yes |
| <a name="input_wait_for_instances"></a> [wait\_for\_instances](#input\_wait\_for\_instances) | Whether to wait for all instances to be created/updated before returning | `bool` | `false` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone. Required, currently only zonal MIGs are supported | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_self_link"></a> [self\_link](#output\_self\_link) | The URL of the created MIG |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
