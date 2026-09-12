# High-Performance Slurm Scale Blueprints for Google Cloud Platform

![Community](https://img.shields.io/badge/-community-%23b8def4?style=plastic)
![Stable](https://img.shields.io/badge/-stable-lightgrey?style=plastic)

This directory contains turnkey, production-grade **Slurm Workload Manager**
scale blueprints for the
[Google Cloud Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs).

These blueprints are engineered and optimized to support mass concurrent batch
bursts, high-throughput computing (HTC), parameter sweeps, genomics pipelines,
and extreme-scale workloads (50,000 to 100,000+ cores) on Google Cloud Platform.

---

## Table of Contents

- [Overview of Blueprints](#overview-of-blueprints)
  - [Choosing Between Single-Region and Multi-Region](#choosing-between-single-region-and-multi-region)
- [Blueprint Descriptions](#blueprint-descriptions)
  - [1. Multi-Region Scale Blueprint (`hpc-slurm-multiregion-scale.yaml`)](#1-multi-region-scale-blueprint-hpc-slurm-multiregion-scaleyaml)
  - [2. Single-Region Scale Blueprint (`hpc-slurm-scale.yaml`)](#2-single-region-scale-blueprint-hpc-slurm-scaleyaml)
- [Customizing Scale Blueprints](#customizing-scale-blueprints)
  - [Adapting Multi-Region Deployments](#adapting-multi-region-deployments)
  - [Adapting Single-Region Deployments](#adapting-single-region-deployments)
- [Architectural and Tuning Comparison Matrix](#architectural-and-tuning-comparison-matrix)
- [Quickstart Deployment Guide](#quickstart-deployment-guide)
  - [Prerequisites](#prerequisites)
  - [Deploying Multi-Region Scale Cluster (96,000+ Cores)](#deploying-multi-region-scale-cluster-96000-cores)
  - [Deploying Single-Region Scale Cluster (50,000 to 102,400 Cores)](#deploying-single-region-scale-cluster-50000-to-102400-cores)
  - [Destroying Infrastructure](#destroying-infrastructure)
- [Workload Validation and Benchmark Runbooks](#workload-validation-and-benchmark-runbooks)
  - [1. Multi-Region High-Throughput Burst (1,500 Parallel Tasks)](#1-multi-region-high-throughput-burst-1500-parallel-tasks)
  - [2. Single-Region Burst (300 Parallel Tasks)](#2-single-region-burst-300-parallel-tasks)
- [Quota Requirements](#quota-requirements)

---

## Overview of Blueprints

| Blueprint File | Scale Target | Geographic Scope | Topology & Networking | Primary Use Case |
| :--- | :--- | :--- | :--- | :--- |
| [**`hpc-slurm-scale.yaml`**](./hpc-slurm-scale.yaml) | **50,000 to 102,400 Cores** (800 Nodes) | **Single Region** (`us-central1`) | Single Regional `/15` High-Capacity Subnet across 4 Zones | **Recommended Default**: Production Batch & HTC workloads requiring local I/O, lowest network latency, and zero inter-region egress cost |
| [**`hpc-slurm-multiregion-scale.yaml`**](./hpc-slurm-multiregion-scale.yaml) | **96,000+ Cores** (1,500 Nodes) | **Multi-Region** (`us-central1`, `us-east4`, `us-west1`) | Global VPC with 3 Regional Subnets across 10 Zones | Regional capacity stockout mitigation for batch & HTC workloads |

### Choosing Between Single-Region and Multi-Region

When choosing a blueprint, consider your workload's communication patterns,
storage access, and regional compute availability:

#### Single-Region Blueprint (`hpc-slurm-scale.yaml`) — Recommended Default

- **Default Recommendation**: For the vast majority of HPC, HTC, and batch
  workloads, the **single-region blueprint is the recommended default**.
- **Lowest Network Latency**: All compute nodes and scheduler daemons reside
  within the same regional Google Cloud network fabric. Communication between
  nodes avoids cross-region hops, providing minimal network delay and maximum
  throughput.
- **Cost Efficiency (Zero Cross-Region Egress)**: Because all traffic stays
  within the region, there are no inter-region data egress charges between
  compute nodes, login nodes, and storage.
- **Optimal Shared Storage I/O**: Cloud Filestore NFS is co-located in the same
  region as all compute instances, delivering low latency and high bandwidth for
  shared file access without WAN bottlenecks.
- **Zonal Stockout Mitigation**: Automatically distributes compute nodes across
  4 zones (`us-central1-a/b/c/f`) with `zone_target_shape: ANY`, providing
  failover protection if a specific zone within the region runs low on capacity.

#### Multi-Region Blueprint: Stockout Mitigation
(`hpc-slurm-multiregion-scale.yaml`)

- **When to Use**: The multi-region blueprint is recommended specifically to
  mitigate **regional capacity stockouts**. While the single-region blueprint is
  preferred for cost and performance, large scale bursts (particularly when
  using Spot VMs) can encounter localized hardware pool exhaustion
  (`ZONE_RESOURCE_POOL_EXHAUSTED`) or regional quota saturation within a single
  region. The multi-region blueprint circumvents this by distributing dynamic
  node provisioning across multiple regions.
- **Multi-Region Capacity Pooling**: By spreading compute instances across
  three distinct Google Cloud regions (`us-central1`, `us-east4`, `us-west1`)
  and 10 zones, Slurm pulls capacity from wherever Google Cloud has available
  hardware pools, mitigating localized datacenter shortages.
- **Key Trade-offs to Consider**:
  - **Higher Network & Egress Cost**: Cross-region communication (such as
    node-to-controller RPCs or data transfers between nodes in different
    regions) incurs standard Google Cloud inter-region data egress charges.
  - **Communication & Storage Latency**: Nodes running in remote regions
    (`us-east4`, `us-west1`) experience network delays when communicating with
    the central Slurm controller and mounting the shared Cloud Filestore NFS
    filesystem (`us-central1`) across regional WAN links.
  - **Ideal Workload Profile**: Loosely coupled, embarrassingly parallel
    workloads (e.g., parameter sweeps, Monte Carlo simulations, independent
    batch task arrays, genomics sequence alignment) where each task runs
    self-contained on an individual node without MPI inter-node communication,
    and where bypassing regional stockouts takes priority over intra-cluster
    network latency.

---

## Blueprint Descriptions

### 1. Multi-Region Scale Blueprint (`hpc-slurm-multiregion-scale.yaml`)

Creates an extreme-scale, multi-region auto-scaling Slurm cluster spanning three
distinct Google Cloud regions (`us-central1`, `us-east4`, and `us-west1`).

- **Blueprint File**:
  [`hpc-slurm-multiregion-scale.yaml`](./hpc-slurm-multiregion-scale.yaml)

> [!NOTE]
> **Example Reference Configuration**: This blueprint provides an illustrative,
> tested example populated with concrete values (such as `us-central1`,
> `us-east4`, and `us-west1` across 10 specific zones, `n2-standard-128` machine
> types, and 500 dynamic nodes per region). These values demonstrate a tested
> scale pattern and are an example of what you can do. You can customize this
> configuration to run in different zones, different regions, or with different
> machine types and node limits based on your project quotas and workload
> requirements.
> See [Customizing Scale Blueprints](#customizing-scale-blueprints) below.

#### Architecture and Key Features

- **Global VPC Topology**: Deploys a single global VPC network with 3 distinct
  `/16` regional subnets (`10.0.0.0/16`, `10.1.0.0/16`, `10.2.0.0/16`).
- **Multi-Regional Dynamic Nodesets**: Creates 3 independent nodesets
  (`nodeset_central`, `nodeset_east`, `nodeset_west`) bound to regional subnet
  self-links.
- **10-Zone Distribution**: Distributes compute instances across 10 distinct
  zones (`us-central1-a/b/c/f`, `us-east4-a/b/c`, `us-west1-a/b/c`), bypassing
  single-datacenter hardware stockouts (`ZONE_RESOURCE_POOL_EXHAUSTED`).
- **Cloud-Bursting Guardrails**:
  - `ResumeRate = 300`: Staggers VM creation waves (300 nodes/min) to maintain
    API stability and protect Filestore NFS.
  - `ResumeTimeout = 900`: 15-minute window preventing premature node dropouts
    during mass boot surges.
  - `SuspendTimeout = 600`: 10-minute graceful cooldown period.
  - `SuspendTime = 300`: Auto-decommissions idle nodes after 5 minutes of
    inactivity ($0 idle compute cost).
  - `SchedulerParameters = batch_sched_delay=1,bf_continue,defer`: Debounces
    backfill locks and optimizes scheduler throughput.
  - `TreeWidth = 65533`: Flat communication hierarchy preventing intermediate
    fanout hops and RPC packet drops in cloud networks.

#### Scale Target

- **Supports up to 1,500 parallel dynamic nodes (96,000+ to 192,000 cores)**
  across 3 regions for massive high-throughput batch arrays:
  - With `n2-standard-128`: 1,500 nodes × 128 vCPUs = **192,000 Cores**
  - With `n2d-standard-64`: 1,500 nodes × 64 vCPUs = **96,000 Cores**

---

### 2. Single-Region Scale Blueprint (`hpc-slurm-scale.yaml`)

Creates a production-tuned single-region auto-scaling Slurm cluster supporting
up to 800 dynamic nodes (**51,200 to 102,400 Cores**) across 4 zones in
`us-central1`.

- **Blueprint File**: [`hpc-slurm-scale.yaml`](./hpc-slurm-scale.yaml)

> [!NOTE]
> **Example Reference Configuration**: The single-region blueprint is populated
> with example values for `us-central1` across 4 zones (`us-central1-a/b/c/f`),
> `n2-standard-128`, and 800 dynamic nodes. You can customize `region`, `zone`,
> `compute_zones`, and machine types to target any region and selection of zones
> where your project has quota.

#### Architecture and Key Features

- **Core Capacity**:
  - With `n2-standard-128`: **800 nodes × 128 vCPUs = 102,400 Cores** (100K+
    Cores max capacity).
  - With `n2d-standard-64`: **800 nodes × 64 vCPUs = 51,200 Cores** (50K+ Cores
    max capacity).
- **High-Throughput Subnet**: Deploys a `/15` primary VPC subnet providing
  131,072 private IP addresses for mass dynamic provisioning.
- **Multi-Zonal Dynamic Failover**: Uses `zones: [us-central1-a, b, c, f]` with
  `zone_target_shape: ANY` for dynamic zonal failover.
- **Full Slurm Scale Tunings**: Configured with identical scale parameters
  (`ResumeRate=300`, `ResumeTimeout=900`, `SuspendTimeout=600`,
  `SuspendTime=300`, `TreeWidth=65533`,
  `SchedulerParameters=batch_sched_delay=1,bf_continue,defer`) to eliminate the
  5-minute timeout cliff on single-region infrastructure.
- **Shared Storage**: Standard Cloud Filestore instance mounted at `/home`.

---

## Customizing Scale Blueprints

Both scale blueprints are reference configurations designed to be readily
customized for your specific geographic preferences, machine requirements, and
Compute Engine quota availability.

### Adapting Multi-Region Deployments

The multi-region blueprint demonstrates an example deployment using three US
regions (`us-central1`, `us-east4`, `us-west1`) and 10 zones. You can customize
any of these values:

1. **Modify Regions and Zones in `vars`**:
   - `region` and `zone`: Sets the primary region and zone for the Slurm
     controller, login node, and Cloud Filestore NFS instance (default:
     `us-central1` and `us-central1-a`).
   - Zonal lists (`central_zones`, `east_zones`, `west_zones`): Define candidate
     zones for compute instance provisioning in each region. Update these lists
     to match the zones where your project has available Spot or On-Demand
     quota, or change them to target alternative regions (e.g., `us-east1` or
     `europe-west4`):

     ```yaml
     vars:
       region: us-central1
       zone: us-central1-a

       # Example: customize zones to match your regional quota allocations
       central_zones:
       - us-central1-a
       - us-central1-b
       - us-central1-f

       east_zones:
       - us-east1-b
       - us-east1-c
       - us-east1-d

       west_zones:
       - us-west2-a
       - us-west2-b
       - us-west2-c
     ```

2. **Update Regional VPC Subnetworks (`modules/network/vpc`)**:
   - If targeting different regions, update `subnet_region` and assign
     non-overlapping CIDR blocks (`subnet_ip`) for each regional subnetwork:

     ```yaml
     subnetworks:
     - subnet_name: central-subnet
       subnet_region: us-central1
       subnet_ip: 10.0.0.0/16
     - subnet_name: east-subnet
       subnet_region: us-east1
       subnet_ip: 10.1.0.0/16
     - subnet_name: west-subnet
       subnet_region: us-west2
       subnet_ip: 10.2.0.0/16
     ```

3. **Update Regional Dynamic Nodesets (`nodeset_*`)**:
   - Update each nodeset module to reference its target `region`, primary
     `zone`, `zones` list, and `subnetwork_self_link`:

     ```yaml
     - id: nodeset_east
       source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
       settings:
         subnetwork_self_link: $(network.subnetworks["us-east1/east-subnet"].self_link)
         region: us-east1
         zone: us-east1-b
         zones: $(vars.east_zones)
         zone_target_shape: ANY
     ```

   - To scale across more or fewer regions (e.g., 2 or 4 regions), add or remove
     nodeset modules and update the `use:` list under `compute_partition`.

4. **Change Compute Machine Types and Node Limits**:
   - Modify `vars.compute_machine_type` to any supported machine family (e.g.,
     `n2d-standard-64`, `c2-standard-60`, `c3-standard-176`).
   - Modify `vars.node_count_dynamic_max_per_region` to scale the per-region VM
     ceiling based on your quota limits.

### Adapting Single-Region Deployments

The single-region blueprint defaults to `us-central1` across 4 zones with 800
nodes. You can adapt it in two ways:

- **Change Target Region and Zones in YAML**:
  Update `vars.region`, `vars.zone`, and `vars.compute_zones` to target your
  desired region and zonal distribution:

  ```yaml
  vars:
    region: us-east4
    zone: us-east4-a
    compute_zones:
    - us-east4-a
    - us-east4-b
    - us-east4-c
  ```

- **Override Variables via CLI**:
  Pass variables directly when generating the deployment with `gcluster create`:

  ```bash
  gcluster create community/examples/slurm-high-throughput/hpc-slurm-scale.yaml \
    --vars project_id=YOUR_PROJECT_ID \
    --vars deployment_name=slurm-scale-east \
    --vars region=us-east4 \
    --vars zone=us-east4-a \
    --vars compute_machine_type=n2d-standard-64 \
    --vars node_count_dynamic_max=500 \
    -o ./slurm-scale-east --force
  ```

---

## Architectural and Tuning Comparison Matrix

| Parameter / Architectural Setting | Single-Region Scale (`hpc-slurm-scale.yaml`) | Multi-Region Global Scale (`hpc-slurm-multiregion-scale.yaml`) |
| :--- | :--- | :--- |
| **Deployment Recommendation** | **Recommended Default** (for most HPC & batch workloads) | **Stockout Mitigation** (when facing regional capacity stockouts) |
| **Scale Capacity** | **50,000 to 102,400 Cores** (800 Nodes) | **96,000+ Cores** (1,500 Nodes) |
| **Geographic Scope** | Single Region (`us-central1`) | **3 Regions** (`us-central1`, `us-east4`, `us-west1`) |
| **Zone Distribution** | 4 Zones (`us-central1-a, b, c, f`) | **10 Zones** across Central, East, and West |
| **Inter-Node Latency** | **Ultra-low** (sub-millisecond intra-region fabric) | **Higher** (cross-region WAN delay between nodes) |
| **Network Egress Cost** | **Zero** cross-region data egress charges | Incurs inter-region data egress charges |
| **Shared Storage (NFS) I/O** | Local regional low-latency file access | Cross-region I/O latency for remote nodes |
| **Stockout Resilience** | High (4 zones within 1 region) | **Maximum** (10 zones across 3 regions) |
| **Network Architecture** | Single Regional `/15` Subnet (131,072 IPs) | Global VPC with 3 Regional `/16` Subnets |
| **Nodeset Layout** | Single Unified Dynamic Nodeset | **3 Regional Nodesets** (`nodeset_central, east, west`) |
| **`ResumeRate`** | **`300 nodes/min`** *(Paced wave rate)* | **`300 nodes/min`** *(Paced wave rate)* |
| **`ResumeTimeout`** | **`900 sec`** *(15-minute buffer for NFS mounts)* | **`900 sec`** *(15-minute buffer for NFS mounts)* |
| **`SuspendTimeout`** | **`600 sec`** *(10-minute graceful cooldown)* | **`600 sec`** *(10-minute graceful cooldown)* |
| **`SuspendTime`** | **`300 sec`** *(5-minute idle decommissioning)* | **`300 sec`** *(5-minute idle decommissioning)* |
| **`SchedulerParameters`** | **`batch_sched_delay=1, bf_continue, defer`** | **`batch_sched_delay=1, bf_continue, defer`** |
| **`TreeWidth`** | **`65533`** *(Flat RPC hierarchy)* | **`65533`** *(Flat RPC hierarchy)* |

---

## Quickstart Deployment Guide

### Prerequisites

- [Google Cloud Cluster Toolkit CLI](https://cloud.google.com/cluster-toolkit/docs/quickstart)
  (`gcluster`) installed.
- Google Cloud Project with Billing and Compute Engine API enabled.
- Google Cloud CLI (`gcloud`) authenticated
  (`gcloud auth application-default login`).

### Deploying Multi-Region Scale Cluster (96,000+ Cores)

```bash
# 1. Create deployment folder
gcluster create community/examples/slurm-high-throughput/hpc-slurm-multiregion-scale.yaml \
  --vars project_id=YOUR_PROJECT_ID \
  --vars deployment_name=slurm-multiregion-scale \
  -o ./slurm-multiregion-scale --force

# 2. Deploy infrastructure
gcluster deploy ./slurm-multiregion-scale/slurm-multiregion-scale --auto-approve
```

### Deploying Single-Region Scale Cluster (50,000 to 102,400 Cores)

```bash
# 1. Create deployment folder
gcluster create community/examples/slurm-high-throughput/hpc-slurm-scale.yaml \
  --vars project_id=YOUR_PROJECT_ID \
  --vars deployment_name=slurm-scale \
  --vars compute_machine_type=n2d-standard-64 \
  -o ./slurm-scale --force

# 2. Deploy infrastructure
gcluster deploy ./slurm-scale/slurm-scale --auto-approve
```

### Destroying Infrastructure

```bash
# For Multi-Region Cluster
gcluster destroy ./slurm-multiregion-scale/slurm-multiregion-scale \
  --auto-approve
rm -rf ./slurm-multiregion-scale

# For Single-Region Cluster
gcluster destroy ./slurm-scale/slurm-scale --auto-approve
rm -rf ./slurm-scale
```

---

## Workload Validation and Benchmark Runbooks

### 1. Multi-Region High-Throughput Burst (1,500 Parallel Tasks)

Connect to the controller:

```bash
gcloud compute ssh slurmmulti-controller \
  --zone=us-central1-a \
  --tunnel-through-iap
```

> [!NOTE]
> The Slurm controller instance name is derived from `deployment_name` by
> stripping non-alphanumeric characters and truncating to 10 characters
> (`slurm-multiregion-scale` $\rightarrow$ `slurmmulti-controller`). You can
> verify the exact instance name from the Terraform post-deployment output or
> by running `gcloud compute instances list --filter="tags:controller"`.

Submit a 1,500-task array across all 3 regions:

```bash
sbatch --array=1-1500 -N 1 -p compute \
  --job-name=multiregion-96k \
  --wrap='hostname; echo "Task $SLURM_ARRAY_TASK_ID ran on $(hostname) in $(basename $(curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/zone))"; sleep 5'
```

Monitor execution:

```bash
squeue
sinfo
sacct -j <JOB_ID> -X \
  --format=JobID,JobName,State,ExitCode,AllocCPUS,NodeList | head -n 30
```

### 2. Single-Region Burst (300 Parallel Tasks)

Connect to the controller:

```bash
gcloud compute ssh slurmscale-controller \
  --zone=us-central1-a \
  --tunnel-through-iap
```

> [!NOTE]
> For the single-region deployment (`deployment_name=slurm-scale`), hyphens are
> stripped to produce `slurmscale-controller`. If deploying with the default
> blueprint name (`hpc-slurm-scale`), the instance name is
> `hpcslurmsc-controller`.

Submit a 300-task array:

```bash
sbatch --array=1-300 -N 1 -p compute \
  --job-name=burst-300 \
  --wrap='hostname; sleep 15'
```

Monitor execution:

```bash
squeue
sinfo
sacct -j <JOB_ID> -X \
  --format=JobID,JobName,State,ExitCode,AllocCPUS,NodeList | head -n 30
```

---

## Quota Requirements

### For `hpc-slurm-multiregion-scale.yaml` (1,500 Nodes / 96,000 Cores)

- **Compute Engine API: Spot N2D CPUs**:
  - `us-central1`: **32,000 CPUs**
  - `us-east4`: **32,000 CPUs**
  - `us-west1`: **32,000 CPUs**
- **Cloud Filestore API**: 1,024 GB (Basic HDD / Standard) in `us-central1`
- **VPC Network**: In-use IP addresses ≥ 1,500 across subnets

### For `hpc-slurm-scale.yaml` (800 Nodes / 51,200 to 102,400 Cores)

- **Compute Engine API: Spot N2D / N2 CPUs**: **51,200 to 102,400 CPUs** in
  `us-central1`
- **Cloud Filestore API**: 1,024 GB (Basic HDD / Standard) in `us-central1`
- **VPC Subnet**: `/15` subnet (131,072 IP addresses)
