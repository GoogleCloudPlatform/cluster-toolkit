<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2023 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.60.0, < 5.0 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 4.60.0, < 5.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.60.0, < 5.0 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 4.60.0, < 5.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_container_node_pool.node_pool](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_container_node_pool) | resource |
| [google_project_iam_member.node_service_account_artifact_registry](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.node_service_account_gcr](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.node_service_account_log_writer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.node_service_account_metric_writer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.node_service_account_monitoring_viewer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.node_service_account_resource_metadata_writer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_compute_default_service_account.default_sa](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_default_service_account) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_auto_upgrade"></a> [auto\_upgrade](#input\_auto\_upgrade) | Whether the nodes will be automatically upgraded. | `bool` | `false` | no |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | projects/{{project}}/locations/{{location}}/clusters/{{cluster}} | `string` | n/a | yes |
| <a name="input_compact_placement"></a> [compact\_placement](#input\_compact\_placement) | Places node pool's nodes in a closer physical proximity in order to reduce network latency between nodes. | `bool` | `false` | no |
| <a name="input_image_type"></a> [image\_type](#input\_image\_type) | The default image type used by NAP once a new node pool is being created. Use either COS\_CONTAINERD or UBUNTU\_CONTAINERD. | `string` | `"COS_CONTAINERD"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | GCE resource labels to be applied to resources. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | The name of a Google Compute Engine machine type. | `string` | `"c2-standard-60"` | no |
| <a name="input_name"></a> [name](#input\_name) | The name of the node pool. If left blank, will default to the machine type. | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID to host the cluster in. | `string` | n/a | yes |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to use with the system node pool | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | <pre>{<br>  "email": null,<br>  "scopes": [<br>    "https://www.googleapis.com/auth/cloud-platform"<br>  ]<br>}</pre> | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision VMs using discounted Spot pricing, allowing for preemption | `bool` | `false` | no |
| <a name="input_taints"></a> [taints](#input\_taints) | Taints to be applied to the system node pool. | <pre>list(object({<br>    key    = string<br>    value  = any<br>    effect = string<br>  }))</pre> | `[]` | no |
| <a name="input_threads_per_core"></a> [threads\_per\_core](#input\_threads\_per\_core) | Sets the number of threads per physical core. By setting threads\_per\_core<br>to 2, Simultaneous Multithreading (SMT) is enabled extending the total number<br>of virtual cores. For example, a machine of type c2-standard-60 will have 60<br>virtual cores with threads\_per\_core equal to 2. With threads\_per\_core equal<br>to 1 (SMT turned off), only the 30 physical cores will be available on the VM.<br><br>The default value of \"0\" will turn off SMT for supported machine types, and<br>will fall back to GCE defaults for unsupported machine types (t2d, shared-core<br>instances, or instances with less than 2 vCPU).<br><br>Disabling SMT can be more performant in many HPC workloads, therefore it is<br>disabled by default where compatible.<br><br>null = SMT configuration will use the GCE defaults for the machine type<br>0 = SMT will be disabled where compatible (default)<br>1 = SMT will always be disabled (will fail on incompatible machine types)<br>2 = SMT will always be enabled (will fail on incompatible machine types) | `number` | `0` | no |
| <a name="input_total_max_nodes"></a> [total\_max\_nodes](#input\_total\_max\_nodes) | Total maximum number of nodes in the NodePool. | `number` | `1000` | no |
| <a name="input_total_min_nodes"></a> [total\_min\_nodes](#input\_total\_min\_nodes) | Total minimum number of nodes in the NodePool. | `number` | `0` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
