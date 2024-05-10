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

| Name | Source | Version |
|------|--------|---------|
| <a name="module_instance_template"></a> [instance\_template](#module\_instance\_template) | github.com/GoogleCloudPlatform/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_instance_template | 6.5.7&depth=1 |

## Resources

| Name | Type |
|------|------|
| [google_compute_instance_group_manager.mig](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance_group_manager) | resource |
| [google_compute_default_service_account.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_default_service_account) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to instances. Key-value pairs. | `map(string)` | `{}` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to create. | `string` | `"n1-standard-1"` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the nodeset. Automatically populated by the module id if not set.<br>If setting manually, ensure a unique value across all nodesets. | `string` | n/a | yes |
| <a name="input_node_feature"></a> [node\_feature](#input\_node\_feature) | Nodeset feature for dynamic registration. Defaults to nodeset name. | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The default region for Cloud resources. | `string` | n/a | yes |
| <a name="input_slurm_bucket_path"></a> [slurm\_bucket\_path](#input\_slurm\_bucket\_path) | GCS Bucket URI of Slurm cluster file storage. | `string` | n/a | yes |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Name of the Slurm cluster. | `string` | n/a | yes |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | Subnet to deploy to. | `string` | n/a | yes |
| <a name="input_target_size"></a> [target\_size](#input\_target\_size) | The target number of running instances for this managed instance group. | `number` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone in which to create compute VMs. Additional zones in the same region can be specified in var.zones. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nodeset_dyn"></a> [nodeset\_dyn](#output\_nodeset\_dyn) | Details of the nodeset. To be used as input to `schedmd-slurm-gcp-v6-partition`. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
