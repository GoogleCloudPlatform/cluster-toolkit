# Deploy a GKE G4 Confidential Computing cluster for ML workloads

This blueprint provisions a Google Kubernetes Engine (GKE) cluster with G4 nodes running on Confidential VMs (`g4-standard-48`) powered by AMD SEV-SNP memory encryption and NVIDIA Blackwell GPUs.

Confidential VMs protect data in-use by keeping memory encrypted in hardware during processing. GKE Confidential Nodes extend this security by isolating node memory from the host hypervisor using AMD SEV (Secure Encrypted Virtualization) technology. Additionally, G4 instances support **Confidential GPUs** which secure memory transfers between the CPU and GPU using hardware-based PCIe encryption (Secure Passthrough).

For more details on GKE Confidential Nodes, refer to the official [Confidential GKE Nodes Overview](https://cloud.google.com/kubernetes-engine/docs/how-to/confidential-gke-nodes) and [Creating Confidential Storage PVs](https://cloud.google.com/kubernetes-engine/docs/how-to/confidential-gke-nodes#creating_chd_pv) documentation.

---

## What this blueprint deploys

This blueprint provisions a secure, isolated high-performance computing environment. Running this deployment will create the following resource footprint in your Google Cloud project:

* **VPC Network:** A custom VPC network (`gke-g4-cvm-net-0`) and regional subnet with secondary IP ranges for secure Pod and Service communication.
* **Regional GKE Cluster:** A highly available regional cluster (`gke-g4-cvm`) with cluster-level **Confidential Nodes** (AMD SEV) and Workload Identity enabled.
* **GKE Node Pools:**
  * A system node pool running on AMD-powered **`n2d-standard-16`** VMs (SEV-compatible).
  * A static GPU node pool running on **`g4-standard-48`** VMs, each containing **1 NVIDIA Blackwell GPU** in hardware-enforced **Confidential GPU** mode (PCIe Secure Passthrough).
* **Kubernetes Storage Class:** A dedicated `StorageClass` (`hyperdisk-balanced-sc`) and `PersistentVolumeClaim` (`hyperdisk-balanced-pvc-0`) are **always created** in the cluster. (Optional support for Customer-Managed Encryption Key (CMEK) and Confidential Storage encryption can be enabled).
* **Local `nvidia-smi` Job Template:** GCluster compiles a local Kubernetes job manifest file in your deployment folder (`gke-g4-cvm/primary/run-nvidia-smi-*.yaml`) to immediately test GPU functionality.
* **IAM Service Accounts:** Secure Google Service Accounts configured for GKE nodes and Workload Identity.

## Prerequisites

1. **Cluster Toolkit:** Ensure you have installed all the dependencies required in cluster toolkit and followed the setup instructions.
    1. Install [dependencies](https://docs.cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).
    2. Set up [Cluster Toolkit](https://docs.cloud.google.com/cluster-toolkit/docs/setup/configure-environment). For building the `gcluster` binary, see [Install Cluster Toolkit](https://docs.cloud.google.com/cluster-toolkit/docs/setup/configure-environment#install).
2. **Quota:** Ensure you have sufficient quota for `g4-standard-48` machines in your chosen region/zone. An active zonal reservation is optional but highly recommended to guarantee capacity.
3. **IP Address:** You will need the public IP address of the machine where you run `gcluster` to configure the cluster's authorized networks.
4. **Terraform State Bucket:** Create a Cloud Storage bucket to store the state of the Terraform deployment. See [Saving Terraform state](#saving-terraform-state) for instructions.
5. **GKE Version:** G4 VM Confidential Nodes with Blackwell GPUs require GKE cluster version **`1.35.3-gke.1389000`** or later. The blueprint is pre-configured to target version `1.36.` to satisfy this constraint.

---

## Saving Terraform state

Create a Cloud Storage bucket with versioning enabled to store the state of the Terraform deployment:

```bash
export PROJECT_ID=YOUR_PROJECT_ID
export BUCKET=YOUR_BUCKET_NAME
export REGION=YOUR_REGION
gcloud storage buckets create gs://${BUCKET} --project=${PROJECT_ID} \
  --default-storage-class=STANDARD --location=${REGION} \
  --uniform-bucket-level-access
gcloud storage buckets update gs://${BUCKET} --versioning
```

Modify the deployment configuration file (`gke-g4-confidential-deployment.yaml`) to use the created bucket as the Terraform remote backend:

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: YOUR_BUCKET_NAME
```

---

## Configuration

Before deploying, fill out the `gke-g4-confidential-deployment.yaml` file with your project-specific values:

| Variable | Description |
| :--- | :--- |
| `bucket` | The name of the GCS bucket used for storing your Terraform state (defined in `terraform_backend_defaults.configuration.bucket`). |
| `project_id` | Your Google Cloud Project ID. |
| `deployment_name` | A unique name for this Cluster Toolkit deployment (e.g., `gke-g4-cvm`). |
| `region` / `zone` | The GCP region and zone (e.g., `us-south1`, `us-south1-a`). |
| `machine_type` | The GCE machine type used for G4 GPU nodes. Must be `g4-standard-48` for RTX 6000 GPU Confidential nodes. Defaults to `g4-standard-48`. |
| `num_gpus` | The number of GPUs to attach to each G4 node. Must be `1` for the `g4-standard-48` shape. Defaults to `1`. |
| `static_node_count` | Number of G4 GPU nodes to provision. |
| `authorized_cidr` | Your public IP address in CIDR notation (e.g., `1.2.3.4/32`). |
| `reservation` | (Optional) The name of a zonal GCE reservation matching `g4-standard-48` to consume capacity from. |
| `enable_confidential_storage` | (Optional) Set to `true` to enable Confidential Storage, encrypting both the Kubernetes dynamic PVs (using CMEK) and the VM boot disks of all GKE nodes (system and workload). Defaults to `false`. |
| `disk_encryption_kms_key` | (Optional) The resource path to your Cloud KMS key used for CMEK storage encryption. Defaults to empty (`""`). |

### (Optional) KMS CMEK Setup for Storage

If enabling Confidential Storage (`enable_confidential_storage: true`), you must set up a Cloud KMS key and grant the necessary IAM permissions to the GKE and Compute Engine service agents before deploying the cluster.

To ensure low latency and compatibility, the Cloud KMS key **must** be created in the same region as your GKE cluster.

1. **Set Environment Variables:**
   Set the following variables in your terminal to simplify the key creation and IAM binding commands:

   ```bash
   export PROJECT_ID=YOUR_PROJECT_ID
   export PROJECT_NUMBER=YOUR_PROJECT_NUMBER # Numeric ID of your project (e.g. 1234567890)
   export REGION=YOUR_REGION                 # Region of your GKE cluster (e.g. us-south1)
   ```

2. **Create KMS Key Ring and Key:**
   Create a regional KeyRing and a CryptoKey in your chosen region:

   ```bash
   gcloud kms keyrings create gke-g4-storage-keyring \
     --location=${REGION} \
     --project=${PROJECT_ID}

   gcloud kms keys create gke-g4-storage-key \
     --location=${REGION} \
     --keyring=gke-g4-storage-keyring \
     --purpose=encryption \
     --project=${PROJECT_ID}
   ```

3. **Grant Permissions to Service Agents:**
   Grant the `roles/cloudkms.cryptoKeyEncrypterDecrypter` role to both the GKE Service Agent and the Compute Engine Service Agent on the KMS key:

   ```bash
   # Grant GKE Service Agent
   gcloud kms keys add-iam-policy-binding gke-g4-storage-key \
     --location=${REGION} \
     --keyring=gke-g4-storage-keyring \
     --member="serviceAccount:service-${PROJECT_NUMBER}@container-engine-robot.iam.gserviceaccount.com" \
     --role="roles/cloudkms.cryptoKeyEncrypterDecrypter" \
     --project=${PROJECT_ID}

   # Grant Compute Engine Service Agent
   gcloud kms keys add-iam-policy-binding gke-g4-storage-key \
     --location=${REGION} \
     --keyring=gke-g4-storage-keyring \
     --member="serviceAccount:service-${PROJECT_NUMBER}@compute-system.iam.gserviceaccount.com" \
     --role="roles/cloudkms.cryptoKeyEncrypterDecrypter" \
     --project=${PROJECT_ID}
   ```

4. **Update Overrides:**
   Set the following variables in `gke-g4-confidential-deployment.yaml` (replace placeholders with your actual values):

   ```yaml
   enable_confidential_storage: true
   disk_encryption_kms_key: "projects/YOUR_PROJECT_ID/locations/YOUR_REGION/keyRings/gke-g4-storage-keyring/cryptoKeys/gke-g4-storage-key"
   ```

   For advanced concepts, see the [Using CMEK in GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/using-cmek) documentation.

---

## Deploy the Cluster

1. Switch to the toolkit directory and build:

   ```bash
   cd ~/cluster-toolkit
   make
   ```

2. Deploy the infrastructure in a single step:

   ```bash
   ./gcluster deploy \
       examples/gke-g4-confidential/gke-g4-confidential.yaml \
       -d examples/gke-g4-confidential/gke-g4-confidential-deployment.yaml
   ```

---

## Verify the Deployment

### Step 1: Run Basic GPU Verification (nvidia-smi)
Cluster Toolkit compiles a basic `nvidia-smi` verification job manifest locally in your deployment folder. This verifies that GKE is recognizing your Blackwell GPU.

1. Submit the generated verification job:

   ```bash
   kubectl create -f gke-g4-cvm/primary/run-nvidia-smi-*.yaml
   ```

2. Wait for the pod to execute and complete:

   ```bash
   kubectl get pods -l name=run-nvidia-smi
   ```

3. Print the logs to verify the GPU status and driver registration:

   ```bash
   kubectl logs jobs/run-nvidia-smi
   ```

**Expected Log Output:**

```text
+-----------------------------------------------------------------------------------------+
| NVIDIA-SMI 550.54.14              Driver Version: 550.54.14      CUDA Version: 12.4     |
|-----------------------------------------+------------------------+----------------------+
| GPU  Name                 Persistence-M | Bus-Id          Disp.A | Volatile Uncorr. ECC |
| Fan  Temp   Perf          Pwr:Usage/Cap |           Memory-Usage | GPU-Util  Compute M. |
|                                         |                        |               MIG M. |
|=========================================+========================+======================|
|   0  NVIDIA RTX PRO 6000 ...        Off | 00000000:00:04.0   Off |                  N/A |
| N/A   37C    P8              15W / 175W |      4MiB / 49140MiB |      0%      Default |
|                                         |                        |                  N/A |
+-----------------------------------------+------------------------+----------------------+
```

---

### Step 2: Run End-to-End Verification & Workload (GPU Computation)
Submit the validation job to run in-cluster hardware checks and execute a GPU matrix multiplication workload:

1. Submit the validation test job:

   ```bash
   kubectl create -f examples/gke-g4-confidential/g4-verification-test.yaml
   ```

2. Monitor the pod status:

   ```bash
   kubectl get pods -l name=g4-verification-test
   ```

3. Print the container logs to verify both the security checks and the GPU workload executed successfully:

   ```bash
   kubectl logs jobs/g4-verification-test
   ```

**Expected Log Output:**

```text
=== 1. CPU SEV Status (dmesg) ===
[    0.359261] Memory Encryption Features active: AMD SEV
=== 2. GPU CC Status (nvidia-smi) ===
CC status: ON
Confidential Compute GPUs Ready state: ready
=== 3. Running GPU Computation Workload ===
PyTorch Version: 2.2.0a0+81ea7a4
CUDA Available: True
Device Name: NVIDIA RTX PRO 6000 Blackwell Server Edition
Initializing tensors on host CPU...
Moving tensors to GPU (Encrypted Secure Passthrough transfer)...
Performing matrix multiplication on GPU...
Moving result back to CPU...
SUCCESS: G4 GPU computation completed successfully!
```

---

### Step 3: (Optional) Verify Confidential Storage (CMEK)
If you enabled Confidential Storage, run the storage validation test job which mounts the encrypted persistent disk to `/data`:

1. Ensure the StorageClass and PVC were created successfully:

   ```bash
   kubectl get storageclass hyperdisk-balanced-sc
   kubectl get pvc hyperdisk-balanced-pvc-0
   ```

   *Note: The PVC stays in a `Pending` state until a Pod attempts to mount it.*

   > **IMPORTANT:** Note on Storage Billing Lifecycle
   >
   > * **Before Workload Starts:** The PVC remains in a `Pending` state. No physical GCE Persistent Disk resource is provisioned, meaning no storage billing charges are incurred while the cluster is idle.
   > * **During Execution:** Once a Pod is created that mounts the PVC, GKE dynamically provisions the 100Gi GCE Hyperdisk and active capacity billing begins.
   > * **After Job Completion:** GKE preserves the PVC in a `Bound` state even after the Pod terminates to protect your data. **Billing continues as long as the PVC remains Bound.**
   > * **To Stop Charges:** Delete the PVC manually once your testing is finished:
   >
   >   ```bash
   >   kubectl delete pvc hyperdisk-balanced-pvc-0
   >   ```
   >
   > **IMPORTANT:** Note on Storage Upgrade Behavior
   >
   > In Kubernetes, a `StorageClass` is **immutable** after creation. If you deploy the cluster with `enable_confidential_storage: false` and later decide to enable it (`true`), GCluster/Terraform will attempt to update the StorageClass, which will be **rejected by the GKE API server**.
   >
   > To upgrade to Confidential Storage later, you must first delete the existing StorageClass and its bound PVCs:
   >
   > ```bash
   > kubectl delete pvc hyperdisk-balanced-pvc-0
   > kubectl delete storageclass hyperdisk-balanced-sc
   > ```
   >
   > After deletion, modify the deployment file and run `./gcluster deploy` again to cleanly recreate the encrypted storage resources.
   >
   > **IMPORTANT:** Note on Re-deployment & State Drift (404 Disk Not Found)
    >
    > For convenience, this blueprint automatically pre-provisions the `hyperdisk-balanced-pvc-0` PVC during deployment.
    >
    > If you plan to **re-deploy or update** your GKE cluster (e.g., modifying node pools or running `./gcluster deploy` again), you must manage your storage state to prevent out-of-sync disk errors. If the GKE cluster or physical disks are recreated while the old Kubernetes PVC objects remain in your local workspace/namespace configuration, GKE will enter a state of "blind drift," causing new workloads to hang in `ContainerCreating` with a `404: Disk not found` error.
    >
    > To prevent or resolve this, always ensure you delete the stale PVC manually from your cluster before re-applying:
    >
    > ```bash
    > kubectl delete pvc hyperdisk-balanced-pvc-0
    > ```
    >
    > Once deleted, running `./gcluster deploy` again will cleanly recreate both the PVC and dynamically provision a fresh active physical disk.

2. Submit the storage validation test job:

   ```bash
   kubectl create -f examples/gke-g4-confidential/g4-verification-storage-test.yaml
   ```

3. Print the logs to verify disk read/write capability:

   ```bash
   kubectl logs jobs/g4-verification-storage-test
   ```

**Expected Log Output:**

```text
=== 1. CPU SEV Status (dmesg) ===
[    0.359847] Memory Encryption Features active: AMD SEV
=== 2. GPU CC Status (nvidia-smi) ===
CC status: ON
Confidential Compute GPUs Ready state: ready
=== 3. Writing to Confidential Storage ===
Writing test file to secure hyperdisk volume...
Reading data back from secure hyperdisk volume...
Confidential Data: G4 SEV CPU + G4 NVIDIA CC GPU
=== 4. Running GPU Computation Workload ===
PyTorch Version: 2.2.0a0+81ea7a4
CUDA Available: True
Device Name: NVIDIA RTX PRO 6000 Blackwell Server Edition
SUCCESS: G4 GPU computation completed successfully!
```

---

## Clean Up

To avoid incurring ongoing charges for the resources created, destroy the deployment:

```bash
./gcluster destroy DEPLOYMENT_NAME
```

Replace `DEPLOYMENT_NAME` with the name of your deployment (defaults to `gke-g4-cvm`).
