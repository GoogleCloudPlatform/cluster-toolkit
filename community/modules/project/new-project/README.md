## Description

This module allows you to create opinionated Google Cloud Platform projects. It
creates projects and configures aspects like Shared VPC connectivity, IAM
access, Service Accounts, and API enablement to follow best practices.

This module is meant for use with Terraform 0.13.

**Note:** This module has been removed from the Cluster Toolkit. The upstream module (`terraform-google-project-factory`) is now the recommended way to create and manage GCP projects.

### Example

```yaml
- id: project
  source: github.com/terraform-google-modules/terraform-google-project-factory?rev=v17.0.0&depth=1
```

This creates a new project with pre-defined project ID, a designated folder and
organization and associated billing account which will be used to pay for
services consumed.

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
| <a name="module_project_factory"></a> [project\_factory](#module\_project\_factory) | terraform-google-modules/project-factory/google | ~> 11.3 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_activate_api_identities"></a> [activate\_api\_identities](#input\_activate\_api\_identities) | The list of service identities (Google Managed service account for the API) to force-create for the project (e.g. in order to grant additional roles).<br/>    APIs in this list will automatically be appended to `activate_apis`.<br/>    Not including the API in this list will follow the default behaviour for identity creation (which is usually when the first resource using the API is created).<br/>    Any roles (e.g. service agent role) must be explicitly listed. See https://cloud.google.com/iam/docs/understanding-roles#service-agent-roles-roles for a list of related roles. | <pre>list(object({<br/>    api   = string<br/>    roles = list(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_activate_apis"></a> [activate\_apis](#input\_activate\_apis) | The list of apis to activate within the project | `list(string)` | <pre>[<br/>  "compute.googleapis.com",<br/>  "serviceusage.googleapis.com",<br/>  "storage.googleapis.com"<br/>]</pre> | no |
| <a name="input_auto_create_network"></a> [auto\_create\_network](#input\_auto\_create\_network) | Create the default network | `bool` | `false` | no |
| <a name="input_billing_account"></a> [billing\_account](#input\_billing\_account) | The ID of the billing account to associate this project with | `string` | n/a | yes |
| <a name="input_bucket_force_destroy"></a> [bucket\_force\_destroy](#input\_bucket\_force\_destroy) | Force the deletion of all objects within the GCS bucket when deleting the bucket (optional) | `bool` | `false` | no |
| <a name="input_bucket_labels"></a> [bucket\_labels](#input\_bucket\_labels) | A map of key/value label pairs to assign to the bucket (optional) | `map(string)` | `{}` | no |
| <a name="input_bucket_location"></a> [bucket\_location](#input\_bucket\_location) | The location for a GCS bucket to create (optional) | `string` | `"US"` | no |
| <a name="input_bucket_name"></a> [bucket\_name](#input\_bucket\_name) | A name for a GCS bucket to create (in the bucket\_project project), useful for Terraform state (optional) | `string` | `""` | no |
| <a name="input_bucket_project"></a> [bucket\_project](#input\_bucket\_project) | A project to create a GCS bucket (bucket\_name) in, useful for Terraform state (optional) | `string` | `""` | no |
| <a name="input_bucket_ula"></a> [bucket\_ula](#input\_bucket\_ula) | Enable Uniform Bucket Level Access | `bool` | `true` | no |
| <a name="input_bucket_versioning"></a> [bucket\_versioning](#input\_bucket\_versioning) | Enable versioning for a GCS bucket to create (optional) | `bool` | `false` | no |
| <a name="input_budget_alert_pubsub_topic"></a> [budget\_alert\_pubsub\_topic](#input\_budget\_alert\_pubsub\_topic) | The name of the Cloud Pub/Sub topic where budget related messages will be published, in the form of `projects/{project_id}/topics/{topic_id}` | `string` | `null` | no |
| <a name="input_budget_alert_spent_percents"></a> [budget\_alert\_spent\_percents](#input\_budget\_alert\_spent\_percents) | A list of percentages of the budget to alert on when threshold is exceeded | `list(number)` | <pre>[<br/>  0.5,<br/>  0.7,<br/>  1<br/>]</pre> | no |
| <a name="input_budget_amount"></a> [budget\_amount](#input\_budget\_amount) | The amount to use for a budget alert | `number` | `null` | no |
| <a name="input_budget_display_name"></a> [budget\_display\_name](#input\_budget\_display\_name) | The display name of the budget. If not set defaults to `Budget For <projects[0]|All Projects>` | `string` | `null` | no |
| <a name="input_budget_monitoring_notification_channels"></a> [budget\_monitoring\_notification\_channels](#input\_budget\_monitoring\_notification\_channels) | A list of monitoring notification channels in the form `[projects/{project_id}/notificationChannels/{channel_id}]`. A maximum of 5 channels are allowed. | `list(string)` | `[]` | no |
| <a name="input_consumer_quotas"></a> [consumer\_quotas](#input\_consumer\_quotas) | The quotas configuration you want to override for the project. | <pre>list(object({<br/>    service = string,<br/>    metric  = string,<br/>    limit   = string,<br/>    value   = string,<br/>  }))</pre> | `[]` | no |
| <a name="input_create_project_sa"></a> [create\_project\_sa](#input\_create\_project\_sa) | Whether the default service account for the project shall be created | `bool` | `true` | no |
| <a name="input_default_network_tier"></a> [default\_network\_tier](#input\_default\_network\_tier) | Default Network Service Tier for resources created in this project. If unset, the value will not be modified. See https://cloud.google.com/network-tiers/docs/using-network-service-tiers and https://cloud.google.com/network-tiers. | `string` | `""` | no |
| <a name="input_default_service_account"></a> [default\_service\_account](#input\_default\_service\_account) | Project default service account setting: can be one of `delete`, `deprivilege`, `disable`, or `keep`. | `string` | `"keep"` | no |
| <a name="input_disable_dependent_services"></a> [disable\_dependent\_services](#input\_disable\_dependent\_services) | Whether services that are enabled and which depend on this service should also be disabled when this service is destroyed. | `bool` | `true` | no |
| <a name="input_disable_services_on_destroy"></a> [disable\_services\_on\_destroy](#input\_disable\_services\_on\_destroy) | Whether project services will be disabled when the resources are destroyed | `bool` | `true` | no |
| <a name="input_domain"></a> [domain](#input\_domain) | The domain name (optional). | `string` | `""` | no |
| <a name="input_enable_shared_vpc_host_project"></a> [enable\_shared\_vpc\_host\_project](#input\_enable\_shared\_vpc\_host\_project) | If this project is a shared VPC host project. If true, you must *not* set svpc\_host\_project\_id variable. Default is false. | `bool` | `false` | no |
| <a name="input_folder_id"></a> [folder\_id](#input\_folder\_id) | The ID of a folder to host this project | `string` | `""` | no |
| <a name="input_grant_services_network_role"></a> [grant\_services\_network\_role](#input\_grant\_services\_network\_role) | Whether or not to grant service agents the network roles on the host project | `bool` | `true` | no |
| <a name="input_grant_services_security_admin_role"></a> [grant\_services\_security\_admin\_role](#input\_grant\_services\_security\_admin\_role) | Whether or not to grant Kubernetes Engine Service Agent the Security Admin role on the host project so it can manage firewall rules | `bool` | `false` | no |
| <a name="input_group_name"></a> [group\_name](#input\_group\_name) | A group to control the project by being assigned group\_role (defaults to project editor) | `string` | `""` | no |
| <a name="input_group_role"></a> [group\_role](#input\_group\_role) | The role to give the controlling group (group\_name) over the project (defaults to project editor) | `string` | `"roles/editor"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Map of labels for project | `map(string)` | `{}` | no |
| <a name="input_lien"></a> [lien](#input\_lien) | Add a lien on the project to prevent accidental deletion | `bool` | `false` | no |
| <a name="input_name"></a> [name](#input\_name) | The name for the project | `string` | `null` | no |
| <a name="input_org_id"></a> [org\_id](#input\_org\_id) | The organization ID. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The ID to give the project. If not provided, the `name` will be used. | `string` | `""` | no |
| <a name="input_project_sa_name"></a> [project\_sa\_name](#input\_project\_sa\_name) | Default service account name for the project. | `string` | `"project-service-account"` | no |
| <a name="input_random_project_id"></a> [random\_project\_id](#input\_random\_project\_id) | Adds a suffix of 4 random characters to the `project_id` | `bool` | `false` | no |
| <a name="input_sa_role"></a> [sa\_role](#input\_sa\_role) | A role to give the default Service Account for the project (defaults to none) | `string` | `""` | no |
| <a name="input_shared_vpc_subnets"></a> [shared\_vpc\_subnets](#input\_shared\_vpc\_subnets) | List of subnets fully qualified subnet IDs (ie. projects/$project\_id/regions/$region/subnetworks/$subnet\_id) | `list(string)` | `[]` | no |
| <a name="input_svpc_host_project_id"></a> [svpc\_host\_project\_id](#input\_svpc\_host\_project\_id) | The ID of the host project which hosts the shared VPC | `string` | `""` | no |
| <a name="input_usage_bucket_name"></a> [usage\_bucket\_name](#input\_usage\_bucket\_name) | Name of a GCS bucket to store GCE usage reports in (optional) | `string` | `""` | no |
| <a name="input_usage_bucket_prefix"></a> [usage\_bucket\_prefix](#input\_usage\_bucket\_prefix) | Prefix in the GCS bucket to store GCE usage reports in (optional) | `string` | `""` | no |
| <a name="input_vpc_service_control_attach_enabled"></a> [vpc\_service\_control\_attach\_enabled](#input\_vpc\_service\_control\_attach\_enabled) | Whether the project will be attached to a VPC Service Control Perimeter | `bool` | `false` | no |
| <a name="input_vpc_service_control_perimeter_name"></a> [vpc\_service\_control\_perimeter\_name](#input\_vpc\_service\_control\_perimeter\_name) | The name of a VPC Service Control Perimeter to add the created project to | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_api_s_account"></a> [api\_s\_account](#output\_api\_s\_account) | API service account email |
| <a name="output_api_s_account_fmt"></a> [api\_s\_account\_fmt](#output\_api\_s\_account\_fmt) | API service account email formatted for terraform use |
| <a name="output_budget_name"></a> [budget\_name](#output\_budget\_name) | The name of the budget if created |
| <a name="output_domain"></a> [domain](#output\_domain) | The organization's domain |
| <a name="output_enabled_api_identities"></a> [enabled\_api\_identities](#output\_enabled\_api\_identities) | Enabled API identities in the project |
| <a name="output_enabled_apis"></a> [enabled\_apis](#output\_enabled\_apis) | Enabled APIs in the project |
| <a name="output_group_email"></a> [group\_email](#output\_group\_email) | The email of the G Suite group with group\_name |
| <a name="output_project_bucket_self_link"></a> [project\_bucket\_self\_link](#output\_project\_bucket\_self\_link) | Project's bucket selfLink |
| <a name="output_project_bucket_url"></a> [project\_bucket\_url](#output\_project\_bucket\_url) | Project's bucket url |
| <a name="output_project_id"></a> [project\_id](#output\_project\_id) | ID of the project that was created |
| <a name="output_project_name"></a> [project\_name](#output\_project\_name) | Name of the project that was created |
| <a name="output_project_number"></a> [project\_number](#output\_project\_number) | Number of the project that was created |
| <a name="output_service_account_display_name"></a> [service\_account\_display\_name](#output\_service\_account\_display\_name) | The display name of the default service account |
| <a name="output_service_account_email"></a> [service\_account\_email](#output\_service\_account\_email) | The email of the default service account |
| <a name="output_service_account_id"></a> [service\_account\_id](#output\_service\_account\_id) | The id of the default service account |
| <a name="output_service_account_name"></a> [service\_account\_name](#output\_service\_account\_name) | The fully-qualified name of the default service account |
| <a name="output_service_account_unique_id"></a> [service\_account\_unique\_id](#output\_service\_account\_unique\_id) | The unique id of the default service account |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
