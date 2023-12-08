## Description

This module provisions 1 or more PBS execution hosts to run jobs in a PBS
Professional cluster. The following extra services are required:

- an existing licensed PBS Professional Server; if provisioned in the cloud, we
  recommend using the [pbspro-server module][pbspro-server].
- a shared filesystem mounted across all PBS hosts to faciliate file transfers
  for jobs and their stdin/stdout logs

[pbspro-server]: ../pbspro-server/README.md

### Example

The following example snippet demonstrates use of the execution host module in
concert with the [pbspro-preinstall], [pbspro-server], and [filestore] modules.

```yaml
  - id: pbspro_execution
    source: community/modules/compute/pbspro-execution
    use:
    - homefs
    - pbspro_setup
    - pbspro_server
    settings:
      instance_count: 10
      machine_type: c2-standard-16
      name_prefix: pbs-exec
```

[pbspro-preinstall]: ../../scripts/pbspro-preinstall/README.md
[filestore]: ../../../../modules/file-system/filestore/README.md

## GPU Support

More information on GPU support in PBS Pro and other HPC Toolkit modules
can be found at [docs/gpu-support.md](../../../../docs/gpu-support.md)

## Support

PBS Professional is licensed and supported by [Altair][pbspro]. This module is
maintained and supported by the HPC Toolkit team in collaboration with Altair.

