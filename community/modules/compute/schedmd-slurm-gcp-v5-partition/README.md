## Description

This module creates a compute partition that can be used as input to the
[schedmd-slurm-gcp-v5-controller](../../scheduler/schedmd-slurm-gcp-v5-controller/README.md).

> **Warning**: updating a partition and running `terraform apply` will not cause
> the slurm controller to update its own configurations (`slurm.conf`) and may
> require some additional manual configuration to become active.

### Example

The following code snippet creates a partition module with:

* a max node count of 200
* VM machine type of `c2-standard-30`
* partition name of "compute"
* connected to the `network1` module via `use`
* nodes mounted to homefs via `use`

```yaml
- id: compute_partition
  source: community/modules/compute/schedmd-slurm-gcp-v5-partition
  use:
  - network1
  - homefs
  settings:
    partition_name: compute
    node_count_dynamic_max: 200
    machine_type: c2-standard-30
```

For a complete example using this module, see
[slurm-gcp-v5-cluster.yaml](../../../examples/slurm-gcp-v5-cluster.yaml).

### Compute VM Zone Policies

> **_WARNING:_** Lenient zone policies can lead to additional egress costs when
> moving data between Google Cloud resources in different zones in the same
> region, such as between filestore and other VM instances. For more information
> on egress fees, see the [Network Pricing][networkpricing] Google Cloud
> documentation.
>
> To avoid egress charges, ensure your compute nodes are created in the same
> zone as the other resources that share data with them by setting
> `zone_policy_deny` to all other zones in the region.

The Slurm on GCP partition modules provide the option to set policies regarding
which zone the compute VM instances will be created in through the
`zone_policy_allow` and `zone_policy_deny` variables.

As an example, see the the following module:

```yaml
- id: partition-with-zone-policy
  source: community/modules/compute/schedmd-slurm-gcp-v5-partition
  settings:
    zone_policy_allow:
    - us-central1-a
    - us-central1-b
    zone_policy_deny: [us-central1-f]
```

In this module, the following is defined:

* `us-central1-a` and `us-central1-b` zones have been explicitly allowed.
* `us-central1-f` has been explicitly denied, therefore no nodes in this
  partition will be created in that zone.
* Since `us-central1-c` was not included in the zone policy, it will default to
  "Allow", which means the partition has the same likelihood of creating a node in
  that zone as the zones explicitly listed under `zone_policy_allow`.

> **_NOTE:_** `zone_policy_allow` does not guarantee the use of specified zones
> because zones are allowed by default. Configure `zone_policy_deny` to ensure
> that zones outside the allowed list are not used.

#### Setting a Single Zone

The `zone` variable is another option for setting the zone policy. If `zone` is
set and neither `zone_policy_deny` nor `zone_policy_allow` are set, the
policy will be configured as follows:

* All _currently active_ zones in the region **at deploy time** will be set in the
 `zone_policy_deny` list, with the exception of the provided `zone`.
* The provided `zone` will be set as the only value in the `zone_policy_allow`
  list.

`zone_policy_allow` and `zone_policy_deny` take precedence over `zone` if both
are set.

> **_NOTE:_** If a new zone is added to the region while the cluster is active,
> nodes in the partition may be created in that zone as well. In this case, the
> partition may need to be redeployed (possible via `enable_reconfigure` if set)
> to ensure the newly added zone is set to "Deny".

[networkpricing]: https://cloud.google.com/vpc/network-pricing

## Support
The HPC Toolkit team maintains the wrapper around the [slurm-on-gcp] terraform
modules. For support with the underlying modules, see the instructions in the
[slurm-gcp README][slurm-gcp-readme].

[slurm-on-gcp]: https://github.com/SchedMD/slurm-gcp
[slurm-gcp-readme]: https://github.com/SchedMD/slurm-gcp#slurm-on-google-cloud-platform

## License
<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2022 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_slurm_partition"></a> [slurm\_partition](#module\_slurm\_partition) | github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_partition | v5.1.0 |

## Resources

