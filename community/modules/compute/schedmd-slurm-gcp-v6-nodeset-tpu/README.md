## Description

This module creates partition of [TPU](https://cloud.google.com/tpu/docs/intro-to-tpu) nodeset.
TPUs are Google's custom-developed application specific ICs to accelerate machine
learning workloads.

### Example

The following code snippet creates TPU partition with following attributes.

- TPU nodeset module is connected to `network` module.
- TPU nodeset is of type `v2-8` and version `2.10.0`, you can check different configuration [configuration](https://cloud.google.com/tpu/docs/supported-tpu-configurations)
- TPU vms are preemptible.
- `preserve_tpu` is set to false. This means, suspended vms will be deleted.
- Partition module uses this defined `tpu_nodeset` module and this partition can
be accessed as `tpu` partition.

```yaml
  - id: tpu_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset-tpu
    use: [network]
    settings:
      node_type: v2-8
      tf_version: 2.10.0
      disable_public_ips: false
      preemptible: true
      preserve_tpu: false

  - id: tpu_partition
    source: community/modules/compute/schedmd-slurm-gcp-v6-partition
    use: [tpu_nodeset]
    settings:
      partition_name: tpu
```

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
| <a name="input_accelerator_config"></a> [accelerator\_config](#input\_accelerator\_config) | Nodeset accelerator config, see https://cloud.google.com/tpu/docs/supported-tpu-configurations for details. | <pre>object({<br/>    topology = string<br/>    version  = string<br/>  })</pre> | <pre>{<br/>  "topology": "",<br/>  "version": ""<br/>}</pre> | no |
| <a name="input_data_disks"></a> [data\_disks](#input\_data\_disks) | The data disks to include in the TPU node | `list(string)` | `[]` | no |
| <a name="input_disable_public_ips"></a> [disable\_public\_ips](#input\_disable\_public\_ips) | DEPRECATED: Use `enable_public_ips` instead. | `bool` | `null` | no |
| <a name="input_docker_image"></a> [docker\_image](#input\_docker\_image) | The gcp container registry id docker image to use in the TPU vms, it defaults to gcr.io/schedmd-slurm-public/tpu:slurm-gcp-6-8-tf-<var.tf\_version> | `string` | `null` | no |
| <a name="input_enable_public_ips"></a> [enable\_public\_ips](#input\_enable\_public\_ips) | If set to true. The node group VMs will have a random public IP assigned to it. Ignored if access\_config is set. | `bool` | `false` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the nodeset. Automatically populated by the module id if not set. <br/>If setting manually, ensure a unique value across all nodesets. | `string` | n/a | yes |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on nodes. | <pre>list(object({<br/>    server_ip     = string,<br/>    remote_mount  = string,<br/>    local_mount   = string,<br/>    fs_type       = string,<br/>    mount_options = string,<br/>  }))</pre> | `[]` | no |
| <a name="input_node_count_dynamic_max"></a> [node\_count\_dynamic\_max](#input\_node\_count\_dynamic\_max) | Maximum number of auto-scaling worker nodes allowed in this partition. <br/>For larger TPU machines, there are multiple worker nodes required per machine (1 for every 8 cores).<br/>See https://cloud.google.com/tpu/docs/v4#large-topologies, for more information about these machine types. | `number` | `0` | no |
| <a name="input_node_count_static"></a> [node\_count\_static](#input\_node\_count\_static) | Number of worker nodes to be statically created. <br/>For larger TPU machines, there are multiple worker nodes required per machine (1 for every 8 cores).<br/>See https://cloud.google.com/tpu/docs/v4#large-topologies, for more information about these machine types. | `number` | `0` | no |
| <a name="input_node_type"></a> [node\_type](#input\_node\_type) | Specify a node type to base the vm configuration upon it. | `string` | `""` | no |
| <a name="input_preemptible"></a> [preemptible](#input\_preemptible) | Should use preemptibles to burst. | `bool` | `false` | no |
| <a name="input_preserve_tpu"></a> [preserve\_tpu](#input\_preserve\_tpu) | Specify whether TPU-vms will get preserve on suspend, if set to true, on suspend vm is stopped, on false it gets deleted | `bool` | `false` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_reserved"></a> [reserved](#input\_reserved) | Specify whether TPU-vms in this nodeset are created under a reservation. | `bool` | `false` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | DEPRECATED: Use `service_account_email` and `service_account_scopes` instead. | <pre>object({<br/>    email  = string<br/>    scopes = set(string)<br/>  })</pre> | `null` | no |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | Service account e-mail address to attach to the TPU-vm. | `string` | `null` | no |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Scopes to attach to the TPU-vm. | `set(string)` | <pre>[<br/>  "https://www.googleapis.com/auth/cloud-platform"<br/>]</pre> | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The name of the subnetwork to attach the TPU-vm of this nodeset to. | `string` | n/a | yes |
| <a name="input_tf_version"></a> [tf\_version](#input\_tf\_version) | Nodeset Tensorflow version, see https://cloud.google.com/tpu/docs/supported-tpu-configurations#tpu_vm for details. | `string` | `"2.14.0"` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone in which to create compute VMs. TPU partitions can only specify a single zone. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nodeset_tpu"></a> [nodeset\_tpu](#output\_nodeset\_tpu) | Details of the nodeset tpu. Typically used as input to `schedmd-slurm-gcp-v6-partition`. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
