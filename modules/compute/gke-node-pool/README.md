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
    source: modules/compute/gke-node-pool
    use: [gke_cluster]
```

Also see a full [GKE example blueprint](../../../examples/hpc-gke.yaml).

### Taints and Tolerations

By default node pools created with this module will be tainted with
`user-workload=true:NoSchedule` to prevent system pods from being scheduled.
User jobs targeting the node pool should include this toleration. This behavior
can be overridden using the `taints` setting. See
[docs](https://cloud.google.com/kubernetes-engine/docs/how-to/node-taints) for
more info.

### Local SSD Storage
GKE offers two options for managing locally attached SSDs.

The first, and recommended, option is for GKE to manage the ephemeral storage
space on the node, which will then be automatically attached to pods which
request an `emptyDir` volume. This can be accomplished using the
[`local_ssd_count_ephemeral_storage`] variable.

The second, more complex, option is for GCP to attach these nodes as raw block
storage. In this case, the cluster administrator is responsible for software
RAID settings, partitioning, formatting and mounting these disks on the host
OS.  Still, this may be desired behavior in use cases which aren't supported
by an `emptyDir` volume (for example, a `ReadOnlyMany` or `ReadWriteMany` PV).
This can be accomplished using the [`local_ssd_count_nvme_block`] variable.

The [`local_ssd_count_ephemeral_storage`] and [`local_ssd_count_nvme_block`]
variables are mutually exclusive and cannot be mixed together.

Also, the number of SSDs which can be attached to a node depends on the
[machine type](https://cloud.google.com/compute/docs/disks#local_ssd_machine_type_restrictions).

See [docs](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/local-ssd)
for more info.

[`local_ssd_count_ephemeral_storage`]: #input\_local\_ssd\_count\_ephemeral\_storage
[`local_ssd_count_nvme_block`]: #input\_local\_ssd\_count\_nvme\_block

### Considerations with GPUs

When a GPU is attached to a node an additional taint is automatically added:
`nvidia.com/gpu=present:NoSchedule`. For jobs to get placed on these nodes, the
equivalent toleration is required. The `gke-job-template` module will
automatically apply this toleration when using a node pool with GPUs.

Nvidia GPU drivers must be installed.  The recommended approach for GKE to install
GPU dirvers is by applying a DaemonSet to the cluster. See
[these instructions](https://cloud.google.com/kubernetes-engine/docs/how-to/gpus#cos).

However, in some cases it may be desired to compile a different driver (such as
a desire to install a newer version, compatibility with the
[Nvidia GPU-operator](https://github.com/NVIDIA/gpu-operator) or other
use-cases). In this case, ensure that you turn off the
[enable_secure_boot](#input\_enable\_secure\_boot) option to allow unsigned
kernel modules to be loaded.

#### Maximize GPU network bandwidth with GPUDirect and multi-networking
For A3 Series machines to achieve optimal performance , GKE provide two networking stacks for remote direct memory access (RDMA):

- A3 High machine types (a3-highgpu-8g): utilize GPUDirect-TCPX to reduce the overhead required to transfer packet payloads to and from GPUs, which significantly improves throughput at scale compared to GPUs that don't use GPUDirect.
- A3 Mega machine types (a3-megagpu-8g): utilize GPUDirect-TCPXO to improve GPU to GPU communication, and further improves GPU to VM communication.

To achieve this, when creating nodepools with A3 Series machine type, pass in a multivpc module to the gke-node-pool module, and the gke-node-pool module would detect the eligible machine type and enable GPUDirect for it. More specifically, the below components will be installed in the nodepool for enabling GPUDirect.

- Install NCCL plugin for GPUDirect [TCPX](https://github.com/GoogleCloudPlatform/container-engine-accelerators/tree/master/gpudirect-tcpx) or [TCPXO](https://github.com/GoogleCloudPlatform/container-engine-accelerators/tree/master/gpudirect-tcpxo)
- Install [NRI](https://github.com/GoogleCloudPlatform/container-engine-accelerators/tree/master/nri_device_injector) device injector plugin
- Provide support for injecting GPUDirect required components(annotations, volumes, rxdm sidecar etc.) into the user workload in the form of Kubernetes Job.
  - Provide sample workload to showcase how it will be updated with the required components injected, and how it can be deployed.
  - Allow user to use the provided script to update their own workload and deploy.

The GPUDirect supports included in the Cluster Toolkit aim to automate the [GPUDirect User Guid](https://cloud.google.com/kubernetes-engine/docs/how-to/gpu-bandwidth-gpudirect-tcpx#install-gpudirect-tcpx-nccl) and provide better usability.

> **_NOTE:_** You must [enable multi networking](https://cloud.google.com/kubernetes-engine/docs/how-to/setup-multinetwork-support-for-pods#create-a-gke-cluster) feature when creating the GKE cluster. When gke-cluster depends on multivpc (with the use keyword), multi networking will be automatically enabled on the cluster creation.
> When gke-cluster or pre-existing-gke-cluster  depends on multivpc (with the use keyword), the [network objects](https://cloud.google.com/kubernetes-engine/docs/how-to/gpu-bandwidth-gpudirect-tcpx#create-gke-environment) required for multi networking will be created on the cluster.

### GPUs Examples

There are several ways to add GPUs to a GKE node pool. See
[docs](https://cloud.google.com/compute/docs/gpus) for more info on GPUs.

The following is a node pool that uses `a2`, `a3` or `g2` machine types which has a
fixed number of attached GPUs, let's call these machine types as "pre-defined gpu machine families":

```yaml
  - id: simple-a2-pool
    source: modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      machine_type: a2-highgpu-1g