| Name | Type |
|------|------|
| [google_compute_default_service_account.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_default_service_account) | data source |
| [google_compute_zones.available](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_zones) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_additional_disks"></a> [additional\_disks](#input\_additional\_disks) | Configurations of additional disks to be included on the partition nodes. | <pre>list(object({<br>    disk_name    = string<br>    device_name  = string<br>    disk_size_gb = number<br>    disk_type    = string<br>    disk_labels  = map(string)<br>    auto_delete  = bool<br>    boot         = bool<br>  }))</pre> | `[]` | no |
| <a name="input_bandwidth_tier"></a> [bandwidth\_tier](#input\_bandwidth\_tier) | Configures the network interface card and the maximum egress bandwidth for VMs.<br>  - Setting `platform_default` respects the Google Cloud Platform API default values for networking.<br>  - Setting `virtio_enabled` explicitly selects the VirtioNet network adapter.<br>  - Setting `gvnic_enabled` selects the gVNIC network adapter (without Tier 1 high bandwidth).<br>  - Setting `tier_1_enabled` selects both the gVNIC adapter and Tier 1 high bandwidth networking.<br>  - Note: both gVNIC and Tier 1 networking require a VM image with gVNIC support as well as specific VM families and shapes.<br>  - See [official docs](https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration) for more details. | `string` | `"platform_default"` | no |
| <a name="input_can_ip_forward"></a> [can\_ip\_forward](#input\_can\_ip\_forward) | Enable IP forwarding, for NAT instances for example. | `bool` | `false` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment. | `string` | n/a | yes |
| <a name="input_disable_smt"></a> [disable\_smt](#input\_disable\_smt) | Disables Simultaneous Multi-Threading (SMT) on instance. | `bool` | `true` | no |
| <a name="input_disk_auto_delete"></a> [disk\_auto\_delete](#input\_disk\_auto\_delete) | Whether or not the boot disk should be auto-deleted. | `bool` | `true` | no |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of boot disk to create for the partition compute nodes. | `number` | `50` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Boot disk type, can be either pd-ssd, local-ssd, or pd-standard. | `string` | `"pd-standard"` | no |
| <a name="input_enable_confidential_vm"></a> [enable\_confidential\_vm](#input\_enable\_confidential\_vm) | Enable the Confidential VM configuration. Note: the instance image must support option. | `bool` | `false` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enables Google Cloud os-login for user login and authentication for VMs.<br>See https://cloud.google.com/compute/docs/oslogin | `bool` | `true` | no |
| <a name="input_enable_placement"></a> [enable\_placement](#input\_enable\_placement) | Enable placement groups. | `bool` | `true` | no |
| <a name="input_enable_shielded_vm"></a> [enable\_shielded\_vm](#input\_enable\_shielded\_vm) | Enable the Shielded VM configuration. Note: the instance image must support option. | `bool` | `false` | no |
| <a name="input_enable_spot_vm"></a> [enable\_spot\_vm](#input\_enable\_spot\_vm) | Enable the partition to use spot VMs (https://cloud.google.com/spot-vms). | `bool` | `false` | no |
| <a name="input_exclusive"></a> [exclusive](#input\_exclusive) | Exclusive job access to nodes. | `bool` | `true` | no |
| <a name="input_gpu"></a> [gpu](#input\_gpu) | Definition of requested GPU resources. | <pre>object({<br>    count = number,<br>    type  = string<br>  })</pre> | `null` | no |
| <a name="input_is_default"></a> [is\_default](#input\_is\_default) | Sets this partition as the default partition by updating the partition\_conf.<br>If "Default" is already set in partition\_conf, this variable will have no effect. | `bool` | `false` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to partition compute instances. List of key key, value pairs. | `any` | `{}` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Compute Platform machine type to use for this partition compute nodes. | `string` | `"c2-standard-60"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata, provided as a map. | `map(string)` | `{}` | no |
| <a name="input_min_cpu_platform"></a> [min\_cpu\_platform](#input\_min\_cpu\_platform) | The name of the minimum CPU platform that you want the instance to use. | `string` | `null` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on the partition compute nodes. | <pre>list(object({<br>    server_ip     = string,<br>    remote_mount  = string,<br>    local_mount   = string,<br>    fs_type       = string,<br>    mount_options = string<br>  }))</pre> | `[]` | no |
| <a name="input_node_conf"></a> [node\_conf](#input\_node\_conf) | Map of Slurm node line configuration. | `map(any)` | `{}` | no |
| <a name="input_node_count_dynamic_max"></a> [node\_count\_dynamic\_max](#input\_node\_count\_dynamic\_max) | Maximum number of nodes allowed in this partition. | `number` | `10` | no |
| <a name="input_node_count_static"></a> [node\_count\_static](#input\_node\_count\_static) | Number of nodes to be statically created. | `number` | `0` | no |
| <a name="input_node_groups"></a> [node\_groups](#input\_node\_groups) | **Preview: This variable is still in development** A list of node groups<br>associated with this partition.<br>The default node group will be prepended to this list based on other input<br>variables to this module. | <pre>list(object({<br>    node_count_static      = number<br>    node_count_dynamic_max = number<br>    group_name             = string<br>    node_conf              = map(string)<br>    additional_disks = list(object({<br>      disk_name    = string<br>      device_name  = string<br>      disk_size_gb = number<br>      disk_type    = string<br>      disk_labels  = map(string)<br>      auto_delete  = bool<br>      boot         = bool<br>    }))<br>    bandwidth_tier         = string<br>    can_ip_forward         = bool<br>    disable_smt            = bool<br>    disk_auto_delete       = bool<br>    disk_labels            = map(string)<br>    disk_size_gb           = number<br>    disk_type              = string<br>    enable_confidential_vm = bool<br>    enable_oslogin         = bool<br>    enable_shielded_vm     = bool<br>    enable_spot_vm         = bool<br>    gpu = object({<br>      count = number<br>      type  = string<br>    })<br>    instance_template   = string<br>    labels              = map(string)<br>    machine_type        = string<br>    metadata            = map(string)<br>    min_cpu_platform    = string<br>    on_host_maintenance = string<br>    preemptible         = bool<br>    service_account = object({<br>      email  = string<br>      scopes = list(string)<br>    })<br>    shielded_instance_config = object({<br>      enable_integrity_monitoring = bool<br>      enable_secure_boot          = bool<br>      enable_vtpm                 = bool<br>    })<br>    spot_instance_config = object({<br>      termination_action = string<br>    })<br>    source_image_family  = string<br>    source_image_project = string<br>    source_image         = string<br>    tags                 = list(string)<br>  }))</pre> | `[]` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Instance availability Policy.<br><br>Note: Placement groups are not supported when on\_host\_maintenance is set to<br>"MIGRATE" and will be deactivated regardless of the value of<br>enable\_placement. To support enable\_placement, ensure on\_host\_maintenance is<br>set to "TERMINATE". | `string` | `"TERMINATE"` | no |
| <a name="input_partition_conf"></a> [partition\_conf](#input\_partition\_conf) | Slurm partition configuration as a map.<br>See https://slurm.schedmd.com/slurm.conf.html#SECTION_PARTITION-CONFIGURATION | `map(string)` | `{}` | no |
| <a name="input_partition_name"></a> [partition\_name](#input\_partition\_name) | The name of the slurm partition. | `string` | n/a | yes |
| <a name="input_preemptible"></a> [preemptible](#input\_preemptible) | Should use preemptibles to burst. | `string` | `false` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The default region for Cloud resources. | `string` | n/a | yes |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the compute instances. If not set, the<br>default compute service account for the given project will be used with the<br>"https://www.googleapis.com/auth/cloud-platform" scope. | <pre>object({<br>    email  = string<br>    scopes = set(string)<br>  })</pre> | `null` | no |
| <a name="input_shielded_instance_config"></a> [shielded\_instance\_config](#input\_shielded\_instance\_config) | Shielded VM configuration for the instance. Note: not used unless<br>enable\_shielded\_vm is 'true'.<br>* enable\_integrity\_monitoring : Compare the most recent boot measurements to the<br>  integrity policy baseline and return a pair of pass/fail results depending on<br>  whether they match or not.<br>* enable\_secure\_boot : Verify the digital signature of all boot components, and<br>  halt the boot process if signature verification fails.<br>* enable\_vtpm : Use a virtualized trusted platform module, which is a<br>  specialized computer chip you can use to encrypt objects like keys and<br>  certificates. | <pre>object({<br>    enable_integrity_monitoring = bool<br>    enable_secure_boot          = bool<br>    enable_vtpm                 = bool<br>  })</pre> | <pre>{<br>  "enable_integrity_monitoring": true,<br>  "enable_secure_boot": true,<br>  "enable_vtpm": true<br>}</pre> | no |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Cluster name, used for resource naming and slurm accounting. If not provided it will default to the first 8 characters of the deployment name (removing any invalid characters). | `string` | `null` | no |
| <a name="input_source_image"></a> [source\_image](#input\_source\_image) | Source disk image. By default, the image used will be the hpc-centos7<br>version of the slurm-gcp public images. More information can be found in the<br>slurm-gcp docs:<br>https://github.com/SchedMD/slurm-gcp/blob/v5.0.2/docs/images.md#public-image | `string` | `null` | no |
| <a name="input_source_image_family"></a> [source\_image\_family](#input\_source\_image\_family) | Source image family. If not provided, the default image family name for the<br>hpc-centos-7 version of the slurm-gcp public images will be used. More<br>information can be found in the slurm-gcp docs:<br>https://github.com/SchedMD/slurm-gcp/blob/v5.0.2/docs/images.md#public-image | `string` | `null` | no |
| <a name="input_source_image_project"></a> [source\_image\_project](#input\_source\_image\_project) | Project path where the source image comes from. If not provided, this value<br>will default to the project hosting the slurm-gcp public images. More<br>information can be found in the slurm-gcp docs:<br>https://github.com/SchedMD/slurm-gcp/blob/v5.0.2/docs/images.md#public-image. | `string` | `null` | no |
| <a name="input_spot_instance_config"></a> [spot\_instance\_config](#input\_spot\_instance\_config) | Configuration for spot VMs. | <pre>object({<br>    termination_action = string<br>  })</pre> | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | Subnet to deploy to. | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Network tag list. | `list(string)` | `[]` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone in which to create all compute VMs. If `zone_policy_deny` or `zone_policy_allow` are set, the `zone` variable will be ignored. | `string` | `null` | no |
| <a name="input_zone_policy_allow"></a> [zone\_policy\_allow](#input\_zone\_policy\_allow) | Partition nodes will prefer to be created in the listed zones. If a zone appears<br>in both zone\_policy\_allow and zone\_policy\_deny, then zone\_policy\_deny will take<br>priority for that zone. | `set(string)` | `[]` | no |
| <a name="input_zone_policy_deny"></a> [zone\_policy\_deny](#input\_zone\_policy\_deny) | Partition nodes will not be created in the listed zones. If a zone appears in<br>both zone\_policy\_allow and zone\_policy\_deny, then zone\_policy\_deny will take<br>priority for that zone. | `set(string)` | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_partition"></a> [partition](#output\_partition) | Details of a slurm partition |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
