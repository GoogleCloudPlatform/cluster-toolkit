## Description

This module creates a login node for a Slurm cluster based on the
[SchedMD/slurm-gcp] [slurm\_instance\_template] and [slurm\_login\_instance]
terraform modules. The login node is used in conjunction with the
[Slurm controller](../schedmd-slurm-gcp-v5-controller/README.md).

[SchedMD/slurm-gcp]: https://github.com/SchedMD/slurm-gcp/tree/5.9.1
[slurm\_login\_instance]: https://github.com/SchedMD/slurm-gcp/tree/5.9.1/terraform/slurm_cluster/modules/slurm_login_instance
[slurm\_instance\_template]: https://github.com/SchedMD/slurm-gcp/tree/5.9.1/terraform/slurm_cluster/modules/slurm_instance_template

### Example

```yaml
- id: slurm_login
  source: community/modules/scheduler/schedmd-slurm-gcp-v5-login
  use:
  - network1
  - slurm_controller
  settings:
    machine_type: n2-standard-4
```

This creates a Slurm login node which is:

* connected to the primary subnet of network1 via `use`
* associated with the `slurm_controller` module as the slurm controller via
  `use`
* of VM machine type `n2-standard-4`

## Custom Images

For more information on creating valid custom images for the login node VM
instances or for custom instance templates, see our [vm-images.md] documentation
page.

[vm-images.md]: ../../../../docs/vm-images.md#slurm-on-gcp-custom-images

## GPU Support

More information on GPU support in Slurm on GCP and other HPC Toolkit modules
can be found at [docs/gpu-support.md](../../../../docs/gpu-support.md)

## Support
The HPC Toolkit team maintains the wrapper around the [slurm-on-gcp] terraform
modules. For support with the underlying modules, see the instructions in the
[slurm-gcp README][slurm-gcp-readme].

[slurm-on-gcp]: https://github.com/SchedMD/slurm-gcp/tree/5.9.1
[slurm-gcp-readme]: https://github.com/SchedMD/slurm-gcp/tree/5.9.1#slurm-on-google-cloud-platform

