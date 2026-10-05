# Discovery Engine and Assistant Module

Provisions Discovery Engine (Vertex AI Search and Conversation) generative chat
engine, data store, and assistant endpoints using native Google Cloud Terraform provider
resources.

---

## Prerequisites

### IAM Permissions

The deploying identity (user or service account) must have the following IAM
roles on the target project:

* `roles/discoveryengine.editor` (or `roles/discoveryengine.admin`)
* `roles/serviceusage.serviceUsageConsumer`

### Quota Project (Application Default Credentials)

The Discovery Engine API requires an explicit quota project when authenticating using local user credentials (ADC). If you see `Error 403: ... requires a quota project`, set the quota project on your credentials:

```bash
gcloud auth application-default set-quota-project <PROJECT_ID>
```

Alternatively, configure the Terraform Google provider environment variables:

```bash
export GOOGLE_USER_PROJECT_OVERRIDE=true
export GOOGLE_BILLING_PROJECT=<PROJECT_ID>
```

## Example

```yaml
- group: discovery-engine
  modules:
  - id: discovery-engine
    source: community/modules/ai/discovery-engine
    settings:
      project_id: $(vars.project_id)
      location: $(vars.location)
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.0.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.0.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_discovery_engine_assistant.assistant](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/discovery_engine_assistant) | resource |
| [google_discovery_engine_data_store.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/discovery_engine_data_store) | resource |
| [google_discovery_engine_search_engine.engine](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/discovery_engine_search_engine) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_assistant_id"></a> [assistant\_id](#input\_assistant\_id) | Unique ID for the Assistant attached to the Discovery Engine. | `string` | n/a | yes |
| <a name="input_collection"></a> [collection](#input\_collection) | Discovery Engine collection ID (e.g., 'default\_collection'). | `string` | `"default_collection"` | no |
| <a name="input_engine_id"></a> [engine\_id](#input\_engine\_id) | Unique ID for the Discovery Engine chat assistant engine. | `string` | n/a | yes |
| <a name="input_location"></a> [location](#input\_location) | Discovery Engine location (e.g., 'global', 'us', 'eu', or an allowlisted in-country location). See [Gemini Enterprise locations](https://cloud.google.com/gemini/enterprise/docs/locations) and [Agent Search locations](https://cloud.google.com/generative-ai-app-builder/docs/locations). | `string` | `"global"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_assistant_name"></a> [assistant\_name](#output\_assistant\_name) | The full resource name of the Assistant. |
| <a name="output_engine_name"></a> [engine\_name](#output\_engine\_name) | The full resource name of the Discovery Engine. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
