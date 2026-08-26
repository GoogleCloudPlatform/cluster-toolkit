# Compact Placement on GKE

[Compact placement policies](https://cloud.google.com/kubernetes-engine/docs/how-to/compact-placement) place GKE node VMs physically close together within the same datacenter rack or network block. This minimizes network latency and maximizes throughput for multi-node distributed AI/ML training and NCCL collective communications.

When a compact placement policy is enabled, Compute Engine (GCE) populates physical host metadata (`resourceStatus.physicalHost`). GKE uses this metadata to automatically label nodes with physical topology labels:

* `cloud.google.com/gce-topology-block`
* `cloud.google.com/gce-topology-subblock`
* `cloud.google.com/gce-topology-host`

These labels are required for **Topology-Aware Scheduling (TAS)** in Kueue.

For more details on configuring placement policies in Cluster Toolkit, see [`placement_policy`](../modules/compute/gke-node-pool/README.md#input_placement_policy) in the `gke-node-pool` module documentation.

---

## Spot

On Spot node pools, Compute Engine provisions instances from spare capacity across the datacenter.

```yaml
# In your deployment file:
spot: true
static_node_count: 2
placement_policy:
  type: COMPACT
```

* **With Compact Placement**: GCE places Spot nodes in the same physical block and attaches `TopologyBlock` labels. Kueue TAS workloads admit and run with low latency.
* **Without Compact Placement**: GCE omits physical host metadata. Workloads managed by Kueue requesting Topology-Aware Scheduling (TAS) will be **suspended indefinitely** with the following error:

  ```text
  Warning Pending kueue-admission couldn't assign flavors to pod set w: no topology domains at level: kubernetes.io/hostname
  ```

* **Capacity Tip**: If you experience difficulty obtaining Spot capacity within a single compact block in a zone, you can remove or comment out `placement_policy`. Standard Kubernetes jobs and raw Pods (without Kueue TAS) will continue to run across scattered Spot capacity.

---

## On-Demand

Standard On-Demand node pools without compact placement are also provisioned across random racks without physical topology guarantees.

```yaml
# In your deployment file:
spot: false
static_node_count: 2
placement_policy:
  type: COMPACT
```

* **With Compact Placement**: GKE automatically creates and manages a compact placement group, populating topology labels for Kueue TAS workloads.
* **Without Compact Placement**: Nodes lack `TopologyBlock` labels, and Kueue TAS workloads will remain suspended. Remove `placement_policy` only if you encounter capacity or quota constraints and are running non-TAS workloads.

---

## Specific Reservations

If you consume a Compute Engine reservation that was created inside a compact placement policy, you must specify the existing policy name in `placement_policy`:

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

## Dynamic Workload Scheduler (DWS Flex Start)

When using Dynamic Workload Scheduler in Flex Start mode, compact placement is configured using a **Workload Policy** module rather than a standard group placement policy.

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
> Compact placement with DWS Flex Start is supported **only** for specific machine types:
>
> * **A3 Ultra** (`a3-ultragpu-8g`)
> * **A4** (`a4-highgpu-8g`, `a4-megagpu-8g`)
> * **H4D** (`h4d-highgpu-8g`)
