# GKE Consumption Options User Guide

Cluster Toolkit provides unified configuration for deploying GKE clusters across multiple Compute Engine consumption models. This guide outlines variable requirements, compact placement behaviors, and machine family constraints across all supported models.

For deep-dive documentation on underlying Google Cloud topology, scheduling, and quota mechanics, see:

* [AI Hypercomputer consumption models](https://docs.cloud.google.com/ai-hypercomputer/docs/consumption-models)
* [Use compact placement policies in Compute Engine](https://cloud.google.com/compute/docs/instances/use-compact-placement-policies)

For module-level variable definitions, refer to [`placement_policy`](../modules/compute/gke-node-pool/README.md#input_placement_policy) in the `gke-node-pool` module documentation.

---

<a id="dws-flex-start"></a>
## Consumption of DWS Flex Start

Dynamic Workload Scheduler (DWS) in Flex Start mode schedules required accelerators concurrently once capacity becomes available.

### Configuration

* Requires `enable_flex_start: true` in your deployment file.
* DWS Flex Start does not work with static nodes; `static_node_count` cannot be set.
* Requires `auto_repair: false` (handled automatically via the blueprint ternary expression).

### Compact Placement

Workload Policy definition: The blueprint defines a custom GCE resource policy using the `workload_policy` module:

```yaml
  - id: workload_policy
    source: modules/compute/resource-policy
    settings:
      name: "workload-policy"
      project_id: $(vars.project_id)
      region: $(vars.region)
      workload_policy:
        type: "HIGH_THROUGHPUT"
        # Optional: physical boundary constraint for compaction.
        # Supported values: "SUBBLOCK", "BLOCK", or "CLUSTER" (default)
        max_topology_distance: "CLUSTER"

  - id: gpu_node_pool
    source: modules/compute/gke-node-pool
    use: [gke_cluster, workload_policy]
    settings:
      enable_flex_start: true
```

**Mapping to Node Pool**: The policy is mapped directly to the GKE node pool Managed Instance Group (MIG). This binds the physical `HIGH_THROUGHPUT` constraint to the pool along with the physical boundary limit (`max_topology_distance`) so that when the GKE Autoscaler requests nodes, they are guaranteed to sit in a physically collocated cluster matching the selected topology constraint.

Compact placement using custom GCE workload policies (like `HIGH_THROUGHPUT` topology constraints) with DWS Flex Start is currently supported for **A4**, **A3 Ultra**, and **H4D** machine types. It is not supported on A3 Mega or A3 High.

TPUs are natively scheduled as compact physical slices via the `tpu_topology` parameter. For TPUs (including Flex Start), users must specify `tpu_topology`, which is mandatory for TPU slice scheduling.

#### Verification via Google Cloud Console

To verify the workload policy configuration using the Google Cloud Console UI:

1. In the Google Cloud Console, navigate to the **Kubernetes Engine > Clusters** page.
2. Click on the name of your GKE cluster.
3. Select the **Nodes** tab.
4. Scroll down to the node pool details and click on the link to the Managed Instance Group (MIG) under the **Instance groups** column.
5. On the Managed Instance Group page, select the **Details** tab.
6. Scroll down to locate the **Workload policy** section.
7. Verify that the attached policy configuration shows the correct type and topology distance.

---

<a id="dws-flex-start--queued-provisioning"></a>
## Consumption of DWS Flex Start + Queued Provisioning

Combines DWS Flex Start with Kueue-driven queued provisioning to dynamically create nodes upon job submission.

### Configuration

* Requires `enable_flex_start: true` and `enable_queued_provisioning: true` in your deployment file.
* Queued provisioning does not work with `static_node_count`; requires `autoscaling_total_min_nodes: 0`.
* Workloads must target `kueue.x-k8s.io/queue-name: dws-local-queue` with nodeSelector `cloud.google.com/gke-flex-start: "true"` and toleration for `cloud.google.com/gke-queued:NoSchedule`.

---

<a id="spot"></a>
## Consumption of Spot

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

<a id="specific-reservations"></a>
## Consumption of Specific Reservations

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

<a id="on-demand"></a>
## Consumption of On-Demand

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

* **Capacity Fallback**: If capacity is constrained in a single physical block within a zone, remove or comment out `placement_policy`. Standard Kubernetes jobs and raw Pods (without Kueue TAS) will continue to run across scattered capacity.
