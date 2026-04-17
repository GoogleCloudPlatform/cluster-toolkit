## Description

This is an internal helper module designed to encapsulate and centralize all hardware-specific logic for Google Cloud TPUs. It is intended to be called by parent modules like `gke-node-pool` to determine if a node pool is TPU-based and to retrieve its specific attributes.

This module's primary responsibilities are:

* Reliably detect if a node pool is for TPUs by checking its `placement_policy`.
* Determine the correct GKE `tpu-accelerator` label based on the machine type family.
* Determine the `number of chips per node` based on the specific machine type.
* Generate the standard **Kubernetes taint** that should be applied to TPU nodes.

This follows the same design pattern as the `gpu-definition` internal module, promoting a clean separation of concerns within the gke-node-pool module.

## Usage

This module is not intended for direct use in a blueprint. It should be called from a parent module like `gke-node-pool`.

```yaml
module "tpu" {
  source = "../../internal/tpu-definition"

  # Pass the parent module's variables to this module
  machine_type     = var.machine_type
  placement_policy = var.placement_policy
}

# Example of consuming the module's outputs in the parent module
locals {
  # The tpu_taint is then used in the node_config's dynamic "taint" block
  tpu_taint = module.tpu.tpu_taint
}
```

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2025 Google LLC

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

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | The machine type of the node pool. | `string` | n/a | yes |
| <a name="input_placement_policy"></a> [placement\_policy](#input\_placement\_policy) | The placement policy for the node pool. | <pre>object({<br/>    type         = string<br/>    name         = optional(string)<br/>    tpu_topology = optional(string)<br/>  })</pre> | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_is_tpu"></a> [is\_tpu](#output\_is\_tpu) | Boolean value indicating if the node pool is for TPUs. |
| <a name="output_tpu_accelerator_type"></a> [tpu\_accelerator\_type](#output\_tpu\_accelerator\_type) | The label value for the TPU accelerator type (e.g., 'tpu-v6e-slice'). |
| <a name="output_tpu_chips_per_node"></a> [tpu\_chips\_per\_node](#output\_tpu\_chips\_per\_node) | The number of TPU chips on each node in the pool. |
| <a name="output_tpu_taint"></a> [tpu\_taint](#output\_tpu\_taint) | A list containing the standard TPU taint object if the node pool is for TPUs. |
| <a name="output_tpu_topology"></a> [tpu\_topology](#output\_tpu\_topology) | The topology of the TPU slice (e.g., '4x4'). |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
