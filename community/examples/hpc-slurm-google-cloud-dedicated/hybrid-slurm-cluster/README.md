# Multi-Cluster Slurm with Cloud Bursting on Google Cloud Dedicated (GCD)

This directory contains the blueprints, playbooks, test scripts, and architecture for demonstrating **Multi-Cluster Slurm with Elastic Cloud Bursting** between two autonomous projects in **Google Cloud Dedicated (GCD)** environments (e.g., `apis-<location>.goog`).

---

## 1. Experiment Setup & Architecture

The testbed simulates an enterprise customer's on-premises HPC cluster bursting into Google Cloud without requiring physical datacenter hardware. It deploys two independent Slurm clusters across two isolated GCD projects connected via high-performance VPC routing:

* **Primary Cluster (`primary-cluster`)**: Simulates the **On-Premises / Primary Cluster** (persistent head node, static compute nodes, and a standalone NFS server).
* **Burst Cluster (`burst-cluster`)**: Simulates the **Cloud Burst Target** in Google Cloud (lightweight head node, 0 static nodes, and dynamic compute nodes that autoscale from `0 -> N` on demand).

```text
====================================================================================================
                        PRIMARY CLUSTER: Primary / Simulated On-Premises (Project A)
====================================================================================================
  [<PROJECT_A_ID>] (VPC: primary-cluster-net, Subnet: 10.128.0.0/20)
  +---------------------------+                 +---------------------------------+
  |  Login Node               | --------------> |  Controller Node                |
  |  (primary-cluster-login)  |                 |  (primary-cluster-controller)   |
  |  User Submission Endpoint |                 |  slurmctld: 6817 | slurmdbd: 6819
  +---------------------------+                 +---------------------------------+
                :                                               |
                : (mounts NFS)                                  v
                :                               +---------------------------------+
                :                               |  Static Compute Nodes           |
                :                               |  (primary-cluster-compute-0, 1) |
                v                               +---------------------------------+
  +-------------------------------------------------------------+
  |  Shared Standalone NFS Storage Instance (10.128.0.2)
  |  Exports: /exports/home -> /home  &  /exports/opt/apps -> /opt/apps
  +-------------------------------------------------------------+
                ^
                | (Cross-Project VPC Peering: peer-b-to-a <---> peer-a-to-b)
================:===================================================================================
                :
                :       BURST CLUSTER: Elastic Cloud Burst Target (Project B)
================:===================================================================================
  [<PROJECT_B_ID>] (VPC: burst-cluster-net, Subnet: 10.130.0.0/20)
  +---------------------------+                 +---------------------------------+
  |  Login Node               | --------------> |  Controller Node                |
  |  (burst-cluster-login)    |                 |  (burst-cluster-controller)     |
  +---------------------------+                 |  slurmctld: 6817                |
                                                +---------------------------------+
                                                                |
                                                                | (Dynamic bulkInsert on demand)
                                                                v
                                                +---------------------------------+
                                                |  Dynamic Compute Nodes (0 -> N) |
                                                |  (burst-cluster-burst-0, 1, ...) |
                                                +---------------------------------+

====================================================================================================
                              CROSS-CLUSTER CONTROL & DATA LINK
====================================================================================================
  1. Mutual SAuth Trust  : Master /etc/slurm/slurm.key synchronized automatically via shared /home
  2. SlurmDBD Routing    : Bidirectional TCP 6819 via AccountingStorageExternalHost on both controllers
  3. Shared Home State   : Both clusters mount Primary Cluster's NFS for unified POSIX directories & logs
  4. Remote Dispatch     : Jobs submitted on Primary Cluster via 'sbatch -M burst --wrap="hostname"'
====================================================================================================
```

---

## 2. Key Differences from Commercial Google Cloud (GCP)

Deploying Multi-Cluster Slurm on **Google Cloud Dedicated (GCD)** partitions requires specific architectural adaptations compared to standard commercial GCP:

