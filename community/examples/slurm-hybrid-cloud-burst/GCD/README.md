# Multi-Cluster Slurm with Cloud Bursting on Google Cloud Dedicated (GCD)

This directory contains the blueprints, playbooks, test scripts, and architecture for demonstrating **Multi-Cluster Slurm with Elastic Cloud Bursting** between two autonomous projects in **Google Cloud Dedicated (GCD)** sovereign environments (e.g., `apis-berlin-build0.goog`).

---

## 1. Experiment Setup & Architecture

The testbed simulates an enterprise customer's on-premises HPC cluster bursting into Google Cloud without requiring physical datacenter hardware. It deploys two independent Slurm clusters across two isolated GCD projects connected via high-performance VPC routing:

* **Cluster A (`clustera`)**: Simulates the **On-Premises / Primary Cluster** (persistent head node, static compute nodes, and a standalone NFS server).
* **Cluster B (`clusterb`)**: Simulates the **Cloud Burst Target** in Google Cloud (lightweight head node, 0 static nodes, and dynamic compute nodes that autoscale from `0 -> N` on demand).

```text
====================================================================================================
                        CLUSTER A: Primary / Simulated On-Premises (Project A)
====================================================================================================
  [<PROJECT_A_ID>] (VPC: default, Subnet: 10.128.0.0/20)
  +---------------------------+                 +---------------------------------+
  |  Login Node               | --------------> |  Controller Node                |
  |  (clustera3-login)        |                 |  (clustera3-controller)         |
  |  User Submission Endpoint |                 |  slurmctld: 6817 | slurmdbd: 6819
  +---------------------------+                 +---------------------------------+
                :                                               |
                : (mounts NFS)                                  v
                :                               +---------------------------------+
                :                               |  Static Compute Nodes           |
                :                               |  (clustera3-compute-0, 1)       |
                v                               +---------------------------------+
  +-------------------------------------------------------------+
  |  Shared Standalone NFS Storage Instance (10.128.0.2)
  |  Exports: /exports/home -> /home  &  /exports/opt/apps -> /opt/apps
  +-------------------------------------------------------------+
                ^
                | (Cross-Project VPC Peering: peer-v4-to-v3 <---> peer-v3-to-v4)
================:===================================================================================
                :
                :       CLUSTER B: Elastic Cloud Burst Target (Project B)
================:===================================================================================
  [<PROJECT_B_ID>] (VPC: clusterb-net, Subnet: 10.130.0.0/20)
  +---------------------------+                 +---------------------------------+
  |  Login Node               | --------------> |  Controller Node                |
  |  (clusterb2-login)        |                 |  (clusterb2-controller)         |
  +---------------------------+                 |  slurmctld: 6817                |
                                                +---------------------------------+
                                                                |
                                                                | (Dynamic bulkInsert on demand)
                                                                v
                                                +---------------------------------+
                                                |  Dynamic Compute Nodes (0 -> N) |
                                                |  (clusterb2-burst-0, 1, ...)    |
                                                +---------------------------------+

====================================================================================================
                              CROSS-CLUSTER CONTROL & DATA LINK
====================================================================================================
  1. Mutual SAuth Trust  : Master /etc/slurm/slurm.key synchronized automatically via shared /home
  2. SlurmDBD Routing    : Bidirectional TCP 6819 via AccountingStorageExternalHost on both controllers
  3. Shared Home State   : Both clusters mount Cluster A's NFS for unified POSIX directories & logs
  4. Remote Dispatch     : Jobs submitted on Cluster A via 'sbatch -M clusterb2 --wrap="hostname"'
====================================================================================================
```

---

## 2. Key Differences from Commercial Google Cloud (GCP)

Deploying Multi-Cluster Slurm on **Google Cloud Dedicated (GCD)** sovereign partitions requires specific architectural adaptations compared to standard commercial GCP:

| Feature / Consideration | Sovereign GCD (`GCD/`) | Standard Commercial GCP |
| :--- | :--- | :--- |
| **API Domain** | Sovereign private (e.g. `apis-berlin-build0.goog`) | Standard public `googleapis.com` |
| **Authentication** | Workforce Identity Federation (WIF) & `wif-login-config.json` | Standard GCP IAM / User / Service Account |
| **Shared Storage Fabric** | **Standalone NFS VM** (Hyperdisk Balanced) | **Google Cloud Filestore** (`BASIC_SSD`, 2.5TB min) |
| **Slurm OS Images** | Custom Rocky Linux images copied / built via Packer | Public SchedMD family (`schedmd-slurm-public`) |
| **Machine Config Metadata**| Requires offline `GHPC_MOCK_MACHINE_CONFIG` export | Directly resolved via GCP Compute API |
| **Compute Batch API** | Requires `ThreadPoolExecutor` patch in `util.py` | Standard `new_batch_http_request()` works natively |
| **User Identity** | Local `/home` keygen & non-OSLogin SSH synchronization | Google Cloud OS Login integration |