## License
<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2023 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.1 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_slurm_login_instance"></a> [slurm\_login\_instance](#module\_slurm\_login\_instance) | github.com/GoogleCloudPlatform/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_login_instance | 5.9.1 |
| <a name="module_slurm_login_template"></a> [slurm\_login\_template](#module\_slurm\_login\_template) | github.com/GoogleCloudPlatform/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_instance_template | 5.9.1 |

## Resources

| Name | Type |
|------|------|
| [google_compute_default_service_account.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_default_service_account) | data source |
| [google_compute_image.slurm](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_image) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_access_config"></a> [access\_config](#input\_access\_config) | Access configurations, i.e. IPs via which the VM instance can be accessed via the Internet. | <pre>list(object({<br>    nat_ip       = string<br>    network_tier = string<br>  }))</pre> | `[]` | no |
| <a name="input_additional_disks"></a> [additional\_disks](#input\_additional\_disks) | List of maps of disks. | <pre>list(object({<br>    disk_name    = string<br>    device_name  = string<br>    disk_type    = string<br>    disk_size_gb = number<br>    disk_labels  = map(string)<br>    auto_delete  = bool<br>    boot         = bool<br>  }))</pre> | `[]` | no |
| <a name="input_can_ip_forward"></a> [can\_ip\_forward](#input\_can\_ip\_forward) | Enable IP forwarding, for NAT instances for example. | `bool` | `false` | no |
| <a name="input_controller_instance_id"></a> [controller\_instance\_id](#input\_controller\_instance\_id) | The server-assigned unique identifier of the controller instance. This value<br>must be supplied as an output of the controller module, typically via `use`. | `string` | n/a | yes |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment. | `string` | n/a | yes |
| <a name="input_disable_login_public_ips"></a> [disable\_login\_public\_ips](#input\_disable\_login\_public\_ips) | If set to false. The login will have a random public IP assigned to it. Ignored if access\_config is set. | `bool` | `true` | no |
| <a name="input_disable_smt"></a> [disable\_smt](#input\_disable\_smt) | Disables Simultaneous Multi-Threading (SMT) on instance. | `bool` | `true` | no |
| <a name="input_disk_auto_delete"></a> [disk\_auto\_delete](#input\_disk\_auto\_delete) | Whether or not the boot disk should be auto-deleted. | `bool` | `true` | no |
| <a name="input_disk_labels"></a> [disk\_labels](#input\_disk\_labels) | Labels specific to the boot disk. These will be merged with var.labels. | `map(string)` | `{}` | no |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Boot disk size in GB. | `number` | `50` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Boot disk type, can be either pd-ssd, pd-standard, pd-balanced, or pd-extreme. | `string` | `"pd-standard"` | no |
| <a name="input_enable_confidential_vm"></a> [enable\_confidential\_vm](#input\_enable\_confidential\_vm) | Enable the Confidential VM configuration. Note: the instance image must support option. | `bool` | `false` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enables Google Cloud os-login for user login and authentication for VMs.<br>See https://cloud.google.com/compute/docs/oslogin | `bool` | `true` | no |
| <a name="input_enable_reconfigure"></a> [enable\_reconfigure](#input\_enable\_reconfigure) | Enables automatic Slurm reconfigure on when Slurm configuration changes (e.g.<br>slurm.conf.tpl, partition details).<br><br>NOTE: Requires Google Pub/Sub API. | `bool` | `false` | no |
| <a name="input_enable_shielded_vm"></a> [enable\_shielded\_vm](#input\_enable\_shielded\_vm) | Enable the Shielded VM configuration. Note: the instance image must support option. | `bool` | `false` | no |
| <a name="input_gpu"></a> [gpu](#input\_gpu) | DEPRECATED: use var.guest\_accelerator | <pre>object({<br>    type  = string<br>    count = number<br>  })</pre> | `null` | no |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br>    type  = string,<br>    count = number<br>  }))</pre> | `[]` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Defines the image that will be used in the Slurm login node VM instances.<br><br>Expected Fields:<br>name: The name of the image. Mutually exclusive with family.<br>family: The image family to use. Mutually exclusive with name.<br>project: The project where the image is hosted.<br><br>For more information on creating custom images that comply with Slurm on GCP<br>see the "Slurm on GCP Custom Images" section in docs/vm-images.md. | `map(string)` | <pre>{<br>  "family": "slurm-gcp-5-9-hpc-centos-7",<br>  "project": "schedmd-slurm-public"<br>}</pre> | no |
| <a name="input_instance_image_custom"></a> [instance\_image\_custom](#input\_instance\_image\_custom) | A flag that designates that the user is aware that they are requesting<br>to use a custom and potentially incompatible image for this Slurm on<br>GCP module.<br><br>If the field is set to false, only the compatible families and project<br>names will be accepted.  The deployment will fail with any other image<br>family or name.  If set to true, no checks will be done.<br><br>See: https://goo.gle/hpc-slurm-images | `bool` | `false` | no |
| <a name="input_instance_template"></a> [instance\_template](#input\_instance\_template) | Self link to a custom instance template. If set, other VM definition<br>variables such as machine\_type and instance\_image will be ignored in favor<br>of the provided instance template.<br><br>For more information on creating custom images for the instance template<br>that comply with Slurm on GCP see the "Slurm on GCP Custom Images" section<br>in docs/vm-images.md. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels, provided as a map. | `map(string)` | `{}` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to create. | `string` | `"n2-standard-2"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata, provided as a map. | `map(string)` | `{}` | no |
| <a name="input_min_cpu_platform"></a> [min\_cpu\_platform](#input\_min\_cpu\_platform) | Specifies a minimum CPU platform. Applicable values are the friendly names of<br>CPU platforms, such as Intel Haswell or Intel Skylake. See the complete list:<br>https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform | `string` | `null` | no |
| <a name="input_network_ip"></a> [network\_ip](#input\_network\_ip) | DEPRECATED: Use `static_ips` variable to assign an internal static ip address. | `string` | `null` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | Network to deploy to. Either network\_self\_link or subnetwork\_self\_link must be specified. | `string` | `null` | no |
| <a name="input_num_instances"></a> [num\_instances](#input\_num\_instances) | Number of instances to create. This value is ignored if static\_ips is provided. | `number` | `1` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Instance availability Policy. | `string` | `"MIGRATE"` | no |
| <a name="input_preemptible"></a> [preemptible](#input\_preemptible) | Allow the instance to be preempted. | `bool` | `false` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_pubsub_topic"></a> [pubsub\_topic](#input\_pubsub\_topic) | The cluster pubsub topic created by the controller when enable\_reconfigure=true. | `string` | `null` | no |
| <a name="input_region"></a> [region](#input\_region) | Region where the instances should be created.<br>Note: region will be ignored if it can be extracted from subnetwork. | `string` | `null` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the login instance. If not set, the<br>default compute service account for the given project will be used with the<br>"https://www.googleapis.com/auth/cloud-platform" scope. | <pre>object({<br>    email  = string<br>    scopes = set(string)<br>  })</pre> | `null` | no |
| <a name="input_shielded_instance_config"></a> [shielded\_instance\_config](#input\_shielded\_instance\_config) | Shielded VM configuration for the instance. Note: not used unless<br>enable\_shielded\_vm is 'true'.<br>- enable\_integrity\_monitoring : Compare the most recent boot measurements to the<br>  integrity policy baseline and return a pair of pass/fail results depending on<br>  whether they match or not.<br>- enable\_secure\_boot : Verify the digital signature of all boot components, and<br>  halt the boot process if signature verification fails.<br>- enable\_vtpm : Use a virtualized trusted platform module, which is a<br>  specialized computer chip you can use to encrypt objects like keys and<br>  certificates. | <pre>object({<br>    enable_integrity_monitoring = bool<br>    enable_secure_boot          = bool<br>    enable_vtpm                 = bool<br>  })</pre> | <pre>{<br>  "enable_integrity_monitoring": true,<br>  "enable_secure_boot": true,<br>  "enable_vtpm": true<br>}</pre> | no |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Cluster name, used for resource naming and slurm accounting. If not provided it will default to the first 8 characters of the deployment name (removing any invalid characters). | `string` | `null` | no |
| <a name="input_source_image"></a> [source\_image](#input\_source\_image) | DEPRECATED: Use `instance_image` instead. | `string` | `null` | no |
| <a name="input_source_image_family"></a> [source\_image\_family](#input\_source\_image\_family) | DEPRECATED: Use `instance_image` instead. | `string` | `null` | no |
| <a name="input_source_image_project"></a> [source\_image\_project](#input\_source\_image\_project) | DEPRECATED: Use `instance_image` instead. | `string` | `null` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script that will be used by the login node VM. | `string` | `""` | no |
| <a name="input_static_ips"></a> [static\_ips](#input\_static\_ips) | List of static IPs for VM instances. | `list(string)` | `[]` | no |
| <a name="input_subnetwork_project"></a> [subnetwork\_project](#input\_subnetwork\_project) | The project that subnetwork belongs to. | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | Subnet to deploy to. Either network\_self\_link or subnetwork\_self\_link must be specified. | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Network tag list. | `list(string)` | `[]` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone where the instances should be created. If not specified, instances will be<br>spread across available zones in the region. | `string` | `null` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
