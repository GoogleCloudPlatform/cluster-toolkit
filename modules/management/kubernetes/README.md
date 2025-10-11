<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2024 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |
| <a name="requirement_google"></a> [google](#requirement\_google) | > 5.0 |
| <a name="requirement_kubernetes"></a> [kubernetes](#requirement\_kubernetes) | >= 2.36 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | > 5.0 |
| <a name="provider_kubernetes"></a> [kubernetes](#provider\_kubernetes) | >= 2.36 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [kubernetes_manifest.this](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/resources/manifest) | resource |
| [google_client_config.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_config) | data source |
| [google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | The full ID of the GKE cluster (e.g., projects/my-project/locations/us-central1/clusters/my-cluster). | `string` | `null` | no |
| <a name="input_content"></a> [content](#input\_content) | Direct content of a YAML manifest. Has precedence over source\_path. | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The GCP project ID where the GKE cluster resides. | `string` | `null` | no |
| <a name="input_source_path"></a> [source\_path](#input\_source\_path) | Path to a single manifest file (.yaml or .tftpl) or a directory of manifests. For a directory, the path must end with a '/'. | `string` | `null` | no |
| <a name="input_template_vars"></a> [template\_vars](#input\_template\_vars) | A map of variables to be used when rendering .tftpl template files. | `map(any)` | `{}` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
