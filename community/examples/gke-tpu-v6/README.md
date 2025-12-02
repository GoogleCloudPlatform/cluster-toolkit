# GKE TPU V6 blueprint

This example shows how a TPU cluster with v6 machines and topology 4x4 can be created. The example also includes a `tpu-available-chips.yaml` that creates a kubernetes service and job. The job includes commands to install `jax` and run a simple command using jax, on the TPU.

Key parameters when working with TPUs:

* `num_slices`: Number of TPU slices required. A slice is a collection of chips all located inside the same TPU Pod connected by high-speed inter chip interconnects (ICI). Slices are described in terms of chips or TensorCores, depending on the TPU version.
* `tpu_topology`: The TPU topology desired. Topology is the number and physical arrangement of the TPU chips in a TPU slice.

## Before you begin

Before you start, make sure you have performed the following tasks:

* Enable the Google Kubernetes Engine API.

* If you want to use the Google Cloud CLI for this task, [install](https://cloud.google.com/sdk/docs/install) and then [initialize](https://cloud.google.com/sdk/docs/initializing) the gcloud CLI. If you previously installed the gcloud CLI, get the latest version by running gcloud components update.
  > **NOTE:** For existing gcloud CLI installations, make sure to set the compute/region and compute/zone properties. By setting default locations, you can avoid errors in gcloud CLI like the following: One of [--zone, --region] must be supplied: Please specify location.

* Ensure that you have enough quota for TPUs.

* Ensure that you have the following roles enabled:
  * `roles/editor`
  * `roles/container.clusterAdmin`
  * `roles/iam.serviceAccountAdmin`

## Create a cluster using Cluster Toolkit

This section guides you through the cluster creation process, ensuring that your project follows best practices.

> **NOTE:** If you would like to create more than one cluster in a project, make sure you update the deployment name.

1. [Launch Cloud Shell](https://cloud.google.com/shell/docs/launching-cloud-shell). You can use a different environment; however, we recommend Cloud Shell because the dependencies are already pre-installed for Cluster Toolkit. If you don't want to use Cloud Shell, follow the instructions to [install dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) to prepare a different environment.

1. Clone the Cluster Toolkit from the git repository:

    ```sh
    cd ~
    git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
    ```

1. Install the Cluster Toolkit:

    ```sh
    cd cluster-toolkit && git checkout main && make
    ```

1. Create a Cloud Storage bucket to store the state of the Terraform deployment:

    ```sh
    gcloud storage buckets create gs://BUCKET_NAME \
    --default-storage-class=STANDARD \
    --location=COMPUTE_REGION \
    --uniform-bucket-level-access
    gcloud storage buckets update gs://BUCKET_NAME --versioning
    ```

   Replace the following variables:

   * `BUCKET_NAME`: the name of the new Cloud Storage bucket.
   * `COMPUTE_REGION`: the compute region where you want to store the state of the Terraform deployment.

1. In the [`community/examples/gke-tpu-v6/gke-tpu-v6-deployment.yaml`](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/community/examples/gke-tpu-v6/gke-tpu-v6-deployment.yaml) file, replace the following variables in the `terraform_backend_defaults` and `vars` sections to match the specific values for your deployment:

   * `bucket`: the name of the Cloud Storage bucket you created in the previous step.
   * `project_id`: your Google Cloud project ID.
   * `region`: the compute region for the cluster.
   * `zone`: the compute zone for the TPUs.
   * `num_slices`: the number of TPU slices to create.
   * `machine_type`: the machine type of the TPU.
   * `tpu_topology`: the TPU placement topology for pod slice node pool.
   * `static_node_count`: the number of TPU nodes in your cluster.
   * `authorized_cidr`: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine to call Terraform.
   * `reservation`: the name of the compute engine reservation of TPU v6 nodes.

    To modify advanced settings, edit `community/examples/gke-tpu-v6/gke-tpu-v6.yaml`.

1. To use on-demand capacity, you can remove the reservation usage by making the following changes.
   1. Remove the `reservation` variable from the [`gke-tpu-v6-deployment.yaml`](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/community/examples/gke-tpu-v6/gke-tpu-v6-deployment.yaml) file.
   1. Remove the `reservation_affinity` block from the nodepool module.

1. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

1. Deploy the blueprint to provision the GKE  infrastructure
    using TPU v6 machine types:

   ```sh
    cd ~/cluster-toolkit
    ./gcluster deploy -d \
    community/examples/gke-tpu-v6/gke-tpu-v6-deployment.yaml \
    community/examples/gke-tpu-v6/gke-tpu-v6.yaml
   ```

## Advanced Blueprint: GKE TPU with GCS Integration

This repository also includes an advanced blueprint, `gke-tpu-v6-advanced.yaml`, designed for production-ready workloads. It builds on the basic blueprint by adding several key features:
* **Dedicated Service Accounts** for nodes and workloads, following security best practices.
* **Automatic creation of two GCS buckets** for training data and checkpoints.
* **Performance-tuned GCS FUSE mounts** pre-configured in the cluster as Persistent Volumes.
* **Optional High-Performance Storage: [Managed Lustre](https://cloud.google.com/managed-lustre/docs/overview)** for high-performance, fully managed parallel file system optimized for heavy AI and HPC workloads. For details of configuring Managed Lustre, please refer to the [appendix](#understanding-managed-lustre-integration)

### Deploying the Advanced Blueprint

The process is nearly identical to the basic deployment.

1. Ensure you have completed steps 1-7 from the "Create a cluster" section above. The same `gke-tpu-v6-deployment.yaml` file can be used.

1. In the final deploy command, simply point to the `gke-tpu-v6-advanced.yaml` blueprint instead.

    ```sh
    cd ~/cluster-toolkit
    ./gcluster deploy -d \
    community/examples/gke-tpu-v6/gke-tpu-v6-deployment.yaml \
    community/examples/gke-tpu-v6/gke-tpu-v6-advanced.yaml
    ```

1. After deployment, the blueprint will output instructions for running a fio benchmark job. This job serves as a validation test to confirm that the GCS mounts are working correctly for both reading and writing. Follow the printed instructions to run the test.

## Run the sample job

The [tpu-available-chips.yaml](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/community/examples/gke-tpu-v6/tpu-available-chips.yaml) file creates a service and a job resource in kubernetes. It is based on https://cloud.google.com/kubernetes-engine/docs/how-to/tpus#tpu-chips-node-pool. The  workload returns the number of TPU chips across all of the nodes in a multi-host TPU slice.

1. Connect to your cluster:

    ```sh
    gcloud container clusters get-credentials gke-tpu-v6 --region=REGION --project_id=PROJECT_ID
    ```

    Replace the `REGION` and `PROJECT_ID` with the ones used in the blueprint.

1. Update the nodeSelector under the template spec of tpu-available-chips.yaml file. The values depend on the tpu accelerator and tpu topology used in the blueprint.

    ```yaml
    nodeSelector:
        cloud.google.com/gke-tpu-accelerator: tpu-v6-slice
        cloud.google.com/gke-tpu-topology: 4x4
    ```

1. Create the resources:

    ```sh
    kubectl create -f ~/cluster-toolkit/community/examples/gke-tpu-v6/tpu-multislice.yaml
    ```

    This command returns a service and a job name.

    The output should be:

    ```sh
    jobset.jobset.x-k8s.io/multislice-job configured
    ```

1. Obtain list of pods using:

    ```sh
    kubectl get pods
    ```

    Identify two pods with prefix `multislice-job-slice`.

1. Display logs of either of the pods using:

    ```sh
    kubectl logs <pod-name>
    ```

    This should display `Global device count: 32` at the end of the logs which is the number of TPU chips across all of the nodes in a multi-host TPU slice.

## Clean up

To avoid recurring charges for the resources used on this page, clean up the resources provisioned by Cluster Toolkit, including the VPC networks and GKE cluster:

   ```sh
   ./gcluster destroy gke-tpu-v6/
   ```

## Appendix

### Useful TPU links
1. [TPU architecture](https://cloud.google.com/tpu/docs/system-architecture-tpu-vm)
2. [TPU v6](https://cloud.google.com/tpu/docs/v6e)

### Understanding the GCS Integration

The advanced blueprint provisions several key technologies to create a robust data pipeline for your TPU workloads. Here are some resources to understand how they work together:
* [Cloud Storage Overview](https://cloud.google.com/storage/docs/introduction#quickstarts): Start here to understand what Cloud Storage buckets are and their role in storing large-scale data.
* [Cloud TPU Storage Options](https://cloud.google.com/tpu/docs/storage-options): Learn about the recommended storage patterns for Cloud TPUs, including why GCS FUSE is a best practice for providing training data.
* [Access GCS Buckets with the GCS FUSE CSI Driver](https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/cloud-storage-fuse-csi-driver): This is the core technical guide explaining how GKE mounts GCS buckets into your pods, which this blueprint automates.
* [Configure Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity): Read this to understand the secure, recommended method for GKE applications to access Google Cloud services like GCS, which this blueprint configures for you.

### Understanding Managed Lustre integration
The advanced blueprint `gke-tpu-v6-advanced.yaml` can also be configured to deploy a Managed Lustre filesystem. Google Cloud **Managed Lustre** delivers a high-performance, fully managed parallel file system optimized for AI and HPC applications. With multi-petabyte-scale capacity and up to 1 TBps throughput, [Managed Lustre](https://cloud.google.com/architecture/optimize-ai-ml-workloads-managed-lustre) facilitates the migration of demanding workloads to the cloud.

#### Enabling Managed Lustre
To enable Managed Lustre, you must make these changes before deploying:

1. In the `gke-tpu-v6-advanced.yaml`:
Find the **vars:** section and **uncomment** the Managed Lustre variables. The defaults provide a high-performance **36000GiB** (~35.16TiB) filesystem with **18 GB/s** of throughput.

2. In the `gke-tpu-v6-advanced.yaml`:
Find the section commented # --- MANAGED LUSTRE ADDITIONS ---. **Uncomment** the entire block of four modules: `private_service_access`, `lustre_firewall_rule`, `managed-lustre`, and `lustre-pv`.

After making these changes, run the `gcluster deploy` command as usual.

#### Using Managed Lustre in a Pod
Once deployed, the `Lustre` filesystem is available to the cluster as a `Persistent Volume (PV)`. To use it in your workloads, you need to create a `Persistent Volume Claim (PVC)` and mount it in your pod.

#### Testing the Lustre Mount

1. Create a file named `lustre-claim-pod.yaml`:

    ```yaml
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      name: my-lustre-claim
    spec:
      accessModes:
      - ReadWriteMany
      # storageClassName must be empty to bind to the manually created PV
      storageClassName: ""
      resources:
        requests:
          # This size must match lustre_size_gib from your variables
          storage: 36000Gi
    ---
    apiVersion: v1
    kind: Pod
    metadata:
      name: lustre-test-pod
    spec:
      containers:
      - name: test-container
        image: ubuntu:22.04
        command: ["/bin/sleep", "infinity"]
        volumeMounts:
        - name: lustre-storage
          mountPath: /mnt/lustre
      volumes:
      - name: lustre-storage
        persistentVolumeClaim:
          claimName: my-lustre-claim
    ```

2. Apply the manifest to your cluster:

    ```yaml
    kubectl apply -f lustre-claim-pod.yaml
    ```

The pod will start, and the Managed Lustre filesystem will be available inside the container at `/mnt/lustre`.

### Understanding Hyperdisk Balanced Integration
The blueprint also supports [Hyperdisk Balanced](https://docs.cloud.google.com/compute/docs/disks/hyperdisks), Google Cloud's high-performance, persistent block storage solution.

To enable Hyperdisk Balanced integration, you must make these changes before deploying:

1. Ensure the GKE cluster is configured to support standard Persistent Disks (the Hyperdisk CSI driver runs automatically once enabled). Verify the `gke-tpu-v6-cluster` module setting `enable_persistent_disk_csi: true` is present.

2. Find the section commented `--- HYPERDISK BALANCED ADDITIONS ---`. Uncomment the entire block containing the following two modules:
   * `hyperdisk-balanced-setup`: This module creates a **StorageClass** and a **PersistentVolumeClaim (PVC)** that will dynamically provision a Hyperdisk Balanced volume in your cluster.
   * `fio-bench-job-hyperdisk`: This job is pre-configured to mount the Hyperdisk volume and run performance tests.

After making these changes, run the `gcluster deploy` command as usual.

#### Testing the Hyperdisk Balanced Mount

1. Connect to your cluster:

    ```sh
    gcloud container clusters get-credentials DEPLOYMENT_NAME --region=REGION --project_id=PROJECT_ID
    ```

    Replace the `DEPLOYMENT_NAME`,`REGION` and `PROJECT_ID` with the ones used in the blueprint.
2. Apply the generated FIO Job manifest, whose path is provided in the final deployment instructions.

    ```sh
    kubectl apply -f <path/to/fio-benchmark.yaml>
    ```

    The job created in the cluster will be named `fio-benchmark`.
  
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
