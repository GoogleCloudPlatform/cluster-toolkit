## Description

This module creates a Kubernetes secret in a specified namespace on a given GKE cluster. It automatically configures the Kubernetes provider using the GKE cluster credentials.

## Example usage

```yaml
- id: k8s_secret
  source: ./modules/security/kubernetes-secret
  settings:
    cluster_id: "projects/my-project/locations/us-central1/clusters/my-cluster"
    namespace: "default"
    secret_name: "my-secret"
    data:
      key1: "sensitive-value1"
      key2: "sensitive-value2"
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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_kubernetes"></a> [kubernetes](#requirement\_kubernetes) | >= 2.10 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_kubernetes"></a> [kubernetes](#provider\_kubernetes) | >= 2.10 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [kubernetes_secret_v1.secret](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/resources/secret_v1) | resource |
| [google_client_config.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_config) | data source |
| [google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_access_token"></a> [access\_token](#input\_access\_token) | The access token for accessing the cluster. If provided, ignores data source lookup. | `string` | `null` | no |
| <a name="input_cluster_ca_certificate"></a> [cluster\_ca\_certificate](#input\_cluster\_ca\_certificate) | The cluster CA certificate of the GKE cluster. Must be base64-encoded. If provided, ignores data source lookup. | `string` | `null` | no |
| <a name="input_cluster_endpoint"></a> [cluster\_endpoint](#input\_cluster\_endpoint) | The endpoint of the GKE cluster. Do not include the https:// prefix. If provided, ignores data source lookup. | `string` | `null` | no |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | The full GCP resource ID of the GKE cluster in the format projects/PROJECT\_ID/locations/LOCATION/clusters/CLUSTER\_NAME | `string` | n/a | yes |
| <a name="input_data"></a> [data](#input\_data) | Key-value map of secret data | `map(string)` | n/a | yes |
| <a name="input_namespace"></a> [namespace](#input\_namespace) | Kubernetes namespace | `string` | n/a | yes |
| <a name="input_secret_name"></a> [secret\_name](#input\_secret\_name) | Name of the Kubernetes secret | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_secret_name"></a> [secret\_name](#output\_secret\_name) | The name of the created Kubernetes secret |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
