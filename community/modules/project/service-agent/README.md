## Description

Creates the [service agent][service-agent] (Google-managed service account) for
a Google Cloud API, and outputs its email and IAM member string so that roles
can be granted to it.

By default the module enables the target API before generating the service
identity, which requires `roles/serviceusage.serviceUsageAdmin` on the project.
Set `enable_service` to false when the API is already enabled and the deployment
service account does not hold that role; the module will then only look up the
service identity. Use `disable_on_destroy` to control whether the API is
disabled when the deployment is torn down.

[service-agent]: https://cloud.google.com/iam/docs/service-agents

### Example

```yaml
- id: batch-service-agent
  source: community/modules/project/service-agent
  settings:
    project_id: $(vars.project_id)
    service: batch.googleapis.com
```

The `service_agent_member` output is preformatted for IAM bindings, so it can be
passed directly to modules that expect a member string:

```yaml
- id: topic-iam
  source: git::https://github.com/terraform-google-modules/terraform-google-iam.git//modules/pubsub_topics_iam
  settings:
    project: $(vars.project_id)
    pubsub_topics: [$(vars.topic_name)]
    mode: additive
    bindings:
      roles/pubsub.publisher:
      - $(batch-service-agent.service_agent_member)
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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.42 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 5.39.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.42 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 5.39.0 |

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
| <a name="input_disable_on_destroy"></a> [disable\_on\_destroy](#input\_disable\_on\_destroy) | Disable the service on destroy if it was enabled (or already enabled) during apply (default: false) | `bool` | `false` | no |
| <a name="input_enable_service"></a> [enable\_service](#input\_enable\_service) | Enable the target API before generating the service identity. Set to false if the API is already enabled and the deployment service account lacks serviceusage.serviceUsageAdmin. | `bool` | `true` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID | `string` | n/a | yes |
| <a name="input_service"></a> [service](#input\_service) | The GCP service API for which the service agent is to be created | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_service_agent_email"></a> [service\_agent\_email](#output\_service\_agent\_email) | Service Agent email |
| <a name="output_service_agent_member"></a> [service\_agent\_member](#output\_service\_agent\_member) | Service Agent IAM member string (e.g., serviceAccount:email@...) |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
