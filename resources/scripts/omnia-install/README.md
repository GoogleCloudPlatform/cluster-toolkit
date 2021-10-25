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
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [null_resource.omnia_install](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_depends"></a> [depends](#input\_depends) | Allows to add explicit dependencies | `list(any)` | `null` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, used to name the cluster | `string` | n/a | yes |
| <a name="input_manager_node"></a> [manager\_node](#input\_manager\_node) | Name of the Omnia manager node | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the Omnia cluster has been created | `string` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | The GCP zone where the Omnia cluster is running | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