---

## 3. Prerequisites & Network Setup

Before deploying the clusters, complete the following environment and network setup:

### A. Environment & Workforce Identity Federation (WIF)
GCD uses private universe domains (e.g., `apis-berlin-build0.goog`). Set up your shell environment:

```bash
# Activate your gcloud configuration for GCD
gcloud config configurations activate <YOUR_GCD_CONFIG>
gcloud auth login --login-config=wif-login-config.json
gcloud auth application-default login --login-config=wif-login-config.json

# Export required environment variables
export GOOGLE_CLOUD_UNIVERSE_DOMAIN="apis-berlin-build0.goog"
export CLOUDSDK_UNIVERSE_DOMAIN="apis-berlin-build0.goog"

# Mock machine specs for Cluster Toolkit offline expansion in GCD:
export GHPC_MOCK_MACHINE_CONFIG='{"cpus": {"c3-standard-176": {"count": 176, "memoryMb": 720896}, "c3-standard-4": {"count": 4, "memoryMb": 16384}}, "gpus": {}, "tpus": {}}'
```

### B. Network Fabric & Overlapping Subnet Resolution
> [!IMPORTANT]
> In GCP, default auto-mode VPC networks in both projects use the **same default subnet CIDR (`10.128.0.0/20`)**. VPC Peering **will fail** if subnets overlap!
> To resolve this, Project B must use a custom VPC network with a non-overlapping range (e.g., `10.130.0.0/20`).

```bash
# 1. Create custom VPC network in Project B:
gcloud compute networks create clusterb-net \
  --subnet-mode=custom \
  --project=<PROJECT_B_ID>

# 2. Create non-overlapping subnet (10.130.0.0/20):
gcloud compute networks subnets create clusterb-subnet \
  --network=clusterb-net \
  --region=u-germany-northeast1 \
  --range=10.130.0.0/20 \
  --project=<PROJECT_B_ID>

# 3. Enable Private Google Access on both subnets:
gcloud compute networks subnets update clusterb-subnet \
  --region=u-germany-northeast1 \
  --enable-private-ip-google-access \
  --project=<PROJECT_B_ID>

gcloud compute networks subnets update default \
  --region=u-germany-northeast1 \
  --enable-private-ip-google-access \
  --project=<PROJECT_A_ID>

# 4. Add firewall rules to clusterb-net:
gcloud compute firewall-rules create clusterb-allow-internal \
  --network=clusterb-net \
  --allow=tcp:1-65535,udp:1-65535,icmp \
  --source-ranges=10.128.0.0/9 \
  --project=<PROJECT_B_ID>

gcloud compute firewall-rules create clusterb-allow-ssh \
  --network=clusterb-net \
  --allow=tcp:22 \
  --project=<PROJECT_B_ID>
```

### C. Establish Bidirectional VPC Peering

> [!IMPORTANT]
> **Bidirectional VPC Peering is Strictly Required**:
> Even if jobs are only dispatched in one direction (Cluster A $\rightarrow$ Cluster B), VPC Peering **must be established in both directions** (`peer-b-to-a` and `peer-a-to-b`). Cluster B requires network routing back to Cluster A to mount NFS `/home`, read the master SAuth key, and exchange SlurmDBD accounting RPCs.

```bash
# 1. From Project B -> Project A:
gcloud compute networks peerings create peer-b-to-a \
  --network=clusterb-net \
  --peer-project=<PROJECT_A_ID> \
  --peer-network=default \
  --project=<PROJECT_B_ID>

# 2. From Project A -> Project B:
gcloud compute networks peerings create peer-a-to-b \
  --network=default \
  --peer-project=<PROJECT_B_ID> \
  --peer-network=clusterb-net \
  --project=<PROJECT_A_ID>
```

### D. Cross-Project Custom Slurm Image Sharing
If reusing a pre-baked Slurm Rocky Linux image across projects:

```bash
gcloud compute images copy rocky-linux-8-optimized-gcp-v20240129 \
  --source-project=<PROJECT_B_ID> \
  --destination-project=<PROJECT_A_ID> \
  --destination-image=rocky-linux-8-optimized-gcp-v20240129
```

*(Or reference `source_image_project: eu0-system:rocky-linux-cloud`, `source_image_family: rocky-linux-8-optimized-gcp`).*

---

## 4. Deployment & Execution Step-by-Step

