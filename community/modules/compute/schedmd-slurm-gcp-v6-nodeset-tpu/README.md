<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_accelerator_config"></a> [accelerator\_config](#input\_accelerator\_config) | Nodeset accelerator config, see https://cloud.google.com/tpu/docs/supported-tpu-configurations for details. | <pre>object({<br>    topology = string<br>    version  = string<br>  })</pre> | <pre>{<br>  "topology": "",<br>  "version": ""<br>}</pre> | no |
| <a name="input_data_disks"></a> [data\_disks](#input\_data\_disks) | The data disks to include in the TPU node | `list(string)` | `[]` | no |
| <a name="input_disable_public_ips"></a> [disable\_public\_ips](#input\_disable\_public\_ips) | If set to false. The node group VMs will have a random public IP assigned to it. Ignored if access\_config is set. | `bool` | `true` | no |
| <a name="input_docker_image"></a> [docker\_image](#input\_docker\_image) | The gcp container registry id docker image to use in the TPU vms, it defaults to gcr.io/schedmd-slurm-public/tpu:slurm-gcp-6-1-tf-<var.tf\_version> | `string` | `null` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the nodeset tpu. | `string` | `"ghpc"` | no |
| <a name="input_node_count_dynamic_max"></a> [node\_count\_dynamic\_max](#input\_node\_count\_dynamic\_max) | Maximum number of dynamic nodes allowed in this partition. | `number` | `1` | no |
| <a name="input_node_count_static"></a> [node\_count\_static](#input\_node\_count\_static) | Number of nodes to be statically created. | `number` | `0` | no |
| <a name="input_node_type"></a> [node\_type](#input\_node\_type) | Specify a node type to base the vm configuration upon it. | `string` | n/a | yes |
| <a name="input_preemptible"></a> [preemptible](#input\_preemptible) | Should use preemptibles to burst. | `bool` | `false` | no |
| <a name="input_preserve_tpu"></a> [preserve\_tpu](#input\_preserve\_tpu) | Specify whether TPU-vms will get preserve on suspend, if set to true, on suspend vm is stopped, on false it gets deleted | `bool` | `true` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the TPU-vm. If none is given, the default service account and scopes will be used. | <pre>object({<br>    email  = string<br>    scopes = set(string)<br>  })</pre> | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The name of the subnetwork to attach the TPU-vm of this nodeset to. | `string` | `null` | no |
| <a name="input_tf_version"></a> [tf\_version](#input\_tf\_version) | Nodeset Tensorflow version, see https://cloud.google.com/tpu/docs/supported-tpu-configurations#tpu_vm for details. | `string` | `"2.9.1"` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone in which to create compute VMs. Additional zones in the same region can be specified in var.zones. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nodeset_tpu"></a> [nodeset\_tpu](#output\_nodeset\_tpu) | Details of the nodeset tpu. Typically used as input to `schedmd-slurm-gcp-v6-partition`. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
