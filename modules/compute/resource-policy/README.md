## Description

This modules create a [resource policy for compute engines](https://cloud.google.com/compute/docs/instances/placement-policies-overview). This policy can be passed to a gke-node-pool module to apply the policy on the node-pool's nodes.

Note: By default, you can't apply compact placement policies with a max distance value to A3 VMs. To request access to this feature, contact your [Technical Account Manager (TAM)](https://cloud.google.com/tam) or the [Sales team](https://cloud.google.com/contact).

### Example

The following example creates a group placement resource policy and applies it to a gke-node-pool.

```yaml
  - id: group_placement_1
    source: modules/compute/resource-policy
    settings:
      name: gp-np-1
      group_placement_max_distance: 2

  - id: node_pool_1
    source: modules/compute/gke-node-pool
    use: [group_placement_1]
    settings:
      machine_type: e2-standard-8
    outputs: [instructions]
```

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
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 6.29.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 6.29.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_compute_resource_policy.policy](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_resource_policy) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_group_placement_max_distance"></a> [group\_placement\_max\_distance](#input\_group\_placement\_max\_distance) | The max distance for group placement policy to use for the node pool's nodes. If set it will add a compact group placement policy.<br/>Note: Placement policies have the [following](https://cloud.google.com/compute/docs/instances/placement-policies-overview#restrictions-compact-policies) restrictions. | `number` | `0` | no |
| <a name="input_name"></a> [name](#input\_name) | The resource policy's name. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID for the resource policy. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region for the the resource policy. | `string` | n/a | yes |
| <a name="input_workload_policy"></a> [workload\_policy](#input\_workload\_policy) | Describes the workload policy | <pre>object({<br/>    type                  = optional(string, null)<br/>    max_topology_distance = optional(string, null)<br/>    accelerator_topology  = optional(string, null)<br/>  })</pre> | <pre>{<br/>  "accelerator_topology": null,<br/>  "max_topology_distance": null,<br/>  "type": null<br/>}</pre> | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_placement_policy"></a> [placement\_policy](#output\_placement\_policy) | Group placement policy to use for placing VMs or GKE nodes placement. `COMPACT` is the only supported value for `type` currently. `name` is the name of the placement policy.<br/>It is assumed that the specified policy exists. To create a placement policy refer to https://cloud.google.com/sdk/gcloud/reference/compute/resource-policies/create/group-placement.<br/>Note: Placement policies have the [following](https://cloud.google.com/compute/docs/instances/placement-policies-overview#restrictions-compact-policies) restrictions.<br/>The value `tpu_topology` is only used for TPU node pools. The `gke-node-pool` module ensures it is configured appropriately for only TPUs during placement policy mapping. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
