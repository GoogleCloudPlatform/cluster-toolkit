# Migration Guide: Migrating from XPK to Cluster Toolkit (`gcluster`)

**Target Audience**: External Google Cloud customers currently using the `xpk` CLI tool for GKE TPU/GPU cluster deployment and AI/ML workload orchestration.

---

## Table of Contents

- [1. Executive Summary & Overview](#1-executive-summary--overview)
- [2. Key Architectural Differences](#2-key-architectural-differences)
- [3. Step 1: Installation & Setup](#3-step-1-installation--setup)
- [4. Step 2: Cluster Infrastructure Migration](#4-step-2-cluster-infrastructure-migration-migrating-from-xpk-cluster-create)
- [5. Step 3: Workload Migration](#5-step-3-workload-migration-migrating-from-xpk-workload-create)
- [6. Step 4: Advanced Workloads & Features](#6-step-4-advanced-workloads--features)
- [7. XPK to Cluster Toolkit Command Mappings](#7-xpk-to-cluster-toolkit-command-mappings)

---

## 1. Executive Summary & Overview

Google Cloud is standardizing AI/ML infrastructure orchestration on **Cluster Toolkit (`gcluster`)**. Cluster Toolkit provides a unified, production-grade framework for both **infrastructure provisioning** (via modular, declarative Terraform blueprints) and **workload submission** (via `gcluster job submit` integrated with JobSet and Kueue).

### Key Benefits of Migrating to Cluster Toolkit

- **Declarative Infrastructure**: Define GKE clusters, node pools, networking, and storage (Filestore, GCS Fuse, Parallelstore, Lustre) as version-controlled YAML blueprints.
- **Shorthand Compute Resolution**: Pass TPU shorthand directly to `--compute-type` (e.g., `--compute-type v6e-16`), automatically deducing machine types, VMs per slice, and TPU topology.
- **Streamlined Workload Submission**: Submit multi-node TPU and GPU workloads directly using `gcluster job submit` integrated with JobSet and Kueue.
- **On-the-Fly Image Building**: Build container images directly during submission using `--base-image` and `--build-context` via Crane when `GCLUSTER_IMAGE_REPO` is set.
- **Inline Storage Mounting & Mount Options**: Mount Cloud Storage buckets (`gs://`), PVCs (`pvc://`), or Filestore (`filestore://`) inline during job submission using the `--mount` flag with custom GCS Fuse options (`options=<options>`), replacing separate `xpk storage attach` workflows.
- **Advanced Features**: Support for Multi-Tier Checkpointing (MTC), Pathways multi-host workloads, parallel containers, and AI Accelerators (NVIDIA A3 High/Mega/Ultra, A4/GB200, Google TPU v4, v5e, v5p, v6e, and TPU v7x).

---

## 2. Key Architectural Differences

| Dimension | XPK CLI | Cluster Toolkit (`gcluster`) |
| :--- | :--- | :--- |
| **Infrastructure Provisioning** | Imperative (`xpk cluster create` with CLI flags) | Declarative Blueprint (`gcluster create <blueprint_file.yaml>` && `gcluster deploy`) |
| **State Management** | Implicit / GCS bucket state | Terraform state backed by GCS |
| **Workload Submission** | `xpk workload create` | `gcluster job submit` |
| **Storage Attaching** | `xpk storage attach` (separate step) | Inline `--mount` flag with `gcluster job submit` (`gs://`, `pvc://`, `filestore://`) |
| **Mount Options** | `--mount-options` | Inline `options=<options>` within `--mount` (GCS `gs://` volumes exclusively) |
| **Parallel Containers** | Enabled by default for TPU v7/v7x | Enabled by default; disable via `--gke-disable-parallel-containers` |
| **Pathways Support** | `xpk workload create-pathways` | `gcluster job submit --pathways` |

---

## 3. Step 1: Installation & Setup

### Install `gcluster`

Download the latest `gcluster` binary release for your operating system:

```bash
# Find all available releases at: https://github.com/GoogleCloudPlatform/cluster-toolkit/releases
# Set the desired version TAG (e.g., v1.89.0)
TAG=vX.Y.Z

# Set your OS (linux or mac) and architecture (amd64 or arm64)
OS="linux"
ARCH="amd64"

# Download and extract the platform-specific bundle in a single step
mkdir -p cluster-toolkit && curl -L https://github.com/GoogleCloudPlatform/cluster-toolkit/releases/download/${TAG}/gcluster_bundle_${OS}_${ARCH}.tgz | tar -xz -C cluster-toolkit && cd cluster-toolkit
```

### Google Cloud Authentication & Environment Configuration

Authenticate `gcloud` and configure your Google Cloud project and Artifact Registry repository for on-the-fly image builds:

```bash
# Google Cloud Authentication
gcloud auth login
gcloud auth application-default login
gcloud config set project YOUR_PROJECT_ID

# Set GCLUSTER_IMAGE_REPO to your Artifact Registry repository name (not the full URL):
export GCLUSTER_IMAGE_REPO="my-ar-repo"
```

> [!NOTE]
> When you use `--base-image` with `--build-context`, `gcluster` automatically constructs the full image registry path using your cluster's region and project ID: `<region>-docker.pkg.dev/<project_id>/$GCLUSTER_IMAGE_REPO/<image_name>:<tag>`.

---

## 4. Step 2: Cluster Infrastructure Migration (migrating from `xpk cluster create`)

Cluster Toolkit splits cluster creation into two steps:
1. **Define Blueprint**: Create a `.yaml` blueprint specifying your cluster, network, and node pool requirements.
2. **Deploy**: Run `gcluster deploy` to provision the cluster.

### XPK Command Example

```bash
xpk cluster create \
  --cluster my-tpu-cluster \
  --project my-gcp-project \
  --zone us-central1-a \
  --tpu-type v6e-16 \
  --num-slices 2 \
  --authorized-networks 10.0.0.0/8 \
  --private \
  --spot
```

### Equivalent Cluster Toolkit Blueprint (`my-tpu-cluster.yaml`)

```yaml
blueprint_name: my-tpu-cluster

vars:
  deployment_name: my-tpu-cluster
  project_id: my-gcp-project
  zone: us-central1-a
  region: us-central1
  num_slices: 2
  machine_type: ct6e-standard-4t
  tpu_topology: 4x4
  enable_private_endpoint: true
  authorized_cidr: 10.0.0.0/8
  spot: true

deployment_groups:
- group: primary
  modules:
  - id: network
    source: modules/network/pre-existing-vpc
  - id: tpu_cluster
    source: modules/scheduler/gke-cluster
    use: [network]
    settings:
      project_id: $(vars.project_id)
      zone: $(vars.zone)
      enable_private_endpoint: $(vars.enable_private_endpoint)
      master_authorized_networks:
      - cidr_block: $(vars.authorized_cidr)
        display_name: Authorized CIDR
  - id: tpu_node_pool
    source: modules/compute/gke-node-pool
    use: [tpu_cluster]
    settings:
      name: tpu-pool
      machine_type: $(vars.machine_type)
      num_slices: $(vars.num_slices)
      spot: $(vars.spot)
      placement_policy:
        type: COMPACT
        tpu_topology: $(vars.tpu_topology)
```

### Deploy the Blueprint

```bash
# Provision GKE cluster and TPU node pools on Google Cloud
gcluster deploy my-tpu-cluster.yaml
```

#### Utilizing Production-Ready Example Blueprints

Cluster Toolkit includes a comprehensive library of validated blueprints within the [examples/](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples) repository for both TPU (v4 through v7x) and GPU (A3/A4) architectures. These templates allow for immediate deployment by injecting runtime overrides through the `--vars` flag, eliminating the necessity of crafting a custom YAML definition from scratch:

```bash
gcluster deploy examples/gke-tpu-v6e/gke-tpu-v6e.yaml \
  --vars project_id=my-gcp-project,zone=us-central1-a,deployment_name=my-tpu-cluster,num_slices=2
```

#### Externalizing Configuration via Deployment Files (`-d`)

A deployment file is a configuration file that allows defining or overriding variables from a blueprint. To prevent changes in the original blueprints, you can provide a deployment file using the `-d` parameter:

```bash
gcluster deploy -d my-cluster-deployment.yaml my-tpu-cluster.yaml
```

Consult the [TPU v6e Blueprint Reference](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples/gke-tpu-v6e/README.md#create-a-cluster-using-cluster-toolkit) for detailed guidance on using deployment files.

#### Deployment Artifacts

Running `gcluster deploy` transparently executes `gcluster create` automatically if the deployment folder does not exist. You only need to run `gcluster create` separately if you wish to inspect or customize the generated files prior to deployment.

When `gcluster create` executes, it expands your declarative blueprint into a dedicated deployment folder (e.g., `my-tpu-cluster/`). Within this, `gcluster` generates the following files and directories:
- **Deployment Group Folders** (e.g., `primary/`, `network/`, or custom group names): Subdirectories corresponding to the `deployment_groups` declared in your blueprint, containing the generated Terraform HCL code for each deployment phase.
- **`instructions.txt`**: A generated summary file providing customized CLI commands and post-deployment instructions for accessing and managing your cluster infrastructure.
- **Shared Terraform Dependencies**: Local copies or references to all Terraform modules and dependencies required by your deployment groups.
- **`.ghpc/`**: An internal metadata directory containing state tracking and the fully expanded blueprint definition (`.ghpc/artifacts/expanded_blueprint.yaml`).

---

## 5. Step 3: Workload Migration (migrating from `xpk workload create`)

Submit workloads in Cluster Toolkit using `gcluster job submit`.

### Compute Type Simplification

You can pass TPU shorthand directly into `--compute-type` (e.g., `--compute-type v6e-16` or `--compute-type v5e-8`). `gcluster` automatically resolves the machine type, calculates VMs per slice, and deduces the TPU topology without requiring an explicit `--topology` flag unless you are overriding defaults.

### 1:1 Flag Mapping Matrix

| XPK `workload create` Flag | `gcluster job submit` Flag | Notes / Differences |
| :--- | :--- | :--- |
| `--workload <NAME>` | `--name <NAME>` | Workload name (max 28 characters) |
| `--cluster <NAME>` | `--cluster <NAME>` | Target GKE cluster (optional if configured in `gcluster job config`) |
| `--project <ID>` | `--project <ID>` | Target Google Cloud project |
| `--zone <ZONE>` / `--region <REGION>` | `--location <LOC>` | Target cluster region or zone |
| `--docker-image <IMG>` | `--image <IMG>` | Container image URL |
| `--command "<CMD>"` | `--command "<CMD>"` | Main entrypoint command |
| `--tpu-type <TYPE>` / `--device-type <TYPE>` | `--compute-type <TYPE>` | Accepts shorthand directly (e.g. `v6e-16`, `v5e-8`) or GCE machine type (`ct6e-standard-4t`) |
| `--num-slices <N>` | `--num-slices <N>` | Number of TPU slices |
| `--num-nodes <N>` | `--num-nodes <N>` | **GPU/CPU jobs only**. Omit `--num-nodes` for TPU jobs |
| `--priority <PRIORITY>` | `--priority <PRIORITY>` | Kueue queue priority (`low`, `medium`, `high`) |
| `--wait-for-job-completion` | `--await-job-completion` | Blocks CLI until job finishes |
| `--env KEY=VAL` | `--env KEY=VAL` | Environment variables |
| `--storage <NAME>` | `--mount <SRC>;<DEST>[;<MODE>][;options=<OPTS>]` | Inline storage mount (`gs://`, `pvc://`, `filestore://`). Note: `gcluster` CLI defaults to `ro` (read-only) if mode is omitted; translated commands explicitly specify `;rw` to preserve XPK's read-write behavior. `options=` is supported exclusively for GCS volumes (`gs://`) |
| `--use-parallel-containers false` | `--gke-disable-parallel-containers` | Explicitly disables parallel containers on TPU v7/v7x hardware |
| `--service-account <SA>` | `--service-account <SA>` | Kubernetes service account name |
| `--max-restarts <N>` | `--restarts <N>` | Maximum JobSet restarts |
| `--ttl-seconds-after-finished <SEC>` | `--gke-ttl-after-finished <SEC>` | TTL after job completion |
| `--termination-grace-period-seconds <SEC>` | `--grace-period <SEC>` | Grace period before SIGKILL |
| `--base-docker-image <IMG>` + `--script-dir <DIR>` | `--base-image <IMG>` + `--build-context <DIR>` | Builds image on the fly (requires `GCLUSTER_IMAGE_REPO` env var) |

### Workload Migration Examples

#### A. Standard Workload Submission

```bash
# Previous XPK Command:
# xpk workload create \
#   --workload llama3-train \
#   --cluster my-tpu-cluster \
#   --project my-gcp-project \
#   --zone us-central1-a \
#   --tpu-type v6e-16 \
#   --docker-image us-docker.pkg.dev/my-project/my-repo/llama3:latest \
#   --command "python3 train.py --batch_size=32" \
#   --env LOG_LEVEL=DEBUG \
#   --priority high

# Cluster Toolkit Equivalent (Shorthand compute type):
gcluster job submit \
  --name llama3-train \
  --cluster my-tpu-cluster \
  --project my-gcp-project \
  --location us-central1-a \
  --image us-docker.pkg.dev/my-project/my-repo/llama3:latest \
  --command 'python3 train.py --batch_size=32' \
  --compute-type v6e-16 \
  --priority high \
  --env LOG_LEVEL=DEBUG
```

#### B. Inline Storage Mounting & Mount Options (`--mount`)

Replace separate `xpk storage attach` commands and storage options with inline `--mount` flags:

```bash
# Mount Cloud Storage with read-write mode and custom GCS Fuse options:
gcluster job submit \
  --name data-job \
  --cluster my-tpu-cluster \
  --project my-gcp-project \
  --location us-central1-a \
  --compute-type v6e-8 \
  --image us-docker.pkg.dev/my-project/my-repo/train:v1 \
  --command 'python3 train.py --data_dir=/mnt/gcs_data' \
  --mount 'gs://my-gcs-bucket/dataset;/mnt/gcs_data;rw;options=implicit-dirs' \
  --mount 'my-pvc-claim;/mnt/pvc_data;ro'
```

> [!IMPORTANT]
> The `options=` parameter is strictly validated by `gcluster` and is supported exclusively for Cloud Storage volumes (`gs://`). Appending options to non-GCS volumes (such as `filestore://` or PVCs) will result in a CLI validation error.

---

## 6. Step 4: Advanced Workloads & Features

### A. Parallel Containers on TPU 7x

For TPU v7 and TPU v7x hardware, parallel container execution is enabled by default to optimize multi-container pods per node. If your workload requires single-container pods per node, pass `--gke-disable-parallel-containers`:

```bash
gcluster job submit \
  --name single-container-tpu7 \
  --cluster my-tpu-cluster \
  --project my-gcp-project \
  --location us-central1-a \
  --compute-type tpu7x-128 \
  --image us-docker.pkg.dev/my-project/my-repo/train:v1 \
  --command 'python3 train.py' \
  --gke-disable-parallel-containers
```

### B. Pathways Workloads (`xpk workload create-pathways`)

Cluster Toolkit natively supports Pathways multi-host workloads via `--pathways`:

```bash
gcluster job submit \
  --pathways \
  --name pw-job \
  --cluster my-tpu-cluster \
  --project my-gcp-project \
  --location us-central1-a \
  --compute-type v5e-16 \
  --image us-docker.pkg.dev/my-project/my-repo/pw-app:v1 \
  --pathways-gcs-location gs://my-bucket/pathways-tmp \
  --pathways-headless
```

### C. Multi-Tier Checkpointing (MTC)

```bash
gcluster job submit \
  --name mtc-job \
  --cluster my-tpu-cluster \
  --project my-gcp-project \
  --location us-central1-a \
  --compute-type v6e-16 \
  --image us-docker.pkg.dev/my-project/my-repo/train:v1 \
  --command 'python3 train.py' \
  --gke-mtc-enabled \
  --gke-mtc-ramdisk-dir /tmp/ramdisk
```

---

## 7. XPK to Cluster Toolkit Command Mappings

| XPK Command | Cluster Toolkit (`gcluster`) Equivalent | Notes |
| :--- | :--- | :--- |
| `xpk cluster create` | `gcluster create <file.yaml> && gcluster deploy <name>` | Provision GKE cluster & TPU node pools |
| `xpk cluster delete` | `gcluster destroy <deployment_name>` | Tear down infrastructure |
| `xpk cluster list` | `gcluster cluster list` | List active clusters |
| `xpk cluster describe` | `gcluster cluster describe` | Describe cluster status |
| `xpk workload create` | `gcluster job submit` | Submits JobSet workload |
| `xpk workload create-pathways` | `gcluster job submit --pathways` | Submits Pathways workload |
| `xpk storage attach` | Inline `--mount` flag with `gcluster job submit` | Supports `gs://`, `pvc://`, `filestore://`. `options=` is GCS `gs://` exclusive |
| `xpk workload list` | `gcluster job list` | List active workloads |
| `xpk workload delete` | `gcluster job cancel <workload_name>` | Cancel running workload |
| `xpk inspector` | `gcluster job inspect` / `gcluster job logs <workload_name>` | Inspect cluster health and workload status, or stream logs |
| `xpk info` | `gcluster cluster info` | View cluster details |
| `xpk config set` | `gcluster job config set` | Update local CLI config (specifically project, cluster and location) |
| `xpk storage list` | `gcluster cluster volume` | View mounted volumes |
