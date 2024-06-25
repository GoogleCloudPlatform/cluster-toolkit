## Description

This module discovers a Google Kubernetes Engine ([GKE](https://cloud.google.com/kubernetes-engine)) cluster that already exists in Google Cloud and
outputs cluster attributes that uniquely identify it for use by other modules.
The module outputs are aligned with the [gke-cluster module][gke-cluster] so that it can be used
as a drop-in substitute when a GKE cluster already exists.

The below sample blueprint discovers the existing GKE cluster named "my-gke-cluster" in "us-central1" region. With the `use` keyword, the
[gke-node-pool] module accepts the `cluser_id`
input variable that uniquely identifies the existing GKE cluster in which the
GKE node pool will be created.

[gke-cluster]: ../gke-cluster/README.md
[gke-node-pool]: ../../compute/gke-node-pool/README.md

### Example

```yaml
- id: existing-gke-cluster
  source: community/modules/scheduler/pre-existing-gke-cluster
  settings:
    project_id: $(vars.project_id)
    cluster_name: my-gke-cluster
    region: us-central1

- id: compute_pool
  source: community/modules/compute/gke-node-pool
  use: [existing-gke-cluster]
```

> **_NOTE:_** The `project_id` and `region` settings would be inferred from the
> deployment variables of the same name, but they are included here for clarity.

## License

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | > 5.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | > 5.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_container_cluster.existing_gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Name of the existing cluster | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project that hosts the existing cluster | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region in which to search for the cluster | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_cluster_id"></a> [cluster\_id](#output\_cluster\_id) | An identifier for the gke cluster with format projects/{{project\_id}}/locations/{{region}}/clusters/{{name}}. |
| <a name="output_gke_cluster_exists"></a> [gke\_cluster\_exists](#output\_gke\_cluster\_exists) | A static flag that signals to downstream modules that a cluster exists. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
