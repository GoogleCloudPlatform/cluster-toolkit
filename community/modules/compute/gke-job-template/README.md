## Description

This module is used to create a Kubernetes job template file.

The job template file can be submitted as is or used as a template for further
customization. Add the `instructions` output to a blueprint (as shown below) to
get instructions on how to use `kubectl` to submit the job.

This module is designed to `use` one or more `gke-node-pool` modules. The job
will be configured to run on any of the specified node pools.

> **_NOTE:_** This is an experimental module and the functionality and
> documentation will likely be updated in the near future. This module has only
> been tested in limited capacity.

### Example

The following example creates a GKE job template file.

```yaml
  - id: job-template
    source: community/modules/compute/gke-job-template
    use: [compute_pool]
    settings:
      node_count: 3
    outputs: [instructions]
```

Also see a full [GKE example blueprint](../../../examples/hpc-gke.yaml).

### Storage Options

This module natively supports:

* Filestore as a shared file system between pods/nodes.
* Pod level ephemeral storage options:
  * memory backed emptyDir
  * local SSD backed emptyDir
  * SSD persistent disk backed ephemeral volume
  * balanced persistent disk backed ephemeral volume

See the [storage-gke.yaml blueprint](../../../examples/storage-gke.yaml) and the
associated [documentation](../../../../examples/README.md#storage-gkeyaml--) for
examples of how to use Filestore and ephemeral storage with this module.

### Requested Resources

When one or more `gke-node-pool` modules are referenced with the `use` field.
The requested resources will be populated to achieve a 1 pod per node packing
while still leaving some headroom for required system pods.

This functionality can be overridden by specifying the desired cpu requirement
using the `requested_cpu_per_pod` setting.

## License

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
| <a name="requirement_local"></a> [local](#requirement\_local) | >= 2.0.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_local"></a> [local](#provider\_local) | >= 2.0.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [local_file.job_template](https://registry.terraform.io/providers/hashicorp/local/latest/docs/resources/file) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_allocatable_cpu_per_node"></a> [allocatable\_cpu\_per\_node](#input\_allocatable\_cpu\_per\_node) | The allocatable cpu per node. Used to claim whole nodes. Generally populated from gke-node-pool via `use` field. | `list(number)` | <pre>[<br>  -1<br>]</pre> | no |
| <a name="input_backoff_limit"></a> [backoff\_limit](#input\_backoff\_limit) | Controls the number of retries before considering a Job as failed. Set to zero for shared fate. | `number` | `0` | no |
| <a name="input_command"></a> [command](#input\_command) | The command and arguments for the container that run in the Pod. The command field corresponds to entrypoint in some container runtimes. | `list(string)` | <pre>[<br>  "hostname"<br>]</pre> | no |
| <a name="input_completion_mode"></a> [completion\_mode](#input\_completion\_mode) | Sets value of `completionMode` on the job. Default uses indexed jobs. See [documentation](https://kubernetes.io/blog/2021/04/19/introducing-indexed-jobs/) for more information | `string` | `"Indexed"` | no |
| <a name="input_ephemeral_volumes"></a> [ephemeral\_volumes](#input\_ephemeral\_volumes) | Will create an emptyDir or ephemeral volume that is backed by the specified type: `memory`, `local-ssd`, `pd-balanced`, `pd-ssd`. `size_gb` is provided in GiB. | <pre>list(object({<br>    type       = string<br>    mount_path = string<br>    size_gb    = number<br>  }))</pre> | `[]` | no |
| <a name="input_has_gpu"></a> [has\_gpu](#input\_has\_gpu) | Indicates that the job should request nodes with GPUs. Typically supplied by a gke-node-pool module. | `list(bool)` | <pre>[<br>  false<br>]</pre> | no |
| <a name="input_image"></a> [image](#input\_image) | The container image the job should use. | `string` | `"debian"` | no |
| <a name="input_k8s_service_account_name"></a> [k8s\_service\_account\_name](#input\_k8s\_service\_account\_name) | Kubernetes service account to run the job as. If null then no service account is specified. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the GKE job template. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_machine_family"></a> [machine\_family](#input\_machine\_family) | The machine family to use in the node selector (example: `n2`). If null then machine family will not be used as selector criteria. | `string` | `null` | no |
| <a name="input_name"></a> [name](#input\_name) | The name of the job. | `string` | `"my-job"` | no |
| <a name="input_node_count"></a> [node\_count](#input\_node\_count) | How many nodes the job should run in parallel. | `number` | `1` | no |
| <a name="input_node_pool_name"></a> [node\_pool\_name](#input\_node\_pool\_name) | A list of node pool names on which to run the job. Can be populated via `use` feild. | `list(string)` | `[]` | no |
| <a name="input_node_selectors"></a> [node\_selectors](#input\_node\_selectors) | A list of node selectors to use to place the job. | <pre>list(object({<br>    key   = string<br>    value = string<br>  }))</pre> | `[]` | no |
| <a name="input_persistent_volume_claims"></a> [persistent\_volume\_claims](#input\_persistent\_volume\_claims) | A list of objects that describes a k8s PVC that is to be used and mounted on the job. Generally supplied by the gke-persistent-volume module. | <pre>list(object({<br>    name          = string<br>    mount_path    = string<br>    mount_options = string<br>    is_gcs        = bool<br>  }))</pre> | `[]` | no |
| <a name="input_random_name_sufix"></a> [random\_name\_sufix](#input\_random\_name\_sufix) | Appends a random suffix to the job name to avoid clashes. | `bool` | `true` | no |
| <a name="input_requested_cpu_per_pod"></a> [requested\_cpu\_per\_pod](#input\_requested\_cpu\_per\_pod) | The requested cpu per pod. If null, allocatable\_cpu\_per\_node will be used to claim whole nodes. If provided will override allocatable\_cpu\_per\_node. | `number` | `-1` | no |
| <a name="input_restart_policy"></a> [restart\_policy](#input\_restart\_policy) | Job restart policy. Only a RestartPolicy equal to `Never` or `OnFailure` is allowed. | `string` | `"Never"` | no |
| <a name="input_tolerations"></a> [tolerations](#input\_tolerations) | Tolerations allow the scheduler to schedule pods with matching taints. Generally populated from gke-node-pool via `use` field. | <pre>list(object({<br>    key      = string<br>    operator = string<br>    value    = string<br>    effect   = string<br>  }))</pre> | <pre>[<br>  {<br>    "effect": "NoSchedule",<br>    "key": "user-workload",<br>    "operator": "Equal",<br>    "value": "true"<br>  }<br>]</pre> | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions for submitting the GKE job. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
