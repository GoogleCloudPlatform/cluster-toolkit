# Multi-Cluster Slurm with Cloud Bursting on Google Cloud Platform (GCP)

This directory contains the blueprints, playbooks, and deployment instructions for running **Multi-Cluster Slurm with Elastic Cloud Bursting** between two independent projects in standard **Google Cloud Platform (GCP)** using **C3 machine instances** (`c3-standard-176`).

---

## 1. Experiment Setup & Architecture

The testbed deploys two autonomous Slurm clusters across two isolated GCP projects connected via VPC Network Peering:

* **Cluster A (`clustera`)**: Simulates the **Primary / On-Premises Cluster** in **Project A** (persistent login node, persistent controller, static compute nodes, and managed Google Cloud Filestore for `/home`).
* **Cluster B (`clusterb`)**: Simulates the **Cloud Burst Target** in **Project B** (lightweight login node, lightweight controller, 0 static nodes, and dynamic compute nodes that autoscale from `0 -> 16` on demand).

```text
====================================================================================================
                        CLUSTER A: Primary Cluster (Project A)
====================================================================================================
  [Project A] (VPC: clustera-c3-net, Subnet: 10.128.0.0/20)
  +---------------------------+                 +---------------------------------+
  |  Login Node               | --------------> |  Controller Node                |
  |  (clustera-c3-login-001)  |                 |  (clustera-c3-controller)       |
  |  User Submission Endpoint |                 |  slurmctld: 6817 | slurmdbd: 6819
  +---------------------------+                 +---------------------------------+
                :                                               |
                : (mounts Filestore)                            v
                :                               +---------------------------------+
                :                               |  Static Compute Nodes           |
                :                               |  (clustera-c3-compute-0, 1)     |
                v                               +---------------------------------+
  +-------------------------------------------------------------+
  |  Google Cloud Filestore (10.128.0.x)                        |
  |  Share: /homeshare -> Mounted at /home                      |
  +-------------------------------------------------------------+
                ^
                | (VPC Network Peering: peer-a-to-b <---> peer-b-to-a)
================:===================================================================================
                :
                :       CLUSTER B: Elastic Cloud Burst Target (Project B)
================:===================================================================================
  [Project B] (VPC: clusterb-c3-net, Subnet: 10.130.0.0/20)
  +---------------------------+                 +---------------------------------+
  |  Login Node               | --------------> |  Controller Node                |
  |  (clusterb-c3-login-001)  |                 |  (clusterb-c3-controller)       |
  +---------------------------+                 |  slurmctld: 6817                |
                                                +---------------------------------+
                                                                |
                                                                | (Dynamic bulkInsert on demand)
                                                                v
                                                +---------------------------------+
                                                |  Dynamic Compute Nodes (0 -> 16)|
                                                |  (clusterb-c3-burst-0, 1, ...)  |
                                                +---------------------------------+

====================================================================================================
                              CROSS-CLUSTER CONTROL & DATA LINK
====================================================================================================
  1. Mutual SAuth Trust  : Master /etc/slurm/slurm.key synchronized automatically via shared /home
  2. SlurmDBD Routing    : Bidirectional TCP 6819 via AccountingStorageExternalHost on both controllers
  3. Shared Home State   : Both clusters share /home via Cluster A Controller's NFS bridge
  4. Remote Dispatch     : Jobs submitted on Cluster A via 'sbatch -M clusterbc3 --wrap="hostname"'
====================================================================================================
```

---

## 2. Network Setup & Non-Overlapping Subnets

> [!IMPORTANT]
> In GCP, VPC Peering **will fail** if the subnets in Project A and Project B have overlapping CIDR blocks!
> You must create custom subnets with non-overlapping ranges (e.g., Project A: `10.128.0.0/20`, Project B: `10.130.0.0/20`).

### 1. In Project A: Create Custom Network & Subnet

```bash
# Create VPC network
gcloud compute networks create clustera-c3-net \
  --subnet-mode=custom \
  --project=<PROJECT_A_ID>

# Create Subnet (10.128.0.0/20) with Private Google Access enabled
gcloud compute networks subnets create clustera-c3-subnet \
  --network=clustera-c3-net \
  --region=us-central1 \
  --range=10.128.0.0/20 \
  --enable-private-ip-google-access \
  --project=<PROJECT_A_ID>

# Add internal firewall rule (allowing traffic from both Cluster A and Cluster B subnets)
gcloud compute firewall-rules create clustera-allow-internal \
  --network=clustera-c3-net \
  --allow=tcp:1-65535,udp:1-65535,icmp \
  --source-ranges=10.128.0.0/20,10.130.0.0/20 \
  --project=<PROJECT_A_ID>

# Add SSH firewall rule
gcloud compute firewall-rules create clustera-allow-ssh \
  --network=clustera-c3-net \
  --allow=tcp:22 \
  --project=<PROJECT_A_ID>
```

