## Description

> **Warning**: this module is now deprecated. We recommend using the Slurm on GCP V5
> [schedmd-slurm-gcp-v5-login](../schedmd-slurm-gcp-v5-login/README.md) instead.

This module creates a login node for a Slurm cluster based on the
[Slurm on GCP][slurm-on-gcp] terraform [login module][login-module]. The login
node is used in conjunction with the
[Slurm controller](../SchedMD-slurm-on-gcp-controller).

> **_Warning:_**: Slurm handles startup scripts differently from virtual
> machines. This will not work in conjuntion with the
> [startup_script](../../../scripts/startup-script/README.md) module.

[login-module]: https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/login

### Example

```yaml
- id: slurm_login
  source: community/modules/scheduler/SchedMD-slurm-on-gcp-login-node
  use:
  - network1
  - homefs
  - slurm_controller
  settings:
    login_machine_type: n2-standard-4
```

This creates a Slurm login node which is:

* connected to the primary subnet of network1 via `use`
* mounted to the homefs filesystem via `use`
* associated with the `slurm_controller` module as the slurm controller via
  `use`
* of VM machine type `n2-standard-4`

## GPU Support

More information on GPU support in Slurm on GCP and other HPC Toolkit modules
can be found at [docs/gpu-support.md](../../../../docs/gpu-support.md)

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_slurm_cluster_login_node"></a> [slurm\_cluster\_login\_node](#module\_slurm\_cluster\_login\_node) | github.com/SchedMD/slurm-gcp//tf/modules/login/ | v4.2.1 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_boot_disk_size"></a> [boot\_disk\_size](#input\_boot\_disk\_size) | Size of boot disk to create for the cluster login node | `number` | `20` | no |
| <a name="input_boot_disk_type"></a> [boot\_disk\_type](#input\_boot\_disk\_type) | Type of boot disk to create for the cluster login node | `string` | `"pd-standard"` | no |
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Name of the cluster | `string` | `null` | no |
| <a name="input_controller_name"></a> [controller\_name](#input\_controller\_name) | FQDN or IP address of the controller node | `string` | n/a | yes |
| <a name="input_controller_secondary_disk"></a> [controller\_secondary\_disk](#input\_controller\_secondary\_disk) | Create secondary disk mounted to controller node | `bool` | `false` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment | `string` | n/a | yes |
| <a name="input_disable_login_public_ips"></a> [disable\_login\_public\_ips](#input\_disable\_login\_public\_ips) | If set to true, create Cloud NAT gateway and enable IAP FW rules | `bool` | `false` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Disk OS image with Slurm preinstalled to use for login node.<br><br>Expected Fields:<br>name: The name of the image. Mutually exclusive with family.<br>family: The image family to use. Mutually exclusive with name.<br>project: The project where the image is hosted.<br>Custom images must comply with Slurm on GCP requirements. | `map(string)` | <pre>{<br>  "family": "schedmd-slurm-21-08-8-hpc-centos-7",<br>  "project": "schedmd-slurm-public"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to login instances. Key-value pairs. | `map(string)` | `{}` | no |
| <a name="input_login_instance_template"></a> [login\_instance\_template](#input\_login\_instance\_template) | Instance template to use to create controller instance | `string` | `null` | no |
| <a name="input_login_machine_type"></a> [login\_machine\_type](#input\_login\_machine\_type) | Machine type to use for login node instances. | `string` | `"n2-standard-2"` | no |
| <a name="input_login_node_count"></a> [login\_node\_count](#input\_login\_node\_count) | Number of login nodes in the cluster | `number` | `1` | no |
| <a name="input_login_scopes"></a> [login\_scopes](#input\_login\_scopes) | Scopes to apply to login nodes. | `list(string)` | <pre>[<br>  "https://www.googleapis.com/auth/monitoring.write",<br>  "https://www.googleapis.com/auth/logging.write",<br>  "https://www.googleapis.com/auth/devstorage.read_only"<br>]</pre> | no |
| <a name="input_login_service_account"></a> [login\_service\_account](#input\_login\_service\_account) | Service Account for compute nodes. | `string` | `null` | no |
| <a name="input_login_startup_script"></a> [login\_startup\_script](#input\_login\_startup\_script) | Custom startup script to run on the login node | `string` | `null` | no |
| <a name="input_munge_key"></a> [munge\_key](#input\_munge\_key) | Specific munge key to use | `any` | `null` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on all instances. | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_region"></a> [region](#input\_region) | Compute Platform region where the Slurm cluster will be located | `string` | n/a | yes |
| <a name="input_shared_vpc_host_project"></a> [shared\_vpc\_host\_project](#input\_shared\_vpc\_host\_project) | Host project of shared VPC | `string` | `null` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Custom startup script to run on the login node. <br>  Will be ignored if `login_startup_script` is specified.<br>  This variable allows Slurm to [use](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules#use-optional) the [startup\_script](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules/scripts/startup-script) module. | `string` | `null` | no |
| <a name="input_subnet_depend"></a> [subnet\_depend](#input\_subnet\_depend) | Used as a dependency between the network and instances | `string` | `""` | no |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | The name of the pre-defined VPC subnet you want the nodes to attach to based on Region. | `string` | `null` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone where the notebook server will be located | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
