# GKE TPU 7x Blueprint

This blueprint provisions a production-ready Google Kubernetes Engine (GKE) cluster designed to run workloads on the **Cloud TPU 7x** accelerator platform. It can be configured to create a TPU slice of any supported size and topology.

The blueprint follows Google Cloud best practices, including:

- Provisioning a GKE cluster with a dedicated **TPU 7x** node pool.
- Creating dedicated IAM Service Accounts for nodes and workloads, following security best practices.
- Enabling Workload Identity for a secure way for your applications to access Google Cloud services.

> NOTE: This guide provides examples for deploying a small, single-node slice (2x2x1), but the parameters in the deployment file can be easily changed to provision larger, multi-node slices.

This guide also includes a `tpu-7x-job.yaml` that creates a Kubernetes Pod and runs a simple command to check for available TPU chips.

## Key parameters when working with TPUs

- `num_slices`: The number of identical, independent **TPU slices (node pools)** to create. A TPU slice is a collection of TPU chips that are physically connected by a dedicated, ultra-low-latency network called the Inter-Chip Interconnect (ICI). This is what allows the nodes to function as a single, cohesive supercomputer. Each node pool created by this variable corresponds to one such slice.
  - **Default value is 1**. Most use-cases require this value to be `1`. This blueprint will create one GKE node pool that contains all the nodes needed for your `tpu_topology`.
  - **Advanced Use**: This variable acts as a multiplier. For example, if you set `num_slices: 3` and `static_node_count: 2`, the blueprint will create three separate node pools (...-pool-0, ...-pool-1, ...-pool-2), each containing `2` TPU nodes. This is an advanced feature for provisioning multiple, smaller, identical slices at once.
- `tpu_topology`: The TPU topology desired. Topology is the number and physical arrangement of the TPU chips in a TPU slice.
  - **What it is**: This defines the shape and total size of your TPU "supercomputer". For the 3D-interconnected TPU 7x, you must specify this in XxYxZ format (e.g., `2x2x1`).
  - **Why it matters**: The product of the dimensions (X*Y*Z) gives you the total number of chips in your slice. This number is essential for calculating the `static_node_count`.
  - **Example**: A tpu_topology of `2x2x1` creates a 4-chip slice. If you then use a `tpu7x-standard-4t` machine type (which has `4` chips per node), you can calculate your required `static_node_count` as `1` (4 total chips / 4 chips per node).
