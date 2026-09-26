# Agent Platform prediction IAM

This module grants prediction access to Gemini Enterprise Agent Platform
(formerly Vertex AI). It creates a custom role with only
`aiplatform.endpoints.predict` and binds it to a Google service account.
It creates no keys and does not manage provider credentials. Set `enabled: false`
to create no IAM resources when an administrator has already granted prediction
access. Choose this before deployment; disabling an existing managed instance
plans deletion of its role and binding.

## Example

Add the following modules to a blueprint deployment group:

```yaml
- id: model_identity
  source: modules/project/service-account
  settings:
    name: model-client
    project_roles: []

- id: vertex_permissions
  source: community/modules/project/vertex-ai-prediction
  use: [model_identity]
```

The `use` reference supplies `service_account_email` from the service account
module. Define `project_id` and `deployment_name` in the blueprint's `vars` and
Toolkit will pass them to the modules automatically.

This module grants prediction access. Workloads must configure authentication
separately, for example using the
[Workload Identity Federation for GKE binding module](../../../../modules/project/workload_identity_binding/README.md).

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
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
| <a name="input_enabled"></a> [enabled](#input\_enabled) | Create the prediction role and binding. Disable when access is provisioned by an administrator. | `bool` | `true` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project containing the Agent Platform publisher model usage. | `string` | n/a | yes |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | Google service account granted prediction access. | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
