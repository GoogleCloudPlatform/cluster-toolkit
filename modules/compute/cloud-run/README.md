## Description

This module deploys a Google Cloud Run (v2) service.

## Example usage

```yaml
- id: cloud_run
  source: modules/compute/cloud-run
  settings:
    project_id: $(vars.project_id)
    region: us-central1
    service_name: my-service
    image: us-docker.pkg.dev/cloudrun/container/hello
    container_port: 8080
    env_vars:
      KEY: "VALUE"
    allow_unauthenticated: true
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.48.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.48.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_cloud_run_v2_service.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloud_run_v2_service) | resource |
| [google_cloud_run_v2_service_iam_member.public_access](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloud_run_v2_service_iam_member) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_allow_unauthenticated"></a> [allow\_unauthenticated](#input\_allow\_unauthenticated) | Whether to allow unauthenticated access | `bool` | `true` | no |
| <a name="input_container_port"></a> [container\_port](#input\_container\_port) | Port the container listens on | `number` | `8080` | no |
| <a name="input_env_vars"></a> [env\_vars](#input\_env\_vars) | Environment variables for the container | `map(string)` | `{}` | no |
| <a name="input_image"></a> [image](#input\_image) | Container Image URL | `string` | n/a | yes |
| <a name="input_ingress"></a> [ingress](#input\_ingress) | Ingress traffic allowed for the service. Possible values: INGRESS\_TRAFFIC\_ALL, INGRESS\_TRAFFIC\_INTERNAL\_ONLY, INGRESS\_TRAFFIC\_INTERNAL\_LOAD\_BALANCER. | `string` | `"INGRESS_TRAFFIC_ALL"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to apply to the Cloud Run service | `any` | `{}` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP Project ID | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | GCP Region | `string` | n/a | yes |
| <a name="input_service_name"></a> [service\_name](#input\_service\_name) | Cloud Run Service Name | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_service_name"></a> [service\_name](#output\_service\_name) | The Name of the Cloud Run service |
| <a name="output_service_url"></a> [service\_url](#output\_service\_url) | The URL of the Cloud Run service |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
