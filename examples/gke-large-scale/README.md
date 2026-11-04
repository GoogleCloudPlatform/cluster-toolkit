# Deploying Large GKE Clusters (1,000–5,000 Nodes)

[Regional GKE clusters with Dataplane V2](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/planning-large-clusters#clusters-5k-nodes) support up to **5,000 nodes**. In Cluster Toolkit, large clusters are split into smaller node pools (for example, 49 CPU pools of 100 nodes, 76 GPU pools of 64 nodes, or 76 TPU slices of 64 hosts).

When creating 1,000 to 5,000 nodes at once, deployments can fail or slow down for several reasons:

1. **Node pool creation timeouts**: Provisioning thousands of nodes in a single step can exceed GKE's node pool creation timeout before all VMs finish booting and reporting ready.
2. **Control plane resizing**: A new GKE cluster starts with a small control plane and automatically resizes as nodes are added. Adding thousands of nodes before the control plane finishes resizing slows down node registration.
3. **Regional and project quotas**: Insufficient quotas for vCPUs, VM instances, SSD boot disk storage, VPC networks, external IP addresses, or GKE concurrent operations per cluster cause resource creation to fail mid-deployment.
4. **IP address and Cloud NAT exhaustion**: Default subnet and pod CIDR ranges run out of IP addresses at scale, and insufficient Cloud NAT IP addresses prevent private nodes from downloading container images at the same time.
5. **Container image pull rate limits**: Thousands of nodes pulling images simultaneously from public container registries (such as Docker Hub) can hit rate limits and stall workload startup.

---

## 1. Blueprints

| Blueprint | Provisioning Model | Target Use Case |
| :--- | :--- | :--- |
| **[`gke-scale-5k-autoscaled.yaml`](./gke-scale-5k-autoscaled.yaml)** | Creates 49 node pools with `0` starting nodes. Scales up automatically when jobs run, or by increasing `autoscaling_total_min_nodes`. | On-demand clusters using the GKE Cluster Autoscaler. |
| **[`gke-scale-5k-static.yaml`](./gke-scale-5k-static.yaml)** | Creates 49 node pools with `static_node_count: 2` first, then increases `static_node_count` to `100` (4,900 nodes) in a second step. | Dedicated clusters with fixed-size node pools (no Cluster Autoscaler). |
| **[`gke-scale-multinic-lustre.yaml`](./gke-scale-multinic-lustre.yaml)** | Deploys 25 node pools (up to 250 `c3-standard-8` nodes) across 8 NICs (`1` primary VPC + `7` secondary `multivpc` networks at MTU `8896`) with an 18 TiB Managed Lustre filesystem. | Multi-NIC / multi-rail workloads requiring high-throughput parallel storage. |

---

## 2. Blueprint Configuration

The blueprints in this directory apply the following settings (see GKE's [Planning large clusters](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/planning-large-clusters) documentation for details):

### Core Cluster & Node Pool Settings

1. **Larger IP ranges (`/16` network = 65,000+ IPs / `/18` primary subnet = 16,382 node IPs, `/11` pod range = 32,768 node blocks at 32 pods/node)**: Prevents running out of IP addresses at 5,000 nodes.
2. **`ips_per_nat: 8` (`vpc`)**: Gives private nodes enough external IPs to download container images at the same time.
3. **`enable_dataplane_v2: true` & `enable_node_local_dns_cache: true` (`gke-cluster`)**: Dataplane V2 is **required** for clusters above 1,000 nodes (`gcluster` creates regional Private Service Connect clusters by default, which is also required). NodeLocal DNSCache caches DNS lookups on each node.
4. **`enable_gcfs: true` (`gke-cluster` & `gke-node-pool`)**: Streams container images so nodes and pods start faster.
5. **`default_max_pods_per_node: 32` / `max_pods_per_node: 32`**: Reserves fewer Pod IPs per node (64 instead of 256).
6. **`system_node_pool_machine_type: e2-standard-8` or `e2-standard-16` (`gke-cluster`)**: Gives GKE system services, Kueue, and JobSet controllers enough CPU and memory for 1,000–5,000 nodes.
7. **`enable_private_nodes: true` & `disk_type: pd-balanced` (`gke-node-pool`)**: Avoids public IP limits and speeds up node boot times.
8. **Shared service accounts (`node_pool_service_account` & `workload_service_account`)**: Shares `gke-np-sa` across all compute node pools and binds `gke-wl-sa` to Workload Identity (`configure_workload_identity_sa: true`), creating only 2 service accounts total. Keep `deployment_name` ≤ 16–20 characters so `${deployment_name}-gke-np-sa` stays within GCP's 30-character service account ID limit.
9. **`timeout_create: 2h`, `timeout_update: 2h`, & `timeout_delete: 2h` (`gke-cluster` & `gke-node-pool`)**: Gives Terraform enough time to wait for GKE cluster operations, with `auto_upgrade: false` and `auto_repair: false` to prevent unplanned node recreations during scale-up.

### Multi-NIC & DRANET Networking Settings (Optional)

1. **`enable_dranet: true` (`gke-node-pool`)**: Use for GPU and TPU node pools (`A3`, `A4`, `CT6E`, `TPU7X`); Cluster Toolkit automatically sets up high-speed secondary networks and drivers (omit `secondary_vpcs` from `compute_pools.use` when `enable_dranet: true`).
2. **`subnetwork_cidr_suffix: 20`, `mtu: 8896`, & `enable_cloud_nat: false` (`multivpc`)**: Gives each secondary network 4,096 IPs (instead of 254) so clusters above 250 nodes do not run out of IPs, and disables unnecessary Cloud Routers, NAT gateways, and IAP firewall rules on internal-only networks.

### Storage & Batch Workload Settings (Optional)

1. **`enable_managed_lustre_csi: true` (`gke-cluster`)**: Connects Managed Lustre storage to your cluster (Lustre is created at the same time as the cluster, so it adds no extra wait time).
2. **`enable_gcsfuse_csi: true` (`gke-cluster`)**: Lets pods read and write directly to Cloud Storage buckets.
3. **`controller_cpu: "4"` & `controller_memory: "8Gi"` (`kubectl-apply` for `kueue` & `jobset`)**: Gives the job schedulers enough CPU and memory to start thousands of pods at the same time.
4. **Use Artifact Registry for container images**: Store your job images in Artifact Registry in the same region so thousands of nodes do not hit public download limits (like Docker Hub).

---

## 3. Prerequisites

### Install Cluster Toolkit

1. Install [dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).
2. Set up [Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).

### GKE Version, APIs, and IAM Permissions

- **GKE Version**: Requires GKE `1.34` or later (`1.34.1-gke.1829001` or later when `enable_dranet: true` is used; the blueprints default to `version_prefix: "1.36."`).
- **Required Google Cloud APIs**: Enable the Compute Engine (`compute.googleapis.com`), Kubernetes Engine (`container.googleapis.com`), Cloud Storage (`storage.googleapis.com`), and Artifact Registry (`artifactregistry.googleapis.com`) APIs. When deploying `gke-scale-multinic-lustre.yaml`, also enable the Managed Lustre (`lustre.googleapis.com`) and Service Networking (`servicenetworking.googleapis.com`) APIs.
- **Required IAM Roles**: Ensure your account has permissions to create compute, GKE, storage, and IAM service account resources (`roles/compute.admin`, `roles/container.admin`, `roles/iam.serviceAccountAdmin`, `roles/resourcemanager.projectIamAdmin`, `roles/storage.admin`, plus `roles/lustre.admin` and `roles/servicenetworking.networksAdmin` for Managed Lustre).

### Quotas and Control Plane Preparation

- **Check regional quotas**: Verify that your GCP project has enough quota in your target region under **IAM & Admin > Quotas & System Limits** (request an increase there or contact Cloud Support if needed):
  1. **CPUs & VMs**: Regional vCPUs (`compute.googleapis.com/cpus` and machine-family metrics such as `compute.googleapis.com/c3_cpus`), VM instances (`compute.googleapis.com/instances`, ~6,000 VMs for 5,000 nodes), instance templates (`compute.googleapis.com/instance_templates` and `compute.googleapis.com/regional_instance_templates`), and managed instance groups (`compute.googleapis.com/instance_group_managers`).
  2. **Boot disk storage**: `pd-balanced` disks count toward SSD storage quota (`compute.googleapis.com/ssd_total_storage` / `SSD_TOTAL_GB`, ~500 TB for 5,000 nodes with 100 GB disks, or ~100–150 TB if `node_disk_size_gb` is lowered to 20–30 GB).
  3. **VPC networks & external IPs (for Multi-NIC)**: VPC networks (`compute.googleapis.com/networks`, default 5 VPCs; an 8-NIC cluster needs at least 10–15 VPCs), subnetworks (`compute.googleapis.com/subnetworks`), and regional external IP addresses (`compute.googleapis.com/regional_static_addresses` and `compute.googleapis.com/regional_in_use_addresses`, default 8 IPs).
  4. **Managed Lustre (optional)**: Zonal Lustre storage capacity (`lustre.googleapis.com/capacity`) and throughput (`lustre.googleapis.com/throughput`).
- **Check GKE concurrent operations & node limits**: Verify **Regional Concurrent Operations** (`container.googleapis.com/operations_per_regional_cluster`, or `container.googleapis.com/operations_per_zonal_cluster` for zonal clusters) and **Nodes per cluster** (`container.googleapis.com/quota/nodes_per_cluster`, default `5,000` nodes including system nodes) under **IAM & Admin > Quotas & System Limits**. If your concurrent operations limit is lower than the `--parallelism` level you plan to use, request an increase from the **Quotas & System Limits** page or contact Cloud Support.
- **Pre-size or warm your control plane**: For dedicated 2,500–5,000-node clusters, ask Cloud Support to pre-size the GKE control plane upfront so you do not have to wait for automatic control-plane resizing when scaling up with `--parallelism 20` to `100`. If your project is not pre-sized, boot a ~100-node pilot wave first (see Option B Step 1) at default parallelism (`10`) to trigger control-plane resizing automatically.

---

## 4. Deployment Steps

### Configure the Blueprint

Open your chosen blueprint (`.yaml`) and edit the `vars` block at the top of the file:

| Variable | Required | Default | Description |
| :--- | :--- | :--- | :--- |
| `project_id` | **Yes** | — | Your Google Cloud project ID. |
| `authorized_cidr` | **Yes** | — | CIDR block allowed to reach the GKE control plane (e.g., `<your-ip>/32`, or `0.0.0.0/0`). |
| `deployment_name` | No | `gke-scale-5k` / `scale-mnic-250` | Cluster and deployment folder name (keep ≤ 16–20 characters). |
| `region` / `zone` | No | `us-central1` / `us-central1-a` | Target GCP region and zone (plus `lustre_zone` in `gke-scale-multinic-lustre.yaml`). |
| `machine_type` | No | `e2-standard-2` / `c3-standard-8` | CPU worker VM machine type (e.g., `e2-standard-2`, `n2-standard-4`, `c3-standard-8`). For GPU or multi-host TPU clusters, start from the dedicated accelerator blueprints (such as [`gke-a3-megagpu`](../gke-a3-megagpu/README.md), [`gke-a4`](../gke-a4/README.md), [`gke-tpu-v6e`](../gke-tpu-v6e/README.md), or [`gke-tpu-7x`](../gke-tpu-7x/README.md)) and apply the scaling settings in Section 2. |
| `num_node_pools` | No | `49` / `25` | Number of worker node pools (or TPU slices). |
| `node_disk_size_gb` | No | `100` | Boot disk size in GB per node (can be lowered to `20–30` with Image Streaming to reduce `SSD_TOTAL_GB` quota usage). |
| `terraform_backend_defaults` | No | Commented out (local state) | Uncomment and set `bucket: <your-tf-state-bucket>` to store Terraform state in GCS. |

---

### Option A: Autoscaled Pools (`gke-scale-5k-autoscaled.yaml`)

#### Step 1 — Deploy Empty Pools

After setting `project_id` and `authorized_cidr` in `gke-scale-5k-autoscaled.yaml`, run:

```bash
./gcluster deploy examples/gke-large-scale/gke-scale-5k-autoscaled.yaml --auto-approve
```

#### Step 2 — Scale Up Your Nodes

- **Scale when jobs run**: Leave `autoscaling_total_min_nodes: 0`. GKE adds nodes when you submit jobs and scales them back to `0` when jobs finish.
- **Keep nodes running 24/7**: Update `autoscaling_total_min_nodes: 100` (49 pools × 100 nodes = 4,900 nodes) in `gke-scale-5k-autoscaled.yaml` while leaving `initial_node_count: 0` unchanged so Terraform updates the pools in place without recreating them, then run:

```bash
./gcluster deploy examples/gke-large-scale/gke-scale-5k-autoscaled.yaml -w --auto-approve
```

---

### Option B: Fixed-Size Pools (`gke-scale-5k-static.yaml`)

If you cannot use the Cluster Autoscaler, deploy your pools in two steps so pool creation does not time out.

#### Step 1 — Start with 2 Nodes per Pool (`static_node_count: 2`)

Set `project_id` and `authorized_cidr` in `gke-scale-5k-static.yaml` (which defaults to `static_node_count: 2`), then run:

```bash
./gcluster deploy examples/gke-large-scale/gke-scale-5k-static.yaml --auto-approve
```

#### Step 2 — Increase to Full Size (`static_node_count: 100`)

Update `static_node_count: 100` in `gke-scale-5k-static.yaml` and re-deploy with `-w`:

```bash
./gcluster deploy examples/gke-large-scale/gke-scale-5k-static.yaml -w --auto-approve  # Or pass --parallelism <N>
```

> **Note:** If your control plane was not pre-sized, GKE pauses between batches to resize the control plane, which can cause some pools to hit a creation timeout on the first run even though the VMs finish starting in the background. Once `kubectl get nodes` shows all nodes `Ready`, re-run the `gcluster deploy` command above to finish.

| Hardware | Example ~5k-Node Layout | Default Control Plane | Pre-Sized Control Plane (`--parallelism`*) |
| :--- | :--- | :--- | :--- |
| **CPU** (`e2`, `n2`, `c3`) | 49 pools × 100 nodes (4,900 nodes) | Default (`10`) | `--parallelism` 20 to 100 |
| **GPU** (`a3-megagpu-8g`, `a4-highgpu-8g`, etc.) | 76 pools × 64 nodes (4,864 nodes) | Default (`10`) | `--parallelism` 8 to 15 |
| **TPU** (`ct6e-standard-4t`, `tpu7x`, etc.) | 76 slices × 64 hosts (4,864 hosts) | Default (`10`) | `--parallelism` 16 to 30 |

*\*Using `--parallelism` above `10` during node pool creation requires a pre-sized control plane and sufficient `container.googleapis.com/operations_per_regional_cluster` quota.*

---

### Option C: Multi-NIC & Managed Lustre (`gke-scale-multinic-lustre.yaml`)

Deploys 25 node pools (`c3-standard-8`) with `1` primary VPC, `7` secondary VPCs (`multivpc`, MTU `8896`), and an 18 TiB Managed Lustre filesystem (`lustre-scale-pvc`) using the two-step workflow.

Set `project_id` and `authorized_cidr` in `gke-scale-multinic-lustre.yaml`, then run:

```bash
# Step 1: Bootstrap with static_node_count: 2 (50 nodes)
./gcluster deploy examples/gke-large-scale/gke-scale-multinic-lustre.yaml --auto-approve

# Step 2: Update static_node_count: 10 in the blueprint (250 nodes / 2,000 NICs), then re-deploy:
./gcluster deploy examples/gke-large-scale/gke-scale-multinic-lustre.yaml -w --auto-approve
```

---

## 5. Submitting Jobs (`gcluster job submit`)

Submit a multi-slice JobSet across the cluster (passing `--service-account workload-identity-k8s-sa` so pods authenticate via Workload Identity):

```bash
# For gke-scale-multinic-lustre, use --cluster scale-mnic-250 --compute-type c3-standard-8 --num-slices 25 --num-nodes 10 --mount "lustre-scale-pvc;/mnt/lustre;rw"
./gcluster job submit \
  --cluster gke-scale-5k \
  --location us-central1 \
  --project "<project_id>" \
  --name scale-gcs-kueue \
  --compute-type e2-standard-2 \
  --num-slices 49 \
  --num-nodes 100 \
  --service-account workload-identity-k8s-sa \
  --mount "gs://<project_id>-gcs-gke-scale-5k;/mnt/gcs;rw;options=implicit-dirs" \
  --image gke.gcr.io/gke-distroless/bash:latest \
  --command 'echo "Hello from ${HOSTNAME}" && ls -la /mnt/gcs' \
  --await-job-completion
```

---

## 6. Deleting the Cluster (`gcluster destroy`)

Deleting node pools does not require booting VMs or registering nodes, and a running 5,000-node cluster already has a resized control plane. Passing a higher `--parallelism` (up to your project's `container.googleapis.com/operations_per_regional_cluster` quota) speeds up teardown:

- **Stop running jobs first**: Delete any active jobs before running `destroy` so open storage or network attachments do not hold up teardown.
- **Default parallelism (`10`)**: Deletes node pools in batches of 10.
- **High parallelism (`--parallelism 50` to `100`)**: Deletes all node pools concurrently in a single wave when `container.googleapis.com/operations_per_regional_cluster` supports it.

```bash
# Default concurrency (10) — use scale-mnic-250 when tearing down Option C
./gcluster destroy gke-scale-5k --robust

# Accelerated teardown (when operations_per_regional_cluster quota is >= 50)
./gcluster destroy gke-scale-5k --parallelism 50 --robust
```