[pbspro]: https://www.altair.com/pbs-professional

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_execution_startup_script"></a> [execution\_startup\_script](#module\_execution\_startup\_script) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script | 50644b2 |
| <a name="module_pbs_execution"></a> [pbs\_execution](#module\_pbs\_execution) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/compute/vm-instance | bb47067 |
| <a name="module_pbs_install"></a> [pbs\_install](#module\_pbs\_install) | github.com/GoogleCloudPlatform/hpc-toolkit//community/modules/scripts/pbspro-install | v1.22.1 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_auto_delete_boot_disk"></a> [auto\_delete\_boot\_disk](#input\_auto\_delete\_boot\_disk) | Controls if boot disk should be auto-deleted when instance is deleted. | `bool` | `true` | no |
| <a name="input_bandwidth_tier"></a> [bandwidth\_tier](#input\_bandwidth\_tier) | Tier 1 bandwidth increases the maximum egress bandwidth for VMs.<br>  Using the `tier_1_enabled` setting will enable both gVNIC and TIER\_1 higher bandwidth networking.<br>  Using the `gvnic_enabled` setting will only enable gVNIC and will not enable TIER\_1.<br>  Note that TIER\_1 only works with specific machine families & shapes and must be using an image th<br>at supports gVNIC. See [official docs](https://cloud.google.com/compute/docs/networking/configure-v<br>m-with-high-bandwidth-configuration) for more details. | `string` | `"not_enabled"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. Cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of disk for instances. | `number` | `200` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Disk type for instances. | `string` | `"pd-standard"` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enable or Disable OS Login with "ENABLE" or "DISABLE". Set to "INHERIT" to inherit project OS Login setting. | `string` | `"ENABLE"` | no |
| <a name="input_enable_public_ips"></a> [enable\_public\_ips](#input\_enable\_public\_ips) | If set to true, instances will have public IPs on the internet. | `bool` | `true` | no |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br>    type  = string,<br>    count = number<br>  }))</pre> | `null` | no |
| <a name="input_instance_count"></a> [instance\_count](#input\_instance\_count) | Number of instances | `number` | `1` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Instance Image<br><br>Expected Fields:<br>name: The name of the image. Mutually exclusive with family.<br>family: The image family to use. Mutually exclusive with name.<br>project: The project where the image is hosted. | `map(string)` | <pre>{<br>  "family": "hpc-centos-7",<br>  "project": "cloud-hpc-image-public"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_local_ssd_count"></a> [local\_ssd\_count](#input\_local\_ssd\_count) | The number of local SSDs to attach to each VM. See https://cloud.google.com/compute/docs/disks/local-ssd. | `number` | `0` | no |
| <a name="input_local_ssd_interface"></a> [local\_ssd\_interface](#input\_local\_ssd\_interface) | Interface to be used with local SSDs. Can be either 'NVME' or 'SCSI'. No effect unless `local_ssd_count` is also set. | `string` | `"NVME"` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for the instance creation | `string` | `"c2-standard-60"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata, provided as a map | `map(string)` | `{}` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Name prefix for PBS execution hostnames | `string` | `null` | no |
| <a name="input_network_interfaces"></a> [network\_interfaces](#input\_network\_interfaces) | A list of network interfaces. The options match that of the terraform<br>network\_interface block of google\_compute\_instance. For descriptions of the<br>subfields or more information see the documentation:<br>https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance#nested_network_interface<br><br>**\_NOTE:\_** If `network_interfaces` are set, `network_self_link` and<br>`subnetwork_self_link` will be ignored, even if they are provided through<br>the `use` field. `bandwidth_tier` and `enable_public_ips` also do not apply<br>to network interfaces defined in this variable.<br><br>Subfields:<br>network            (string, required if subnetwork is not supplied)<br>subnetwork         (string, required if network is not supplied)<br>subnetwork\_project (string, optional)<br>network\_ip         (string, optional)<br>nic\_type           (string, optional, choose from ["GVNIC", "VIRTIO\_NET"])<br>stack\_type         (string, optional, choose from ["IPV4\_ONLY", "IPV4\_IPV6"])<br>queue\_count        (number, optional)<br>access\_config      (object, optional)<br>ipv6\_access\_config (object, optional)<br>alias\_ip\_range     (list(object), optional) | <pre>list(object({<br>    network            = string,<br>    subnetwork         = string,<br>    subnetwork_project = string,<br>    network_ip         = string,<br>    nic_type           = string,<br>    stack_type         = string,<br>    queue_count        = number,<br>    access_config = list(object({<br>      nat_ip                 = string,<br>      public_ptr_domain_name = string,<br>      network_tier           = string<br>    })),<br>    ipv6_access_config = list(object({<br>      public_ptr_domain_name = string,<br>      network_tier           = string<br>    })),<br>    alias_ip_range = list(object({<br>      ip_cidr_range         = string,<br>      subnetwork_range_name = string<br>    }))<br>  }))</pre> | `[]` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network to attach the VM. | `string` | `"default"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except for when `placement_policy`, spot provisioning, or GPUs require it to be `TERMINATE` | `string` | `null` | no |
| <a name="input_pbs_exec"></a> [pbs\_exec](#input\_pbs\_exec) | Root path in which to install PBS | `string` | `"/opt/pbs"` | no |
| <a name="input_pbs_execution_rpm_url"></a> [pbs\_execution\_rpm\_url](#input\_pbs\_execution\_rpm\_url) | Path to PBS Pro Execution Host RPM file | `string` | n/a | yes |
| <a name="input_pbs_home"></a> [pbs\_home](#input\_pbs\_home) | PBS working directory | `string` | `"/var/spool/pbs"` | no |
| <a name="input_pbs_server"></a> [pbs\_server](#input\_pbs\_server) | IP address or DNS name of PBS server host | `string` | n/a | yes |
| <a name="input_placement_policy"></a> [placement\_policy](#input\_placement\_policy) | Control where your VM instances are physically located relative to each other within a zone. | <pre>object({<br>    vm_count                  = number,<br>    availability_domain_count = number,<br>    collocation               = string,<br>  })</pre> | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which Google Cloud resources will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Default region for creating resources | `string` | n/a | yes |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the instance. See https://www.terraform.io/docs/providers/google/r/compute_instance_template.html#service_account. | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | <pre>{<br>  "email": null,<br>  "scopes": [<br>    "https://www.googleapis.com/auth/devstorage.read_write",<br>    "https://www.googleapis.com/auth/logging.write",<br>    "https://www.googleapis.com/auth/monitoring.write",<br>    "https://www.googleapis.com/auth/servicecontrol",<br>    "https://www.googleapis.com/auth/service.management.readonly",<br>    "https://www.googleapis.com/auth/trace.append"<br>  ]<br>}</pre> | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision VMs using discounted Spot pricing, allowing for preemption | `bool` | `false` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script used on the instance | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork to attach the VM. | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Network tags, provided as a list | `list(string)` | `[]` | no |
| <a name="input_threads_per_core"></a> [threads\_per\_core](#input\_threads\_per\_core) | Sets the number of threads per physical core. By setting threads\_per\_core<br>to 2, Simultaneous Multithreading (SMT) is enabled extending the total number<br>of virtual cores. For example, a machine of type c2-standard-60 will have 60<br>virtual cores with threads\_per\_core equal to 2. With threads\_per\_core equal<br>to 1 (SMT turned off), only the 30 physical cores will be available on the VM.<br><br>The default value of \"0\" will turn off SMT for supported machine types, and<br>will fall back to GCE defaults for unsupported machine types (t2d, shared-core<br>instances, or instances with less than 2 vCPU).<br><br>Disabling SMT can be more performant in many HPC workloads, therefore it is<br>disabled by default where compatible.<br><br>null = SMT configuration will use the GCE defaults for the machine type<br>0 = SMT will be disabled where compatible (default)<br>1 = SMT will always be disabled (will fail on incompatible machine types)<br>2 = SMT will always be enabled (will fail on incompatible machine types) | `number` | `0` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Default zone for creating resources | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
