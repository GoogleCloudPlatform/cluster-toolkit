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
| <a name="input_compute_disk_size_gb"></a> [compute\_disk\_size\_gb](#input\_compute\_disk\_size\_gb) | Size of boot disk to create for the partition compute nodes | `number` | `20` | no |
| <a name="input_compute_disk_type"></a> [compute\_disk\_type](#input\_compute\_disk\_type) | Type of boot disk to create for the partition compute nodes | `string` | `"pd-standard"` | no |
| <a name="input_enable_placement"></a> [enable\_placement](#input\_enable\_placement) | Enable placement groups | `bool` | `false` | no |
| <a name="input_gpu_count"></a> [gpu\_count](#input\_gpu\_count) | Number of GPUs attached to the partition compute instances | `number` | `0` | no |
| <a name="input_gpu_type"></a> [gpu\_type](#input\_gpu\_type) | Type of GPUs attached to the partition compute instances | `string` | `null` | no |
| <a name="input_image"></a> [image](#input\_image) | Image to be used of the compute VMs in this partition | `string` | `"projects/schedmd-slurm-public/global/images/family/schedmd-slurm-20-11-7-hpc-centos-7"` | no |
| <a name="input_image_hyperthreads"></a> [image\_hyperthreads](#input\_image\_hyperthreads) | Enable or disabling hypethreading | `bool` | `true` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to partition compute instances. List of key key, value pairs. | `any` | `{}` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Compute Platform machine type to use for this partition compute nodes | `string` | `"c2-standard-60"` | no |
| <a name="input_max_node_count"></a> [max\_node\_count](#input\_max\_node\_count) | Maximum number of nodes allowed in this partition | `number` | `10` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on the partition compute nodes. | <pre>list(object({<br>    server_ip     = string,<br>    remote_mount  = string,<br>    local_mount   = string,<br>    fs_type       = string,<br>    mount_options = string<br>  }))</pre> | `[]` | no |
| <a name="input_partition_name"></a> [partition\_name](#input\_partition\_name) | The name of the slurm partition | `string` | n/a | yes |
| <a name="input_preemptible_bursting"></a> [preemptible\_bursting](#input\_preemptible\_bursting) | Should use preemptibles to burst | `bool` | `false` | no |
| <a name="input_static_node_count"></a> [static\_node\_count](#input\_static\_node\_count) | Number of nodes to be statically created | `number` | `0` | no |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | The name of the pre-defined VPC subnet you want the nodes to attach to based on Region. | `string` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone where the notebook server will be located | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_partition"></a> [partition](#output\_partition) | The partition structure containing all the set variables |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->