<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2021 Google LLC

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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name                                                         | Type     |
| ------------------------------------------------------------ | -------- |
| [google_project_service](https://https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/google_project_service) | resource |

## Inputs

| Name | Description | Type | Default |
|------|-------------|------|---------|
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project id for which APIs will be enabled | `string` | n/a     |
| <a name="input_gcp_service_list"></a> [gcp\_service\_list](#input\_gcp\_service\_list) | List of APIs or services needs to be enabled. | `list(string)` | "file.googleapis.com",      "compute.googleapis.com",      "container.googleapis.com",      "cloudresourcemanager.googleapis.com",      "billingbudgets.googleapis.com",      "sourcerepo.googleapis.com",      "logging.googleapis.com",      "monitoring.googleapis.com",      "bigquery.googleapis.com",      "sqladmin.googleapis.com",      "servicenetworking.googleapis.com",      "iap.googleapis.com |

<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->