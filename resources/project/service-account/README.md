## Description
Allows creation of service accounts for a Google Cloud Platform project.


### Example
```
- source: ./resources/service-account
  kind: terraform
  id: service_acct
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
| <a name="module_service_accounts"></a> [service\_accounts](#module\_service\_accounts) | terraform-google-modules/service-accounts/google | ~> 3.0 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_display_name"></a> [display\_name](#input\_display\_name) | display names of the created service accounts | `string` | `""` | no |
| <a name="input_names"></a> [names](#input\_names) | names of the services accounts to create | `list(string)` | n/a | yes |
| <a name="input_prefix"></a> [prefix](#input\_prefix) | prefix applied to service account names | `string` | `""` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of the project | `string` | n/a | yes |
| <a name="input_project_roles"></a> [project\_roles](#input\_project\_roles) | list of roles to apply to created service accounts | `list(string)` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_email_address"></a> [email\_address](#output\_email\_address) | singular service account email address |
| <a name="output_email_addresses"></a> [email\_addresses](#output\_email\_addresses) | list of service account email addresses |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
