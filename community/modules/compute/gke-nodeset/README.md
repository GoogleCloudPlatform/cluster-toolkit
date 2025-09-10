<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.84 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.84 |

## Modules

| Name | Source | Version |
|------|--------|---------|
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
| <a name="input_allocatable_gpu_per_node"></a> [allocatable\_gpu\_per\_node](#input\_allocatable\_gpu\_per\_node) | Number of GPUs available for scheduling pods on each node. | `number` | `0` | no |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | projects/{{project}}/locations/{{location}}/clusters/{{cluster}} | `string` | n/a | yes |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the nodes. | `list(any)` | `[]` | no |
| <a name="input_has_gpu"></a> [has\_gpu](#input\_has\_gpu) | If set to true, the nodeset template's Pod spec will contain request/limit for gpu resource. | `bool` | `false` | no |
| <a name="input_has_tpu"></a> [has\_tpu](#input\_has\_tpu) | If set to true, the nodeset template's Pod spec will contain request/limit for TPU resource, open port 8740 for TPU communication and add toleration for google.com/tpu. | `bool` | `false` | no |
| <a name="input_image"></a> [image](#input\_image) | The image for slurm daemon | `string` | n/a | yes |
| <a name="input_instance_templates"></a> [instance\_templates](#input\_instance\_templates) | The URLs of Instance Templates | `list(string)` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | The name of a Google Compute Engine machine type. | `string` | `"c2-standard-60"` | no |
| <a name="input_node_count_static"></a> [node\_count\_static](#input\_node\_count\_static) | The number of static nodes in node-pool | `number` | n/a | yes |
| <a name="input_node_pool_names"></a> [node\_pool\_names](#input\_node\_pool\_names) | If set to true. The node group VMs will have a random public IP assigned to it. Ignored if access\_config is set. | `list(string)` | n/a | yes |
| <a name="input_nodeset_name"></a> [nodeset\_name](#input\_nodeset\_name) | The nodeset name | `string` | `"gkenodeset"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID to host the cluster in. | `string` | n/a | yes |
| <a name="input_pvc_name"></a> [pvc\_name](#input\_pvc\_name) | An object that describes a k8s PVC created by this module. | `string` | n/a | yes |
| <a name="input_slurm_bucket"></a> [slurm\_bucket](#input\_slurm\_bucket) | GCS Bucket of Slurm cluster file storage. | `any` | n/a | yes |
| <a name="input_slurm_bucket_dir"></a> [slurm\_bucket\_dir](#input\_slurm\_bucket\_dir) | Path directory within `bucket_name` for Slurm cluster file storage. | `string` | n/a | yes |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Cluster name, used in slurm controller | `string` | n/a | yes |
| <a name="input_slurm_controller_instance"></a> [slurm\_controller\_instance](#input\_slurm\_controller\_instance) | Slurm cluster controller instance | `any` | n/a | yes |
| <a name="input_slurm_namespace"></a> [slurm\_namespace](#input\_slurm\_namespace) | slurm namespace for charts | `string` | `"slurm"` | no |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | Primary subnetwork object | `any` | n/a | yes |
| <a name="input_tpu_accelerator"></a> [tpu\_accelerator](#input\_tpu\_accelerator) | Name of the TPU accelerator (cloud.google.com/gke-tpu-accelerator annotation). Required when has\_tpu=true | `string` | `null` | no |
| <a name="input_tpu_chips_per_node"></a> [tpu\_chips\_per\_node](#input\_tpu\_chips\_per\_node) | Number of TPU chips per node. Required when has\_tpu=true | `number` | `0` | no |
| <a name="input_tpu_topology"></a> [tpu\_topology](#input\_tpu\_topology) | TPU topology. Required when has\_tpu=true | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nodeset_name"></a> [nodeset\_name](#output\_nodeset\_name) | Name of the new Slinky nodset |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
