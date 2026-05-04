
## Description
This community script module manages Helm chart deployment lifecycles inside a GKE cluster using the raw `helm` CLI binary via a `local-exec` provisioner block.
It utilizes the **`helm upgrade --install`** flag sequence, meaning it is fully capable of both **installing new charts** for the first time, as well as **upgrading active charts** during multi-stage deployment configurations.

### Production-Ready Dynamic Authentication
The module handles its own cluster authentication on-the-fly by executing `gcloud container clusters get-credentials` at the start of the provisioner shell block. It does not rely on pre-cached host machine credentials, making it 100% self-sufficient and safe to use inside clean automated CI/CD pipelines (such as Cloud Build).

## Requirements
The host machine running Terraform must have the following tools available in its `PATH`:

* `gcloud` (Google Cloud SDK)
* `helm` (Helm CLI binary)

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
| <a name="requirement_null"></a> [null](#requirement\_null) | >= 3.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_null"></a> [null](#provider\_null) | >= 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [null_resource.helm_upgrade](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_chart_name"></a> [chart\_name](#input\_chart\_name) | Name of the Helm chart to install or upgrade. | `string` | n/a | yes |
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Name of the GKE cluster. | `string` | n/a | yes |
| <a name="input_location"></a> [location](#input\_location) | Location (region or zone) of the GKE cluster. | `string` | n/a | yes |
| <a name="input_namespace"></a> [namespace](#input\_namespace) | Kubernetes namespace to install the release into. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID of the GKE cluster. | `string` | n/a | yes |
| <a name="input_release_name"></a> [release\_name](#input\_release\_name) | Name of the Helm release. | `string` | n/a | yes |
| <a name="input_set_values"></a> [set\_values](#input\_set\_values) | List of key-value pairs to set in the helm chart. | <pre>list(object({<br/>    name  = string<br/>    value = string<br/>  }))</pre> | n/a | yes |
| <a name="input_values_yaml"></a> [values\_yaml](#input\_values\_yaml) | List of paths to values.yaml files to pass to helm upgrade. | `list(string)` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_completed"></a> [completed](#output\_completed) | Indicator that the Helm upgrade completed. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
