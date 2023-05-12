## Description

This module creates a Google Kubernetes Engine
([GKE](https://cloud.google.com/kubernetes-engine)) node pool.

> **_NOTE:_** This is an experimental module and the functionality and
> documentation will likely be updated in the near future. This module has only
> been tested in limited capacity.

### Example

The following example creates a GKE node group.

```yaml
  - id: compute_pool
    source: community/modules/compute/gke-node-pool
    use: [gke_cluster]
```

Also see a full [GKE example blueprint](../../../examples/gke.yaml).

### Taints and Tolerations

By default node pools created with this module will be tainted with
`user-workload=true:NoSchedule` to prevent system pods from being scheduled.
User jobs targeting the node pool should include this toleration. This behavior
can be overridden using the `taints` setting. See
[docs](https://cloud.google.com/kubernetes-engine/docs/how-to/node-taints) for
more info.

### Considerations with GPUs

When a GPU is attached to a node an additional taint is automatically added:
`nvidia.com/gpu=present:NoSchedule`. For jobs to get placed on these nodes, the
equivalent toleration is required. The `gke-job-template` module will
automatically apply this toleration when using a node pool with GPUs.

Nvidia GPU drivers must be installed by applying a DaemonSet to the cluster. See
[these instructions](https://cloud.google.com/kubernetes-engine/docs/how-to/gpus#cos).

### GPUs Examples

There are several ways to add GPUs to a GKE node pool. See
[docs](https://cloud.google.com/compute/docs/gpus) for more info on GPUs.

The following is a node pool that uses `a2` or `g2` machine types which has a
fixed number of attached GPUs:

```yaml
  - id: simple-a2-pool
    source: community/modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      machine_type: a2-highgpu-1g
```

> **Note**: It is not necessary to define the [`guest_accelerator`] setting when
> using `a2` or `g2` machines as information about GPUs, such as type and count,
> is automatically inferred from the machine type.

The following scenarios require the [`guest_accelerator`] block is specified:

- To partition an A100 GPU into multiple GPUs on an A2 family machine.
- To specify a time sharing configuration on a GPUs.
- To attach a GPU to an N1 family machine.

The following is an example of
[partitioning](https://cloud.google.com/kubernetes-engine/docs/how-to/gpus-multi)
an A100 GPU:

```yaml
  - id: multi-instance-gpu-pool
    source: community/modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      machine_type: a2-highgpu-1g
      guest_accelerator:
      - type: nvidia-tesla-a100
        count: 1
        gpu_partition_size: 1g.5gb
        gpu_sharing_config: null
```

> **Note**: Once we define the [`guest_accelerator`] block, all fields must be
> defined. Use `null` for optional fields.

[`guest_accelerator`]: https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_cluster#nested_guest_accelerator

The following is an example of
[GPU time sharing](https://cloud.google.com/kubernetes-engine/docs/concepts/timesharing-gpus)
(with partitioned GPUs):

```yaml
  - id: time-sharing-gpu-pool
    source: community/modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      machine_type: a2-highgpu-1g
      guest_accelerator:
      - type: nvidia-tesla-a100
        count: 1
        gpu_partition_size: 1g.5gb
        gpu_sharing_config:
        - gpu_sharing_strategy: TIME_SHARING
          max_shared_clients_per_gpu: 3
```

Finally, the following is an example of using a GPU attached to an `n1` machine:

```yaml
  - id: t4-pool
    source: community/modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      machine_type: n1-standard-16
      guest_accelerator:
      - type: nvidia-tesla-t4
        count: 2
        gpu_partition_size: null
        gpu_sharing_config: null
```

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0 |
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
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of disk for each node. | `number` | `100` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Disk type for each node. | `string` | `"pd-standard"` | no |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br>    type               = string<br>    count              = number<br>    gpu_partition_size = string<br>    gpu_sharing_config = list(object({<br>      gpu_sharing_strategy       = string<br>      max_shared_clients_per_gpu = number<br>    }))<br>  }))</pre> | `null` | no |
| <a name="input_image_type"></a> [image\_type](#input\_image\_type) | The default image type used by NAP once a new node pool is being created. Use either COS\_CONTAINERD or UBUNTU\_CONTAINERD. | `string` | `"COS_CONTAINERD"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | GCE resource labels to be applied to resources. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | The name of a Google Compute Engine machine type. | `string` | `"c2-standard-60"` | no |
| <a name="input_name"></a> [name](#input\_name) | The name of the node pool. If left blank, will default to the machine type. | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID to host the cluster in. | `string` | n/a | yes |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to use with the system node pool | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | <pre>{<br>  "email": null,<br>  "scopes": [<br>    "https://www.googleapis.com/auth/cloud-platform"<br>  ]<br>}</pre> | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision VMs using discounted Spot pricing, allowing for preemption | `bool` | `false` | no |
| <a name="input_taints"></a> [taints](#input\_taints) | Taints to be applied to the system node pool. | <pre>list(object({<br>    key    = string<br>    value  = any<br>    effect = string<br>  }))</pre> | <pre>[<br>  {<br>    "effect": "NO_SCHEDULE",<br>    "key": "user-workload",<br>    "value": true<br>  }<br>]</pre> | no |
| <a name="input_threads_per_core"></a> [threads\_per\_core](#input\_threads\_per\_core) | Sets the number of threads per physical core. By setting threads\_per\_core<br>to 2, Simultaneous Multithreading (SMT) is enabled extending the total number<br>of virtual cores. For example, a machine of type c2-standard-60 will have 60<br>virtual cores with threads\_per\_core equal to 2. With threads\_per\_core equal<br>to 1 (SMT turned off), only the 30 physical cores will be available on the VM.<br><br>The default value of \"0\" will turn off SMT for supported machine types, and<br>will fall back to GCE defaults for unsupported machine types (t2d, shared-core<br>instances, or instances with less than 2 vCPU).<br><br>Disabling SMT can be more performant in many HPC workloads, therefore it is<br>disabled by default where compatible.<br><br>null = SMT configuration will use the GCE defaults for the machine type<br>0 = SMT will be disabled where compatible (default)<br>1 = SMT will always be disabled (will fail on incompatible machine types)<br>2 = SMT will always be enabled (will fail on incompatible machine types) | `number` | `0` | no |
| <a name="input_total_max_nodes"></a> [total\_max\_nodes](#input\_total\_max\_nodes) | Total maximum number of nodes in the NodePool. | `number` | `1000` | no |
| <a name="input_total_min_nodes"></a> [total\_min\_nodes](#input\_total\_min\_nodes) | Total minimum number of nodes in the NodePool. | `number` | `0` | no |
| <a name="input_zones"></a> [zones](#input\_zones) | A list of zones to be used. Zones must be in region of cluster. If null, cluster zones will be inherited. Note `zones` not `zone`; does not work with `zone` deployment variable. | `list(string)` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_allocatable_cpu_per_node"></a> [allocatable\_cpu\_per\_node](#output\_allocatable\_cpu\_per\_node) | Number of CPUs available for scheduling pods on each node. |
| <a name="output_has_gpu"></a> [has\_gpu](#output\_has\_gpu) | Boolean value indicating whether nodes in the pool are configured with GPUs. |
| <a name="output_node_pool_name"></a> [node\_pool\_name](#output\_node\_pool\_name) | Name of the node pool. |
| <a name="output_tolerations"></a> [tolerations](#output\_tolerations) | Tolerations needed for a pod to be scheduled on this node pool. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
