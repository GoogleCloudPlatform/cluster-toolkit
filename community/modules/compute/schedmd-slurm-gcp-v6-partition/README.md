## Description

This module creates a compute partition that can be used as input to the
[schedmd-slurm-gcp-v6-controller](../../scheduler/schedmd-slurm-gcp-v6-controller/README.md).

The partition module is designed to work alongside the
[schedmd-slurm-gcp-v6-nodeset](../schedmd-slurm-gcp-v6-nodeset/README.md)
module. A partition can be made up of one or
more nodesets, provided either through `use` (preferred) or defined manually
in the `nodeset` variable.

### Example

The following code snippet creates a partition module with:

* 2 nodesets added via `use`.
  * The first nodeset is made up of machines of type `c2-standard-30`.
  * The second nodeset is made up of machines of type `c2-standard-60`.
  * Both nodesets have a maximum count of 200 dynamically created nodes.
* partition name of "compute".
* connected to the `network` module via `use`.
* nodes mounted to homefs via `use`.

```yaml
- id: nodeset_1
  source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
  use:
  - network
  settings:
    name: c30
    node_count_dynamic_max: 200
    machine_type: c2-standard-30

- id: nodeset_2
  source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
  use:
  - network
  settings:
    name: c60
    node_count_dynamic_max: 200
    machine_type: c2-standard-60

- id: compute_partition
  source: community/modules/compute/schedmd-slurm-gcp-v6-partition
  use:
  - homefs
  - nodeset_1
  - nodeset_2
  settings:
    partition_name: compute
```

## Support

The Cluster Toolkit team maintains the wrapper around the [slurm-on-gcp] terraform
modules. For support with the underlying modules, see the instructions in the
[slurm-gcp README][slurm-gcp-readme].

[slurm-on-gcp]: https://github.com/GoogleCloudPlatform/slurm-gcp
[slurm-gcp-readme]: https://github.com/GoogleCloudPlatform/slurm-gcp#slurm-on-google-cloud-platform

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
| <a name="input_exclusive"></a> [exclusive](#input\_exclusive) | Exclusive job access to nodes. When set to true nodes execute single job and are deleted<br/>after job exits. If set to false, multiple jobs can be scheduled on one node. | `bool` | `true` | no |
| <a name="input_is_default"></a> [is\_default](#input\_is\_default) | Sets this partition as the default partition by updating the partition\_conf.<br/>If "Default" is already set in partition\_conf, this variable will have no effect. | `bool` | `false` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | DEPRECATED | <pre>list(object({<br/>    server_ip             = string,<br/>    remote_mount          = string,<br/>    local_mount           = string,<br/>    fs_type               = string,<br/>    mount_options         = string,<br/>    client_install_runner = map(string)<br/>    mount_runner          = map(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_nodeset"></a> [nodeset](#input\_nodeset) | A list of nodesets.<br/>For type definition see community/modules/scheduler/schedmd-slurm-gcp-v6-controller/variables.tf::nodeset | `list(any)` | `[]` | no |
| <a name="input_nodeset_dyn"></a> [nodeset\_dyn](#input\_nodeset\_dyn) | Defines dynamic nodesets, as a list. | <pre>list(object({<br/>    nodeset_name    = string<br/>    nodeset_feature = string<br/>  }))</pre> | `[]` | no |
| <a name="input_nodeset_tpu"></a> [nodeset\_tpu](#input\_nodeset\_tpu) | Define TPU nodesets, as a list. | <pre>list(object({<br/>    node_count_static      = optional(number, 0)<br/>    node_count_dynamic_max = optional(number, 5)<br/>    nodeset_name           = string<br/>    enable_public_ip       = optional(bool, false)<br/>    node_type              = string<br/>    accelerator_config = optional(object({<br/>      topology = string<br/>      version  = string<br/>      }), {<br/>      topology = ""<br/>      version  = ""<br/>    })<br/>    tf_version   = string<br/>    preemptible  = optional(bool, false)<br/>    preserve_tpu = optional(bool, false)<br/>    zone         = string<br/>    data_disks   = optional(list(string), [])<br/>    docker_image = optional(string, "")<br/>    network_storage = optional(list(object({<br/>      server_ip     = string<br/>      remote_mount  = string<br/>      local_mount   = string<br/>      fs_type       = string<br/>      mount_options = string<br/>    })), [])<br/>    subnetwork = string<br/>    service_account = optional(object({<br/>      email  = optional(string)<br/>      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])<br/>    }))<br/>    project_id = string<br/>    reserved   = optional(string, false)<br/>  }))</pre> | `[]` | no |
| <a name="input_partition_conf"></a> [partition\_conf](#input\_partition\_conf) | Slurm partition configuration as a map.<br/>See https://slurm.schedmd.com/slurm.conf.html#SECTION_PARTITION-CONFIGURATION | `map(string)` | `{}` | no |
| <a name="input_partition_name"></a> [partition\_name](#input\_partition\_name) | The name of the slurm partition. | `string` | n/a | yes |
| <a name="input_resume_timeout"></a> [resume\_timeout](#input\_resume\_timeout) | Maximum time permitted (in seconds) between when a node resume request is issued and when the node is actually available for use.<br/>If null is given, then a smart default will be chosen depending on nodesets in partition.<br/>This sets 'ResumeTimeout' in partition\_conf.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_ResumeTimeout_1 for details. | `number` | `300` | no |
| <a name="input_suspend_time"></a> [suspend\_time](#input\_suspend\_time) | Nodes which remain idle or down for this number of seconds will be placed into power save mode by SuspendProgram.<br/>This sets 'SuspendTime' in partition\_conf.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_SuspendTime_1 for details.<br/>NOTE: use value -1 to exclude partition from suspend.<br/>NOTE 2: if `var.exclusive` is set to true (default), nodes are deleted immediately after job finishes. | `number` | `300` | no |
| <a name="input_suspend_timeout"></a> [suspend\_timeout](#input\_suspend\_timeout) | Maximum time permitted (in seconds) between when a node suspend request is issued and when the node is shutdown.<br/>If null is given, then a smart default will be chosen depending on nodesets in partition.<br/>This sets 'SuspendTimeout' in partition\_conf.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_SuspendTimeout_1 for details. | `number` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nodeset"></a> [nodeset](#output\_nodeset) | Details of a nodesets in this partition |
| <a name="output_nodeset_dyn"></a> [nodeset\_dyn](#output\_nodeset\_dyn) | Details of a dynamic nodesets in this partition |
| <a name="output_nodeset_tpu"></a> [nodeset\_tpu](#output\_nodeset\_tpu) | Details of a TPU nodesets in this partition |
| <a name="output_partitions"></a> [partitions](#output\_partitions) | Details of a slurm partition |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
