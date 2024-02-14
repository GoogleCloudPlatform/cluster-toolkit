## Description

This module creates a DDN EXAScaler Cloud Lustre file system using
[exascaler-cloud-terraform](https://github.com/DDNStorage/exascaler-cloud-terraform/tree/master/gcp).

More information about the architecture can be found at
[Architecture: Lustre file system in Google Cloud using DDN EXAScaler][architecture].

For more information on this and other network storage options in the Cloud HPC
Toolkit, see the extended [Network Storage documentation](../../../../docs/network_storage.md).

> **Warning**: This file system has a license cost as described in the pricing
> section of the [DDN EXAScaler Cloud Marketplace Solution][marketplace].
>
> **Note**: By default security.public_key is set to `null`, therefore the
> admin user is not created. To ensure the admin user is created, provide a
> public key via the security setting.
>
> **Note**: This module's instances require access to Google APIs and
> therefore, instances must have public IP address or it must be used in a
> subnetwork where [Private Google Access][private-google-access] is enabled.

[private-google-access]: https://cloud.google.com/vpc/docs/configure-private-google-access
[marketplace]: https://console.developers.google.com/marketplace/product/ddnstorage/exascaler-cloud
[architecture]: https://cloud.google.com/architecture/lustre-architecture

## Mounting

To mount the DDN EXAScaler Lustre file system you must first install the DDN
Luster client and then call the proper `mount` command.

Both of these steps are automatically handled with the use of the `use` command
in a selection of HPC Toolkit modules. See the [compatibility matrix][matrix] in
the network storage doc for a complete list of supported modules.
the [hpc-enterprise-slurm.yaml](../../../../examples/hpc-enterprise-slurm.yaml) for an
example of using this module with Slurm.

If mounting is not automatically handled as described above, the DDN-EXAScaler
module outputs runners that can be used with the startup-script module to
install the client and mount the file system. See the following example:

```yaml
  # This file system has an associated license cost.
  # https://console.developers.google.com/marketplace/product/ddnstorage/exascaler-cloud
  - id: lustrefs
    source: community/modules/file-system/DDN-EXAScaler
    use: [network1]
    settings: {local_mount: /scratch}

  - id: mount-at-startup
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(lustrefs.install_ddn_lustre_client_runner)
      - $(lustrefs.mount_runner)

```

See [additional documentation][ddn-install-docs] from DDN EXAScaler.

[ddn-install-docs]: https://github.com/DDNStorage/exascaler-cloud-terraform/tree/master/gcp#install-new-exascaler-cloud-clients
[matrix]: ../../../../docs/network_storage.md#compatibility-matrix

## Support

EXAScaler Cloud includes self-help support with access to publicly available
documents and videos. Premium support includes 24x7x365 access to DDN's experts,
along with support community access, automated notifications of updates and
other premium support features. For more information, visit
[EXAscaler Cloud on GCP][exa-gcp].

[exa-gcp]: https://console.cloud.google.com/marketplace/product/ddnstorage/exascaler-cloud

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

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_ddn_exascaler"></a> [ddn\_exascaler](#module\_ddn\_exascaler) | github.com/DDNStorage/exascaler-cloud-terraform//gcp | a3355d50deebe45c0556b45bd599059b7c06988d |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_boot"></a> [boot](#input\_boot) | Boot disk properties | <pre>object({<br>    disk_type   = string<br>    auto_delete = bool<br>    script_url  = string<br>  })</pre> | <pre>{<br>  "auto_delete": true,<br>  "disk_type": "pd-standard",<br>  "script_url": null<br>}</pre> | no |
| <a name="input_cls"></a> [cls](#input\_cls) | Compute client properties | <pre>object({<br>    node_type  = string<br>    node_cpu   = string<br>    nic_type   = string<br>    node_count = number<br>    public_ip  = bool<br>  })</pre> | <pre>{<br>  "nic_type": "GVNIC",<br>  "node_count": 0,<br>  "node_cpu": "Intel Cascade Lake",<br>  "node_type": "n2-standard-2",<br>  "public_ip": true<br>}</pre> | no |
| <a name="input_clt"></a> [clt](#input\_clt) | Compute client target properties | <pre>object({<br>    disk_bus   = string<br>    disk_type  = string<br>    disk_size  = number<br>    disk_count = number<br>  })</pre> | <pre>{<br>  "disk_bus": "SCSI",<br>  "disk_count": 0,<br>  "disk_size": 256,<br>  "disk_type": "pd-standard"<br>}</pre> | no |
| <a name="input_fsname"></a> [fsname](#input\_fsname) | EXAScaler filesystem name, only alphanumeric characters are allowed, and the value must be 1-8 characters long | `string` | `"exacloud"` | no |
| <a name="input_image"></a> [image](#input\_image) | DEPRECATED: Source image properties | `any` | `null` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Source image properties<br><br>Expected Fields:<br>name: Unavailable with this module.<br>family: The image family to use.<br>project: The project where the image is hosted. | `map(string)` | <pre>{<br>  "family": "exascaler-cloud-6-2-rocky-linux-8-optimized-gcp",<br>  "project": "ddn-public"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to EXAScaler Cloud deployment. Key-value pairs. | `map(string)` | `{}` | no |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Mountpoint (at the client instances) for this EXAScaler system | `string` | `"/shared"` | no |
| <a name="input_mds"></a> [mds](#input\_mds) | Metadata server properties | <pre>object({<br>    node_type  = string<br>    node_cpu   = string<br>    nic_type   = string<br>    node_count = number<br>    public_ip  = bool<br>  })</pre> | <pre>{<br>  "nic_type": "GVNIC",<br>  "node_count": 1,<br>  "node_cpu": "Intel Cascade Lake",<br>  "node_type": "n2-standard-32",<br>  "public_ip": true<br>}</pre> | no |
| <a name="input_mdt"></a> [mdt](#input\_mdt) | Metadata target properties | <pre>object({<br>    disk_bus   = string<br>    disk_type  = string<br>    disk_size  = number<br>    disk_count = number<br>    disk_raid  = bool<br>  })</pre> | <pre>{<br>  "disk_bus": "SCSI",<br>  "disk_count": 1,<br>  "disk_raid": false,<br>  "disk_size": 3500,<br>  "disk_type": "pd-ssd"<br>}</pre> | no |
| <a name="input_mgs"></a> [mgs](#input\_mgs) | Management server properties | <pre>object({<br>    node_type  = string<br>    node_cpu   = string<br>    nic_type   = string<br>    node_count = number<br>    public_ip  = bool<br>  })</pre> | <pre>{<br>  "nic_type": "GVNIC",<br>  "node_count": 1,<br>  "node_cpu": "Intel Cascade Lake",<br>  "node_type": "n2-standard-32",<br>  "public_ip": true<br>}</pre> | no |
| <a name="input_mgt"></a> [mgt](#input\_mgt) | Management target properties | <pre>object({<br>    disk_bus   = string<br>    disk_type  = string<br>    disk_size  = number<br>    disk_count = number<br>    disk_raid  = bool<br>  })</pre> | <pre>{<br>  "disk_bus": "SCSI",<br>  "disk_count": 1,<br>  "disk_raid": false,<br>  "disk_size": 128,<br>  "disk_type": "pd-standard"<br>}</pre> | no |
| <a name="input_mnt"></a> [mnt](#input\_mnt) | Monitoring target properties | <pre>object({<br>    disk_bus   = string<br>    disk_type  = string<br>    disk_size  = number<br>    disk_count = number<br>    disk_raid  = bool<br>  })</pre> | <pre>{<br>  "disk_bus": "SCSI",<br>  "disk_count": 1,<br>  "disk_raid": false,<br>  "disk_size": 128,<br>  "disk_type": "pd-standard"<br>}</pre> | no |
| <a name="input_network_properties"></a> [network\_properties](#input\_network\_properties) | Network options. 'network\_self\_link' or 'network\_properties' must be provided. | <pre>object({<br>    routing = string<br>    tier    = string<br>    id      = string<br>    auto    = bool<br>    mtu     = number<br>    new     = bool<br>    nat     = bool<br>  })</pre> | `null` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self-link of the VPC network to where the system is connected.  Ignored if 'network\_properties' is provided. 'network\_self\_link' or 'network\_properties' must be provided. | `string` | `null` | no |
| <a name="input_oss"></a> [oss](#input\_oss) | Object Storage server properties | <pre>object({<br>    node_type  = string<br>    node_cpu   = string<br>    nic_type   = string<br>    node_count = number<br>    public_ip  = bool<br>  })</pre> | <pre>{<br>  "nic_type": "GVNIC",<br>  "node_count": 3,<br>  "node_cpu": "Intel Cascade Lake",<br>  "node_type": "n2-standard-16",<br>  "public_ip": true<br>}</pre> | no |
| <a name="input_ost"></a> [ost](#input\_ost) | Object Storage target properties | <pre>object({<br>    disk_bus   = string<br>    disk_type  = string<br>    disk_size  = number<br>    disk_count = number<br>    disk_raid  = bool<br>  })</pre> | <pre>{<br>  "disk_bus": "SCSI",<br>  "disk_count": 1,<br>  "disk_raid": false,<br>  "disk_size": 3500,<br>  "disk_type": "pd-ssd"<br>}</pre> | no |
| <a name="input_prefix"></a> [prefix](#input\_prefix) | EXAScaler Cloud deployment prefix (`null` defaults to 'exascaler-cloud') | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Compute Platform project that will host the EXAScaler filesystem | `string` | n/a | yes |
| <a name="input_security"></a> [security](#input\_security) | Security options | <pre>object({<br>    admin              = string<br>    public_key         = string<br>    block_project_keys = bool<br>    enable_os_login    = bool<br>    enable_local       = bool<br>    enable_ssh         = bool<br>    enable_http        = bool<br>    ssh_source_ranges  = list(string)<br>    http_source_ranges = list(string)<br>  })</pre> | <pre>{<br>  "admin": "stack",<br>  "block_project_keys": false,<br>  "enable_http": false,<br>  "enable_local": false,<br>  "enable_os_login": true,<br>  "enable_ssh": false,<br>  "http_source_ranges": [<br>    "0.0.0.0/0"<br>  ],<br>  "public_key": null,<br>  "ssh_source_ranges": [<br>    "0.0.0.0/0"<br>  ]<br>}</pre> | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account name used by deploy application | <pre>object({<br>    new   = bool<br>    email = string<br>  })</pre> | <pre>{<br>  "email": null,<br>  "new": false<br>}</pre> | no |
| <a name="input_subnetwork_address"></a> [subnetwork\_address](#input\_subnetwork\_address) | The IP range of internal addresses for the subnetwork. Ignored if 'subnetwork\_properties' is provided. | `string` | `null` | no |
| <a name="input_subnetwork_properties"></a> [subnetwork\_properties](#input\_subnetwork\_properties) | Subnetwork properties. 'subnetwork\_self\_link' or 'subnetwork\_properties' must be provided. | <pre>object({<br>    address = string<br>    private = bool<br>    id      = string<br>    new     = bool<br>  })</pre> | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self-link of the VPC subnetwork to where the system is connected. Ignored if 'subnetwork\_properties' is provided. 'subnetwork\_self\_link' or 'subnetwork\_properties' must be provided. | `string` | `null` | no |
| <a name="input_waiter"></a> [waiter](#input\_waiter) | Waiter to check progress and result for deployment. | `string` | `null` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone where the servers will be located | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_client_config_script"></a> [client\_config\_script](#output\_client\_config\_script) | Script that will install DDN EXAScaler lustre client. The machine running this script must be on the same network & subnet as the EXAScaler. |
| <a name="output_http_console"></a> [http\_console](#output\_http\_console) | HTTP address to access the system web console. |
| <a name="output_install_ddn_lustre_client_runner"></a> [install\_ddn\_lustre\_client\_runner](#output\_install\_ddn\_lustre\_client\_runner) | Runner that encapsulates the `client_config_script` output on this module. |
| <a name="output_mount_command"></a> [mount\_command](#output\_mount\_command) | Command to mount the file system. `client_config_script` must be run first. |
| <a name="output_mount_runner"></a> [mount\_runner](#output\_mount\_runner) | Runner to mount the DDN EXAScaler Lustre file system |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a EXAScaler system to be mounted by other systems. |
| <a name="output_private_addresses"></a> [private\_addresses](#output\_private\_addresses) | Private IP addresses for all instances. |
| <a name="output_ssh_console"></a> [ssh\_console](#output\_ssh\_console) | Instructions to ssh into the instances. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
