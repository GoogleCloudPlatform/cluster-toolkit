<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | ~> 1.0 |
| <a name="requirement_local"></a> [local](#requirement\_local) | ~> 2.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_local"></a> [local](#provider\_local) | ~> 2.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_instance_template"></a> [instance\_template](#module\_instance\_template) | ../internal_instance_template | n/a |

## Resources

| Name | Type |
|------|------|
| [local_file.startup](https://registry.terraform.io/providers/hashicorp/local/latest/docs/data-sources/file) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_access_config"></a> [access\_config](#input\_access\_config) | Access configurations, i.e. IPs via which the VM instance can be accessed via the Internet. | <pre>list(object({<br/>    nat_ip       = string<br/>    network_tier = string<br/>  }))</pre> | `[]` | no |
| <a name="input_additional_disks"></a> [additional\_disks](#input\_additional\_disks) | List of maps of disks. | <pre>list(object({<br/>    disk_name    = string<br/>    device_name  = string<br/>    disk_type    = string<br/>    disk_size_gb = number<br/>    disk_labels  = map(string)<br/>    auto_delete  = bool<br/>    boot         = bool<br/>  }))</pre> | `[]` | no |
| <a name="input_additional_networks"></a> [additional\_networks](#input\_additional\_networks) | Additional network interface details for GCE, if any. | <pre>list(object({<br/>    network            = string<br/>    subnetwork         = string<br/>    subnetwork_project = string<br/>    network_ip         = string<br/>    nic_type           = string<br/>    access_config = list(object({<br/>      nat_ip       = string<br/>      network_tier = string<br/>    }))<br/>    ipv6_access_config = list(object({<br/>      network_tier = string<br/>    }))<br/>  }))</pre> | `[]` | no |
| <a name="input_bandwidth_tier"></a> [bandwidth\_tier](#input\_bandwidth\_tier) | Tier 1 bandwidth increases the maximum egress bandwidth for VMs.<br/>Using the `virtio_enabled` setting will only enable VirtioNet and will not enable TIER\_1.<br/>Using the `tier_1_enabled` setting will enable both gVNIC and TIER\_1 higher bandwidth networking.<br/>Using the `gvnic_enabled` setting will only enable gVNIC and will not enable TIER\_1.<br/>Note that TIER\_1 only works with specific machine families & shapes and must be using an image that supports gVNIC. See [official docs](https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration) for more details. | `string` | `"platform_default"` | no |
| <a name="input_can_ip_forward"></a> [can\_ip\_forward](#input\_can\_ip\_forward) | Enable IP forwarding, for NAT instances for example. | `bool` | `false` | no |
| <a name="input_disable_smt"></a> [disable\_smt](#input\_disable\_smt) | Disables Simultaneous Multi-Threading (SMT) on instance. | `bool` | `false` | no |
| <a name="input_disk_auto_delete"></a> [disk\_auto\_delete](#input\_disk\_auto\_delete) | Whether or not the boot disk should be auto-deleted. | `bool` | `true` | no |
| <a name="input_disk_labels"></a> [disk\_labels](#input\_disk\_labels) | Labels to be assigned to boot disk, provided as a map. | `map(string)` | `{}` | no |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Boot disk size in GB. | `number` | `100` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Boot disk type, can be either pd-ssd, local-ssd, or pd-standard. | `string` | `"pd-standard"` | no |
| <a name="input_enable_confidential_vm"></a> [enable\_confidential\_vm](#input\_enable\_confidential\_vm) | Enable the Confidential VM configuration. Note: the instance image must support option. | `bool` | `false` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enables Google Cloud os-login for user login and authentication for VMs.<br/>See https://cloud.google.com/compute/docs/oslogin | `bool` | `true` | no |
| <a name="input_enable_shielded_vm"></a> [enable\_shielded\_vm](#input\_enable\_shielded\_vm) | Enable the Shielded VM configuration. Note: the instance image must support option. | `bool` | `false` | no |
| <a name="input_gpu"></a> [gpu](#input\_gpu) | GPU information. Type and count of GPU to attach to the instance template. See<br/>https://cloud.google.com/compute/docs/gpus more details.<br/>- type : the GPU type<br/>- count : number of GPUs | <pre>object({<br/>    type  = string<br/>    count = number<br/>  })</pre> | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels, provided as a map | `map(string)` | `{}` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to create. | `string` | `"n1-standard-1"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata, provided as a map. | `map(string)` | `{}` | no |
| <a name="input_min_cpu_platform"></a> [min\_cpu\_platform](#input\_min\_cpu\_platform) | Specifies a minimum CPU platform. Applicable values are the friendly names of<br/>CPU platforms, such as Intel Haswell or Intel Skylake. See the complete list:<br/>https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform | `string` | `null` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Prefix for template resource. | `string` | `"default"` | no |
| <a name="input_network"></a> [network](#input\_network) | The name or self\_link of the network to attach this interface to. Use network<br/>attribute for Legacy or Auto subnetted networks and subnetwork for custom<br/>subnetted networks. | `string` | `null` | no |
| <a name="input_network_ip"></a> [network\_ip](#input\_network\_ip) | Private IP address to assign to the instance if desired. | `string` | `""` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Instance availability Policy | `string` | `"MIGRATE"` | no |
| <a name="input_preemptible"></a> [preemptible](#input\_preemptible) | Allow the instance to be preempted. | `bool` | `false` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region where the instance template should be created. | `string` | `null` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the instances. See<br/>'main.tf:local.service\_account' for the default. | <pre>object({<br/>    email  = string<br/>    scopes = set(string)<br/>  })</pre> | `null` | no |
| <a name="input_shielded_instance_config"></a> [shielded\_instance\_config](#input\_shielded\_instance\_config) | Shielded VM configuration for the instance. Note: not used unless<br/>enable\_shielded\_vm is 'true'.<br/>- enable\_integrity\_monitoring : Compare the most recent boot measurements to the<br/>  integrity policy baseline and return a pair of pass/fail results depending on<br/>  whether they match or not.<br/>- enable\_secure\_boot : Verify the digital signature of all boot components, and<br/>  halt the boot process if signature verification fails.<br/>- enable\_vtpm : Use a virtualized trusted platform module, which is a<br/>  specialized computer chip you can use to encrypt objects like keys and<br/>  certificates. | <pre>object({<br/>    enable_integrity_monitoring = bool<br/>    enable_secure_boot          = bool<br/>    enable_vtpm                 = bool<br/>  })</pre> | <pre>{<br/>  "enable_integrity_monitoring": true,<br/>  "enable_secure_boot": true,<br/>  "enable_vtpm": true<br/>}</pre> | no |
| <a name="input_slurm_bucket_path"></a> [slurm\_bucket\_path](#input\_slurm\_bucket\_path) | GCS Bucket URI of Slurm cluster file storage. | `string` | n/a | yes |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Cluster name, used for resource naming. | `string` | n/a | yes |
| <a name="input_slurm_instance_role"></a> [slurm\_instance\_role](#input\_slurm\_instance\_role) | Slurm instance type. Must be one of: controller; login; compute; or null. | `string` | n/a | yes |
| <a name="input_source_image"></a> [source\_image](#input\_source\_image) | Source disk image. | `string` | `""` | no |
| <a name="input_source_image_family"></a> [source\_image\_family](#input\_source\_image\_family) | Source image family. | `string` | `""` | no |
| <a name="input_source_image_project"></a> [source\_image\_project](#input\_source\_image\_project) | Project where the source image comes from. If it is not provided, the provider project is used. | `string` | `""` | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision as a SPOT preemptible instance.<br/>See https://cloud.google.com/compute/docs/instances/spot for more details. | `bool` | `false` | no |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | The name of the subnetwork to attach this interface to. The subnetwork must<br/>exist in the same region this instance will be created in. Either network or<br/>subnetwork must be provided. | `string` | `null` | no |
| <a name="input_subnetwork_project"></a> [subnetwork\_project](#input\_subnetwork\_project) | The ID of the project in which the subnetwork belongs. If it is not provided, the provider project is used. | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Network tag list. | `list(string)` | `[]` | no |
| <a name="input_termination_action"></a> [termination\_action](#input\_termination\_action) | Which action to take when Compute Engine preempts the VM. Value can be: 'STOP', 'DELETE'. The default value is 'STOP'.<br/>See https://cloud.google.com/compute/docs/instances/spot for more details. | `string` | `"STOP"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instance_template"></a> [instance\_template](#output\_instance\_template) | Instance template details |
| <a name="output_name"></a> [name](#output\_name) | Name of instance template |
| <a name="output_self_link"></a> [self\_link](#output\_self\_link) | Self\_link of instance template |
| <a name="output_service_account"></a> [service\_account](#output\_service\_account) | Service account object, includes email and scopes. |
| <a name="output_tags"></a> [tags](#output\_tags) | Tags that will be associated with instance(s) |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
