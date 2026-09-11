# Discovery Engine and Assistant Module

Provisions Discovery Engine generative chat engine and assistant endpoints using Terraform `http` data stanzas to communicate with `v1` (GA) Discovery Engine REST endpoints.

---

## Example

```yaml
- group: discovery-engine
  modules:
  - id: discovery-engine
    source: community/modules/discovery-engine
    settings:
      project_id: $(vars.project_id)
      location: $(vars.location)
      base_url: $(vars.base_url)
      collection: $(vars.collection)
      engine_id: $(vars.engine)
      assistant_id: $(vars.assistant)
```

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2026 Google LLC

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
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.4.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [terraform_data.assistant](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.engine](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_assistant_id"></a> [assistant\_id](#input\_assistant\_id) | Unique ID for the Assistant attached to the Discovery Engine. | `string` | n/a | yes |
| <a name="input_base_url"></a> [base\_url](#input\_base\_url) | Base URL for the Discovery Engine API. | `string` | `"discoveryengine.googleapis.com"` | no |
| <a name="input_collection"></a> [collection](#input\_collection) | Discovery Engine collection ID (e.g., 'default\_collection'). | `string` | `"default_collection"` | no |
| <a name="input_engine_id"></a> [engine\_id](#input\_engine\_id) | Unique ID for the Discovery Engine chat assistant engine. | `string` | n/a | yes |
| <a name="input_location"></a> [location](#input\_location) | Discovery Engine location (e.g., 'global'). | `string` | `"global"` | no |
| <a name="input_module_instance_id"></a> [module\_instance\_id](#input\_module\_instance\_id) | Unique ID of this module instance (automatically populated by gcluster). | `string` | `"discovery-engine-assistant"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_assistant_id"></a> [assistant\_id](#output\_assistant\_id) | The ID of the created Assistant. |
| <a name="output_assistant_name"></a> [assistant\_name](#output\_assistant\_name) | The full resource name of the Assistant. |
| <a name="output_engine_id"></a> [engine\_id](#output\_engine\_id) | The ID of the created Discovery Engine. |
| <a name="output_engine_name"></a> [engine\_name](#output\_engine\_name) | The full resource name of the Discovery Engine. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