> [!IMPORTANT]
> **Strict Deployment Order (Cluster A before Cluster B)**:
> Cluster A **must be deployed first and reach running state** before Cluster B is provisioned.
> Cluster A provisions the shared NFS server VM (`/home`), generates the master SAuth encryption key (`slurm.key`), and writes its internal IP (`cluster_a_ctrl_ip`). Cluster B requires Cluster A's NFS Server IP (`NFS_IP`) and Controller IP (`CTRL_A_IP`) to mount `/home` and bootstrap on boot.

### Step 1: Deploy Cluster A (Primary in Project A)

Navigate to your Cluster Toolkit directory:

```bash
# 1. Create deployment definition for Cluster A:
./gcluster create community/examples/slurm-hybrid-cloud-burst/GCD/clustera.yaml \
  --vars "\
project_id=<PROJECT_A_ID>,\
deployment_name=cluster-a3,\
region=u-germany-northeast1,\
zone=u-germany-northeast1-a,\
universe_domain=apis-berlin-build0.goog,\
service_account_email=<PROJECT_A_NUMBER>-compute@developer.eu0-system.iam.gserviceaccount.com,\
source_image_project=eu0-system:rocky-linux-cloud,\
source_image_family=rocky-linux-8-optimized-gcp" \
  -w --force --validation-level=IGNORE

# 2. Deploy Cluster A infrastructure
./gcluster deploy cluster-a3 --auto-approve --only setup,cluster
```

---

### Step 2: Extract Cluster A NFS & Controller IPs

Once Cluster A finishes deploying, retrieve the internal IPs:

```bash
# 1. Get NFS Server IP:
NFS_IP=$(gcloud compute instances list \
  --project=<PROJECT_A_ID> \
  --filter="name ~ nfs-instance" \
  --format="value(networkInterfaces[0].networkIP)")
echo "Cluster A NFS Server IP: ${NFS_IP}"

# 2. Get Controller IP:
CTRL_A_IP=$(gcloud compute instances list \
  --project=<PROJECT_A_ID> \
  --filter="name ~ controller" \
  --format="value(networkInterfaces[0].networkIP)")
echo "Cluster A Controller IP: ${CTRL_A_IP}"
```

---

### Step 3: Deploy Cluster B (Cloud Burst Target in Project B)

Deploy Cluster B in `<PROJECT_B_ID>`, connecting to `clusterb-net` and passing `NFS_IP`:

```bash
./gcluster create community/examples/slurm-hybrid-cloud-burst/GCD/clusterb.yaml \
  --vars "\
project_id=<PROJECT_B_ID>,\
deployment_name=cluster-b2,\
region=u-germany-northeast1,\
zone=u-germany-northeast1-a,\
universe_domain=apis-berlin-build0.goog,\
service_account_email=<PROJECT_B_NUMBER>-compute@developer.eu0-system.iam.gserviceaccount.com,\
source_image_project=eu0-system:rocky-linux-cloud,\
source_image_family=rocky-linux-8-optimized-gcp,\
nfs_server_ip=${NFS_IP},\
clustera_controller_ip=${CTRL_A_IP}" \
  -w --force --validation-level=IGNORE

# Deploy Cluster B infrastructure
./gcluster deploy cluster-b2 --auto-approve
```

---

### Step 4: Verify SlurmDBD Auto-Registration on Cluster A

In order to submit jobs from Cluster A to Cluster B, **no explicit manual steps or cross-registration commands are required**. All SlurmDBD registration and inter-cluster cryptographic trust are handled automatically by the cluster blueprints:
* Cluster A exports its master `slurm.key` and controller IP to the shared NFS home directory.
* When Cluster B boots, its startup script mounts the shared NFS, imports the matching `slurm.key`, and points to Cluster A's SlurmDBD.
* Cluster B's controller automatically registers itself into Cluster A's SlurmDBD during its initialization.

After both clusters finish Slurm initialization (typically 3–5 minutes after deployment), Cluster A will see both clusters without any manual intervention.

#### Check Registered Clusters on Cluster A Login Node

SSH to `clustera3-slurm-login-001` (or your active Cluster A login node):

```bash
gcloud compute ssh clustera3-slurm-login-001 --zone=u-germany-northeast1-a --project=<PROJECT_A_ID>

sudo /usr/local/bin/sacctmgr list clusters
```

*Expected Output:*

```text
   Cluster     ControlHost  ControlPort   RPC     Share GrpJobs       GrpTRES GrpSubmit MaxJobs       MaxTRES MaxSubmit     MaxWall                  QOS   Def QOS 
---------- --------------- ------------ ----- --------- ------- ------------- --------- ------- ------------- --------- ----------- -------------------- --------- 
clustera3g     10.128.0.x          6818 11264         1                                                                                           normal           
clusterb2g     10.130.0.x          6818 11264                                                                                                                      
```

