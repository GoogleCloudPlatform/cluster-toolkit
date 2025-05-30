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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 6.34.1 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | ~> 6.34.1 |

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_a2_16_nodeset"></a> [a2\_16\_nodeset](#module\_a2\_16\_nodeset) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset | v1.38.0&depth=1 |
| <a name="module_a2_16_partition"></a> [a2\_16\_partition](#module\_a2\_16\_partition) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition | v1.38.0&depth=1 |
| <a name="module_a2_8_nodeset"></a> [a2\_8\_nodeset](#module\_a2\_8\_nodeset) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset | v1.38.0&depth=1 |
| <a name="module_a2_8_partition"></a> [a2\_8\_partition](#module\_a2\_8\_partition) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition | v1.38.0&depth=1 |
| <a name="module_c2_nodeset"></a> [c2\_nodeset](#module\_c2\_nodeset) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset | v1.38.0&depth=1 |
| <a name="module_c2_partition"></a> [c2\_partition](#module\_c2\_partition) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition | v1.38.0&depth=1 |
| <a name="module_c2d_nodeset"></a> [c2d\_nodeset](#module\_c2d\_nodeset) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset | v1.38.0&depth=1 |
| <a name="module_c2d_partition"></a> [c2d\_partition](#module\_c2d\_partition) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition | v1.38.0&depth=1 |
| <a name="module_c3_nodeset"></a> [c3\_nodeset](#module\_c3\_nodeset) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset | v1.38.0&depth=1 |
| <a name="module_c3_partition"></a> [c3\_partition](#module\_c3\_partition) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition | v1.38.0&depth=1 |
| <a name="module_compute_sa"></a> [compute\_sa](#module\_compute\_sa) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/project/service-account | v1.38.0&depth=1 |
| <a name="module_controller_sa"></a> [controller\_sa](#module\_controller\_sa) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/project/service-account | v1.37.0&depth=1 |
| <a name="module_h3_nodeset"></a> [h3\_nodeset](#module\_h3\_nodeset) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset | v1.38.0&depth=1 |
| <a name="module_h3_partition"></a> [h3\_partition](#module\_h3\_partition) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition | v1.38.0&depth=1 |
| <a name="module_homefs"></a> [homefs](#module\_homefs) | github.com/GoogleCloudPlatform/cluster-toolkit//modules/file-system/filestore | v1.37.0&depth=1 |
| <a name="module_hpc_dashboard"></a> [hpc\_dashboard](#module\_hpc\_dashboard) | github.com/GoogleCloudPlatform/cluster-toolkit//modules/monitoring/dashboard | v1.38.0&depth=1 |
| <a name="module_login_sa"></a> [login\_sa](#module\_login\_sa) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/project/service-account | v1.38.0&depth=1 |
| <a name="module_n2_nodeset"></a> [n2\_nodeset](#module\_n2\_nodeset) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-nodeset | v1.38.0&depth=1 |
| <a name="module_n2_partition"></a> [n2\_partition](#module\_n2\_partition) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/compute/schedmd-slurm-gcp-v6-partition | v1.38.0&depth=1 |
| <a name="module_network"></a> [network](#module\_network) | github.com/GoogleCloudPlatform/cluster-toolkit//modules/network/vpc | v1.38.0&depth=1 |
| <a name="module_projectsfs"></a> [projectsfs](#module\_projectsfs) | github.com/GoogleCloudPlatform/cluster-toolkit//modules/file-system/filestore | v1.38.0&depth=1 |
| <a name="module_scratchfs"></a> [scratchfs](#module\_scratchfs) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/file-system/DDN-EXAScaler | v1.38.0&depth=1 |
| <a name="module_slurm_controller"></a> [slurm\_controller](#module\_slurm\_controller) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/scheduler/schedmd-slurm-gcp-v6-controller | v1.38.0&depth=1 |
| <a name="module_slurm_login"></a> [slurm\_login](#module\_slurm\_login) | github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/scheduler/schedmd-slurm-gcp-v6-login | v1.38.0&depth=1 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Toolkit deployment variable: deployment\_name | `string` | n/a | yes |
| <a name="input_gpu_zones"></a> [gpu\_zones](#input\_gpu\_zones) | Toolkit deployment variable: gpu\_zones | `list(any)` | n/a | yes |
| <a name="input_instance_image_custom"></a> [instance\_image\_custom](#input\_instance\_image\_custom) | Toolkit deployment variable: instance\_image\_custom | `bool` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Toolkit deployment variable: labels | `any` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Toolkit deployment variable: project\_id | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Toolkit deployment variable: region | `string` | n/a | yes |
| <a name="input_slurm_image"></a> [slurm\_image](#input\_slurm\_image) | Toolkit deployment variable: slurm\_image | `any` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | Toolkit deployment variable: zone | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions_hpc_dashboard"></a> [instructions\_hpc\_dashboard](#output\_instructions\_hpc\_dashboard) | Generated output from module 'hpc\_dashboard' |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->