### 2. In Project B: Create Custom Network & Subnet

```bash
# Create VPC network
gcloud compute networks create clusterb-c3-net \
  --subnet-mode=custom \
  --project=<PROJECT_B_ID>

# Create Subnet (10.130.0.0/20) with Private Google Access enabled
gcloud compute networks subnets create clusterb-c3-subnet \
  --network=clusterb-c3-net \
  --region=us-central1 \
  --range=10.130.0.0/20 \
  --enable-private-ip-google-access \
  --project=<PROJECT_B_ID>

# Add internal firewall rule (allowing traffic from both Cluster A and Cluster B subnets)
gcloud compute firewall-rules create clusterb-allow-internal \
  --network=clusterb-c3-net \
  --allow=tcp:1-65535,udp:1-65535,icmp \
  --source-ranges=10.128.0.0/20,10.130.0.0/20 \
  --project=<PROJECT_B_ID>

# Add SSH firewall rule
gcloud compute firewall-rules create clusterb-allow-ssh \
  --network=clusterb-c3-net \
  --allow=tcp:22 \
  --project=<PROJECT_B_ID>
```

### 3. Establish Bidirectional VPC Peering
> [!IMPORTANT]
> VPC Network Peering **must be configured in both directions** to become active in Google Cloud. Bi-directional connectivity is mandatory even if jobs are only submitted from Cluster A to Cluster B, because Cluster B nodes must initiate outbound connections to mount Cluster A's shared `/home` over NFS, report job completion states, and route SlurmDBD accounting back to Cluster A.

```bash
# 1. Peer from Project A -> Project B:
gcloud compute networks peerings create peer-a-to-b \
  --network=clustera-c3-net \
  --peer-project=<PROJECT_B_ID> \
  --peer-network=clusterb-c3-net \
  --project=<PROJECT_A_ID>

# 2. Peer from Project B -> Project A:
gcloud compute networks peerings create peer-b-to-a \
  --network=clusterb-c3-net \
  --peer-project=<PROJECT_A_ID> \
  --peer-network=clustera-c3-net \
  --project=<PROJECT_B_ID>
```

---

## 3. Deployment & Execution Step-by-Step

> [!IMPORTANT]
> **Strict Deployment Order (Cluster A before Cluster B)**:
> Cluster A **must be deployed first and reach running state** before Cluster B is provisioned.
> Cluster A generates the master SAuth encryption key (`slurm.key`), sets up the shared `/home` NFS bridge, and writes its internal IP (`cluster_a_ctrl_ip`). Cluster B requires Cluster A's Controller IP (`CTRL_A_IP`) to mount `/home` and bootstrap on boot.

### Step 1: Deploy Cluster A (Project A)

Navigate to your Cluster Toolkit directory:

```bash
# 1. Create deployment files from clustera.yaml:
./gcluster create community/examples/slurm-hybrid-cloud-burst/clustera.yaml \
  --vars "\
project_id=<PROJECT_A_ID>,\
deployment_name=clustera-c3,\
region=us-central1,\
zone=us-central1-a" \
  -w --force --validation-level=IGNORE

# 2. Deploy Cluster A infrastructure:
./gcluster deploy clustera-c3 --auto-approve
```

---

### Step 2: Extract Cluster A Controller IP

Once Cluster A finishes deploying, query the internal IP of the Controller VM (which acts as both the Slurm control host and the NFS bridge for `/home`):

```bash
CTRL_A_IP=$(gcloud compute instances list \
  --project=<PROJECT_A_ID> \
  --filter="name ~ controller" \
  --format="value(networkInterfaces[0].networkIP)")
echo "Cluster A Controller IP: ${CTRL_A_IP}"
```

---

### Step 3: Deploy Cluster B (Project B)

> [!NOTE]
> In GCP, VPC Network Peering is non-transitive, so Cluster B accesses the shared `/home` via Cluster A's controller IP (`${CTRL_A_IP}:/home`), which acts as the NFS bridge over the peering link.

Deploy Cluster B in `Project B`, passing `nfs_server_ip=${CTRL_A_IP}`, `nfs_remote_mount=/home`, and `clustera_controller_ip=${CTRL_A_IP}`:

```bash
./gcluster create community/examples/slurm-hybrid-cloud-burst/clusterb.yaml \
  --vars "\
project_id=<PROJECT_B_ID>,\
deployment_name=clusterb-c3,\
region=us-central1,\
zone=us-central1-a,\
nfs_server_ip=${CTRL_A_IP},\
nfs_remote_mount=/home,\
clustera_controller_ip=${CTRL_A_IP}" \
  -w --force --validation-level=IGNORE

# Deploy Cluster B infrastructure:
./gcluster deploy clusterb-c3 --auto-approve
```