Both clusters are immediately registered with their internal `ControlHost` IPs, control ports (`6818`), and active RPC versions (`11264`), ready for cross-cluster job dispatch.

---

### Step 5: Execute and Monitor Cross-Cluster Bursting Jobs

#### 1. Health Checks on Cluster B Login Node

SSH to `clusterb2-slurm-login-001` and verify local partition and hostname test:

```bash
gcloud compute ssh clusterb2-slurm-login-001 --zone=u-germany-northeast1-a --project=<PROJECT_B_ID>
```

Inside the session, run:

```bash
sinfo
srun -N1 hostname
```

#### 2. Remote Job Dispatch from Cluster A Login Node

SSH to `clustera3-slurm-login-001`:

```bash
gcloud compute ssh clustera3-slurm-login-001 --zone=u-germany-northeast1-a --project=<PROJECT_A_ID>
```

##### a. Check Local and Remote Cluster Status

```bash
# Check local cluster partition:
sinfo

# Check remote burst cluster partition:
sinfo -M clusterb2
```

##### b. Submit Remote Job to Dynamically Burst Nodes on Cluster B

```bash
sbatch -M clusterb2 --wrap="hostname"
```

##### c. Monitor Remote Job and Node Lifecycle

```bash
squeue -M clusterb2
```

*Job Lifecycle & Node Provisioning Progression:*
1. **`CF` (Configuring)**: Slurm executes `resume.py`, issuing asynchronous GCE Compute API requests in Project B to dynamically power on `clusterb2-burstbnodeset-[0-1]`.
2. **`R` (Running)**: Compute nodes boot, mount Cluster A's shared `/home` over the VPC, execute the job, and write outputs directly to `/home/$USER/slurm-<JOB_ID>.out`.
3. **`CG` (Completing)**: Job finishes and nodes return to `idle`.
4. **`idle~` (Power Down)**: After `SuspendTimeout` (default 600s), `suspend.py` issues API deletion calls to delete the VMs, returning idle compute cost to $0.

##### d. Direct Interactive Remote Execution

```bash
srun -M clusterb2 -N1 hostname
```

#### 3. Verify Output & Cross-Project Parity

```bash
cat ~/slurm-*.out
```

*Expected Output:*

```text
clusterb2-burstbnodeset-0
```

---

## 5. Key GCD Patches & Adaptations (Changes from GCP Blueprints)

The GCD blueprints (`GCD/clustera.yaml` and `GCD/clusterb.yaml`) adapt the standard commercial GCP blueprints to run in private sovereign cloud environments:

### A. GCD-Specific Platform Adaptations
1. **`BatchHttpRequest` Private Universe Hang Fix (`/slurm/scripts/util.py`)**:
   * In private sovereign clouds (`apis-berlin-build0.goog`), `googleapiclient` hardcodes batch requests to public `https://compute.googleapis.com/batch/compute/v1`. This causes `suspend.py` and `slurmsync.py` to hang indefinitely.
   * The startup script patches `util.py` to replace `BatchHttpRequest` with Python's native `ThreadPoolExecutor`, executing requests concurrently against the regional sovereign endpoint.

2. **State Disk Mount Idempotency Fix (`/slurm/scripts/setup.py`)**:
   * Fixes setup failure on reboot when `/var/spool/slurm` state disk is already mounted (`mountpoint -q {mount_point} || mount {mount_point}`).

3. **Deterministic POSIX UID Hashing & Non-OSLogin SSH Synchronization**:
   * In sovereign GCD environments where OS Login is disabled (`enable_oslogin: false`), sequential `useradd` creates UID/GID drift across clusters.
   * The startup scripts automatically hash usernames to deterministic numeric UIDs (`USER_UID=$((2000 + UID_HASH % 58000))`) and use `flock` on shared NFS `/home` to safely generate shared SSH keypairs without race conditions.

4. **Standalone NFS Server Architecture**:
   * Commercial GCP uses Google Cloud Filestore (`BASIC_SSD`). In GCD, Cluster A provisions a standalone NFS Server VM (`modules/file-system/nfs-server`) backed by Hyperdisk Balanced.

### B. Shared Multi-Cluster Bursting Mechanism (Common to GCP & GCD)
* **Synchronous SAuth Key Propagation**:
  * Eliminates cross-project race conditions by exporting `slurm.key` from Cluster A directly to `/home/shared_slurm.key` and having Cluster B import it synchronously on controller/login boot.
