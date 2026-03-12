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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.2 |
| <a name="requirement_helm"></a> [helm](#requirement\_helm) | ~> 2.17 |
| <a name="requirement_http"></a> [http](#requirement\_http) | ~> 3.0 |
| <a name="requirement_kubectl"></a> [kubectl](#requirement\_kubectl) | >= 1.7.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.2 |
| <a name="provider_kubectl"></a> [kubectl](#provider\_kubectl) | >= 1.7.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_install_cert_manager"></a> [install\_cert\_manager](#module\_install\_cert\_manager) | ../kubectl-apply/helm_install | n/a |
| <a name="module_install_mldiagnostics_connection_operator"></a> [install\_mldiagnostics\_connection\_operator](#module\_install\_mldiagnostics\_connection\_operator) | ../kubectl-apply/helm_install | n/a |
| <a name="module_install_mldiagnostics_webhook"></a> [install\_mldiagnostics\_webhook](#module\_install\_mldiagnostics\_webhook) | ../kubectl-apply/helm_install | n/a |

## Resources

| Name | Type |
|------|------|
| [kubectl_manifest.mldiagnostics_namespace](https://registry.terraform.io/providers/gavinbunney/kubectl/latest/docs/resources/manifest) | resource |
| [google_client_config.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_config) | data source |
| [google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_cert_manager"></a> [cert\_manager](#input\_cert\_manager) | Install cert-manager | <pre>object({<br/>    install = optional(bool, false)<br/>  })</pre> | `{}` | no |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | An identifier for the gke cluster resource with format projects/<project\_id>/locations/<region>/clusters/<name>. | `string` | n/a | yes |
| <a name="input_gke_cluster_exists"></a> [gke\_cluster\_exists](#input\_gke\_cluster\_exists) | Add a variable to enforce dependency ordering in Terraform | `bool` | `false` | no |
| <a name="input_mldiagnostics_connection_operator"></a> [mldiagnostics\_connection\_operator](#input\_mldiagnostics\_connection\_operator) | Install mldiagnostics connection operator | <pre>object({<br/>    install = optional(bool, false)<br/>  })</pre> | `{}` | no |
| <a name="input_mldiagnostics_webhook"></a> [mldiagnostics\_webhook](#input\_mldiagnostics\_webhook) | Install mldiagnostics webhook | <pre>object({<br/>    install = optional(bool, false)<br/>  })</pre> | `{}` | no |
| <a name="input_namespace"></a> [namespace](#input\_namespace) | Namespace for mldiagnostics | `string` | `"gke-mldiagnostics"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID that hosts the gke cluster. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | GKE ML Diagnostics cluster created |
