# Multi-Cluster Slurm with Cloud Bursting on Google Cloud Platform (GCP)

This directory contains the blueprints, playbooks, and deployment instructions for running **Multi-Cluster Slurm with Elastic Cloud Bursting** between two independent projects in standard **Google Cloud Platform (GCP)** using **C3 machine instances** (`c3-standard-176`).

---

## 1. Experiment Setup & Architecture

The testbed deploys two autonomous Slurm clusters across two isolated GCP projects connected via VPC Network Peering:

* **Primary Cluster (`primary-cluster`)**: Simulates the **Primary / On-Premises Cluster** in **Project A** (persistent login node, persistent controller, static compute nodes, and managed Google Cloud Filestore for `/home`).
* **Burst Cluster (`burst-cluster`)**: Simulates the **Cloud Burst Target** in **Project B** (lightweight login node, lightweight controller, 0 static nodes, and dynamic compute nodes that autoscale from `0 -> 16` on demand).

```text
====================================================================================================
                        PRIMARY CLUSTER: Primary Cluster (Project A)
====================================================================================================
  [Project A] (VPC: primary-cluster-net, Subnet: 10.128.0.0/20)
  +---------------------------+                 +---------------------------------+
  |  Login Node               | --------------> |  Controller Node                |
  |  (primary-cluster-login)  |                 |  (primary-cluster-controller)   |
  |  User Submission Endpoint |                 |  slurmctld: 6817 | slurmdbd: 6819
  +---------------------------+                 +---------------------------------+
                :                                               |
                : (mounts Filestore)                            v
                :                               +---------------------------------+
                :                               |  Static Compute Nodes           |
                :                               |  (primary-cluster-compute-0, 1) |
                v                               +---------------------------------+
  +-------------------------------------------------------------+
  |  Google Cloud Filestore (10.128.0.x)                        |
  |  Share: /homeshare -> Mounted at /home                      |
  +-------------------------------------------------------------+
                ^
                | (VPC Network Peering: peer-a-to-b <---> peer-b-to-a)
================:===================================================================================
                :
                :       BURST CLUSTER: Elastic Cloud Burst Target (Project B)
================:===================================================================================
  [Project B] (VPC: burst-cluster-net, Subnet: 10.130.0.0/20)
  +---------------------------+                 +---------------------------------+
  |  Login Node               | --------------> |  Controller Node                |
  |  (burst-cluster-login)    |                 |  (burst-cluster-controller)     |
  +---------------------------+                 |  slurmctld: 6817                |
                                                +---------------------------------+
                                                                |
                                                                | (Dynamic bulkInsert on demand)
                                                                v
                                                +---------------------------------+
                                                |  Dynamic Compute Nodes (0 -> 16)|
                                                |  (burst-cluster-burst-0, 1, ...) |
                                                +---------------------------------+

====================================================================================================
                              CROSS-CLUSTER CONTROL & DATA LINK
====================================================================================================
  1. Mutual SAuth Trust  : Master /etc/slurm/slurm.key synchronized automatically via shared /home
  2. SlurmDBD Routing    : Bidirectional TCP 6819 via AccountingStorageExternalHost on both controllers
  3. Shared Home State   : Both clusters share /home via Primary Cluster Controller's NFS bridge
  4. Remote Dispatch     : Jobs submitted on Primary Cluster via 'sbatch -M burst --wrap="hostname"'
====================================================================================================
```

---

## 2. Deployment & Execution Step-by-Step

> [!IMPORTANT]
> **Strict Deployment Order (Primary Cluster before Burst Cluster)**:
>
> 1. Primary Cluster provisions its VPC network (`10.128.0.0/20`), shared Filestore (`/home`), and Slurm head node.
> 2. Burst Cluster deploys its VPC network first using `--only setup` (`10.130.0.0/20`).
> 3. Bidirectional VPC Peering is established between the two active networks.
> 4. Burst Cluster provisions its compute nodes and controller using `--only cluster`, mounting Filestore over the peering tunnel.

### Step 1: Deploy Primary Cluster (Project A)

Configure the `vars` block in `community/examples/slurm-hybrid-cloud-burst/primary-cluster.yaml` with your `project_id`:

```yaml
vars:
  project_id: <PROJECT_A_ID>
```

Navigate to your Cluster Toolkit directory:

```bash
# 1. Create deployment files from primary-cluster.yaml:
./gcluster create community/examples/slurm-hybrid-cloud-burst/primary-cluster.yaml

# 2. Deploy Primary Cluster infrastructure (VPC network, Filestore, and Slurm nodes):
./gcluster deploy primary-cluster --auto-approve
```

---

### Step 2: Extract Primary Cluster Controller IP

Once Primary Cluster finishes deploying, query the internal IP of the Controller VM (which acts as both the Slurm control host and the NFS bridge for `/home`):

```bash
PRIMARY_CTRL_IP=$(gcloud compute instances list \
  --project=<PROJECT_A_ID> \
  --filter="name ~ controller" \
  --format="value(networkInterfaces[0].networkIP)")
echo "Primary Cluster Controller IP: ${PRIMARY_CTRL_IP}"
```

---

### Step 3: Deploy Burst Cluster Network (Project B)

Configure the `vars` block in `community/examples/slurm-hybrid-cloud-burst/burst-cluster.yaml` with your `project_id` and the `nfs_server_ip` set to `${PRIMARY_CTRL_IP}` from Step 2:

