## Description

This resource creates a slurm controller node via the SchedMD/slurm-gcp
[controller](https://github.com/SchedMD/slurm-gcp/tree/master/tf/modules/controller)
module.

**Warning**: Slurm handles startup scripts differently from virtual machines.
This will not work in conjuntion with the [startup_script](../../../scripts/startup-script/README.md)
resource.

### Example

```yaml
- source: ./resources/third-party/scheduler/SchedMD-slurm-on-gcp-controller
  kind: terraform
  id: slurm_controller
  settings:
    subnetwork_name: ((module.network1.primary_subnetwork.name))
    login_node_count: 1
    network_storage:
    - $(homefs.network_storage)
    login_network_storage:
    - $(homefs.network_storage)
    partitions:
    - $(compute_partition.partition)
```

This creates a controller node connected to the primary subnetwork with 1 login
node (defined elsewhere). The controller will also have the homefs file system
mounted and manage one partition. For more context see the
[hpc-cluster-small example](../../../../examples/hpc-cluster-small.yaml).

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
| <a name="module_slurm_cluster_controller"></a> [slurm\_cluster\_controller](#module\_slurm\_cluster\_controller) | github.com/SchedMD/slurm-gcp//tf/modules/controller/ | v4.1.3 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_boot_disk_size"></a> [boot\_disk\_size](#input\_boot\_disk\_size) | Size of boot disk to create for the cluster controller node | `number` | `50` | no |
| <a name="input_boot_disk_type"></a> [boot\_disk\_type](#input\_boot\_disk\_type) | Type of boot disk to create for the cluster controller node | `string` | `"pd-standard"` | no |
| <a name="input_cloudsql"></a> [cloudsql](#input\_cloudsql) | Define an existing CloudSQL instance to use instead of instance-local MySQL | <pre>object({<br>    server_ip = string,<br>    user      = string,<br>    password  = string,<br>    db_name   = string<br>  })</pre> | `null` | no |
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Name of the cluster | `string` | `null` | no |
| <a name="input_compute_node_scopes"></a> [compute\_node\_scopes](#input\_compute\_node\_scopes) | Scopes to apply to compute nodes. | `list(string)` | <pre>[<br>  "https://www.googleapis.com/auth/monitoring.write",<br>  "https://www.googleapis.com/auth/logging.write",<br>  "https://www.googleapis.com/auth/devstorage.read_only"<br>]</pre> | no |
| <a name="input_compute_node_service_account"></a> [compute\_node\_service\_account](#input\_compute\_node\_service\_account) | Service Account for compute nodes. | `string` | `null` | no |
| <a name="input_compute_startup_script"></a> [compute\_startup\_script](#input\_compute\_startup\_script) | Custom startup script to run on the compute nodes | `string` | `null` | no |
| <a name="input_controller_image"></a> [controller\_image](#input\_controller\_image) | Slurm image to use for the controller instance | `string` | `"projects/schedmd-slurm-public/global/images/family/schedmd-slurm-21-08-4-hpc-centos-7"` | no |
| <a name="input_controller_instance_template"></a> [controller\_instance\_template](#input\_controller\_instance\_template) | Instance template to use to create controller instance | `string` | `null` | no |
| <a name="input_controller_machine_type"></a> [controller\_machine\_type](#input\_controller\_machine\_type) | Compute Platform machine type to use in controller node creation | `string` | `"n2-standard-2"` | no |
| <a name="input_controller_scopes"></a> [controller\_scopes](#input\_controller\_scopes) | Scopes to apply to the controller | `list(string)` | <pre>[<br>  "https://www.googleapis.com/auth/cloud-platform",<br>  "https://www.googleapis.com/auth/devstorage.read_only"<br>]</pre> | no |
| <a name="input_controller_secondary_disk"></a> [controller\_secondary\_disk](#input\_controller\_secondary\_disk) | Create secondary disk mounted to controller node | `bool` | `false` | no |
| <a name="input_controller_secondary_disk_size"></a> [controller\_secondary\_disk\_size](#input\_controller\_secondary\_disk\_size) | Size of disk for the secondary disk | `number` | `100` | no |
| <a name="input_controller_secondary_disk_type"></a> [controller\_secondary\_disk\_type](#input\_controller\_secondary\_disk\_type) | Disk type (pd-ssd or pd-standard) for secondary disk | `string` | `"pd-ssd"` | no |
| <a name="input_controller_service_account"></a> [controller\_service\_account](#input\_controller\_service\_account) | Service Account for the controller | `string` | `null` | no |
| <a name="input_controller_startup_script"></a> [controller\_startup\_script](#input\_controller\_startup\_script) | Custom startup script to run on the controller | `string` | `null` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment | `string` | n/a | yes |
| <a name="input_disable_compute_public_ips"></a> [disable\_compute\_public\_ips](#input\_disable\_compute\_public\_ips) | If set to true, create Cloud NAT gateway and enable IAP FW rules | `bool` | `true` | no |
| <a name="input_disable_controller_public_ips"></a> [disable\_controller\_public\_ips](#input\_disable\_controller\_public\_ips) | If set to true, create Cloud NAT gateway and enable IAP FW rules | `bool` | `false` | no |
| <a name="input_intel_select_solution"></a> [intel\_select\_solution](#input\_intel\_select\_solution) | Configure the cluster to meet the performance requirement of the Intel Select Solution | `string` | `null` | no |
| <a name="input_jwt_key"></a> [jwt\_key](#input\_jwt\_key) | Specific libjwt key to use | `any` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to controller instance. List of key key, value pairs. | `any` | `{}` | no |
| <a name="input_login_node_count"></a> [login\_node\_count](#input\_login\_node\_count) | Number of login nodes in the cluster | `number` | `0` | no |
| <a name="input_munge_key"></a> [munge\_key](#input\_munge\_key) | Specific munge key to use | `any` | `null` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on all instances. | <pre>list(object({<br>    server_ip     = string,<br>    remote_mount  = string,<br>    local_mount   = string,<br>    fs_type       = string,<br>    mount_options = string<br>  }))</pre> | `[]` | no |
| <a name="input_partition"></a> [partition](#input\_partition) | An array of configurations for specifying multiple machine types residing in their own Slurm partitions. | <pre>list(object({<br>    name                 = string,<br>    machine_type         = string,<br>    max_node_count       = number,<br>    zone                 = string,<br>    image                = string,<br>    image_hyperthreads   = bool,<br>    compute_disk_type    = string,<br>    compute_disk_size_gb = number,<br>    compute_labels       = any,<br>    cpu_platform         = string,<br>    gpu_type             = string,<br>    gpu_count            = number,<br>    network_storage = list(object({<br>      server_ip     = string,<br>      remote_mount  = string,<br>      local_mount   = string,<br>      fs_type       = string,<br>      mount_options = string<br>    })),<br>    preemptible_bursting = string,<br>    vpc_subnet           = string,<br>    exclusive            = bool,<br>    enable_placement     = bool,<br>    regional_capacity    = bool,<br>    regional_policy      = any,<br>    instance_template    = string,<br>    static_node_count    = number<br>  }))</pre> | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Compute Platform project that will host the Slurm cluster | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Compute Platform region where the Slurm cluster will be located | `string` | n/a | yes |
| <a name="input_shared_vpc_host_project"></a> [shared\_vpc\_host\_project](#input\_shared\_vpc\_host\_project) | Host project of shared VPC | `string` | `null` | no |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | The name of the pre-defined VPC subnet you want the nodes to attach to based on Region. | `string` | `null` | no |
| <a name="input_suspend_time"></a> [suspend\_time](#input\_suspend\_time) | Idle time (in sec) to wait before nodes go away | `number` | `300` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone where the servers will be located | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_controller_name"></a> [controller\_name](#output\_controller\_name) | Name of the controller node |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