```

> **Note**: It is not necessary to define the [`guest_accelerator`] setting when
> using pre-defined gpu machine families as information about GPUs, such as type, count and
> `gpu_driver_installation_config`, is automatically inferred from the machine type.
> Optional fields such as `gpu_partition_size` need to be specified only if they have
> non-default values.

The following scenarios require the [`guest_accelerator`] block is specified:

- To partition an A100 GPU into multiple GPUs on an A2 family machine.
- To specify a time sharing configuration on a GPUs.
- To attach a GPU to an N1 family machine.

The following is an example of
[partitioning](https://cloud.google.com/kubernetes-engine/docs/how-to/gpus-multi)
an A100 GPU:

> **Note**: In the following example, `type`, `count` and `gpu_driver_installation_config` are picked up automatically.

```yaml
  - id: multi-instance-gpu-pool
    source: modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      machine_type: a2-highgpu-1g
      guest_accelerator:
      - gpu_partition_size: 1g.5gb
```

[`guest_accelerator`]: https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_cluster#nested_guest_accelerator

The following is an example of
[GPU time sharing](https://cloud.google.com/kubernetes-engine/docs/concepts/timesharing-gpus)
(with partitioned GPUs):

```yaml
  - id: time-sharing-gpu-pool
    source: modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      machine_type: a2-highgpu-1g
      guest_accelerator:
      - gpu_partition_size: 1g.5gb
        gpu_sharing_config:
          gpu_sharing_strategy: TIME_SHARING
          max_shared_clients_per_gpu: 3
```

Following is an example of using a GPU attached to an `n1` machine:

```yaml
  - id: t4-pool
    source: modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      machine_type: n1-standard-16
      guest_accelerator:
      - type: nvidia-tesla-t4
        count: 2
```

The following is an example of using a GPU (with sharing config) attached to an `n1` machine:

```yaml
  - id: n1-t4-pool
    source: community/modules/compute/gke-node-pool
    use: [gke_cluster]
    settings:
      name: n1-t4-pool
      machine_type: n1-standard-1
      guest_accelerator:
      - type: nvidia-tesla-t4
        count: 2
        gpu_driver_installation_config:
          gpu_driver_version: "LATEST"
        gpu_sharing_config:
          max_shared_clients_per_gpu: 2
          gpu_sharing_strategy: "TIME_SHARING"
