<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3.0 |

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_slurm_cluster"></a> [slurm\_cluster](#module\_slurm\_cluster) | github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster | 6.1.1 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_controller"></a> [controller](#input\_controller) | Controller configuration. DO NOT configure manually, use `controller` module instead. | `any` | <pre>{<br>  "disk_type": "pd-standard",<br>  "machine_type": "n1-standard-4"<br>}</pre> | no |
| <a name="input_debug_mode"></a> [debug\_mode](#input\_debug\_mode) | Developer debug mode:<br>- Do not create cluster resources.<br>- Output `debug` variable containing cluster configuration. | `bool` | `false` | no |
| <a name="input_name"></a> [name](#input\_name) | Cluster name, used for resource naming and slurm accounting. | `string` | n/a | yes |
| <a name="input_nodeset"></a> [nodeset](#input\_nodeset) | Nodesets configuration. DO NOT configure manually, use `nodeset` module instead. | `list(any)` | `[]` | no |
| <a name="input_partition"></a> [partition](#input\_partition) | Partitions configuration. DO NOT configure manually, use `partition` module instead. | `list(any)` | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region to create resources in. | `string` | n/a | yes |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | Subnet to deploy to. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_debug"></a> [debug](#output\_debug) | Debug output, present IFF var.debug\_mode is true |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
