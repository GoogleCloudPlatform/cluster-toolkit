## Description

This resource will install [DellHPC Omnia](https://github.com/dellhpc/omnia)
onto a cluster supporting a slurm controller and compute nodes. To see a full
example using omnia-install, see the
[omnia-cluster-simple example](../../../examples/omnia-cluster-simple.yaml).

**Warning**: This resource is still under development and not fully supported.
Some steps in the installation have addition dependencies listed below. This
runs `gcloud compute ssh` and `gcloud compute scp` on the machine creating the
deployment (i.e. where you run `terraform apply`).

### Additional Dependencies

**The following apply on the machine where `terraform apply` is called**

* [gcloud](https://cloud.google.com/sdk/gcloud)
* [python3](https://www.python.org/download/releases/3.0/)
* [jinja2](https://palletsprojects.com/p/jinja/) python package

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

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_compute_ips"></a> [compute\_ips](#input\_compute\_ips) | IPs of the Omnia compute nodes | `list(string)` | n/a | yes |
| <a name="input_install_dir"></a> [install\_dir](#input\_install\_dir) | Path where omnia will be installed | `string` | `"/apps"` | no |
| <a name="input_manager_ips"></a> [manager\_ips](#input\_manager\_ips) | IPs of the Omnia manager nodes | `list(string)` | n/a | yes |
| <a name="input_omnia_username"></a> [omnia\_username](#input\_omnia\_username) | Name of the user that installs omnia | `string` | `"omnia"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_add_omnia_user_script"></a> [add\_omnia\_user\_script](#output\_add\_omnia\_user\_script) | An ansible script that adds the user that install omnia |
| <a name="output_inventory_file"></a> [inventory\_file](#output\_inventory\_file) | The inventory file for the omnia cluster |
| <a name="output_runners"></a> [runners](#output\_runners) | The runners to setup and install omnia on the manager |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
