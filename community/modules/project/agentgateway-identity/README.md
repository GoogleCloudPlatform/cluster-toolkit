# agentgateway identities

Creates separate Google service accounts for GKE nodes and the agentgateway proxy,
grants node permissions, and binds the proxy Kubernetes service account through
Workload Identity Federation for GKE. It creates no keys. Agent Platform prediction access is managed by the
[vertex-ai-prediction module](../vertex-ai-prediction/README.md).

Set `create_service_accounts: false` and supply `node_service_account_email` and
`proxy_service_account_email` to use existing identities without creating service
accounts or modifying IAM policies. Inputs are validated during Terraform planning.
The module does not verify effective IAM permissions on existing accounts.

```yaml
- id: identity
  source: community/modules/project/agentgateway-identity
  settings:
    create_service_accounts: false
    node_service_account_email: nodes@YOUR_PROJECT.iam.gserviceaccount.com
    proxy_service_account_email: proxy@YOUR_PROJECT.iam.gserviceaccount.com
```

Define `project_id`, `deployment_name`, and `namespace` in the blueprint variables.
See the [administrator setup](../../../examples/agentgateway-gke/README.md#use-existing-service-accounts)
for the permissions and binding required before deployment. Existing identities
are not managed or deleted by this module. Choose the mode before the first
deployment; switching a managed deployment to existing mode plans deletion of
its previously managed IAM resources.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.44.0 |
| <a name="requirement_time"></a> [time](#requirement\_time) | ~> 0.13 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.44.0 |
| <a name="provider_time"></a> [time](#provider\_time) | ~> 0.13 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_project_iam_member.node](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_service_account.node](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_account) | resource |
| [google_service_account.proxy](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_account) | resource |
| [google_service_account_iam_member.workload_identity](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_account_iam_member) | resource |
| [time_sleep.iam_ready](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/sleep) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_create_service_accounts"></a> [create\_service\_accounts](#input\_create\_service\_accounts) | Create service accounts and IAM bindings. When false, use existing accounts without IAM writes. | `bool` | `true` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Unique deployment name used to name managed service accounts. | `string` | n/a | yes |
| <a name="input_namespace"></a> [namespace](#input\_namespace) | Kubernetes namespace containing the agentgateway-proxy service account. | `string` | n/a | yes |
| <a name="input_node_service_account_email"></a> [node\_service\_account\_email](#input\_node\_service\_account\_email) | Existing node service account. Required when create\_service\_accounts is false; otherwise leave empty. | `string` | `""` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project containing the GKE cluster and managed service accounts. | `string` | n/a | yes |
| <a name="input_proxy_service_account_email"></a> [proxy\_service\_account\_email](#input\_proxy\_service\_account\_email) | Existing proxy service account. Required when create\_service\_accounts is false; otherwise leave empty. | `string` | `""` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_node_service_account_email"></a> [node\_service\_account\_email](#output\_node\_service\_account\_email) | Node service account email, created or supplied. |
| <a name="output_proxy_service_account_email"></a> [proxy\_service\_account\_email](#output\_proxy\_service\_account\_email) | Proxy service account email, created or supplied. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