```

Finally, the following is adding multivpc to a node pool:

```yaml
  - id: network
    source: modules/network/vpc
    settings:
      subnetwork_name: gke-subnet
      secondary_ranges:
        gke-subnet:
        - range_name: pods
          ip_cidr_range: 10.4.0.0/14
        - range_name: services
          ip_cidr_range: 10.0.32.0/20

  - id: multinetwork
    source: modules/network/multivpc
    settings:
      network_name_prefix: multivpc-net
      network_count: 8
      global_ip_address_range: 172.16.0.0/12
      subnetwork_cidr_suffix: 16

  - id: gke-cluster
    source: modules/scheduler/gke-cluster
    use: [network, multinetwork]
    settings:
      cluster_name: $(vars.deployment_name)

  - id: a3-megagpu_pool
    source: modules/compute/gke-node-pool
    use: [gke-cluster, multinetwork]
    settings:
      machine_type: a3-megagpu-8g
      ...
```

## Using GCE Reservations
You can reserve Google Compute Engine instances in a specific zone to ensure resources are available for their workloads when needed. For more details on how to manage reservations, see [Reserving Compute Engine zonal resources](https://cloud.google.com/compute/docs/instances/reserving-zonal-resources).

After creating a reservation, you can consume the reserved GCE VM instances in GKE. GKE clusters deployed using Cluster Toolkit support the same consumption modes as Compute Engine: NO_RESERVATION(default), ANY_RESERVATION, SPECIFIC_RESERVATION.

This can be accomplished using [`reservation_affinity`](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/modules/compute/gke-node-pool/README.md#input_reservation_affinity).

```yaml
# Target any reservation
reservation_affinity:
  consume_reservation_type: ANY_RESERVATION

# Target a specific reservation
reservation_affinity:
  consume_reservation_type: SPECIFIC_RESERVATION
  specific_reservations:
  - name: specific-reservation-1
