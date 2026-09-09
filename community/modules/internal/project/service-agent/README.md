# Service Agent Module

Manages Google Cloud service agent identities.

## Example Usage

```yaml
- group: iam
  modules:
  - id: compute-service-agent
    source: community/modules/internal/project/service-agent
    settings:
      project_id: $(vars.project_id)
      service: compute.googleapis.com
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.42 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 4.42 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.42 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 4.42 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google-beta_google_project_service_identity.service_identity](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_project_service_identity) | resource |
| [google_project_service.enabled_service](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_service) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID | `string` | n/a | yes |
| <a name="input_service"></a> [service](#input\_service) | The GCP service API for which the service agent is to be created | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_service_agent_email"></a> [service\_agent\_email](#output\_service\_agent\_email) | Service Agent email |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
