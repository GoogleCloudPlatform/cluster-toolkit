# GKE Multi-NIC Accelerator Node Pools: Configuring Policy Routing & Multi-Network Support

This document describes how to configure GKE managed multi-network (multi-NIC) policy routing for high-performance GPU and TPU node pools (such as multi-slice TPU or multi-node GPU clusters) deployed using the Cluster Toolkit.

---

## 1. Multi-NIC Networking Overview

When deploying large-scale distributed training workloads (using JAX/PJRT or PyTorch/NCCL) on multi-slice TPU clusters or multi-node GPU clusters, node communication spans multiple network interfaces:

* **`nic0` (Primary interface)**: Used for GKE orchestration, API server communications, and distributed system coordination (e.g. JAX coordinator, PyTorch master node).
* **`nic1` through `nicN` (Secondary interfaces / DCN network)**: High-speed, dedicated networks for accelerator-to-accelerator data collectives.

In standard Cluster Toolkit blueprints, these networks are deployed as custom VPC networks (e.g., `net-0` and `net-1`).

### The Asymmetric Routing Issue
By default, the operating system routes all outgoing traffic using the default gateway associated with the primary interface (`eth0` / `nic0`).

When a distributed workload initializes:
1. The coordination server (e.g. JAX coordinator) resolves and distributes worker IPs using their `nic0` addresses.
2. The accelerator runtime backend (like `libtpu.so` or `NCCL`) binds peer-to-peer communication sockets (e.g., port `8471` or GPU RDMA ports) to the high-speed secondary interface (`eth1` / `nic1`).
3. During peer-to-peer data synchronization, worker nodes send packets from their local `nic1` interface targeting the peer's `nic0` IP address.
4. Because the target is on the `nic0` subnet, the receiving node attempts to respond via its default gateway (`nic0`).
5. This creates an **asymmetric route** (traffic going out of `nic1` but returning via `nic0`).
6. The Google Cloud VPC network fabric enforces strict anti-spoofing checks (source IP validation) at the physical hypervisor layer and silently drops asymmetric packets.
7. As a result, the distributed collective initialization hangs indefinitely during the first parameter synchronization step.

---

## 2. GKE Automated Routing Solution (DRANET / ANP)

GKE natively solves this problem using **Accelerator Network Profiles (ANP)**. When configured on a node pool, GKE automatically installs and manages host-level routing rules (`ip rule` and `ip route` tables) on the VM instances.

These rules implement policy-based routing:
* Any packet originating from a secondary interface (e.g. `eth1`) is forced to return via the gateway of that same secondary network interface.
* This ensures complete routing symmetry, preventing GCP hypervisor-level packet drops and allowing distributed JAX/NCCL collectives to connect successfully.

For GKE documentation, refer to:
* [GKE Multi-Network Support](https://cloud.google.com/kubernetes-engine/docs/how-to/multi-nets-tpu-gpu)
* [Accelerator Network Profiles](https://cloud.google.com/kubernetes-engine/docs/concepts/about-multi-nets-tpu-gpu#accelerator-network-profiles)

---

## 3. Cluster Toolkit Configuration Guide

In Cluster Toolkit, GKE policy routing and DRA driver installation are controlled by the `enable_dranet` variable in the `gke-node-pool` module:

```terraform
variable "enable_dranet" {
  type        = bool
  default     = false
  description = "Enable GKE managed Dynamic Resource Allocation (DRA) driver for networking (DRANET) and Accelerator Network Profile (ANP)..."
}
```

### Automatic Calculation Behavior
By default, the `gke-node-pool` module attempts to auto-detect whether to enable DRANET. However, the auto-detection logic in [modules/compute/gke-node-pool/main.tf](file:///usr/local/google/home/neelgoyal/cluster-toolkit/modules/compute/gke-node-pool/main.tf#L60) disables it if the node pool uses custom secondary networks (`additional_networks`):

```terraform
enable_dranet_actual = var.enable_dranet != null ? var.enable_dranet : (... && length(var.additional_networks) == 0 && ...)
```

Since multi-slice/multi-node blueprints explicitly define custom secondary networks, `length(var.additional_networks)` is non-zero, causing GKE policy routing to be disabled by default.

### Configuration Blueprint Steps

To configure a blueprint for multi-network accelerator (TPU v5p/v6e, GPU A3/A4) workloads, apply the following settings:

1. **GKE Cluster Module (`gke-cluster`)**:
   * Explicitly set **`enable_dataplane_v2: true`** (GKE managed DRANET **requires** Dataplane V2).
   * Keep the `additional_networks` setting to define the secondary network interfaces.

   Example configuration:

   ```yaml
   - id: gke-tpu-cluster
     source: modules/scheduler/gke-cluster
     use: [gke-network, service_account]
     settings:
       enable_dataplane_v2: true
       additional_networks: $(gke-tpu-net-1.instance_additional_networks)
   ```

2. **GKE Node Pool Module (`gke-node-pool`)**:
   * Explicitly set **`enable_dranet: true`** (enables GKE Accelerator Network Profile).
   * **Remove** the `additional_networks` block (ANP dynamically configures node network attachments under the hood; passing manual `additional_networks` alongside it will fail the `dranet_additional_networks_conflict` check).
   * For **TPU** node pools, override the device class by setting **`dranet_device_class_name: "netdev.google.com"`**. For **GPU** node pools, omit this setting (it defaults to `"mrdma.google.com"`).

   Example TPU node pool configuration:

   ```yaml
   - id: gke-tpu-pool
     source: modules/compute/gke-node-pool
     use: [gke-tpu-cluster, service_account]
     settings:
       enable_dranet: true
       dranet_device_class_name: netdev.google.com
       # additional_networks must NOT be present here!
   ```

This configuration ensures:
1. `accelerator_network_profile = "auto"` is passed to the GKE node pool resource.
2. The `cloud.google.com/gke-networking-dra-driver: "true"` label is applied to the GKE nodes.
3. The correct device class (`netdev.google.com` for TPUs) is registered in the cluster.
4. The GKE node agent correctly provisions host policy routing tables, allowing seamless multi-slice JAX or PyTorch training execution.
