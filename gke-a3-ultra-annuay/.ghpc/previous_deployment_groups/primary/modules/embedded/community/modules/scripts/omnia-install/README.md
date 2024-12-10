## Description

This module will create a set of startup-script runners that will install and
run [DellHPC Omnia](https://github.com/dellhpc/omnia) version 1.3 onto a set of
VMs representing a slurm controller and compute nodes. For a full example using
omnia-install, see the [omnia-cluster example].

**Warning**: This module will create a user named "omnia" by default which has
sudo permissions. You may want to remove this user and/or it's permissions from
each node.

[omnia-cluster example]: ../../../../community/examples/omnia-cluster.yaml

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

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_compute_ips"></a> [compute\_ips](#input\_compute\_ips) | IPs of the Omnia compute nodes | `list(string)` | n/a | yes |
| <a name="input_install_dir"></a> [install\_dir](#input\_install\_dir) | Path where omnia will be installed, defaults to omnia user home directory (/home/omnia).<br/>If specifying this path, please make sure it is on a shared file system, accessible by all omnia nodes. | `string` | `""` | no |
| <a name="input_manager_ips"></a> [manager\_ips](#input\_manager\_ips) | IPs of the Omnia manager nodes | `list(string)` | n/a | yes |
| <a name="input_omnia_username"></a> [omnia\_username](#input\_omnia\_username) | Name of the user that installs omnia | `string` | `"omnia"` | no |
| <a name="input_slurm_uid"></a> [slurm\_uid](#input\_slurm\_uid) | User ID of the slurm user | `number` | `981` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_copy_inventory_runner"></a> [copy\_inventory\_runner](#output\_copy\_inventory\_runner) | Runner to copy the inventory to the omnia manager using the startup-script module |
| <a name="output_install_omnia_runner"></a> [install\_omnia\_runner](#output\_install\_omnia\_runner) | Runner to install Omnia using an ansible playbook. The startup-script module<br/>will automatically handle installation of ansible.<br/>- id: example-startup-script<br/>  source: modules/scripts/startup-script<br/>  settings:<br/>    runners:<br/>    - $(your-omnia-id.install\_omnia\_runner)<br/>... |
| <a name="output_inventory_file"></a> [inventory\_file](#output\_inventory\_file) | The inventory file for the omnia cluster |
| <a name="output_omnia_user_warning"></a> [omnia\_user\_warning](#output\_omnia\_user\_warning) | Warn developers that the omnia user was created with sudo permissions |
| <a name="output_setup_omnia_node_runner"></a> [setup\_omnia\_node\_runner](#output\_setup\_omnia\_node\_runner) | Runner to create the omnia user using an ansible playbook. The startup-script<br/>module will automatically handle installation of ansible.<br/>- id: example-startup-script<br/>  source: modules/scripts/startup-script<br/>  settings:<br/>    runners:<br/>    - $(your-omnia-id.setup\_omnia\_node\_runner)<br/>... |
| <a name="output_setup_omnia_node_script"></a> [setup\_omnia\_node\_script](#output\_setup\_omnia\_node\_script) | An ansible script that adds the user that install omnia |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
