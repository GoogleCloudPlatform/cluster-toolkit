<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

No requirements.

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_ansible_extra_arguments"></a> [ansible\_extra\_arguments](#input\_ansible\_extra\_arguments) | n/a | `list(string)` | <pre>[<br>  "-vv"<br>]</pre> | no |
| <a name="input_ansible_playbook_files"></a> [ansible\_playbook\_files](#input\_ansible\_playbook\_files) | n/a | `list(string)` | n/a | yes |
| <a name="input_ansible_user"></a> [ansible\_user](#input\_ansible\_user) | n/a | `string` | `"packer"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | n/a | `string` | n/a | yes |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | n/a | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->