## Description

Allows creation of service accounts for a Google Cloud Platform project.

### Example

```yaml
- id: service_acct
  source: community/modules/project/service-account
  kind: terraform
  settings:
  - project_id: $(vars.project_id)
  - names: [ "instance_acct" ]
  - project_roles: [
    "roles/viewer",
    "roles/storage.objectViewer",
  ]
```

This creates a service account in GCP project "project_id" with the name
"instance_acct". It will have the two roles "viewer" and
"storage.objectViewer".

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
| <a name="module_service_accounts"></a> [service\_accounts](#module\_service\_accounts) | terraform-google-modules/service-accounts/google | ~> 4.1 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_billing_account_id"></a> [billing\_account\_id](#input\_billing\_account\_id) | If assigning billing role, specify a billing account (default is to assign at the organizational level). | `string` | `""` | no |
| <a name="input_description"></a> [description](#input\_description) | Default description of the created service accounts (defaults to no description). | `string` | `""` | no |
| <a name="input_descriptions"></a> [descriptions](#input\_descriptions) | List of descriptions of the created service accounts (elements default to the value of description). | `list(string)` | `[]` | no |
| <a name="input_display_name"></a> [display\_name](#input\_display\_name) | display names of the created service accounts. | `string` | `""` | no |
| <a name="input_generate_keys"></a> [generate\_keys](#input\_generate\_keys) | Generate keys for service accounts. | `bool` | `false` | no |
| <a name="input_grant_billing_role"></a> [grant\_billing\_role](#input\_grant\_billing\_role) | Grant billing user role. | `bool` | `false` | no |
| <a name="input_grant_xpn_roles"></a> [grant\_xpn\_roles](#input\_grant\_xpn\_roles) | Grant roles for shared VPC management. | `bool` | `true` | no |
| <a name="input_names"></a> [names](#input\_names) | Names of the services accounts to create. | `list(string)` | `[]` | no |
| <a name="input_org_id"></a> [org\_id](#input\_org\_id) | Id of the organization for org-level roles. | `string` | `""` | no |
| <a name="input_prefix"></a> [prefix](#input\_prefix) | prefix applied to service account names | `string` | `""` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of the project | `string` | n/a | yes |
| <a name="input_project_roles"></a> [project\_roles](#input\_project\_roles) | list of roles to apply to created service accounts | `list(string)` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_email"></a> [email](#output\_email) | Service account email (for single use). |
| <a name="output_emails"></a> [emails](#output\_emails) | Service account emails by name. |
| <a name="output_emails_list"></a> [emails\_list](#output\_emails\_list) | Service account emails s list. |
| <a name="output_iam_email"></a> [iam\_email](#output\_iam\_email) | IAM-format service account email (for single use). |
| <a name="output_iam_emails"></a> [iam\_emails](#output\_iam\_emails) | IAM-format service account emails by name. |
| <a name="output_iam_emails_list"></a> [iam\_emails\_list](#output\_iam\_emails\_list) | IAM-format service account emails s list. |
| <a name="output_key"></a> [key](#output\_key) | Service account key (for single use). |
| <a name="output_keys"></a> [keys](#output\_keys) | Map of service account keys. |
| <a name="output_service_account"></a> [service\_account](#output\_service\_account) | Service account resource (for single use). |
| <a name="output_service_accounts"></a> [service\_accounts](#output\_service\_accounts) | Service account resources as list. |
| <a name="output_service_accounts_map"></a> [service\_accounts\_map](#output\_service\_accounts\_map) | Service account resources by name. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
