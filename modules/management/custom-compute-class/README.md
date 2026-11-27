## Description

This module templates and applies Google Kubernetes Engine (GKE)
[Custom Compute Class (`ComputeClass`)](https://cloud.google.com/kubernetes-engine/docs/concepts/about-custom-compute-classes)
CRDs (`apiVersion: cloud.google.com/v1`) to a GKE cluster by wrapping
`modules/management/kubectl-apply`.

It supports two complementary configuration modes:

* **Structured inline definitions (`compute_classes`)**: Declare one or more
  `ComputeClass` resources directly in blueprint YAML with prioritized machine
  families, Spot fallback, node labels, taints, and autoscaling policies.
* **Custom manifest or template (`custom_manifest_path` and
  `custom_template_vars`)**: Render an external YAML manifest or `.tftpl`
  template file for advanced CRD fields.

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |

## Providers

No providers.

## Modules

| Name | Source | Version |
| ---- | ------ | ------- |
| <a name="module_kubectl_apply"></a> [kubectl\_apply](#module\_kubectl\_apply) | ../kubectl-apply | n/a |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_access_token"></a> [access\_token](#input\_access\_token) | GCP OAuth2 access token for authenticating to Kubernetes API. | `string` | `null` | no |
| <a name="input_cluster_ca_certificate"></a> [cluster\_ca\_certificate](#input\_cluster\_ca\_certificate) | Base64 encoded cluster CA certificate. | `string` | `null` | no |
| <a name="input_cluster_endpoint"></a> [cluster\_endpoint](#input\_cluster\_endpoint) | Cluster control plane endpoint host. | `string` | `null` | no |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | The GKE cluster ID (e.g. projects/PROJECT\_ID/locations/LOCATION/clusters/CLUSTER\_NAME). | `string` | n/a | yes |
| <a name="input_compute_classes"></a> [compute\_classes](#input\_compute\_classes) | List of Custom Compute Classes to template and apply. | <pre>list(object({<br/>    name                    = string<br/>    description             = optional(string, "")<br/>    node_pool_auto_creation = optional(bool, true)<br/>    autopilot               = optional(bool, false)<br/><br/>    priorities = list(object({<br/>      machine_family        = string<br/>      spot                  = optional(bool, false)<br/>      min_cores             = optional(number)<br/>      min_memory_gb         = optional(number)<br/>      min_cpu_platform      = optional(string)<br/>      placement_policy_name = optional(string)<br/>      zones                 = optional(list(string), [])<br/>    }))<br/><br/>    node_pool_config = optional(object({<br/>      image_type      = optional(string, "cos_containerd")<br/>      service_account = optional(string)<br/>      node_labels     = optional(map(string), {})<br/>      taints = optional(list(object({<br/>        key    = string<br/>        value  = string<br/>        effect = string<br/>      })), [])<br/>      gvnic_enabled   = optional(bool, true)<br/>      image_streaming = optional(bool, true)<br/>    }))<br/><br/>    autoscaling_policy = optional(object({<br/>      consolidation_delay_minutes = optional(number, 1)<br/>      consolidation_threshold     = optional(number, 0)<br/>    }))<br/>  }))</pre> | `[]` | no |
| <a name="input_custom_manifest_path"></a> [custom\_manifest\_path](#input\_custom\_manifest\_path) | Optional path to a custom ComputeClass YAML file or template. | `string` | `null` | no |
| <a name="input_custom_template_vars"></a> [custom\_template\_vars](#input\_custom\_template\_vars) | Optional template variables for custom\_manifest\_path if it is a templatefile. | `any` | `{}` | no |
| <a name="input_module_id"></a> [module\_id](#input\_module\_id) | The ID of the module as defined in the blueprint. Injected by ghpc. | `string` | `"custom-compute-class"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The GCP project ID. If omitted, inferred from cluster\_id. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_compute_class_names"></a> [compute\_class\_names](#output\_compute\_class\_names) | List of applied ComputeClass CRD names. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
