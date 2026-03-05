<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.84 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.84 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_home_pv"></a> [home\_pv](#module\_home\_pv) | ../../../../modules/file-system/gke-persistent-volume | n/a |
| <a name="module_kubectl_apply"></a> [kubectl\_apply](#module\_kubectl\_apply) | ../../../../modules/management/kubectl-apply | n/a |
| <a name="module_slurm_key_pv"></a> [slurm\_key\_pv](#module\_slurm\_key\_pv) | ../../../../modules/file-system/gke-persistent-volume | n/a |

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket_object.gke_nodeset_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/storage_bucket) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | projects/{{project}}/locations/{{location}}/clusters/{{cluster}} | `string` | n/a | yes |
| <a name="input_filestore_id"></a> [filestore\_id](#input\_filestore\_id) | An array of identifier for a filestore with the format `projects/{{project}}/locations/{{location}}/instances/{{name}}`. | `list(string)` | n/a | yes |
| <a name="input_image"></a> [image](#input\_image) | The image for slurm daemon | `string` | n/a | yes |
| <a name="input_instance_templates"></a> [instance\_templates](#input\_instance\_templates) | The URLs of Instance Templates | `list(string)` | n/a | yes |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on nodes. | <pre>list(object({<br/>    server_ip             = string,<br/>    remote_mount          = string,<br/>    local_mount           = string,<br/>    fs_type               = string,<br/>    mount_options         = string,<br/>    client_install_runner = map(string)<br/>    mount_runner          = map(string)<br/>  }))</pre> | n/a | yes |
| <a name="input_node_count_static"></a> [node\_count\_static](#input\_node\_count\_static) | The number of static nodes in node-pool | `number` | n/a | yes |
| <a name="input_node_pool_names"></a> [node\_pool\_names](#input\_node\_pool\_names) | If set to true. The node group VMs will have a random public IP assigned to it. Ignored if access\_config is set. | `list(string)` | n/a | yes |
| <a name="input_nodeset_name"></a> [nodeset\_name](#input\_nodeset\_name) | The nodeset name | `string` | `"gkenodeset"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID to host the cluster in. | `string` | n/a | yes |
| <a name="input_slurm_bucket"></a> [slurm\_bucket](#input\_slurm\_bucket) | GCS Bucket of Slurm cluster file storage. | `any` | n/a | yes |
| <a name="input_slurm_bucket_dir"></a> [slurm\_bucket\_dir](#input\_slurm\_bucket\_dir) | Path directory within `bucket_name` for Slurm cluster file storage. | `string` | n/a | yes |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Cluster name, used in slurm controller | `string` | n/a | yes |
| <a name="input_slurm_controller_instance"></a> [slurm\_controller\_instance](#input\_slurm\_controller\_instance) | Slurm cluster controller instance | `any` | n/a | yes |
| <a name="input_slurm_namespace"></a> [slurm\_namespace](#input\_slurm\_namespace) | slurm namespace for charts | `string` | `"slurm"` | no |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | Primary subnetwork object | `any` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nodeset_name"></a> [nodeset\_name](#output\_nodeset\_name) | Name of the new Slinky nodeset |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
