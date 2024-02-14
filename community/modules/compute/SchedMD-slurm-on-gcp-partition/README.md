## Description

> **Warning**: this module is now deprecated. We recommend using the Slurm on GCP V5
> [schedmd-slurm-gcp-v5-partition](../schedmd-slurm-gcp-v5-partition/README.md) and
> [schedmd-slurm-gcp-v5-node-group](../schedmd-slurm-gcp-v5-node-group/README.md) instead.

This module creates a compute partition that be can used as input to
[SchedMD-slurm-on-gcp-controller](../../scheduler/SchedMD-slurm-on-gcp-controller/README.md).

> **Warning**: updating a partition will not cause the slurm controller to
> update its configurations. In other words, it will not update an already
> deployed Slurm cluster.

### Example

The following code snippet creates a partition module with:

* a max node count of 200
* VM machine type of `c2-standard-30`
* partition name of "compute"
* connected to the `network1` module via `use`
* Mounted to homefs via `use`

```yaml
- id: compute_partition
  source: community/modules/compute/SchedMD-slurm-on-gcp-partition
  use: [network1, homefs]
  settings:
    max_node_count: 200
    partition_name: compute
    machine_type: c2-standard-30
```

## GPU Support

More information on GPU support in Slurm on GCP and other HPC Toolkit modules
can be found at [docs/gpu-support.md](../../../../docs/gpu-support.md)

## Support
The HPC Toolkit team maintains the wrapper around the [slurm-on-gcp] terraform
modules. For support with the underlying modules, see the instructions in the
[slurm-gcp README][slurm-gcp-readme].

[slurm-on-gcp]: https://github.com/SchedMD/slurm-gcp
[slurm-gcp-readme]: https://github.com/SchedMD/slurm-gcp#slurm-on-google-cloud-platform

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_bandwidth_tier"></a> [bandwidth\_tier](#input\_bandwidth\_tier) | Configures the network interface card and the maximum egress bandwidth for VMs.<br>  - Setting `platform_default` respects the Google Cloud Platform API default values for networking.<br>  - Setting `virtio_enabled` explicitly selects the VirtioNet network adapter.<br>  - Setting `gvnic_enabled` selects the gVNIC network adapter (without Tier 1 high bandwidth).<br>  - Setting `tier_1_enabled` selects both the gVNIC adapter and Tier 1 high bandwidth networking.<br>  - Note: both gVNIC and Tier 1 networking require a VM image with gVNIC support as well as specific VM families and shapes.<br>  - See [official docs](https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration) for more details. | `string` | `"platform_default"` | no |
| <a name="input_compute_disk_size_gb"></a> [compute\_disk\_size\_gb](#input\_compute\_disk\_size\_gb) | Size of boot disk to create for the partition compute nodes | `number` | `20` | no |
| <a name="input_compute_disk_type"></a> [compute\_disk\_type](#input\_compute\_disk\_type) | Type of boot disk to create for the partition compute nodes | `string` | `"pd-standard"` | no |
| <a name="input_cpu_platform"></a> [cpu\_platform](#input\_cpu\_platform) | The name of the minimum CPU platform that you want the instance to use. | `string` | `null` | no |
| <a name="input_enable_placement"></a> [enable\_placement](#input\_enable\_placement) | Enable compact placement policies for jobs requiring low latency networking. | `bool` | `true` | no |
| <a name="input_exclusive"></a> [exclusive](#input\_exclusive) | Exclusive job access to nodes | `bool` | `true` | no |
| <a name="input_gpu_count"></a> [gpu\_count](#input\_gpu\_count) | Number of GPUs attached to the partition compute instances | `number` | `0` | no |
| <a name="input_gpu_type"></a> [gpu\_type](#input\_gpu\_type) | Type of GPUs attached to the partition compute instances | `string` | `null` | no |
| <a name="input_image_hyperthreads"></a> [image\_hyperthreads](#input\_image\_hyperthreads) | Enable hyperthreading | `bool` | `false` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Defines the image that will be used by the compute VMs in this partition.<br>Expected Fields:<br>name: The name of the image. Mutually exclusive with family.<br>family: The image family to use. Mutually exclusive with name.<br>project: The project where the image is hosted.<br>Custom images must comply with Slurm on GCP requirements. | `map(string)` | <pre>{<br>  "family": "schedmd-slurm-21-08-8-hpc-centos-7",<br>  "project": "schedmd-slurm-public"<br>}</pre> | no |
| <a name="input_instance_template"></a> [instance\_template](#input\_instance\_template) | Instance template to use to create partition instances | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to partition compute instances. Key-value pairs. | `map(string)` | `{}` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Compute Platform machine type to use for this partition compute nodes | `string` | `"c2-standard-60"` | no |
| <a name="input_max_node_count"></a> [max\_node\_count](#input\_max\_node\_count) | Maximum number of nodes allowed in this partition | `number` | `50` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on the partition compute nodes. | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_partition_name"></a> [partition\_name](#input\_partition\_name) | The name of the slurm partition | `string` | n/a | yes |
| <a name="input_preemptible_bursting"></a> [preemptible\_bursting](#input\_preemptible\_bursting) | Should use preemptibles to burst | `string` | `false` | no |
| <a name="input_regional_capacity"></a> [regional\_capacity](#input\_regional\_capacity) | If True, then create instances in the region that has available capacity. Specify the region in the zone field. | `bool` | `false` | no |
| <a name="input_regional_policy"></a> [regional\_policy](#input\_regional\_policy) | locationPolicy definition for regional bulkInsert() | `any` | `{}` | no |
| <a name="input_static_node_count"></a> [static\_node\_count](#input\_static\_node\_count) | Number of nodes to be statically created | `number` | `0` | no |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | The name of the pre-defined VPC subnet you want the nodes to attach to based on Region. | `string` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone where the notebook server will be located | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_partition"></a> [partition](#output\_partition) | The partition structure containing all the set variables |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
