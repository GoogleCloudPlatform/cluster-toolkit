<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |
| <a name="requirement_null"></a> [null](#requirement\_null) | >= 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_null"></a> [null](#provider\_null) | >= 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [null_resource.dependencies](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [null_resource.script](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_enable_cleanup_compute"></a> [enable\_cleanup\_compute](#input\_enable\_cleanup\_compute) | Enables automatic cleanup of compute nodes and resource policies (e.g.<br/>placement groups) managed by this module, when cluster is destroyed.<br/><br/>*WARNING*: Toggling this off will impact the running workload.<br/>Deployed compute nodes will be destroyed. | `bool` | n/a | yes |
| <a name="input_endpoint_versions"></a> [endpoint\_versions](#input\_endpoint\_versions) | Version of the API to use (The compute service is the only API currently supported) | <pre>object({<br/>    compute = string<br/>  })</pre> | n/a | yes |
| <a name="input_gcloud_path_override"></a> [gcloud\_path\_override](#input\_gcloud\_path\_override) | Directory of the gcloud executable to be used during cleanup | `string` | n/a | yes |
| <a name="input_nodeset"></a> [nodeset](#input\_nodeset) | Nodeset to cleanup | <pre>object({<br/>    nodeset_name         = string<br/>    subnetwork_self_link = string<br/>    additional_networks = list(object({<br/>      subnetwork = string<br/>    }))<br/>  })</pre> | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID | `string` | n/a | yes |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Name of the Slurm cluster | `string` | n/a | yes |
| <a name="input_universe_domain"></a> [universe\_domain](#input\_universe\_domain) | Domain address for alternate API universe | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