---

### Step 4: Verify SlurmDBD Auto-Registration on Cluster A

In order to submit jobs from Cluster A to Cluster B, **no explicit manual steps or cross-registration commands are required**. All SlurmDBD registration and inter-cluster cryptographic trust are handled automatically by the cluster blueprints:
* Cluster A exports its master `slurm.key` and controller IP to the shared NFS home directory.
* When Cluster B boots, its startup script mounts the shared NFS, imports the matching `slurm.key`, and points to Cluster A's SlurmDBD.
* Cluster B's controller automatically registers itself into Cluster A's SlurmDBD during its initialization.

After both clusters finish Slurm initialization (typically 3–5 minutes after deployment), Cluster A will see both clusters without any manual intervention.

#### Check Registered Clusters on Cluster A Login Node

SSH to `clustera-c3-slurm-login-001` (or your active Cluster A login node):

```bash
gcloud compute ssh clustera-c3-slurm-login-001 --zone=us-central1-a --project=<PROJECT_A_ID>

sudo /usr/local/bin/sacctmgr list clusters
```

*Expected Output:*

```text
   Cluster     ControlHost  ControlPort   RPC     Share GrpJobs       GrpTRES GrpSubmit MaxJobs       MaxTRES MaxSubmit     MaxWall                  QOS   Def QOS 
---------- --------------- ------------ ----- --------- ------- ------------- --------- ------- ------------- --------- ----------- -------------------- --------- 
clusterac3      10.128.0.x         6818 11264         1                                                                                           normal           
clusterbc3      10.130.0.x         6818 11264                                                                                                                      
```

Both clusters are immediately registered with their internal `ControlHost` IPs, control ports (`6818`), and active RPC versions (`11264`), ready for cross-cluster job dispatch.

---

### Step 5: Execute and Monitor Cross-Cluster Bursting Jobs

#### 1. Local Smoke Test on Cluster B Login Node

SSH to `clusterb-c3-slurm-login-001` to verify partition availability and local provisioning:

```bash
gcloud compute ssh clusterb-c3-slurm-login-001 --zone=us-central1-a --project=<PROJECT_B_ID>
```

Inside the session, run:

```bash
sinfo
srun -N1 hostname
```

#### 2. Remote Job Dispatch from Cluster A Login Node

SSH to `clustera-c3-slurm-login-001`:

```bash
gcloud compute ssh clustera-c3-slurm-login-001 --zone=us-central1-a --project=<PROJECT_A_ID>
```

##### a. Check Local and Remote Cluster Status

```bash
# Check local cluster partition:
sinfo

# Check remote burst cluster partition:
sinfo -M clusterbc3
```

##### b. Submit Batch Job to Dynamically Burst Nodes on Cluster B

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

sbatch -M clusterbc3 ~/hybrid_test.sh
```

##### c. Monitor Remote Job and Node Lifecycle

```bash
squeue -M clusterbc3
```

*Job Lifecycle & Node Provisioning Progression:*
1. **`CF` (Configuring)**: Slurm executes `resume.py`, issuing asynchronous GCE Compute API requests in Project B to dynamically power on `clusterb-c3-burst_nodeset-[0-1]`.
2. **`R` (Running)**: Compute nodes boot, mount Cluster A's shared `/home` over VPC peering, execute the job, and write outputs directly to `/home/$USER/gcp_burst_<JOB_ID>.out`.
3. **`CG` (Completing)**: Job finishes and nodes return to `idle`.
4. **`idle~` (Power Down)**: After `SuspendTimeout` (default 600s), `suspend.py` issues API deletion calls to delete the VMs, returning idle compute cost to $0.

##### d. Direct Interactive Remote Execution

```bash
srun -M clusterbc3 -N1 hostname
```

#### 3. Verify Output and Shared Storage Parity

Verify the output of the completed batch job and shared storage synchronization:

```bash
cat ~/gcp_burst_*.out
```

*Expected Output (`gcp_burst_*.out`):*

```text
Job 101 executing on clusterb-c3-burst_nodeset-0 at Wed Sep  2 12:00:00 UTC 2026
Target Slurm Cluster: clusterbc3
Allocated Nodes: clusterb-c3-burst_nodeset-[0-1]
clusterb-c3-burst_nodeset-0
clusterb-c3-burst_nodeset-1
```

```bash
cat ~/shared_gcp_storage.log
```

*Expected Output (`shared_gcp_storage.log`):*

```text
Storage verification from clusterb-c3-burst_nodeset-0 at Wed Sep  2 12:00:01 UTC 2026
```
