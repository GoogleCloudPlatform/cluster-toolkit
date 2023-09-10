<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3.0 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_exclusive"></a> [exclusive](#input\_exclusive) | Exclusive job access to nodes. | `bool` | `true` | no |
| <a name="input_is_default"></a> [is\_default](#input\_is\_default) | If this is true, jobs submitted without a partition specification will utilize this partition.<br>This sets 'Default' in partition\_conf.<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_Default for details. | `bool` | `false` | no |
| <a name="input_name"></a> [name](#input\_name) | The name of the slurm partition. | `string` | n/a | yes |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | A list of network attached storage mounts to be configured on the partition compute nodes. | <pre>list(object({<br>    server_ip     = string,<br>    remote_mount  = string,<br>    local_mount   = string,<br>    fs_type       = string,<br>    mount_options = string, # REVIEWER_NOTE: removed runners<br>  }))</pre> | `[]` | no |
| <a name="input_nodeset"></a> [nodeset](#input\_nodeset) | A list of nodesets associated with this partition. <br>DO NOT specifi manually, use the nodeset module instead. | `list(any)` | `[]` | no |
| <a name="input_partition_conf"></a> [partition\_conf](#input\_partition\_conf) | Slurm partition configuration as a map.<br>See https://slurm.schedmd.com/slurm.conf.html#SECTION_PARTITION-CONFIGURATION | `map(string)` | `{}` | no |
| <a name="input_resume_timeout"></a> [resume\_timeout](#input\_resume\_timeout) | Maximum time permitted (in seconds) between when a node resume request is issued and when the node is actually available for use.<br>If null is given, then a smart default will be chosen depending on nodesets in partition.<br>This sets 'ResumeTimeout' in partition\_conf.<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_ResumeTimeout_1 for details. | `number` | `null` | no |
| <a name="input_suspend_time"></a> [suspend\_time](#input\_suspend\_time) | Nodes which remain idle or down for this number of seconds will be placed into power save mode by SuspendProgram.<br>This sets 'SuspendTime' in partition\_conf.<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_SuspendTime_1 for details.<br>NOTE: use value -1 to exclude partition from suspend. | `number` | `300` | no |
| <a name="input_suspend_timeout"></a> [suspend\_timeout](#input\_suspend\_timeout) | Maximum time permitted (in seconds) between when a node suspend request is issued and when the node is shutdown.<br>If null is given, then a smart default will be chosen depending on nodesets in partition.<br>This sets 'SuspendTimeout' in partition\_conf.<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_SuspendTimeout_1 for details. | `number` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nodeset"></a> [nodeset](#output\_nodeset) | Nodesets configuration, to be used by the cluster module. |
| <a name="output_partition"></a> [partition](#output\_partition) | Parition configuration, to be used by the cluster module. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
