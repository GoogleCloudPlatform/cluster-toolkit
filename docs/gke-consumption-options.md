# GKE Consumption Options User Guide

Cluster Toolkit provides unified configuration for deploying GKE clusters across multiple Compute Engine consumption models. This guide outlines variable requirements, compact placement behaviors, and machine family constraints across all supported models.

For deep-dive documentation on underlying Google Cloud topology, scheduling, and quota mechanics, see:

* [AI Hypercomputer GKE consumption options](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#use-cluster-toolkit)
* [Dynamic Workload Scheduler (DWS)](https://cloud.google.com/kubernetes-engine/docs/concepts/dws)
* [Topology-Aware Scheduling (TAS) in GKE](https://cloud.google.com/kubernetes-engine/docs/concepts/topology-aware-scheduling)
* [Use compact placement policies in Compute Engine](https://cloud.google.com/compute/docs/instances/use-compact-placement-policies)
* [View GKE node topology](https://cloud.google.com/ai-hypercomputer/docs/manage/node-topology)
* [View compute instance topology](https://cloud.google.com/ai-hypercomputer/docs/manage/instance-topology)

For module-level variable definitions, refer to [`placement_policy`](../modules/compute/gke-node-pool/README.md#input_placement_policy) in the `gke-node-pool` module documentation.

---

## DWS Flex Start

Dynamic Workload Scheduler (DWS) in Flex Start mode schedules required accelerators concurrently once capacity becomes available.

### Configuration
* Uncomment `enable_flex_start: true` in your deployment file.
* DWS Flex Start does not work with static nodes; `static_node_count` cannot be set.
* Requires `auto_repair: false` (handled automatically via the blueprint ternary expression).

### Compact Placement
To enable compact placement with DWS Flex Start, add the `resource-policy` module to your blueprint under `deployment_groups.primary.modules`:

```yaml
  - id: workload_policy
    source: modules/compute/resource-policy
    settings:
      name: "workload-policy"
      project_id: $(vars.project_id)
      region: $(vars.region)
      workload_policy:
        type: "HIGH_THROUGHPUT"

  - id: gpu_node_pool
    source: modules/compute/gke-node-pool
    use: [gke_cluster, workload_policy]
    settings:
      enable_flex_start: true
```

> [!WARNING]
> Compact placement with DWS Flex Start is supported **only** on **A3 Ultra** (`a3-ultragpu-8g`), **A4** (`a4-highgpu-8g`, `a4-megagpu-8g`), and **H4D** (`h4d-highgpu-8g`). It is **not supported** on A3 Mega or A3 High.

---

## DWS Flex Start + Queued Provisioning

Combines DWS Flex Start with Kueue-driven queued provisioning to dynamically create nodes upon job submission.

### Configuration
* Uncomment `enable_flex_start: true` and `enable_queued_provisioning: true` in your deployment file.
* Queued provisioning does not work with `static_node_count`; requires `autoscaling_total_min_nodes: 0`.
* Requires `kueue_configuration_path: $(ghpc_stage("../dws-sample-workloads/dws-queues.yaml.tftpl"))`.
* Workloads must target `kueue.x-k8s.io/queue-name: dws-local-queue` with nodeSelector `cloud.google.com/gke-flex-start: "true"` and toleration for `cloud.google.com/gke-queued:NoSchedule`.

### Compact Placement
* Compact placement policies and workload policies are **not supported** for GPU node pools when `enable_queued_provisioning` is enabled (`placement_policy` cannot be `COMPACT` in `gke-node-pool`). Dynamic node allocation is governed asynchronously by Kueue via `ProvisioningRequest`.

---

## Spot

Provisions instances from spare Compute Engine capacity across the zone.

### Configuration
* Requires `spot: true` and defining `static_node_count`.

### Compact Placement
For multi-node workloads utilizing Kueue Topology-Aware Scheduling (TAS), compact placement is required on A3 Mega, A3 High, and general GPU VMs to expose `cloud.google.com/gce-topology-block` labels:

```yaml
# In your deployment file:
spot: true
static_node_count: 2
placement_policy:
  type: COMPACT
```

* **With Compact Placement**: Attaches physical topology labels so Kueue TAS workloads can admit and schedule.
* **Without Compact Placement**: GCE omits topology metadata, causing Kueue TAS workloads to remain suspended.
* **Capacity Fallback**: If Spot capacity is constrained in a single physical block within a zone, remove or comment out `placement_policy`. Standard Kubernetes jobs and raw Pods (without Kueue TAS) will continue to run across scattered Spot capacity.

---

## Specific Reservations

Targets pre-purchased or allocated Compute Engine reservations.

### Configuration
* Requires setting `reservation_affinity` with `consume_reservation_type: SPECIFIC_RESERVATION` and specifying reservation names.
* Requires defining `static_node_count`.

### Compact Placement
If the reservation was created within an existing compact placement policy, pass the policy name:

```yaml
# In your deployment file:
reservation_affinity:
  consume_reservation_type: SPECIFIC_RESERVATION
  specific_reservations:
  - name: my-reservation
static_node_count: 2
placement_policy:
  type: COMPACT
  name: my-existing-placement-policy
```

---

## On-Demand (Default)

Standard on-demand Compute Engine allocation.

### Configuration
* Default model. Requires defining `static_node_count` (`spot` defaults to `false`).

### Compact Placement

```yaml
# In your deployment file:
spot: false
static_node_count: 2
placement_policy:
  type: COMPACT
```

* **A3 Mega / A3 High**: Explicitly configuring `placement_policy: { type: COMPACT }` is required to attach physical topology labels for Kueue TAS.
* **A4 / A3 Ultra**: Topology labels and compact placement are applied automatically by default for On-Demand nodes.