```

The following requirements need to be satisfied for the node pool nodes to be able to use a specific reservation:
1. A reservation with the name must exist in the specified project(`var.project_id`) and one of the specified zones(`var.zones`).
2. Its consumption type must be `specific`.
3. Its GCE VM Properties must match with those of the Node Pool; Machine type, Accelerators (GPU Type and count), Local SSD disk type and count.

If you want to utilise a shared reservation, the owner project of the shared reservation needs to be explicitly specified like the following. Note that a shared reservation can be used by the project that hosts the reservation (owner project) and by the projects the reservation is shared with (consumer projects). See how to [create and use a shared reservation](https://cloud.google.com/compute/docs/instances/reservations-shared).

```yaml
reservation_affinity:
  consume_reservation_type: SPECIFIC_RESERVATION
  specific_reservations:
  - name: specific-reservation-shared
    project: shared_reservation_owner_project_id
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.5 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.2 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 7.2 |
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.2 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 7.2 |
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_gpu"></a> [gpu](#module\_gpu) | ../../internal/gpu-definition | n/a |
| <a name="module_kubectl_apply"></a> [kubectl\_apply](#module\_kubectl\_apply) | ../../management/kubectl-apply | n/a |
| <a name="module_tpu"></a> [tpu](#module\_tpu) | ../../internal/tpu-definition | n/a |

## Resources

| Name | Type |
|------|------|
| [google-beta_google_container_node_pool.node_pool](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_container_node_pool) | resource |
| [null_resource.enable_tcpx_in_workload](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [null_resource.enable_tcpxo_in_workload](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [null_resource.install_dependencies](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [google_compute_machine_types.machine_info](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_machine_types) | data source |
| [google_compute_region_instance_template.instance_template](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_region_instance_template) | data source |
| [google_compute_reservation.specific_reservations](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_reservation) | data source |
| [google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_additional_networks"></a> [additional\_networks](#input\_additional\_networks) | Additional network interface details for GKE, if any. Providing additional networks adds additional node networks to the node pool | <pre>list(object({<br/>    network            = string<br/>    subnetwork         = string<br/>    subnetwork_project = string<br/>    network_ip         = string<br/>    nic_type           = string<br/>    stack_type         = string<br/>    queue_count        = number<br/>    access_config = list(object({<br/>      nat_ip       = string<br/>      network_tier = string<br/>    }))<br/>    ipv6_access_config = list(object({<br/>      network_tier = string<br/>    }))<br/>    alias_ip_range = list(object({<br/>      ip_cidr_range         = string<br/>      subnetwork_range_name = string<br/>    }))<br/>  }))</pre> | `[]` | no |
| <a name="input_auto_repair"></a> [auto\_repair](#input\_auto\_repair) | Whether the nodes will be automatically repaired. | `bool` | `true` | no |
| <a name="input_auto_upgrade"></a> [auto\_upgrade](#input\_auto\_upgrade) | Whether the nodes will be automatically upgraded. | `bool` | `false` | no |
| <a name="input_autoscaling_total_max_nodes"></a> [autoscaling\_total\_max\_nodes](#input\_autoscaling\_total\_max\_nodes) | Total maximum number of nodes in the NodePool. | `number` | `1000` | no |
| <a name="input_autoscaling_total_min_nodes"></a> [autoscaling\_total\_min\_nodes](#input\_autoscaling\_total\_min\_nodes) | Total minimum number of nodes in the NodePool. | `number` | `0` | no |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | projects/{{project}}/locations/{{location}}/clusters/{{cluster}} | `string` | n/a | yes |
| <a name="input_compact_placement"></a> [compact\_placement](#input\_compact\_placement) | DEPRECATED: Use `placement_policy` | `bool` | `null` | no |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of disk for each node. | `number` | `100` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Disk type for each node. | `string` | `null` | no |
| <a name="input_enable_flex_start"></a> [enable\_flex\_start](#input\_enable\_flex\_start) | If true, start the node pool with Flex Start provisioning model.<br/>To learn more about flex-start mode, please refer to<br/>https://cloud.google.com/kubernetes-engine/docs/how-to/dws-flex-start-training and<br/>https://cloud.google.com/kubernetes-engine/docs/how-to/provisioningrequest | `bool` | `false` | no |
| <a name="input_enable_gcfs"></a> [enable\_gcfs](#input\_enable\_gcfs) | Enable the Google Container Filesystem (GCFS). See [restrictions](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_cluster#gcfs_config). | `bool` | `false` | no |
| <a name="input_enable_numa_aware_scheduling"></a> [enable\_numa\_aware\_scheduling](#input\_enable\_numa\_aware\_scheduling) | Enable [NUMA-aware](https://cloud.google.com/kubernetes-engine/distributed-cloud/bare-metal/docs/vm-runtime/numa) scheduling. | `bool` | `false` | no |
| <a name="input_enable_private_nodes"></a> [enable\_private\_nodes](#input\_enable\_private\_nodes) | Whether nodes have internal IP addresses only. | `bool` | `true` | no |
| <a name="input_enable_queued_provisioning"></a> [enable\_queued\_provisioning](#input\_enable\_queued\_provisioning) | If true, enables Dynamic Workload Scheduler and adds the cloud.google.com/gke-queued taint to the node pool. | `bool` | `false` | no |
| <a name="input_enable_secure_boot"></a> [enable\_secure\_boot](#input\_enable\_secure\_boot) | Enable secure boot for the nodes.  Keep enabled unless custom kernel modules need to be loaded. See [here](https://cloud.google.com/compute/shielded-vm/docs/shielded-vm#secure-boot) for more info. | `bool` | `true` | no |
| <a name="input_gke_version"></a> [gke\_version](#input\_gke\_version) | GKE version | `string` | n/a | yes |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br/>    type  = optional(string)<br/>    count = optional(number, 0)<br/>    gpu_driver_installation_config = optional(object({<br/>      gpu_driver_version = string<br/>    }), { gpu_driver_version = "DEFAULT" })<br/>    gpu_partition_size = optional(string)<br/>    gpu_sharing_config = optional(object({<br/>      gpu_sharing_strategy       = string<br/>      max_shared_clients_per_gpu = number<br/>    }))<br/>  }))</pre> | `[]` | no |
| <a name="input_host_maintenance_interval"></a> [host\_maintenance\_interval](#input\_host\_maintenance\_interval) | Specifies the frequency of planned maintenance events. | `string` | `""` | no |
| <a name="input_image_type"></a> [image\_type](#input\_image\_type) | The default image type used by NAP once a new node pool is being created. Use either COS\_CONTAINERD or UBUNTU\_CONTAINERD. | `string` | `"COS_CONTAINERD"` | no |
| <a name="input_initial_node_count"></a> [initial\_node\_count](#input\_initial\_node\_count) | The initial number of nodes for the pool. In regional clusters, this is the number of nodes per zone. Changing this setting after node pool creation will not make any effect. It cannot be set with static\_node\_count and must be set to a value between autoscaling\_total\_min\_nodes and autoscaling\_total\_max\_nodes. | `number` | `null` | no |
| <a name="input_internal_ghpc_module_id"></a> [internal\_ghpc\_module\_id](#input\_internal\_ghpc\_module\_id) | DO NOT SET THIS MANUALLY. Automatically populates with module id (unique blueprint-wide). | `string` | n/a | yes |
| <a name="input_is_reservation_active"></a> [is\_reservation\_active](#input\_is\_reservation\_active) | Whether the specified reservation is already created. | `bool` | `true` | no |
| <a name="input_kubernetes_labels"></a> [kubernetes\_labels](#input\_kubernetes\_labels) | Kubernetes labels to be applied to each node in the node group. Key-value pairs. <br/>(The `kubernetes.io/` and `k8s.io/` prefixes are reserved by Kubernetes Core components and cannot be specified) | `map(string)` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | GCE resource labels to be applied to resources. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_local_ssd_count_ephemeral_storage"></a> [local\_ssd\_count\_ephemeral\_storage](#input\_local\_ssd\_count\_ephemeral\_storage) | The number of local SSDs to attach to each node to back ephemeral storage.<br/>Uses NVMe interfaces.  Must be supported by `machine_type`.<br/>When set to null,  default value either is [set based on machine\_type](https://cloud.google.com/compute/docs/disks/local-ssd#choose_number_local_ssds) or GKE decides about default value.<br/>[See above](#local-ssd-storage) for more info. | `number` | `null` | no |
| <a name="input_local_ssd_count_nvme_block"></a> [local\_ssd\_count\_nvme\_block](#input\_local\_ssd\_count\_nvme\_block) | The number of local SSDs to attach to each node to back block storage.<br/>Uses NVMe interfaces.  Must be supported by `machine_type`.<br/>When set to null,  default value either is [set based on machine\_type](https://cloud.google.com/compute/docs/disks/local-ssd#choose_number_local_ssds) or GKE decides about default value.<br/>[See above](#local-ssd-storage) for more info. | `number` | `null` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | The name of a Google Compute Engine machine type. | `string` | `"c2-standard-60"` | no |
| <a name="input_max_pods_per_node"></a> [max\_pods\_per\_node](#input\_max\_pods\_per\_node) | The maximum number of pods per node in this node pool. This will force replacement. | `number` | `null` | no |
| <a name="input_max_run_duration"></a> [max\_run\_duration](#input\_max\_run\_duration) | The duration (in whole seconds) of the instance. Instance will run and be terminated after then. | `number` | `null` | no |
| <a name="input_name"></a> [name](#input\_name) | The name of the node pool. If not set, automatically populated by machine type and module id (unique blueprint-wide) as suffix.<br/>If setting manually, ensure a unique value across all gke-node-pools. | `string` | `null` | no |
| <a name="input_num_node_pools"></a> [num\_node\_pools](#input\_num\_node\_pools) | Number of node pools to create. This is same as num\_slices. | `number` | `1` | no |
| <a name="input_num_slices"></a> [num\_slices](#input\_num\_slices) | Number of TPUs slices to create. This is same as num\_node\_pools. | `number` | `1` | no |
| <a name="input_placement_policy"></a> [placement\_policy](#input\_placement\_policy) | Group placement policy to use for the node pool's nodes. `COMPACT` is the only supported value for `type` currently. `name` is the name of the placement policy. `tpu_topology` is the TPU placement topology for pod slice node pool.<br/>It is assumed that the specified policy exists. To create a placement policy refer to https://cloud.google.com/sdk/gcloud/reference/compute/resource-policies/create/group-placement.<br/>Note: Placement policies have the [following](https://cloud.google.com/compute/docs/instances/placement-policies-overview#restrictions-compact-policies) restrictions. | <pre>object({<br/>    type         = string<br/>    name         = optional(string)<br/>    tpu_topology = optional(string)<br/>  })</pre> | <pre>{<br/>  "name": null,<br/>  "tpu_topology": null,<br/>  "type": null<br/>}</pre> | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID to host the cluster in. | `string` | n/a | yes |
| <a name="input_reservation_affinity"></a> [reservation\_affinity](#input\_reservation\_affinity) | Reservation resource to consume. When targeting SPECIFIC\_RESERVATION, specific\_reservations needs be specified.<br/>Even though specific\_reservations is a list, only one reservation is allowed by the NodePool API.<br/>It is assumed that the specified reservation exists and has available capacity.<br/>For a shared reservation, specify the project\_id as well in which it was created.<br/>To create a reservation refer to https://cloud.google.com/compute/docs/instances/reservations-single-project and https://cloud.google.com/compute/docs/instances/reservations-shared | <pre>object({<br/>    consume_reservation_type = string<br/>    specific_reservations = optional(list(object({<br/>      name    = string<br/>      project = optional(string)<br/>    })))<br/>  })</pre> | <pre>{<br/>  "consume_reservation_type": "NO_RESERVATION",<br/>  "specific_reservations": []<br/>}</pre> | no |
| <a name="input_run_workload_script"></a> [run\_workload\_script](#input\_run\_workload\_script) | Whether execute the script to create a sample workload and inject rxdm sidecar into workload. Currently, implemented for A3-Highgpu and A3-Megagpu only. | `bool` | `true` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | DEPRECATED: use service\_account\_email and scopes. | <pre>object({<br/>    email  = string,<br/>    scopes = set(string)<br/>  })</pre> | `null` | no |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | Service account e-mail address to use with the node pool | `string` | `null` | no |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Scopes to to use with the node pool. | `set(string)` | <pre>[<br/>  "https://www.googleapis.com/auth/cloud-platform"<br/>]</pre> | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision VMs using discounted Spot pricing, allowing for preemption | `bool` | `false` | no |
| <a name="input_static_node_count"></a> [static\_node\_count](#input\_static\_node\_count) | The static number of nodes in the node pool. If set, autoscaling will be disabled. | `number` | `null` | no |
| <a name="input_taints"></a> [taints](#input\_taints) | Taints to be applied to the system node pool. | <pre>list(object({<br/>    key    = string<br/>    value  = any<br/>    effect = string<br/>  }))</pre> | `[]` | no |
| <a name="input_threads_per_core"></a> [threads\_per\_core](#input\_threads\_per\_core) | Sets the number of threads per physical core. By setting threads\_per\_core<br/>to 2, Simultaneous Multithreading (SMT) is enabled extending the total number<br/>of virtual cores. For example, a machine of type c2-standard-60 will have 60<br/>virtual cores with threads\_per\_core equal to 2. With threads\_per\_core equal<br/>to 1 (SMT turned off), only the 30 physical cores will be available on the VM.<br/><br/>The default value of \"0\" will turn off SMT for supported machine types, and<br/>will fall back to GCE defaults for unsupported machine types (t2d, shared-core<br/>instances, or instances with less than 2 vCPU).<br/><br/>Disabling SMT can be more performant in many HPC workloads, therefore it is<br/>disabled by default where compatible.<br/><br/>null = SMT configuration will use the GCE defaults for the machine type<br/>0 = SMT will be disabled where compatible (default)<br/>1 = SMT will always be disabled (will fail on incompatible machine types)<br/>2 = SMT will always be enabled (will fail on incompatible machine types) | `number` | `0` | no |
| <a name="input_timeout_create"></a> [timeout\_create](#input\_timeout\_create) | Timeout for creating a node pool | `string` | `null` | no |
| <a name="input_timeout_update"></a> [timeout\_update](#input\_timeout\_update) | Timeout for updating a node pool | `string` | `null` | no |
| <a name="input_total_max_nodes"></a> [total\_max\_nodes](#input\_total\_max\_nodes) | DEPRECATED: Use autoscaling\_total\_max\_nodes. | `number` | `null` | no |
| <a name="input_total_min_nodes"></a> [total\_min\_nodes](#input\_total\_min\_nodes) | DEPRECATED: Use autoscaling\_total\_min\_nodes. | `number` | `null` | no |
| <a name="input_upgrade_settings"></a> [upgrade\_settings](#input\_upgrade\_settings) | Defines node pool upgrade settings. It is highly recommended that you define all max\_surge and max\_unavailable.<br/>If max\_surge is not specified, it would be set to a default value of 0.<br/>If max\_unavailable is not specified, it would be set to a default value of 1. | <pre>object({<br/>    strategy        = string<br/>    max_surge       = optional(number)<br/>    max_unavailable = optional(number)<br/>  })</pre> | <pre>{<br/>  "max_surge": 0,<br/>  "max_unavailable": 1,<br/>  "strategy": "SURGE"<br/>}</pre> | no |
| <a name="input_zones"></a> [zones](#input\_zones) | A list of zones to be used. Zones must be in region of cluster. If null, cluster zones will be inherited. Note `zones` not `zone`; does not work with `zone` deployment variable. | `list(string)` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_allocatable_cpu_per_node"></a> [allocatable\_cpu\_per\_node](#output\_allocatable\_cpu\_per\_node) | Number of CPUs available for scheduling pods on each node. |
| <a name="output_allocatable_gpu_per_node"></a> [allocatable\_gpu\_per\_node](#output\_allocatable\_gpu\_per\_node) | Number of GPUs available for scheduling pods on each node. |
| <a name="output_cluster_id"></a> [cluster\_id](#output\_cluster\_id) | An identifier for the gke cluster with format projects/{{project\_id}}/locations/{{region}}/clusters/{{name}}. |
| <a name="output_guest_accelerator"></a> [guest\_accelerator](#output\_guest\_accelerator) | The accelerator type of the nodes. |
| <a name="output_has_gpu"></a> [has\_gpu](#output\_has\_gpu) | Boolean value indicating whether nodes in the pool are configured with GPUs. |
| <a name="output_instance_templates"></a> [instance\_templates](#output\_instance\_templates) | The URLs of Instance Templates |
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions for submitting the sample GPUDirect enabled job. |
| <a name="output_machine_type"></a> [machine\_type](#output\_machine\_type) | Machine Type |
| <a name="output_node_count_static"></a> [node\_count\_static](#output\_node\_count\_static) | The number of static nodes in node-pool. |
| <a name="output_node_pool_names"></a> [node\_pool\_names](#output\_node\_pool\_names) | Names of the node pools. |
| <a name="output_static_gpu_count"></a> [static\_gpu\_count](#output\_static\_gpu\_count) | Total number of GPUs in the node pool. Available only for static node pools. |
| <a name="output_tolerations"></a> [tolerations](#output\_tolerations) | Tolerations needed for a pod to be scheduled on this node pool. |
| <a name="output_tpu_accelerator_type"></a> [tpu\_accelerator\_type](#output\_tpu\_accelerator\_type) | The label value for the TPU accelerator type (e.g., 'tpu-v6e-slice'). |
| <a name="output_tpu_chips_per_node"></a> [tpu\_chips\_per\_node](#output\_tpu\_chips\_per\_node) | The number of TPU chips on each node in the pool. |
| <a name="output_tpu_topology"></a> [tpu\_topology](#output\_tpu\_topology) | The topology of the TPU slice (e.g., '4x4'). |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
