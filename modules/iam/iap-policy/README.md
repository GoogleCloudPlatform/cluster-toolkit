## Description

This module configures IAM policy for Identity-Aware Proxy (IAP) on a Google Cloud Backend Service.

## Example usage

```yaml
- id: iap_policy
  source: modules/iam/iap-policy
  settings:
    project_id: $(vars.project_id)
    backend_service_id: "your-backend-service-id"
    iap_members:
    - "user:example@google.com"
```

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

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_iap_web_backend_service_iam_member.iap_accessor](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/iap_web_backend_service_iam_member) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_backend_service_id"></a> [backend\_service\_id](#input\_backend\_service\_id) | The ID of the IAP-secured Google Cloud Backend Service (usually obtained from the gke-backend-fetcher module). | `string` | n/a | yes |
| <a name="input_iap_members"></a> [iap\_members](#input\_iap\_members) | List of IAM members to grant the 'roles/iap.httpsResourceAccessor' role (e.g. ['user:example@google.com', 'group:admins@google.com', 'serviceAccount:sa@project.iam.gserviceaccount.com']). | `list(string)` | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID where the backend service resides. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_backend_service_id"></a> [backend\_service\_id](#output\_backend\_service\_id) | The Backend Service ID the policy was applied to. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
