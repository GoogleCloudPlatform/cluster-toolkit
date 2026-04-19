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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.2 |
| <a name="requirement_helm"></a> [helm](#requirement\_helm) | ~> 2.17 |
| <a name="requirement_kubectl"></a> [kubectl](#requirement\_kubectl) | >= 1.7.0 |
| <a name="requirement_kubernetes"></a> [kubernetes](#requirement\_kubernetes) | >= 3.0.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.2 |
| <a name="provider_kubernetes"></a> [kubernetes](#provider\_kubernetes) | >= 3.0.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

| Name | Source | Version |
| ---- | ------ | ------- |
| <a name="module_install_mldiagnostics_connection_operator"></a> [install\_mldiagnostics\_connection\_operator](#module\_install\_mldiagnostics\_connection\_operator) | ../kubectl-apply/helm_install | n/a |
| <a name="module_install_mldiagnostics_webhook"></a> [install\_mldiagnostics\_webhook](#module\_install\_mldiagnostics\_webhook) | ../kubectl-apply/helm_install | n/a |

## Resources

| Name | Type |
| ---- | ---- |
| [kubernetes_labels.workload_namespace_labels](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/resources/labels) | resource |
| [terraform_data.validate_cert_manager](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.validate_namespace](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.validate_sa](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [google_client_config.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_config) | data source |
| [google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |
| [kubernetes_all_namespaces.all](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/data-sources/all_namespaces) | data source |
| [kubernetes_service_account_v1.workload_sa](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/data-sources/service_account_v1) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | An identifier for the gke cluster resource with format projects/<project\_id>/locations/<region>/clusters/<name>. | `string` | n/a | yes |
| <a name="input_gke_cluster_exists"></a> [gke\_cluster\_exists](#input\_gke\_cluster\_exists) | A static flag that signals to downstream modules that a cluster has been created. | `bool` | `false` | no |
| <a name="input_gke_version"></a> [gke\_version](#input\_gke\_version) | GKE version of the cluster | `string` | `null` | no |
| <a name="input_k8s_service_account_name"></a> [k8s\_service\_account\_name](#input\_k8s\_service\_account\_name) | Kubernetes service account name used by the gke cluster | `string` | `"workload-identity-k8s-sa"` | no |
| <a name="input_kubectl_apply_ready"></a> [kubectl\_apply\_ready](#input\_kubectl\_apply\_ready) | A static flag that signals to downstream modules that upstream dependencies are ready. | `any` | `false` | no |
| <a name="input_mldiagnostics"></a> [mldiagnostics](#input\_mldiagnostics) | Unified settings for mldiagnostics | <pre>object({<br/>    enable                      = optional(bool, false)<br/>    injection_webhook_version   = optional(string, "0.25.0")<br/>    connection_operator_version = optional(string, "0.21.0")<br/>  })</pre> | `{}` | no |
| <a name="input_namespace"></a> [namespace](#input\_namespace) | The namespace where ML workloads will run and diagnostics should be enabled. | `string` | `"default"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID that hosts the gke cluster. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_instructions"></a> [instructions](#output\_instructions) | GKE ML Diagnostics cluster created |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