```yaml
vars:
  project_id: <PROJECT_B_ID>
  nfs_server_ip: <PRIMARY_CTRL_IP> # Set to Primary Cluster Controller internal IP
```

Create the deployment files and provision **only** Burst Cluster's VPC network and firewalls (`setup` group):

```bash
# 1. Create deployment files from burst-cluster.yaml:
./gcluster create community/examples/slurm-hybrid-cloud-burst/burst-cluster.yaml

# 2. Deploy ONLY Burst Cluster VPC network and firewalls:
./gcluster deploy burst-cluster --only setup --auto-approve
```

---

### Step 4: Establish Bidirectional VPC Peering

Now that both VPC networks exist (`primary-cluster-net` in Project A and `burst-cluster-net` in Project B), establish bidirectional VPC Network Peering:

> [!IMPORTANT]
> VPC Network Peering **must be configured in both directions** to become active in Google Cloud. Bi-directional connectivity is mandatory even if jobs are only submitted from Primary Cluster to Burst Cluster, because Burst Cluster nodes must initiate outbound connections to mount Primary Cluster's shared `/home` over NFS, report job completion states, and route SlurmDBD accounting back to Primary Cluster.

```bash
# 1. Peer from Project A -> Project B:
gcloud compute networks peerings create peer-a-to-b \
  --network=primary-cluster-net \
  --peer-project=<PROJECT_B_ID> \
  --peer-network=burst-cluster-net \
  --project=<PROJECT_A_ID>

# 2. Peer from Project B -> Project A:
gcloud compute networks peerings create peer-b-to-a \
  --network=burst-cluster-net \
  --peer-project=<PROJECT_A_ID> \
  --peer-network=primary-cluster-net \
  --project=<PROJECT_B_ID>
```

---

### Step 5: Deploy Burst Cluster Slurm Compute Nodes

Once peering is active, deploy Burst Cluster's Slurm cluster (`cluster` group). The Slurm nodes boot, reach Primary Cluster's Filestore over the peering tunnel, and join the multi-cluster environment:

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
gcloud compute ssh primary-cluster-slurm-login-001 --zone=us-central1-a --project=<PROJECT_A_ID> --tunnel-through-iap

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
gcloud compute ssh burst-cluster-slurm-login-001 --zone=us-central1-a --project=<PROJECT_B_ID> --tunnel-through-iap
```

Inside the session, run:

```bash
sinfo
srun -N1 hostname
```

#### 2. Remote Job Dispatch from Primary Cluster Login Node

SSH to `primary-cluster-slurm-login-001`:

```bash
gcloud compute ssh primary-cluster-slurm-login-001 --zone=us-central1-a --project=<PROJECT_A_ID> --tunnel-through-iap
```

##### a. Check Local and Remote Cluster Status

```bash
# Check local cluster partition:
sinfo

# Check remote burst cluster partition:
sinfo -M burst
```

##### b. Submit Batch Job to Dynamically Burst Nodes on Burst Cluster

```bash
cat << 'EOF' > ~/hybrid_test.sh
#!/bin/bash
#SBATCH --job-name=gcp_c3_burst
#SBATCH --output=/home/%u/gcp_burst_%j.out
#SBATCH --error=/home/%u/gcp_burst_%j.err
#SBATCH --nodes=2
#SBATCH --time=00:10:00

echo "Job $SLURM_JOB_ID executing on $(hostname) at $(date)"
echo "Target Slurm Cluster: $SLURM_CLUSTER_NAME"
echo "Allocated Nodes: $SLURM_JOB_NODELIST"
srun hostname
echo "Storage verification from $(hostname) at $(date)" >> /home/$USER/shared_gcp_storage.log
EOF

sbatch -M burst ~/hybrid_test.sh
```

##### c. Monitor Remote Job and Node Lifecycle

```bash
squeue -M burst
```

*Job Lifecycle & Node Provisioning Progression:*
1. **`CF` (Configuring)**: Slurm executes `resume.py`, issuing asynchronous GCE Compute API requests in Project B to dynamically power on `burst-cluster-burst_nodeset-[0-1]`.
2. **`R` (Running)**: Compute nodes boot, mount Primary Cluster's shared `/home` over VPC peering, execute the job, and write outputs directly to `/home/$USER/gcp_burst_<JOB_ID>.out`.
3. **`CG` (Completing)**: Job finishes and nodes return to `idle`.
4. **`idle~` (Power Down)**: After `SuspendTimeout` (default 600s), `suspend.py` issues API deletion calls to delete the VMs, returning idle compute cost to $0.

##### d. Direct Interactive Remote Execution

```bash
srun -M burst -N1 hostname
```

#### 3. Verify Output and Shared Storage Parity

Verify the output of the completed batch job and shared storage synchronization:

```bash
cat ~/gcp_burst_*.out
```

*Expected Output (`gcp_burst_*.out`):*

```text
Job 101 executing on burst-cluster-burst_nodeset-0 at Wed Sep  2 12:00:00 UTC 2026
Target Slurm Cluster: burst
Allocated Nodes: burst-cluster-burst_nodeset-[0-1]
burst-cluster-burst_nodeset-0
burst-cluster-burst_nodeset-1
```

```bash
cat ~/shared_gcp_storage.log
```

*Expected Output (`shared_gcp_storage.log`):*

```text
Storage verification from burst-cluster-burst_nodeset-0 at Wed Sep  2 12:00:01 UTC 2026
```
