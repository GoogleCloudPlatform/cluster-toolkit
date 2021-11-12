## Description
This resource creates a login node for a Slurm cluster based on the
SchedMD/slurm-gcp [login module](https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/login).
The login node is used in conjunction with the [controller](../SchedMD-slurm-on-gcp-controller).

**Warning**: Slurm handles startup scripts differently from virtual machines.
This will not work in conjuntion with the [startup_script](../../../scripts/startup-script/README.md)
resource.

### Example
```
- source: ./resources/third-party/scheduler/SchedMD-slurm-on-gcp-login-node
  kind: terraform
  id: slurm_login
  settings:
    subnetwork_name: ((module.network1.primary_subnetwork.name))
    network_storage:
    - $(homefs.network_storage)
    login_network_storage:
    - $(homefs.network_storage)
    controller_name: $(slurm_controller.controller_node_name)
```
This creates a Slurm login node connected to the primary subnet of network1 with
the homefs filesystem mounted and connecting to `slurm_controller` as the
slurm controller node. For more context see the
[hpc-cluster-small example](../../../../examples/hpc-cluster-small.yaml)

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

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_slurm_cluster_login_node"></a> [slurm\_cluster\_login\_node](#module\_slurm\_cluster\_login\_node) | github.com/SchedMD/slurm-gcp//tf/modules/login/ | v4.0.4 |

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
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to login instances. List of key key, value pairs. | `any` | `{}` | no |
| <a name="input_login_image"></a> [login\_image](#input\_login\_image) | Disk OS image with Slurm preinstalled to use for login node | `string` | `"projects/schedmd-slurm-public/global/images/family/schedmd-slurm-20-11-7-hpc-centos-7"` | no |
| <a name="input_login_instance_template"></a> [login\_instance\_template](#input\_login\_instance\_template) | Instance template to use to create controller instance | `string` | `null` | no |
| <a name="input_login_machine_type"></a> [login\_machine\_type](#input\_login\_machine\_type) | Machine type to use for login node instances. | `string` | `"n2-standard-2"` | no |
| <a name="input_login_node_count"></a> [login\_node\_count](#input\_login\_node\_count) | Number of login nodes in the cluster | `number` | `1` | no |
| <a name="input_login_scopes"></a> [login\_scopes](#input\_login\_scopes) | Scopes to apply to login nodes. | `list(string)` | <pre>[<br>  "https://www.googleapis.com/auth/monitoring.write",<br>  "https://www.googleapis.com/auth/logging.write"<br>]</pre> | no |
| <a name="input_login_service_account"></a> [login\_service\_account](#input\_login\_service\_account) | Service Account for compute nodes. | `string` | `null` | no |
| <a name="input_munge_key"></a> [munge\_key](#input\_munge\_key) | Specific munge key to use | `any` | `null` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on all instances. | <pre>list(object({<br>    server_ip    = string,<br>    remote_mount = string,<br>    local_mount  = string,<br>    fs_type      = string,<br>  mount_options = string }))</pre> | `[]` | no |
| <a name="input_region"></a> [region](#input\_region) | Compute Platform region where the Slurm cluster will be located | `string` | n/a | yes |
| <a name="input_shared_vpc_host_project"></a> [shared\_vpc\_host\_project](#input\_shared\_vpc\_host\_project) | Host project of shared VPC | `string` | `null` | no |
| <a name="input_subnet_depend"></a> [subnet\_depend](#input\_subnet\_depend) | Used as a dependency between the network and instances | `string` | `""` | no |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | The name of the pre-defined VPC subnet you want the nodes to attach to based on Region. | `string` | `null` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone where the notebook server will be located | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
