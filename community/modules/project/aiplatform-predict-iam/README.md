# Agent Platform prediction IAM

This module grants prediction access to Gemini Enterprise Agent Platform
(formerly Vertex AI). It creates a custom role with only
`aiplatform.endpoints.predict` and binds it to a Google service account.
Creating the custom role requires `iam.roles.create` on the deploying identity
(for example, through `roles/iam.roleAdmin`). Using the broader predefined
`roles/aiplatform.user` role avoids custom-role creation, but assigning either
role still requires permission to update the project's IAM policy.

It creates no keys and does not manage provider credentials. Omit this module
when an administrator has already granted prediction access. Choose this before
deployment; removing an existing managed instance plans deletion of its role
and binding.

## Example

Add the following modules to a blueprint deployment group:

```yaml
- id: model_identity
  source: modules/project/service-account
  settings:
    name: model-client
    project_roles: []

- id: vertex_permissions
  source: community/modules/project/aiplatform-predict-iam
  use: [model_identity]
```

The `use` reference supplies `service_account_email` from the service account
module. Define `project_id` and `deployment_name` in the blueprint's `vars` and
Toolkit will pass them to the modules automatically.

This module grants prediction access. Workloads must configure authentication
separately, for example using the
[Workload Identity Federation for GKE binding module](../../../../modules/project/workload_identity_binding/README.md).

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.44.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.44.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_project_iam_custom_role.predict](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_custom_role) | resource |
| [google_project_iam_member.predict](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Unique deployment name, used to name the custom role. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project containing the Agent Platform publisher model usage. | `string` | n/a | yes |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | Google service account granted prediction access. | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