- `static_node_count`: The **fixed, exact number of nodes (VMs)** required to build **each individual TPU slice** (*as defined by `num_slices`*) in your `tpu_topology`. **This is the most critical parameter to set correctly**. Cloud TPU slices are created as a single, rigid hardware unit with physical, high-speed interconnects (ICI) between all nodes. This is different from standard CPU or GPU node pools. For more details please refer to the [appendix](#how-to-calculate-static_node_count)
  - **It is an exact requirement, not a maximum**. If your topology requires 4 nodes, you must set this value to `4`. Providing a different number will cause the deployment to **fail**.
  - **It does not support autoscaling**. Because the hardware is physically interconnected, the size of the slice is fixed at creation time. There are no "*dynamic*" options.

## Before you begin

Before you start, make sure you have performed the following tasks:

1. Enable the Google Kubernetes Engine API.
2. If you want to use the Google Cloud CLI for this task, [install](https://cloud.google.com/sdk/docs/install) and then [initialize](https://cloud.google.com/sdk/docs/initializing) the gcloud CLI. If you previously installed the gcloud CLI, get the latest version by running `gcloud components update`.
   - **NOTE**: For existing gcloud CLI installations, make sure to set the compute/region and compute/zone properties. By setting default locations, you can avoid errors in gcloud CLI like the following: One of [--zone, --region] must be supplied: Please specify location.
3. Ensure that you have enough quota for **TPU 7x** in the specified region.
4. Ensure that you have the following roles enabled:
   - `roles/editor`
   - `roles/container.clusterAdmin`
   - `roles/iam.serviceAccountAdmin`
5. **Note the GKE Version Requirement**: Be aware that Cloud TPU 7x requires a specific minimum GKE version to function correctly.
   - **Minimum Version**: `1.34.0-gke.1662000` or later.
   - **Blueprint Configuration**: The provided `gke-tpu-7x.yaml` blueprint is already configured to use a compatible version from the `RAPID` release channel. If you customize the blueprint, ensure you do not select a version older than this minimum requirement.

## Create a cluster using Cluster Toolkit

This section guides you through the cluster creation process.

1. [Launch Cloud Shell](https://cloud.google.com/shell/docs/launching-cloud-shell). You can use a different environment; however, we recommend Cloud Shell because the dependencies are already pre-installed for Cluster Toolkit. If you don't want to use Cloud Shell, follow the instructions to [install dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) to prepare a different environment.
2. Clone the Cluster Toolkit from the git repository:

    ```bash
    cd ~
    git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
    ```

3. Install the Cluster Toolkit:

    ```bash
    cd cluster-toolkit && git checkout main && make
    ```

4. Create a Cloud Storage bucket to store the state of the Terraform deployment:

    ```bash
    gcloud storage buckets create gs://BUCKET_NAME \
      --default-storage-class=STANDARD \
      --location=COMPUTE_REGION \
      --uniform-bucket-level-access
    gcloud storage buckets update gs://BUCKET_NAME --versioning
    ```

   Replace the following variables:
   - `BUCKET_NAME`: the name of the new Cloud Storage bucket (e.g., `tpu-7x-bucket`).
   - `COMPUTE_REGION`: the compute region for the cluster (e.g., `us-central1`).

5. In the `examples/gke-tpu-7x/gke-tpu-7x-deployment.yaml` file, replace the following variables in the `terraform_backend_defaults` and `vars` sections to match the specific values for your deployment:
   - `bucket`: the name of the Cloud Storage bucket you created in the previous step.
   - `project_id`: your Google Cloud project ID.
   - `region`: the compute region for the cluster.
   - `zone`: the compute zone for the TPUs.
   - `num_slices`: the number of TPU slices (node pools) to create.
   - `machine_type`: the machine type of the TPU.
   - `tpu_topology`: the TPU placement topology for the node pool.
   - `static_node_count`: the number of TPU nodes in each TPU slice (node pool).z

    > (Note: This is calculated by dividing the total chips in the topology by the chips per machine. E.g. a `2x2x1` topology (4 chips) using `tpu7x-standard-4t` machines (4 chips) requires `static_node_count: 1`). For details refer to the [Appendix](#how-to-calculate-static_node_count).
   - `authorized_cidr`: The IP address range you want to allow to connect with the cluster.
   - `reservation`: the name of the compute engine reservation for your TPU 7x nodes.

6. To modify advanced settings, edit `examples/gke-tpu-7x/gke-tpu-7x.yaml`.
7. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

    ```bash
    gcloud auth application-default login
    ```

8. Deploy the blueprint to provision the GKE infrastructure:

    ```bash
     cd ~/cluster-toolkit
     ./gcluster deploy -d \
     examples/gke-tpu-7x/gke-tpu-7x-deployment.yaml \
     examples/gke-tpu-7x/gke-tpu-7x.yaml
    ```

   This process will take several minutes.

## Advanced Blueprint: GKE TPU 7x

This repository also includes an advanced blueprint, `gke-tpu-7x-advanced.yaml`, designed for production-ready workloads. It builds on the basic blueprint by adding several key features:

- Essential **multi-VPC** network architecture required for optimized high-throughput inter-chip communication
- Automatic creation of two GCS buckets for training data and checkpoints.
- Performance-tuned GCS FUSE mounts pre-configured in the cluster as Persistent Volumes.
- **Optional** High-Performance Storage: [Hyperdisk Balanced](https://docs.cloud.google.com/compute/docs/disks/hyperdisks) support for highly available and consistent performance across GKE nodes. For details of configuring Hyperdisk Balanced, please refer to the [appendix](#understanding-hyperdisk-balanced-integration).
- **Optional** High-Performance Storage: [Managed Lustre](https://cloud.google.com/managed-lustre/docs/overview) for high-performance, fully managed parallel file system optimized for heavy AI and HPC workloads. For details of configuring Managed Lustre, please refer to the [appendix](#understanding-managed-lustre-integration).
- **Optional** Shared File Storage: [Filestore](https://docs.cloud.google.com/filestore/docs/overview) for managed NFS capabilities allowing multiple TPU hosts to share logs, code, or datasets. For details, refer to the [appendix](#understanding-filestore-integration).

### Deploying the Advanced Blueprint

The process is nearly identical to the basic deployment.

1. Ensure you have completed steps 1-7 from the "Create a cluster" section above. The same `gke-tpu-7x-deployment.yaml` file can be used.
2. In the final deploy command, simply point to the `gke-tpu-7x-advanced.yaml` blueprint instead.

    ```sh
    cd ~/cluster-toolkit
    ./gcluster deploy -d \
    community/examples/gke-tpu-7x/gke-tpu-7x-deployment.yaml \
    community/examples/gke-tpu-7x/gke-tpu-7x-advanced.yaml
    ```

3. After deployment, the blueprint will output instructions for running a fio benchmark job. This job serves as a validation test to confirm that the GCS mounts are working correctly for both reading and writing. Follow the printed instructions to run the test.

### Advanced Scheduling with Kueue

This blueprint supports [Kueue](https://kueue.sigs.k8s.io/), a kubernetes-native system for managing quotas and job queuing. This is enabled by default in the advanced blueprint (`gke-tpu-7x-advanced.yaml`).

1. **Quota:** The blueprint automatically calculates and sets a `google.com/tpu` quota in the `ClusterQueue` matching the total static TPU capacity of your cluster (slices x nodes x chips).
2. **Submit a Job:** To submit a job to the queue, add the label `kueue.x-k8s.io/queue-name: user-queue` to your Job or JobSet manifest.

    A sample job file is provided: `kueue-job-sample.yaml`.

    ```sh
    kubectl create -f ~/cluster-toolkit/examples/gke-tpu-7x/kueue-job-sample.yaml
    ```

3. **Validation:** Check the status of your workload.

    ```sh
    kubectl get workloads
    ```

## Run the sample job

The `tpu-7x-job.yaml` file creates a Pod resource in Kubernetes. The workload installs JAX and a specific `libtpu` library, and then returns the number of TPU chips it can detect.

1. Connect to your cluster:

    ```bash
    gcloud container clusters get-credentials DEPLOYMENT_NAME \
      --region=REGION \
      --project=PROJECT_ID
    ```

   Replace `DEPLOYMENT_NAME`, `REGION`, and `PROJECT_ID` with the ones used in your `gke-tpu-7x-deployment.yaml` file.
2. Update the `nodeSelector` in the job file.
   Open the `examples/gke-tpu-7x/tpu-7x-job.yaml` file. Ensure the `nodeSelector` values match the accelerator label and topology used in your deployment file.
   > *You can find the correct labels for your cluster by running `kubectl get nodes --show-labels`*

   For the example deployment, the values are:

    ```yaml
    nodeSelector:
        cloud.google.com/gke-tpu-accelerator: tpu7x
        cloud.google.com/gke-tpu-topology: 2x2x1
    ```

3. Update Resource Request:

    ```yaml
    # In tpu-7x-job.yaml, inside the spec container
    resources:
      requests:
        google.com/tpu: <CHIPS_PER_NODE> # e.g., 4 for tpu7x-standard-4t
      limits:
        google.com/tpu: <CHIPS_PER_NODE>
    ```

4. Create the resources:
   The sample job file already contains the correct node selectors for the example deployment.

    ```bash
    kubectl create -f ~/cluster-toolkit/examples/gke-tpu-7x/tpu-7x-job.yaml
    ```

   This command returns a Pod name.
5. Obtain the list of pods:

    ```bash
    kubectl get pods -w
    ```

   Wait for the pod to show a status of `Completed`.
6. Display logs of the pod:

    ```bash
    kubectl logs <pod-name>
    ```

   If successful, this should display `Global device count: 8` near the end of the logs. [`jax.device_count()`](https://cloud.google.com/tpu/docs/jax-pods) reports the number of devices your workload is actively using. It actually counts the TensorCores, GKE allocates by Chip. For more details refer to the [Appendix](#how-to-calculate-the-expected-device-count-after-running-the-gke-tpu-7x-jobyaml)

## Clean up

To avoid recurring charges for the resources used, clean up the resources provisioned by Cluster Toolkit:

```bash
./gcluster destroy DEPLOYMENT_NAME
```

## Appendix

### How to Calculate `static_node_count`

- The formula is: (`Total Chips in Topology`) / (`Chips per Machine`)

#### Example 1: *Single-Node Slice* (The default for this blueprint)

- `tpu_topology`: `2x2x1` (Total chips = 2*2*1 = **4**)
- `machine_type`: `tpu7x-standard-4t` (Chips per machine = **4**)
- **Calculation**: `4 / 4 = 1`
- **Correct value**: `static_node_count: 1`

#### Example 2: *Multi-Node Slice*

- `tpu_topology`: `4x4x4` (Total chips = 4*4*4 = **64**)
- `machine_type`: `tpu7x-standard-4t` (Chips per machine = **4**)
- **Calculation**: `64 / 4 = 16`
- **Correct value**: `static_node_count: 16`

#### Example 3: *Multiple Identical Slices (Advanced)*

This example shows how `num_slices` and `static_node_count` work together. The goal is to create three separate 8-chip slices.

- `num_slices`: `3` (We want three independent node pools)
- `tpu_topology`: `2x2x2` (Each slice will have 2*2*2 = 8 chips)
- `machine_type`: `tpu7x-standard-4t` (Chips per machine = 4)
- **Calculation**: `8 / 4 = 2`
- **Correct value**: `static_node_count: 2`
- **Result**: This configuration will create three separate GKE node pools, and each of those node pools will contain two nodes. The total number of TPU nodes created will be `3 * 2 = 6`.

### How to Calculate the Expected Device Count after running the `gke-tpu-7x-job.yaml`

The `jax.device_count()` command reports the total number of `TensorCores` in your slice, not the number of chips. To verify your setup, you must calculate this value based on your deployment configuration.

> The formula is: (Total Chips in Topology) * (2 TensorCores per TPU 7x Chip)

- Example Calculation:
  - If you deployed a `tpu_topology` of `2x2x1`
    - Total Chips = 2*2*1 = 4
    - Expected Device Count = 4 chips * 2 TensorCores/chip = 8
    - Your log output should be Global device count: 8.
  - If you deployed a larger `tpu_topology` of `4x4x4`
    - Total Chips = 4*4*4 = 64
    - Expected Device Count = 64 chips * 2 TensorCores/chip = 128
    - Your log output should be Global device count: 128.

### Understanding the GCS Integration

The blueprint provisions several key technologies to create a robust data pipeline for your TPU workloads. Here are some resources to understand how they work together:

- [Cloud Storage Overview](https://cloud.google.com/storage/docs/introduction#quickstarts): Start here to understand what Cloud Storage buckets are and their role in storing large-scale data.
- [Cloud TPU Storage Options](https://cloud.google.com/tpu/docs/storage-options): Learn about the recommended storage patterns for Cloud TPUs, including why GCS FUSE is a best practice for providing training data.
- [Access GCS Buckets with the GCS FUSE CSI Driver](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/cloud-storage-fuse-csi-driver): This is the core technical guide explaining how GKE mounts GCS buckets into your pods, which this blueprint automates.
- [Configure Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity): Read this to understand the secure, recommended method for GKE applications to access Google Cloud services like GCS, which this blueprint configures for you.

### Understanding Managed Lustre integration

The advanced blueprint `gke-tpu-7x-advanced.yaml` can also be configured to deploy a Managed Lustre filesystem. Google Cloud **Managed Lustre** delivers a high-performance, fully managed parallel file system optimized for AI and HPC applications. With multi-petabyte-scale capacity and up to 1 TBps throughput, [Managed Lustre](https://cloud.google.com/architecture/optimize-ai-ml-workloads-managed-lustre) facilitates the migration of demanding workloads to the cloud.

#### Enabling Managed Lustre

To enable Managed Lustre, you must make these changes before deploying:

1. Find the **vars:** section and **uncomment** the Managed Lustre variables. The defaults provide a high-performance **36000GiB** (~35.16TiB) filesystem with **18 GB/s** of throughput.
2. Find the section commented # --- MANAGED LUSTRE ADDITIONS ---. **Uncomment** the entire block of four modules: `private_service_access`, `lustre_firewall_rule`, `managed-lustre`, and `lustre-pv`.

After making these changes, run the `gcluster deploy` command as usual.

#### Using Managed Lustre in a Pod

Once deployed, the `Lustre` filesystem is available to the cluster as a `Persistent Volume (PV)`.

#### Testing the Lustre Mount

1. Connect to your cluster:

    ```sh
    gcloud container clusters get-credentials DEPLOYMENT_NAME --region=REGION --project=PROJECT_ID
    ```

   Replace the `DEPLOYMENT_NAME`,`REGION` and `PROJECT_ID` with the ones used in the blueprint.
2. List all PVCs in the relevant namespace. If you haven't specified a namespace, it's likely the default namespace.

    ```sh
    # To list PVCs in the default namespace
    kubectl get pvc
    ```

   Ideally, it should be named `<DEPLOYMENT_NAME>-vpc`.
3. Create a file named `lustre-claim-pod.yaml`:

    ```yaml
    apiVersion: v1
    kind: Pod
    metadata:
      name: lustre-claim-pod
    spec:
      containers:
      - name: app
        image: busybox
        command: ["/bin/sh", "-c", "sleep 36000"] # Keep the container running
        volumeMounts:
        - mountPath: "/mnt/lustre"
          name: lustre-volume
      volumes:
      - name: lustre-volume
        persistentVolumeClaim:
          claimName: my-lustre-claim # Must match the PVC name obtained above
    ```

4. Apply the manifest to your cluster:

    ```yaml
    kubectl apply -f <path/to/lustre-claim-pod.yaml>
    ```

5. Check if the pod is running and the volume is mounted:

    ```sh
    kubectl get pod lustre-claim-pod
    # Wait for the pod to be in the 'Running' state

    kubectl exec -it lustre-claim-pod -- /bin/sh

    df -h /mnt/lustre
    mount | grep lustre
    ```

   You should see the Managed Lustre file system mounted at /mnt/lustre, and you can now read/write data to this path from within the container.

### Understanding Hyperdisk Balanced Integration

The blueprint supports [Hyperdisk Balanced](https://docs.cloud.google.com/compute/docs/disks/hyperdisks), Google Cloud's high-performance, persistent block storage solution.

To enable Hyperdisk Balanced integration, you must make these changes before deploying:

1. Ensure the GKE cluster is configured to support standard Persistent Disks (the Hyperdisk CSI driver runs automatically once enabled). Verify the `gke-tpu-7x-cluster` module setting `enable_persistent_disk_csi: true` is present.
2. Find the section commented `--- HYPERDISK BALANCED ADDITIONS ---`. Uncomment the entire block containing the following two modules:
   - `hyperdisk-balanced-setup`: This module creates a **StorageClass** and a **PersistentVolumeClaim (PVC)** that will dynamically provision a Hyperdisk Balanced volume in your cluster.
   - `fio-bench-job-hyperdisk`: This job is pre-configured to mount the Hyperdisk volume and run performance tests.

After making these changes, run the `gcluster deploy` command as usual.

#### Testing the Hyperdisk Balanced Mount

1. Connect to your cluster:

    ```sh
    gcloud container clusters get-credentials DEPLOYMENT_NAME --region=REGION --project=PROJECT_ID
    ```

   Replace the `DEPLOYMENT_NAME`,`REGION` and `PROJECT_ID` with the ones used in the blueprint.
2. Apply the generated FIO Job manifest, whose path is provided in the final deployment instructions.

    ```sh
    kubectl apply -f <path/to/fio-benchmark.yaml>
    ```

   The job created in the cluster will be named `fio-benchmark-HdB`.
3. Monitor the job until it completes and obtain the list of pods:

    ```bash
    kubectl get jobs 
    kubectl get pods
    ```

4. View the logs of the completed pod to check the benchmark results:

    ```bash
    kubectl logs <pod-name>
    ```

   The logs of the pod verifies the disk is mounted successfully and performs a mixed I/O test to validate the disk's provisioned performance.

### Understanding Filestore integration

To enable Filestore integration, perform the following steps before deploying:

1. In the `gke-tpu-7x-cluster` module settings, ensure `enable_filestore_csi: true` is set.
2. Find the section commented `--- FILESTORE ADDITIONS ---`. Uncomment the following modules:
   - `filestore`: Provisions the Filestore instance and specifies the `local_mount` point.
   - `shared-filestore-pv`: Creates the Kubernetes Persistent Volume and Claim.
   - `shared-fs-job`: (Optional) A test job template to verify multi-node shared writing.

#### Testing the Shared Filestore Mount
The blueprint includes a sample job (`shared-fs-job`) that demonstrates how two different pods can write to and read from the same file simultaneously.

1. Connect to your cluster:

    ```sh
    gcloud container clusters get-credentials DEPLOYMENT_NAME --region=REGION --project=PROJECT_ID
    ```

    Replace the `DEPLOYMENT_NAME`,`REGION` and `PROJECT_ID` with the ones used in the blueprint.

2. Apply the Filestore test manifest,whose path is provided in the final deployment instructions:

    ```sh
    kubectl apply -f <path/to/shared-fs-job.yaml>
    ```

3. Verify the Shared Output: Once the pods are running, check the logs of the first pod to see it reading data written by the second pod:

    ```sh
    # Get pod names
    kubectl get pods
    
    # Check logs for the first pod
    kubectl logs <pod-name-0>
    ```

The logs will display content from `shared_output.txt`, showing timestamps and hostnames from both pods, confirming that the filesystem is truly shared.
