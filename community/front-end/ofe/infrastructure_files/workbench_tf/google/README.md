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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | ~> 1.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.87.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | >= 2.0 |
| <a name="requirement_time"></a> [time](#requirement\_time) | >= 0.7.2 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.87.0 |
| <a name="provider_random"></a> [random](#provider\_random) | >= 2.0 |
| <a name="provider_time"></a> [time](#provider\_time) | >= 0.7.2 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_project_radlab_ds_analytics"></a> [project\_radlab\_ds\_analytics](#module\_project\_radlab\_ds\_analytics) | terraform-google-modules/project-factory/google | ~> 11.0 |
| <a name="module_vpc_ai_notebook"></a> [vpc\_ai\_notebook](#module\_vpc\_ai\_notebook) | terraform-google-modules/network/google | ~> 3.0 |
| <a name="module_waitforstartup"></a> [waitforstartup](#module\_waitforstartup) | ./wait-for-startup | n/a |

## Resources

| Name | Type |
|------|------|
| [google_compute_instance_iam_member.oslogin_permissions](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance_iam_member) | resource |
| [google_notebooks_instance.ai_notebook](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/notebooks_instance) | resource |
| [google_project_iam_member.sa_p_notebook_permissions](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_organization_policy.external_ip_policy](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_organization_policy) | resource |
| [google_project_organization_policy.shielded_vm_policy](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_organization_policy) | resource |
| [google_project_organization_policy.trustedimage_project_policy](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_organization_policy) | resource |
| [google_project_service.enabled_services](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_service) | resource |
| [google_service_account.sa_p_notebook](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_account) | resource |
| [google_service_account_iam_member.sa_ai_notebook_user_iam](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_account_iam_member) | resource |
| [google_storage_bucket_object.startup_script](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [random_id.default](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [time_sleep.wait_120_seconds](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/sleep) | resource |
| [google_compute_network.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_network) | data source |
| [google_compute_subnetwork.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork) | data source |
| [google_project.existing_project](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/project) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_billing_account_id"></a> [billing\_account\_id](#input\_billing\_account\_id) | Billing Account associated to the GCP Resources | `string` | `""` | no |
| <a name="input_boot_disk_size_gb"></a> [boot\_disk\_size\_gb](#input\_boot\_disk\_size\_gb) | The size of the boot disk in GB attached to this instance | `number` | `100` | no |
| <a name="input_boot_disk_type"></a> [boot\_disk\_type](#input\_boot\_disk\_type) | Disk types for notebook instances | `string` | `"PD_SSD"` | no |
| <a name="input_create_network"></a> [create\_network](#input\_create\_network) | If the module has to be deployed in an existing network, set this variable to false. | `bool` | `false` | no |
| <a name="input_create_project"></a> [create\_project](#input\_create\_project) | Set to true if the module has to create a project.  If you want to deploy in an existing project, set this variable to false. | `bool` | `false` | no |
| <a name="input_enable_services"></a> [enable\_services](#input\_enable\_services) | Enable the necessary APIs on the project.  When using an existing project, this can be set to false. | `bool` | `false` | no |
| <a name="input_folder_id"></a> [folder\_id](#input\_folder\_id) | Folder ID where the project should be created. It can be skipped if already setting organization\_id. Leave blank if the project should be created directly underneath the Organization node. | `string` | `""` | no |
| <a name="input_image_family"></a> [image\_family](#input\_image\_family) | DEPRECATED: Image of the AI notebook. | `string` | `null` | no |
| <a name="input_image_project"></a> [image\_project](#input\_image\_project) | DEPRECATED: Google Cloud project where the image is hosted. | `string` | `null` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Image of the AI notebook.<br><br>Expected Fields:<br>name: The name of the image. Mutually exclusive with family.<br>family: The image family to use. Mutually exclusive with name.<br>project: The project where the image is hosted. | `map(string)` | <pre>{<br>  "family": "tf-latest-cpu",<br>  "project": "deeplearning-platform-release"<br>}</pre> | no |
| <a name="input_ip_cidr_range"></a> [ip\_cidr\_range](#input\_ip\_cidr\_range) | Unique IP CIDR Range for AI Notebooks subnet | `string` | `"10.142.190.0/24"` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Type of VM you would like to spin up | `string` | `"n1-standard-1"` | no |
| <a name="input_network_name"></a> [network\_name](#input\_network\_name) | Name of the network to be created. | `string` | `"ai-notebook"` | no |
| <a name="input_organization_id"></a> [organization\_id](#input\_organization\_id) | Organization ID where GCP Resources need to get spin up. It can be skipped if already setting folder\_id | `string` | `""` | no |
| <a name="input_owner_id"></a> [owner\_id](#input\_owner\_id) | Billing Account associated to the GCP Resources | `list(any)` | <pre>[<br>  ""<br>]</pre> | no |
| <a name="input_project"></a> [project](#input\_project) | Project in which to launch the AI Notebooks. | `string` | `""` | no |
| <a name="input_project_name"></a> [project\_name](#input\_project\_name) | Project name or ID, if it's an existing project. | `string` | `"gcluster-discovery"` | no |
| <a name="input_random_id"></a> [random\_id](#input\_random\_id) | Adds a suffix of 4 random characters to the `project_id` | `string` | `null` | no |
| <a name="input_region"></a> [region](#input\_region) | Cloud Region associated to the AI Notebooks. | `string` | `"us-east4"` | no |
| <a name="input_set_external_ip_policy"></a> [set\_external\_ip\_policy](#input\_set\_external\_ip\_policy) | Enable org policy to allow External (Public) IP addresses on virtual machines. | `bool` | `false` | no |
| <a name="input_set_shielded_vm_policy"></a> [set\_shielded\_vm\_policy](#input\_set\_shielded\_vm\_policy) | Apply org policy to disable shielded VMs. | `bool` | `false` | no |
| <a name="input_set_trustedimage_project_policy"></a> [set\_trustedimage\_project\_policy](#input\_set\_trustedimage\_project\_policy) | Apply org policy to set the trusted image projects. | `bool` | `false` | no |
| <a name="input_subnet_name"></a> [subnet\_name](#input\_subnet\_name) | Name of the subnet where to deploy the Notebooks. | `string` | `"subnet-ai-notebook"` | no |
| <a name="input_trusted_user"></a> [trusted\_user](#input\_trusted\_user) | User who is allowed to access the notebook | `string` | n/a | yes |
| <a name="input_wb_startup_script_bucket"></a> [wb\_startup\_script\_bucket](#input\_wb\_startup\_script\_bucket) | Name for the bucket where the workbench startup script is stored. | `string` | `""` | no |
| <a name="input_wb_startup_script_name"></a> [wb\_startup\_script\_name](#input\_wb\_startup\_script\_name) | Name & Path for the wb startup script file when uploaded to GCP cloud storage | `string` | `""` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Cloud Zone associated to the AI Notebooks | `string` | `"us-east4-c"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_deployment_id"></a> [deployment\_id](#output\_deployment\_id) | RADLab Module Deployment ID |
| <a name="output_notebook_instance_name"></a> [notebook\_instance\_name](#output\_notebook\_instance\_name) | Notebook Instance Names |
| <a name="output_notebook_proxy_uri"></a> [notebook\_proxy\_uri](#output\_notebook\_proxy\_uri) | Notebook Proxy URIs |
| <a name="output_project_radlab_ds_analytics_id"></a> [project\_radlab\_ds\_analytics\_id](#output\_project\_radlab\_ds\_analytics\_id) | Analytics Project ID |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
