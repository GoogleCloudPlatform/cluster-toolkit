## Description

This resource creates one or more simple [compute VM instances](https://cloud.google.com/compute/docs/instances).

### Example

```yaml
- source: ./resources/compute/simple-instance
  kind: terraform
  id: compute
  settings:
    instance_count: 8
    name_prefix: compute
    network_self_link: $(network1.network_self_link)
    machine_type: c2-standard-60
    network_storage:
    - $(homefs.network_storage)
```

This creates a cluster of 8 compute VMs named `compute-[0-7]` on the network
defined by the `network1` resource. The VMs are of type c2-standard-60 and mount
the `homefs` file system resource.

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 3.73 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 3.73 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_compute_instance.compute_vm](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_instance) | resource |
| [google_compute_disk.boot_disk](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk) | resource |
| [google_compute_image.compute_image](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_image) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_bandwidth_tier"></a> [bandwidth\_tier](#input\_bandwidth\_tier) | Tier 1 bandwidth increases the maximum egress bandwidth for VMs. <br>  Using the `tier_1_enabled` setting will enable both gVNIC and TIER\_1 higher bandwidth networking. <br>  Using the `gvnic_enabled` setting will only enable gVNIC and will not enable TIER\_1. <br>  Note that TIER\_1 only works with specific machine families & shapes and must be using an image that supports gVNIC. | `string` | `"not_enabled"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, used to name the cluster | `string` | n/a | yes |
| <a name="input_disable_public_ips"></a> [disable\_public\_ips](#input\_disable\_public\_ips) | If set to true, instances will not have public IPs | `bool` | `false` | no |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of disk for instances. | `number` | `200` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Disk type for instances. | `string` | `"pd-standard"` | no |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br>    type  = string,<br>    count = number<br>  }))</pre> | `[]` | no |
| <a name="input_instance_count"></a> [instance\_count](#input\_instance\_count) | Number of instances | `number` | `1` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Instance Image | <pre>object({<br>    family  = string,<br>    project = string<br>  })</pre> | <pre>{<br>  "family": "hpc-centos-7",<br>  "project": "cloud-hpc-image-public"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. List key, value pairs. | `any` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for the instance creation | `string` | `"c2-standard-60"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata, provided as a map | `map(string)` | `{}` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Name Prefix | `string` | `null` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network to attach the VM. | `string` | `"default"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. | <pre>list(object({<br>    server_ip     = string,<br>    remote_mount  = string,<br>    local_mount   = string,<br>    fs_type       = string,<br>    mount_options = string<br>  }))</pre> | `[]` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Describes maintenance behavior for the instance. | `string` | `"MIGRATE"` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the instance. See https://www.terraform.io/docs/providers/google/r/compute_instance_template.html#service_account. | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | <pre>{<br>  "email": null,<br>  "scopes": [<br>    "https://www.googleapis.com/auth/devstorage.read_only",<br>    "https://www.googleapis.com/auth/logging.write",<br>    "https://www.googleapis.com/auth/monitoring.write",<br>    "https://www.googleapis.com/auth/servicecontrol",<br>    "https://www.googleapis.com/auth/service.management.readonly",<br>    "https://www.googleapis.com/auth/trace.append"<br>  ]<br>}</pre> | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script used on the instance | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork to attach the VM. | `string` | `null` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_internal_ip"></a> [internal\_ip](#output\_internal\_ip) | Internal IP of the instances |
| <a name="output_name"></a> [name](#output\_name) | Name of any instance created |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
