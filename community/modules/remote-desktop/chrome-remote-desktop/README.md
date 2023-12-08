## Description

This module creates a GPU accelerated virtual machine that can be accessed using
Chrome Remote Desktop.

> **Note**: This is an experimental module. This module has only been tested in
> limited capacity with the HPC Toolkit. The module interface may have undergo
> breaking changes in the future.

### Example

The following example will create a single GPU accelerated remote desktop.

```yaml
  - id: remote-desktop
    source: community/modules/remote-desktop/chrome-remote-desktop
    use: [network1]
    settings:
      install_nvidia_driver: true
```

### Setting up the Remote Desktop

1. Once the remote desktop has been deployed, navigate to https://remotedesktop.google.com/headless.
1. Click through `Begin`, `Next`, & `Authorize`.
1. Copy the code snippet for `Debian Linux`.
1. SSH into the remote desktop machine. It will be listed under
   [VM Instances](https://console.cloud.google.com/compute/instances) in the
   Google Cloud web console.
1. Run the copied command and follow instructions to set up a PIN.
1. You should now see your machine listed on the
   [Chrome Remote Desktop page](https://remotedesktop.google.com/access) under `Remote devices`.
1. Click on your machine and enter PIN if prompted.

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.12.31 |

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_client_startup_script"></a> [client\_startup\_script](#module\_client\_startup\_script) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script | 50644b2 |
| <a name="module_instances"></a> [instances](#module\_instances) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/compute/vm-instance | bb47067 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_add_deployment_name_before_prefix"></a> [add\_deployment\_name\_before\_prefix](#input\_add\_deployment\_name\_before\_prefix) | If true, the names of VMs and disks will always be prefixed with `deployment_name` to enable uniqueness across deployments.<br>See `name_prefix` for further details on resource naming behavior. | `bool` | `false` | no |
| <a name="input_auto_delete_boot_disk"></a> [auto\_delete\_boot\_disk](#input\_auto\_delete\_boot\_disk) | Controls if boot disk should be auto-deleted when instance is deleted. | `bool` | `true` | no |
| <a name="input_bandwidth_tier"></a> [bandwidth\_tier](#input\_bandwidth\_tier) | Tier 1 bandwidth increases the maximum egress bandwidth for VMs.<br>  Using the `tier_1_enabled` setting will enable both gVNIC and TIER\_1 higher bandwidth networking.<br>  Using the `gvnic_enabled` setting will only enable gVNIC and will not enable TIER\_1.<br>  Note that TIER\_1 only works with specific machine families & shapes and must be using an image th<br>at supports gVNIC. See [official docs](https://cloud.google.com/compute/docs/networking/configure-v<br>m-with-high-bandwidth-configuration) for more details. | `string` | `"not_enabled"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. Cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of disk for instances. | `number` | `200` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Disk type for instances. | `string` | `"pd-balanced"` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enable or Disable OS Login with "ENABLE" or "DISABLE". Set to "INHERIT" to inherit project OS Login setting. | `string` | `"ENABLE"` | no |
| <a name="input_enable_public_ips"></a> [enable\_public\_ips](#input\_enable\_public\_ips) | If set to true, instances will have public IPs on the internet. | `bool` | `true` | no |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. Requires virtual workstation accelerator if Nvidia Grid Drivers are required | <pre>list(object({<br>    type  = string,<br>    count = number<br>  }))</pre> | <pre>[<br>  {<br>    "count": 1,<br>    "type": "nvidia-tesla-t4-vws"<br>  }<br>]</pre> | no |
| <a name="input_install_nvidia_driver"></a> [install\_nvidia\_driver](#input\_install\_nvidia\_driver) | Installs the nvidia driver (true/false). For details, see https://cloud.google.com/compute/docs/gpus/install-drivers-gpu | `bool` | n/a | yes |
| <a name="input_instance_count"></a> [instance\_count](#input\_instance\_count) | Number of instances | `number` | `1` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Image used to build chrome remote desktop node. The default image is from<br>family= \"debian-11\" and project = \"debian-cloud\". An alternative image is<br>from family  = \"ubuntu-2204-lts\" and project = \"ubuntu-os-cloud\".<br><br>Expected Fields:<br>name: The name of the image. Mutually exclusive with family.<br>family: The image family to use. Mutually exclusive with name.<br>project: The project where the image is hosted. | `map(string)` | <pre>{<br>  "family": "debian-11",<br>  "project": "debian-cloud"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. Key-value pairs. | `map(string)` | `{}` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for the instance creation. Must be N1 family if GPU is used. | `string` | `"n1-standard-8"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata, provided as a map | `map(string)` | `{}` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | An optional name for all VM and disk resources. <br>If not supplied, `deployment_name` will be used. <br>When `name_prefix` is supplied, and `add_deployment_name_before_prefix` is set, <br>then resources are named by "<`deployment_name`>-<`name_prefix`>-<#>". | `string` | `null` | no |
| <a name="input_network_interfaces"></a> [network\_interfaces](#input\_network\_interfaces) | A list of network interfaces. The options match that of the terraform<br>network\_interface block of google\_compute\_instance. For descriptions of the<br>subfields or more information see the documentation:<br>https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance#nested_network_interface<br>**\_NOTE:\_** If `network_interfaces` are set, `network_self_link` and<br>`subnetwork_self_link` will be ignored, even if they are provided through<br>the `use` field. `bandwidth_tier` and `enable_public_ips` also do not apply<br>to network interfaces defined in this variable.<br>Subfields:<br>network            (string, required if subnetwork is not supplied)<br>subnetwork         (string, required if network is not supplied)<br>subnetwork\_project (string, optional)<br>network\_ip         (string, optional)<br>nic\_type           (string, optional, choose from ["GVNIC", "VIRTIO\_NET"])<br>stack\_type         (string, optional, choose from ["IPV4\_ONLY", "IPV4\_IPV6"])<br>queue\_count        (number, optional)<br>access\_config      (object, optional)<br>ipv6\_access\_config (object, optional)<br>alias\_ip\_range     (list(object), optional) | <pre>list(object({<br>    network            = string,<br>    subnetwork         = string,<br>    subnetwork_project = string,<br>    network_ip         = string,<br>    nic_type           = string,<br>    stack_type         = string,<br>    queue_count        = number,<br>    access_config = list(object({<br>      nat_ip                 = string,<br>      public_ptr_domain_name = string,<br>      network_tier           = string<br>    })),<br>    ipv6_access_config = list(object({<br>      public_ptr_domain_name = string,<br>      network_tier           = string<br>    })),<br>    alias_ip_range = list(object({<br>      ip_cidr_range         = string,<br>      subnetwork_range_name = string<br>    }))<br>  }))</pre> | `[]` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network to attach the VM. | `string` | `"default"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except for when `placement_policy`, spot provisioning, or GPUs require it to be `TERMINATE` | `string` | `"TERMINATE"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which Google Cloud resources will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Default region for creating resources | `string` | n/a | yes |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the instance. See https://www.terraform.io/docs/providers/google/r/compute_instance_template.html#service_account. | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | <pre>{<br>  "email": null,<br>  "scopes": [<br>    "https://www.googleapis.com/auth/cloud-platform"<br>  ]<br>}</pre> | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision VMs using discounted Spot pricing, allowing for preemption | `bool` | `false` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script used on the instance | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork to attach the VM. | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Network tags, provided as a list | `list(string)` | `[]` | no |
| <a name="input_threads_per_core"></a> [threads\_per\_core](#input\_threads\_per\_core) | Sets the number of threads per physical core | `number` | `2` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Default zone for creating resources | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instance_name"></a> [instance\_name](#output\_instance\_name) | Name of the first instance created, if any. |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | script to load and run all runners, as a string value. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
