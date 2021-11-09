## Description

This resource creates a DDN EXAScaler Cloud Lustre file system using
[exascaler-cloud-terraform](https://github.com/DDNStorage/exascaler-cloud-terraform/tree/master/gcp).

**Please note**: This resource's instances require access to Google APIs and therefore, instances must have public IP address or it must be used in a subnetwork where [Private Google Access](https://cloud.google.com/vpc/docs/configure-private-google-access) is enabled.

**WARNING**: This is an experimental resource and is not fully supported.

**WARNING**: This file system has a license cost as described in the pricing
section of the [DDN EXAScaler Cloud Marketplace Solution](https://console.developers.google.com/marketplace/product/ddnstorage/exascaler-cloud).

## License
<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2021 Google LLC

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
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 3.0 |

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_ddn_exascaler"></a> [ddn\_exascaler](#module\_ddn\_exascaler) | github.com/DDNStorage/exascaler-cloud-terraform//gcp | b063430 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_admin"></a> [admin](#input\_admin) | User for remote SSH access | <pre>object({<br>    username       = string<br>    ssh_public_key = string<br>  })</pre> | <pre>{<br>  "ssh_public_key": "~/.ssh/id_rsa.pub",<br>  "username": "admin"<br>}</pre> | no |
| <a name="input_boot"></a> [boot](#input\_boot) | Boot disk properties | <pre>object({<br>    disk_type   = string<br>    auto_delete = bool<br>  })</pre> | <pre>{<br>  "auto_delete": true,<br>  "disk_type": "pd-standard"<br>}</pre> | no |
| <a name="input_cls"></a> [cls](#input\_cls) | Compute client properties | <pre>object({<br>    node_type  = string<br>    node_cpu   = string<br>    nic_type   = string<br>    node_count = number<br>    public_ip  = bool<br>  })</pre> | <pre>{<br>  "nic_type": "GVNIC",<br>  "node_count": 0,<br>  "node_cpu": "Intel Cascade Lake",<br>  "node_type": "n2-standard-2",<br>  "public_ip": true<br>}</pre> | no |
| <a name="input_clt"></a> [clt](#input\_clt) | Compute client target properties | <pre>object({<br>    disk_bus   = string<br>    disk_type  = string<br>    disk_size  = number<br>    disk_count = number<br>  })</pre> | <pre>{<br>  "disk_bus": "SCSI",<br>  "disk_count": 0,<br>  "disk_size": 256,<br>  "disk_type": "pd-standard"<br>}</pre> | no |
| <a name="input_fsname"></a> [fsname](#input\_fsname) | EXAScaler filesystem name, only alphanumeric characters are allowed, and the value must be 1-8 characters long | `string` | `"exacloud"` | no |
| <a name="input_image"></a> [image](#input\_image) | Source image properties | <pre>object({<br>    project = string<br>    name    = string<br>  })</pre> | <pre>{<br>  "name": "exascaler-cloud-v522-centos7",<br>  "project": "ddn-public"<br>}</pre> | no |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Mountpoint (at the client instances) for this EXAScaler system | `string` | `"/shared"` | no |
| <a name="input_mds"></a> [mds](#input\_mds) | Metadata server properties | <pre>object({<br>    node_type  = string<br>    node_cpu   = string<br>    nic_type   = string<br>    node_count = number<br>    public_ip  = bool<br>  })</pre> | <pre>{<br>  "nic_type": "GVNIC",<br>  "node_count": 1,<br>  "node_cpu": "Intel Cascade Lake",<br>  "node_type": "n2-standard-32",<br>  "public_ip": true<br>}</pre> | no |
| <a name="input_mdt"></a> [mdt](#input\_mdt) | Metadata target properties | <pre>object({<br>    disk_bus   = string<br>    disk_type  = string<br>    disk_size  = number<br>    disk_count = number<br>  })</pre> | <pre>{<br>  "disk_bus": "SCSI",<br>  "disk_count": 1,<br>  "disk_size": 3500,<br>  "disk_type": "pd-ssd"<br>}</pre> | no |
| <a name="input_mgs"></a> [mgs](#input\_mgs) | Management server properties | <pre>object({<br>    node_type  = string<br>    node_cpu   = string<br>    nic_type   = string<br>    node_count = number<br>    public_ip  = bool<br>  })</pre> | <pre>{<br>  "nic_type": "GVNIC",<br>  "node_count": 1,<br>  "node_cpu": "Intel Cascade Lake",<br>  "node_type": "n2-standard-2",<br>  "public_ip": true<br>}</pre> | no |
| <a name="input_mgt"></a> [mgt](#input\_mgt) | Management target properties | <pre>object({<br>    disk_bus   = string<br>    disk_type  = string<br>    disk_size  = number<br>    disk_count = number<br>  })</pre> | <pre>{<br>  "disk_bus": "SCSI",<br>  "disk_count": 1,<br>  "disk_size": 128,<br>  "disk_type": "pd-standard"<br>}</pre> | no |
| <a name="input_mnt"></a> [mnt](#input\_mnt) | Monitoring target properties | <pre>object({<br>    disk_bus   = string<br>    disk_type  = string<br>    disk_size  = number<br>    disk_count = number<br>  })</pre> | <pre>{<br>  "disk_bus": "SCSI",<br>  "disk_count": 1,<br>  "disk_size": 128,<br>  "disk_type": "pd-standard"<br>}</pre> | no |
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | The name of the VPC network to where the system is connected. | `string` | `null` | no |
| <a name="input_network_properties"></a> [network\_properties](#input\_network\_properties) | Network properties. Ignored if network\_name is supplied. | <pre>object({<br>    routing = string<br>    tier    = string<br>    name    = string<br>    auto    = bool<br>    mtu     = number<br>    new     = bool<br>    nat     = bool<br>  })</pre> | <pre>{<br>  "auto": false,<br>  "mtu": 1500,<br>  "name": "default",<br>  "nat": false,<br>  "new": false,<br>  "routing": "REGIONAL",<br>  "tier": "STANDARD"<br>}</pre> | no |
| <a name="input_oss"></a> [oss](#input\_oss) | Object Storage server properties | <pre>object({<br>    node_type  = string<br>    node_cpu   = string<br>    nic_type   = string<br>    node_count = number<br>    public_ip  = bool<br>  })</pre> | <pre>{<br>  "nic_type": "GVNIC",<br>  "node_count": 3,<br>  "node_cpu": "Intel Cascade Lake",<br>  "node_type": "n2-standard-16",<br>  "public_ip": true<br>}</pre> | no |
| <a name="input_ost"></a> [ost](#input\_ost) | Object Storage target properties | <pre>object({<br>    disk_bus   = string<br>    disk_type  = string<br>    disk_size  = number<br>    disk_count = number<br>  })</pre> | <pre>{<br>  "disk_bus": "SCSI",<br>  "disk_count": 1,<br>  "disk_size": 3500,<br>  "disk_type": "pd-ssd"<br>}</pre> | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Compute Platform project that will host the EXAScaler filesystem | `string` | n/a | yes |
| <a name="input_security"></a> [security](#input\_security) | Various firewall related rules | <pre>object({<br>    enable_local      = bool<br>    enable_ssh        = bool<br>    ssh_source_range  = string<br>    enable_http       = bool<br>    http_source_range = string<br>  })</pre> | <pre>{<br>  "enable_http": false,<br>  "enable_local": false,<br>  "enable_ssh": false,<br>  "http_source_range": "0.0.0.0/0",<br>  "ssh_source_range": "0.0.0.0/0"<br>}</pre> | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account name used by deploy application | <pre>object({<br>    new  = bool<br>    name = string<br>  })</pre> | <pre>{<br>  "name": "default",<br>  "new": false<br>}</pre> | no |
| <a name="input_subnetwork_address"></a> [subnetwork\_address](#input\_subnetwork\_address) | The IP range of internal addresses for the subnetwork | `string` | `null` | no |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | The name of the VPC subnetwork to where the system is connected. | `string` | `null` | no |
| <a name="input_subnetwork_properties"></a> [subnetwork\_properties](#input\_subnetwork\_properties) | Subnetwork properties. Ignored if subnetwork\_name is supplied. | <pre>object({<br>    address = string<br>    private = bool<br>    name    = string<br>    new     = bool<br>  })</pre> | <pre>{<br>  "address": "10.0.0.0/16",<br>  "name": "default",<br>  "new": false,<br>  "private": true<br>}</pre> | no |
| <a name="input_waiter"></a> [waiter](#input\_waiter) | Waiter to check progress and result for deployment. | `string` | `null` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone where the servers will be located | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_mount_command"></a> [mount\_command](#output\_mount\_command) | Command to mount the file system. |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a EXAScaler system to be mounted by other systems. |
| <a name="output_private_addresses"></a> [private\_addresses](#output\_private\_addresses) | Private IP addresses for all instances. |
| <a name="output_ssh_console"></a> [ssh\_console](#output\_ssh\_console) | Instructions to ssh into the instances. |
| <a name="output_web_console"></a> [web\_console](#output\_web\_console) | HTTP address to access the system web console. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
