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

The HPC Toolkit team maintains the wrapper around the [slurm-on-gcp] terraform
modules. For support with the underlying modules, see the instructions in the
[slurm-gcp README][slurm-gcp-readme].

[slurm-on-gcp]: https://github.com/SchedMD/slurm-gcp
[slurm-gcp-readme]: https://github.com/SchedMD/slurm-gcp#slurm-on-google-cloud-platform

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
| <a name="input_exclusive"></a> [exclusive](#input\_exclusive) | Exclusive job access to nodes. | `bool` | `true` | no |
| <a name="input_is_default"></a> [is\_default](#input\_is\_default) | Sets this partition as the default partition by updating the partition\_conf.<br>If "Default" is already set in partition\_conf, this variable will have no effect. | `bool` | `false` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on the partition compute nodes. | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_nodeset"></a> [nodeset](#input\_nodeset) | Define nodesets, as a list. | <pre>list(object({<br>    node_count_static      = optional(number, 0)<br>    node_count_dynamic_max = optional(number, 1)<br>    node_conf              = optional(map(string), {})<br>    nodeset_name           = string<br>    additional_disks = optional(list(object({<br>      disk_name    = optional(string)<br>      device_name  = optional(string)<br>      disk_size_gb = optional(number)<br>      disk_type    = optional(string)<br>      disk_labels  = optional(map(string), {})<br>      auto_delete  = optional(bool, true)<br>      boot         = optional(bool, false)<br>    })), [])<br>    bandwidth_tier         = optional(string, "platform_default")<br>    can_ip_forward         = optional(bool, false)<br>    disable_smt            = optional(bool, false)<br>    disk_auto_delete       = optional(bool, true)<br>    disk_labels            = optional(map(string), {})<br>    disk_size_gb           = optional(number)<br>    disk_type              = optional(string)<br>    enable_confidential_vm = optional(bool, false)<br>    enable_placement       = optional(bool, false)<br>    enable_public_ip       = optional(bool, false)<br>    enable_oslogin         = optional(bool, true)<br>    enable_shielded_vm     = optional(bool, false)<br>    gpu = optional(object({<br>      count = number<br>      type  = string<br>    }))<br>    instance_template   = optional(string)<br>    labels              = optional(map(string), {})<br>    machine_type        = optional(string)<br>    metadata            = optional(map(string), {})<br>    min_cpu_platform    = optional(string)<br>    network_tier        = optional(string, "STANDARD")<br>    on_host_maintenance = optional(string)<br>    preemptible         = optional(bool, false)<br>    region              = optional(string)<br>    service_account = optional(object({<br>      email  = optional(string)<br>      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])<br>    }))<br>    shielded_instance_config = optional(object({<br>      enable_integrity_monitoring = optional(bool, true)<br>      enable_secure_boot          = optional(bool, true)<br>      enable_vtpm                 = optional(bool, true)<br>    }))<br>    source_image_family  = optional(string)<br>    source_image_project = optional(string)<br>    source_image         = optional(string)<br>    subnetwork_self_link = string<br>    spot                 = optional(bool, false)<br>    tags                 = optional(list(string), [])<br>    termination_action   = optional(string)<br>    zones                = optional(list(string), [])<br>    zone_target_shape    = optional(string, "ANY_SINGLE_ZONE")<br>  }))</pre> | `[]` | no |
| <a name="input_nodeset_tpu"></a> [nodeset\_tpu](#input\_nodeset\_tpu) | Define TPU nodesets, as a list. | <pre>list(object({<br>    node_count_static      = optional(number, 0)<br>    node_count_dynamic_max = optional(number, 1)<br>    nodeset_name           = string<br>    enable_public_ip       = optional(bool, false)<br>    node_type              = string<br>    accelerator_config = optional(object({<br>      topology = string<br>      version  = string<br>      }), {<br>      topology = ""<br>      version  = ""<br>    })<br>    tf_version   = string<br>    preemptible  = optional(bool, false)<br>    preserve_tpu = optional(bool, true)<br>    zone         = string<br>    data_disks   = optional(list(string), [])<br>    docker_image = optional(string, "")<br>    subnetwork   = string<br>    service_account = optional(object({<br>      email  = optional(string)<br>      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])<br>    }))<br>  }))</pre> | `[]` | no |
| <a name="input_partition_conf"></a> [partition\_conf](#input\_partition\_conf) | Slurm partition configuration as a map.<br>See https://slurm.schedmd.com/slurm.conf.html#SECTION_PARTITION-CONFIGURATION | `map(string)` | `{}` | no |
| <a name="input_partition_name"></a> [partition\_name](#input\_partition\_name) | The name of the slurm partition. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_nodeset"></a> [nodeset](#output\_nodeset) | Details of a nodesets in this partition |
| <a name="output_nodeset_tpu"></a> [nodeset\_tpu](#output\_nodeset\_tpu) | Details of a nodesets tpu in this partition |
| <a name="output_partitions"></a> [partitions](#output\_partitions) | Details of a slurm partition |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
