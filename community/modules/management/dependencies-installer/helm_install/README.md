<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_helm"></a> [helm](#requirement\_helm) | ~> 2.17 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_helm"></a> [helm](#provider\_helm) | ~> 2.17 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [helm_release.apply_chart](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_atomic"></a> [atomic](#input\_atomic) | If set, the installation process purges chart on failure ('helm install --atomic'). The --wait flag will be set automatically if atomic is used. | `bool` | `false` | no |
| <a name="input_chart_name"></a> [chart\_name](#input\_chart\_name) | Name of the Helm chart (can be a chart reference, path to a packaged chart, path to an unpacked chart directory, or a URL). | `string` | n/a | yes |
| <a name="input_chart_repository"></a> [chart\_repository](#input\_chart\_repository) | URL of the Helm chart repository. Set to null or omit if 'chart\_name' is a path or URL. | `string` | `null` | no |
| <a name="input_chart_version"></a> [chart\_version](#input\_chart\_version) | Version of the Helm chart to install. If omitted, the latest version will be selected (unless 'devel' is true). | `string` | `null` | no |
| <a name="input_cleanup_on_fail"></a> [cleanup\_on\_fail](#input\_cleanup\_on\_fail) | Allow deletion of new resources created in this upgrade when the upgrade fails ('helm upgrade --cleanup-on-fail'). | `bool` | `false` | no |
| <a name="input_create_namespace"></a> [create\_namespace](#input\_create\_namespace) | Set to true to create the namespace if it does not exist ('helm install --create-namespace'). | `bool` | `true` | no |
| <a name="input_dependency_update"></a> [dependency\_update](#input\_dependency\_update) | Run 'helm dependency update' before installing the chart (useful if chart\_name is a local path to an unpacked chart with dependencies). | `bool` | `false` | no |
| <a name="input_description"></a> [description](#input\_description) | Set an optional description for the Helm release. | `string` | `null` | no |
| <a name="input_devel"></a> [devel](#input\_devel) | Use development versions, too ('helm install --devel'). Equivalent to version '>0.0.0-0'. If 'chart\_version' is set, this is ignored. | `bool` | `false` | no |
| <a name="input_disable_crd_hooks"></a> [disable\_crd\_hooks](#input\_disable\_crd\_hooks) | Prevent CRD hooks from running, but run other hooks ('helm install --no-crd-hook'). | `bool` | `false` | no |
| <a name="input_disable_openapi_validation"></a> [disable\_openapi\_validation](#input\_disable\_openapi\_validation) | If set, the installation process will not validate rendered templates against the Kubernetes OpenAPI Schema ('helm install --disable-openapi-validation'). | `bool` | `false` | no |
| <a name="input_disable_webhooks"></a> [disable\_webhooks](#input\_disable\_webhooks) | Prevent hooks from running ('helm install --no-hooks'). | `bool` | `false` | no |
| <a name="input_force_update"></a> [force\_update](#input\_force\_update) | Force resource update through delete/recreate if needed ('helm upgrade --force'). Use with caution. | `bool` | `false` | no |
| <a name="input_keyring"></a> [keyring](#input\_keyring) | Location of public keys used for verification ('helm install --keyring'). Used if 'verify' is true. | `string` | `null` | no |
| <a name="input_lint"></a> [lint](#input\_lint) | Run the helm chart linter during the plan ('helm lint'). | `bool` | `false` | no |
| <a name="input_max_history"></a> [max\_history](#input\_max\_history) | Limit the maximum number of revisions saved per release ('helm upgrade --history-max'). 0 for no limit. | `number` | `null` | no |
| <a name="input_namespace"></a> [namespace](#input\_namespace) | Kubernetes namespace to install the Helm release into. | `string` | `"default"` | no |
| <a name="input_pass_credentials"></a> [pass\_credentials](#input\_pass\_credentials) | Pass credentials to all domains ('helm install --pass-credentials'). Use with caution. | `bool` | `false` | no |
| <a name="input_postrender"></a> [postrender](#input\_postrender) | Configuration for a post-rendering executable ('helm install --post-renderer'). Should be an object with 'binary\_path' attribute. | <pre>object({<br/>    binary_path = string # Path to the post-renderer executable<br/>  })</pre> | `null` | no |
| <a name="input_recreate_pods"></a> [recreate\_pods](#input\_recreate\_pods) | Perform pods restart for the resource if applicable ('helm upgrade --recreate-pods'). Note: This flag is deprecated in Helm CLI v3 itself. | `bool` | `false` | no |
| <a name="input_release_name"></a> [release\_name](#input\_release\_name) | Name of the Helm release. | `string` | n/a | yes |
| <a name="input_render_subchart_notes"></a> [render\_subchart\_notes](#input\_render\_subchart\_notes) | If set, render subchart notes along with the parent chart's notes ('helm install --render-subchart-notes'). | `bool` | `false` | no |
| <a name="input_reset_values"></a> [reset\_values](#input\_reset\_values) | When upgrading, reset the values to the ones built into the chart ('helm upgrade --reset-values'). | `bool` | `false` | no |
| <a name="input_reuse_values"></a> [reuse\_values](#input\_reuse\_values) | When upgrading, reuse the last release's values and merge in any overrides ('helm upgrade --reuse-values'). If 'reset\_values' is specified, this is ignored. | `bool` | `false` | no |
| <a name="input_set_values"></a> [set\_values](#input\_set\_values) | List of objects defining values to set ('helm install --set'). | <pre>list(object({<br/>    name  = string                     # Path to the value (e.g., 'service.type', 'replicaCount')<br/>    value = string                     # The value to set<br/>    type  = optional(string, "string") # Type of value ('string', 'json', 'yaml', 'file')<br/>  }))</pre> | `[]` | no |
| <a name="input_skip_crds"></a> [skip\_crds](#input\_skip\_crds) | If set, no CRDs will be installed ('helm install --skip-crds'). By default, CRDs are installed if not present. | `bool` | `false` | no |
| <a name="input_timeout"></a> [timeout](#input\_timeout) | Time in seconds to wait for any individual Kubernetes operation (like Jobs for hooks) ('helm install --timeout'). | `number` | `300` | no |
| <a name="input_values_yaml"></a> [values\_yaml](#input\_values\_yaml) | List of YAML strings or paths to YAML files containing chart values ('helm install -f'). Can use file() or templatefile(). | `list(string)` | `[]` | no |
| <a name="input_verify"></a> [verify](#input\_verify) | Verify the package before installing it ('helm install --verify'). | `bool` | `false` | no |
| <a name="input_wait"></a> [wait](#input\_wait) | Will wait until all resources are in a ready state before marking the release as successful ('helm install --wait'). | `bool` | `true` | no |
| <a name="input_wait_for_jobs"></a> [wait\_for\_jobs](#input\_wait\_for\_jobs) | If 'wait' is enabled, will wait until all Jobs have been completed before marking the release as successful ('helm install --wait-for-jobs'). | `bool` | `false` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
