## Description

Allows creation of service accounts for a Google Cloud Platform project.

### Example

```yaml
- id: service_acct
  source: community/modules/project/service-account
  settings:
    project_id: $(vars.project_id)
    name: instance_acct
    project_roles:
    - logging.logWriter
    - monitoring.metricWriter
    - storage.objectViewer
```

This creates a service account in GCP project "project_id" with the name
"instance_acct". It will have the 3 roles listed for all resources within the
project.

### Usage with startup-script module

When this module is used in conjunction with the [startup-script] module, the
service account must be granted (at least) read access to the bucket. This can
be achieved by granting project-wide access as shown above or by specifying the
service account as a bucket viewer in the startup-script module:

```yaml
- id: service_acct
  source: community/modules/project/service-account
  settings:
    project_id: $(vars.project_id)
    name: instance_acct
    project_roles:
    - logging.logWriter
    - monitoring.metricWriter
- id: script
  source: modules/scripts/startup-script
  settings:
    bucket_viewers:
    - $(service_acct.service_account_iam_email)
```

[startup-script]: ../../../../modules/scripts/startup-script/README.md

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
| <a name="module_service_account"></a> [service\_account](#module\_service\_account) | terraform-google-modules/service-accounts/google | ~> 4.2 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_billing_account_id"></a> [billing\_account\_id](#input\_billing\_account\_id) | If assigning billing role, specify a billing account (default is to assign at the organizational level). | `string` | `""` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment (will be prepended to service account name) | `string` | n/a | yes |
| <a name="input_description"></a> [description](#input\_description) | Description of the created service account. | `string` | `"Service Account"` | no |
| <a name="input_descriptions"></a> [descriptions](#input\_descriptions) | Deprecated; create single service accounts using var.description. | `list(string)` | `null` | no |
| <a name="input_display_name"></a> [display\_name](#input\_display\_name) | Display name of the created service account. | `string` | `"Service Account"` | no |
| <a name="input_generate_keys"></a> [generate\_keys](#input\_generate\_keys) | Generate keys for service account. | `bool` | `false` | no |
| <a name="input_grant_billing_role"></a> [grant\_billing\_role](#input\_grant\_billing\_role) | Grant billing user role. | `bool` | `false` | no |
| <a name="input_grant_xpn_roles"></a> [grant\_xpn\_roles](#input\_grant\_xpn\_roles) | Grant roles for shared VPC management. | `bool` | `true` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the service account to create. | `string` | n/a | yes |
| <a name="input_names"></a> [names](#input\_names) | Deprecated; create single service accounts using var.name. | `list(string)` | `null` | no |
| <a name="input_org_id"></a> [org\_id](#input\_org\_id) | Id of the organization for org-level roles. | `string` | `""` | no |
| <a name="input_prefix"></a> [prefix](#input\_prefix) | Deprecated; prefix now set using var.deployment\_name | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of the project | `string` | n/a | yes |
| <a name="input_project_roles"></a> [project\_roles](#input\_project\_roles) | List of roles to grant to service account (e.g. "storage.objectViewer" or "compute.instanceAdmin.v1" | `list(string)` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_key"></a> [key](#output\_key) | Service account key (if creation was requested) |
| <a name="output_service_account_email"></a> [service\_account\_email](#output\_service\_account\_email) | Service account e-mail address |
| <a name="output_service_account_iam_email"></a> [service\_account\_iam\_email](#output\_service\_account\_iam\_email) | Service account IAM binding format (serviceAccount:name@example.com) |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
