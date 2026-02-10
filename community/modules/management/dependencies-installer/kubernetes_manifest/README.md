<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_kubernetes"></a> [kubernetes](#requirement\_kubernetes) | ~> 2.23 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_kubernetes"></a> [kubernetes](#provider\_kubernetes) | ~> 2.23 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [kubernetes_manifest.apply_manifests](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/resources/manifest) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_content"></a> [content](#input\_content) | The YAML body to apply to gke cluster. | `string` | `null` | no |
| <a name="input_field_manager"></a> [field\_manager](#input\_field\_manager) | (Optional) Configure field manager options. The `name` is the name of the field manager. The `force_conflicts` flag allows overriding conflicts. | <pre>object({<br/>    name            = optional(string, null)<br/>    force_conflicts = optional(bool, false)<br/>  })</pre> | `null` | no |
| <a name="input_resource_timeouts"></a> [resource\_timeouts](#input\_resource\_timeouts) | (Optional) Configure custom timeouts for the create, update, and delete operations of the resource. These timeouts also govern the duration for any 'wait' conditions to be met. | <pre>object({<br/>    create = optional(string, null)<br/>    update = optional(string, null)<br/>    delete = optional(string, null)<br/>  })</pre> | <pre>{<br/>  "create": "15m",<br/>  "delete": "5m",<br/>  "update": "10m"<br/>}</pre> | no |
| <a name="input_source_path"></a> [source\_path](#input\_source\_path) | The source for manifest(s) to apply to gke cluster. Acceptable sources are a local yaml or template (.tftpl) file path, a directory (ends with '/') containing yaml or template files, and a url for a yaml file. | `string` | `""` | no |
| <a name="input_template_vars"></a> [template\_vars](#input\_template\_vars) | The values to populate template file(s) with. | `any` | `null` | no |
| <a name="input_wait_for_fields"></a> [wait\_for\_fields](#input\_wait\_for\_fields) | (Optional) A map of attribute paths and desired patterns to be matched. After each apply the provider will wait for all attributes listed here to reach a value that matches the desired pattern. | `map(string)` | `{}` | no |
| <a name="input_wait_for_rollout"></a> [wait\_for\_rollout](#input\_wait\_for\_rollout) | Wait or not for Deployments and APIService to complete rollout. See [kubectl wait](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_wait/) for more details. | `bool` | `true` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