| Feature / Consideration | Google Cloud Dedicated (GCD) | Standard Commercial GCP |
| :--- | :--- | :--- |
| **API Domain** | Private universe domain (e.g. `apis-<location>.goog`) | Standard public `googleapis.com` |
| **Authentication** | Workforce Identity Federation (WIF) & `wif-login-config.json` | Standard GCP IAM / User / Service Account |
| **Shared Storage Fabric** | **Standalone NFS VM** (Hyperdisk Balanced) | **Google Cloud Filestore** (`BASIC_SSD`, 2.5TB min) |
| **Slurm OS Images** | Custom Rocky Linux images copied / built via Packer | Public SchedMD family (`schedmd-slurm-public`) |
| **Machine Config Metadata**| Requires offline `GHPC_MOCK_MACHINE_CONFIG` export | Directly resolved via GCP Compute API |
| **Compute Batch API** | Handled natively via `ThreadPoolExecutor` (PR #6144) | Standard `new_batch_http_request()` works natively |
| **User Identity** | Local `/home` keygen & non-OSLogin SSH synchronization | Google Cloud OS Login integration |

---

## 3. Prerequisites & Network Setup

Before deploying the clusters, complete the following environment and network setup:

### A. Environment & Workforce Identity Federation (WIF)
GCD uses private universe domains (e.g., `apis-<location>.goog`). Set up your shell environment:

```bash
# Activate your gcloud configuration for GCD
gcloud config configurations activate <YOUR_GCD_CONFIG>
gcloud auth login --login-config=wif-login-config.json
gcloud auth application-default login --login-config=wif-login-config.json

# Export required environment variables
export GOOGLE_CLOUD_UNIVERSE_DOMAIN="apis-<location>.goog"
export CLOUDSDK_UNIVERSE_DOMAIN="apis-<location>.goog"

# Mock machine specs for Cluster Toolkit offline expansion in GCD:
export GHPC_MOCK_MACHINE_CONFIG='{"cpus": {"c3-standard-176": {"count": 176, "memoryMb": 720896}, "c3-standard-4": {"count": 4, "memoryMb": 16384}}, "gpus": {}, "tpus": {}}'
```

### B. Network Fabric & Overlapping Subnet Resolution
In Google Cloud Dedicated (GCD), projects do not have auto-created default networks by design. Furthermore, cross-project VPC Peering requires non-overlapping subnet CIDRs.

To ensure deterministic, conflict-free routing without requiring manual network creation commands, the Cluster Toolkit blueprints automate network provisioning via the embedded `modules/network/vpc` module:
* **Primary Cluster (`primary-cluster.yaml`)**: Provisions `primary-cluster-net` with subnet `10.128.0.0/20` and firewall rules permitting internal cross-cluster traffic.
* **Burst Cluster (`burst-cluster.yaml`)**: Provisions `burst-cluster-net` in a dedicated `setup` deployment group with non-overlapping subnet `10.130.0.0/20` and peering firewall rules.

> [!NOTE]
> **Pre-existing VPC Networks in Enterprise GCD Landing Zones**:
> In enterprise GCD environments where networking is managed centrally via Fabric FAST or CFT Shared VPCs, you can deploy into pre-existing networks instead. Simply replace `modules/network/vpc` with `modules/network/pre-existing-vpc` in the blueprints and configure your network and subnetwork names.

### C. Cross-Project Custom Slurm Image Sharing
If reusing a pre-baked Slurm Rocky Linux image across projects:

```bash
gcloud compute images copy rocky-linux-8-optimized-gcp-v20240129 \
  --source-project=<PROJECT_B_ID> \
  --destination-project=<PROJECT_A_ID> \
  --destination-image=rocky-linux-8-optimized-gcp-v20240129
```

*(Or reference `source_image_project: <IMAGE_PROJECT>`, `source_image_family: rocky-linux-8-optimized-gcp`).*

---

## 4. Deployment & Execution Step-by-Step

> [!IMPORTANT]
> **Strict Deployment Order (Primary Cluster before Burst Cluster)**:
>
> Primary Cluster **must be deployed first and reach running state** before Burst Cluster is provisioned.
> Primary Cluster provisions its VPC network (`primary-cluster-net`), the shared NFS server VM (`/home`), generates the master SAuth encryption key (`slurm.key`), and writes its internal IP (`primary_cluster_ctrl_ip`). Burst Cluster requires Primary Cluster's NFS Server IP (`NFS_IP`) to mount `/home` and bootstrap on boot.

### One-Time Setup: Build Custom Slurm OS Image with Packer

In Google Cloud Dedicated (GCD), public SchedMD marketplace images are not available. A custom Slurm VM image (Rocky Linux 8 with Slurm v6) must be built inside the project using the embedded `build-slurm` Packer deployment group in `primary-cluster.yaml`.

> [!NOTE]
> **This build step is only required ONCE per project/environment.**
>
> Once the image is baked into family `slurm-c3-rocky8`:
>
> * It is permanently saved in your project and can be reused indefinitely across Primary Cluster and Burst Cluster.
> * Standard cluster deployments only need to deploy `--only setup,cluster`, skipping the ~20-minute Packer build.
> * If you are reusing a pre-baked Slurm image or copied one from another project (see Section 3.C), you can skip this step entirely.

To build the Slurm image:

```bash
# 1. Create deployment definition for Primary Cluster:
./gcluster create community/examples/hpc-slurm-google-cloud-dedicated/hybrid-slurm-cluster/primary-cluster.yaml

# 2. Deploy VPC network and build scripts:
./gcluster deploy primary-cluster --only setup --auto-approve

# 3. Build the custom Slurm OS image with Packer (takes ~20-25 minutes):
./gcluster deploy primary-cluster --only build-slurm --auto-approve
```

Once the Packer build completes, the image is registered under family `slurm-c3-rocky8` in your project and is ready for cluster provisioning.

---

### Step 1: Deploy Primary Cluster (Project A)

Configure the `vars` block in `community/examples/hpc-slurm-google-cloud-dedicated/hybrid-slurm-cluster/primary-cluster.yaml` with your project parameters:

```yaml
vars:
  project_id: <PROJECT_A_ID>
  deployment_name: primary-cluster
  region: <REGION>
  zone: <ZONE>
  universe_domain: apis-<location>.goog
  service_account_email: <PROJECT_A_NUMBER>-compute@developer.gserviceaccount.com
  authorized_ssh_ranges: ["<CLIENT_IP_OR_CIDR>"] # e.g. ["x.x.x.x/32"], or ["0.0.0.0/0"] for open access
```

Navigate to your Cluster Toolkit directory:

```bash
# 1. Create deployment definition for Primary Cluster:
./gcluster create community/examples/hpc-slurm-google-cloud-dedicated/hybrid-slurm-cluster/primary-cluster.yaml

# 2. Deploy Primary Cluster infrastructure (VPC network, NFS, and Slurm nodes):
#    Since the Slurm image was already built (or copied), deploy only setup and cluster:
./gcluster deploy primary-cluster --auto-approve --only setup,cluster
```

---

### Step 2: Extract Primary Cluster NFS Server IP

Once Primary Cluster finishes deploying, retrieve the internal IP of the standalone NFS Server VM:

```bash
NFS_IP=$(gcloud compute instances list \
  --project=<PROJECT_A_ID> \
  --filter="name ~ primary-cluster AND name ~ nfs" \
  --format="value(networkInterfaces[0].networkIP)")
echo "Primary Cluster NFS Server IP: ${NFS_IP}"
```

> [!NOTE]
> Unlike commercial GCP (where Primary Cluster's Controller VM acts as the NFS bridge), GCD provisions a dedicated, standalone NFS server instance (`NFS_IP`). Primary Cluster's Controller IP does not need to be manually configured because Primary Cluster automatically writes its internal IP to `/home/.primary_cluster_ctrl_ip`, which Burst Cluster's controller discovers dynamically on boot after mounting `/home`.

---

### Step 3: Deploy Burst Cluster Network (Project B)

Configure the `vars` block in `community/examples/hpc-slurm-google-cloud-dedicated/hybrid-slurm-cluster/burst-cluster.yaml` with your project parameters and the `NFS_IP` from Step 2:

```yaml
vars:
  project_id: <PROJECT_B_ID>
  deployment_name: burst-cluster
  region: <REGION>
  zone: <ZONE>
  universe_domain: apis-<location>.goog
  service_account_email: <PROJECT_B_NUMBER>-compute@developer.gserviceaccount.com
  authorized_ssh_ranges: ["<CLIENT_IP_OR_CIDR>"] # e.g. ["x.x.x.x/32"], or ["0.0.0.0/0"] for open access
  nfs_server_ip: <NFS_IP> # Set to Primary Cluster NFS Server IP
```

Create the deployment files and provision **only** Burst Cluster's VPC network and firewalls (`setup` group):

```bash
# 1. Create deployment definition for Burst Cluster:
./gcluster create community/examples/hpc-slurm-google-cloud-dedicated/hybrid-slurm-cluster/burst-cluster.yaml

# 2. Deploy ONLY Burst Cluster's VPC network and firewalls:
./gcluster deploy burst-cluster --only setup --auto-approve
```

---

### Step 4: Establish Bidirectional VPC Peering

Now that both VPC networks exist (`primary-cluster-net` in Project A and `burst-cluster-net` in Project B), establish bidirectional VPC Network Peering:

> [!IMPORTANT]
> **Bidirectional VPC Peering is Strictly Required**:
>
> Even if jobs are only dispatched in one direction (Primary Cluster $\rightarrow$ Burst Cluster), VPC Peering **must be established in both directions** (`peer-b-to-a` and `peer-a-to-b`). Burst Cluster requires network routing back to Primary Cluster to mount NFS `/home`, read the master SAuth key, and exchange SlurmDBD accounting RPCs.

```bash
# 1. From Project B -> Project A:
gcloud compute networks peerings create peer-b-to-a \
  --network=burst-cluster-net \
  --peer-project=<PROJECT_A_ID> \
  --peer-network=primary-cluster-net \
  --project=<PROJECT_B_ID>

# 2. From Project A -> Project B:
gcloud compute networks peerings create peer-a-to-b \
  --network=primary-cluster-net \
  --peer-project=<PROJECT_B_ID> \
  --peer-network=burst-cluster-net \
  --project=<PROJECT_A_ID>
```

---

### Step 5: Deploy Burst Cluster Slurm Compute Nodes

Once peering is active, deploy Burst Cluster's Slurm cluster (`cluster` group). The Slurm nodes boot, reach Primary Cluster's NFS storage over the peering tunnel, and join the multi-cluster environment:

```bash
# Deploy Burst Cluster Slurm compute nodes and controller:
./gcluster deploy burst-cluster --only cluster --auto-approve
```

---

### Step 6: Verify SlurmDBD Auto-Registration on Primary Cluster

In order to submit jobs from Primary Cluster to Burst Cluster, **no explicit manual steps or cross-registration commands are required**. All SlurmDBD registration and inter-cluster cryptographic trust are handled automatically by the cluster blueprints:

* Primary Cluster exports its master `slurm.key` and controller IP to the shared NFS home directory.
* When Burst Cluster boots, its startup script mounts the shared NFS, imports the matching `slurm.key`, and points to Primary Cluster's SlurmDBD.
* Burst Cluster's controller automatically registers itself into Primary Cluster's SlurmDBD during its initialization.

After both clusters finish Slurm initialization (typically 3–5 minutes after deployment), Primary Cluster will see both clusters without any manual intervention.

#### Check Registered Clusters on Primary Cluster Login Node

SSH to `primary-cluster-slurm-login-001` (or your active Primary Cluster login node):

```bash
gcloud compute ssh primary-cluster-slurm-login-001 --zone=<ZONE> --project=<PROJECT_A_ID>

sudo /usr/local/bin/sacctmgr list clusters
```

*Expected Output:*

```text
   Cluster     ControlHost  ControlPort   RPC     Share GrpJobs       GrpTRES GrpSubmit MaxJobs       MaxTRES MaxSubmit     MaxWall                  QOS   Def QOS
---------- --------------- ------------ ----- --------- ------- ------------- --------- ------- ------------- --------- ----------- -------------------- ---------
primary         10.128.0.x         6818 11264         1                                                                                           normal
burst           10.130.0.x         6818 11264
```

Both clusters are immediately registered with their internal `ControlHost` IPs, control ports (`6818`), and active RPC versions (`11264`), ready for cross-cluster job dispatch.

---

### Step 7: Execute and Monitor Cross-Cluster Bursting Jobs

#### 1. Local Smoke Test on Burst Cluster Login Node

SSH to `burst-cluster-slurm-login-001` to verify partition availability and local provisioning:

```bash
gcloud compute ssh burst-cluster-slurm-login-001 --zone=<ZONE> --project=<PROJECT_B_ID>
```

Inside the session, run:

```bash
sinfo
srun -N1 hostname
```

#### 2. Remote Job Dispatch from Primary Cluster Login Node

SSH to `primary-cluster-slurm-login-001`:

```bash
gcloud compute ssh primary-cluster-slurm-login-001 --zone=<ZONE> --project=<PROJECT_A_ID>
```

##### a. Check Local and Remote Cluster Status

```bash
# Check local cluster partition:
sinfo

# Check remote burst cluster partition:
sinfo -M burst
```

##### b. Submit Remote Job to Dynamically Burst Nodes on Burst Cluster

```bash
sbatch -M burst --wrap="hostname"
```

##### c. Monitor Remote Job and Node Lifecycle

```bash
squeue -M burst
```

*Job Lifecycle & Node Provisioning Progression:*
1. **`CF` (Configuring)**: Slurm executes `resume.py`, issuing asynchronous GCE Compute API requests in Project B to dynamically power on `burst-cluster-burst_b_nodeset-[0-1]`.
2. **`R` (Running)**: Compute nodes boot, mount Primary Cluster's shared `/home` over the VPC, execute the job, and write outputs directly to `/home/$USER/slurm-<JOB_ID>.out`.
3. **`CG` (Completing)**: Job finishes and nodes return to `idle`.
4. **`idle~` (Power Down)**: After `SuspendTimeout` (default 600s), `suspend.py` issues API deletion calls to delete the VMs, returning idle compute cost to $0.

##### d. Direct Interactive Remote Execution

```bash
srun -M burst -N1 hostname
```

#### 3. Verify Output & Cross-Project Parity

```bash
cat ~/slurm-*.out
```

*Expected Output:*

```text
burst-cluster-burst_b_nodeset-0
```

---

## 5. Key Architectural Details & Platform Adaptations

The blueprints (`primary-cluster.yaml` and `burst-cluster.yaml`) are specifically architected for Google Cloud Dedicated (GCD) sovereign environments:

### A. GCD-Specific Platform Adaptations
1. **`BatchHttpRequest` Private Universe Domain Handling (`/slurm/scripts/util.py`)**:
   * In GCD private universe domains (e.g., `apis-<location>.goog`), standard Google API batch execution (`BatchHttpRequest`) is unsupported.
   * Turnkey support for private universe domains using `ThreadPoolExecutor` is natively built into Cluster Toolkit's Slurm v6 controller scripts (merged in PR #6144), eliminating the need for runtime script patching.

2. **Deterministic POSIX UID Hashing & Non-OSLogin SSH Synchronization**:
   * In GCD environments where OS Login is disabled (`enable_oslogin: false`), sequential `useradd` creates UID/GID drift across clusters.
   * The startup scripts automatically hash usernames to deterministic numeric UIDs (`USER_UID=$((2000 + UID_HASH % 58000))`) and use `flock` on shared NFS `/home` to safely generate shared SSH keypairs without race conditions.

3. **Standalone NFS Server Architecture**:
   * Instead of managed services like Google Cloud Filestore, Primary Cluster provisions a dedicated standalone NFS Server VM (`modules/file-system/nfs-server`) backed by Hyperdisk Balanced to serve `/home` across both clusters over VPC peering.

### B. Cross-Cluster SAuth & Bursting Mechanism
* **Synchronous SAuth Key Propagation**:
  * Eliminates cross-project race conditions by exporting `slurm.key` from Primary Cluster directly to `/home/.shared_slurm.key` and having Burst Cluster import it synchronously on controller/login boot.
