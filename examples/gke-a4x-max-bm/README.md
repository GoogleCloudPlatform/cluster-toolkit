### Requirements

The following requirements apply to an AI-optimized A4X-Max Bare Metal GKE cluster:

1. Your project must be allowlisted to use A4X-Max machine type. Please work with your account team to get your project allowlisted.
2. The recommended GKE version for A4X-Max support is 1.34.1-gke.3849001. The GB300 GPUs in A4X-Max require a minimum of the 580.95.05 GPU driver version. GKE, by default, automatically installs this driver version on all A4X-Max nodes that run the required minimum version for A4X-Max, which is 1.34.1-gke.3849001

### Creation of cluster

1. [Launch Cloud Shell](https://docs.cloud.google.com/shell/docs/launching-cloud-shell). You can use a different environment; however, we recommend Cloud Shell because the dependencies are already pre-installed for Cluster Toolkit. If you don't want to use Cloud Shell, follow the instructions to [install dependencies](https://docs.cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) to prepare a different environment.
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
        --project=PROJECT_ID \
        --location=COMPUTE_REGION_TERRAFORM_STATE \
        --uniform-bucket-level-access
    gcloud storage buckets update gs://BUCKET_NAME --versioning
    ```

5. Replace the following variables:
   * BUCKET_NAME: the name of the new Cloud Storage bucket.
   * PROJECT_ID: your Google Cloud project ID.
   * COMPUTE_REGION_TERRAFORM_STATE: the compute region where you want to store the state of the Terraform deployment.

6. In the [examples/gke-a4x-max-bm/gke-a4x-max-bm-deployment.yaml blueprint from the GitHub repo](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples/gke-a4x-max-bm/gke-a4x-max-bm-deployment.yaml), fill in the following settings in the terraform\_backend\_defaults and vars sections to match the specific values for your deployment:
   * DEPLOYMENT_NAME: a unique name for the deployment, which must be between 6 and 30 characters in length. If the deployment name isn't unique within a project, cluster creation fails. The default value is gke-a4x-max-bm.
   * BUCKET_NAME: the name of the Cloud Storage bucket you created in the previous step.
   * PROJECT_ID: your Google Cloud project ID.
   * COMPUTE_REGION: the compute region for the cluster.
   * COMPUTE_ZONE: the compute zone for the node pool of A4X Max machines. Note that this zone should match the zone where machines are available in your reservation.
   * NODE_COUNT: the number of A4X Max nodes in your cluster's node pool, which must be 18 nodes or less. We recommend using 18 nodes to obtain the GPU topology of 1x72 in one subblock using an NVLink domain.
   * IP_ADDRESS/SUFFIX: the IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine that you want to use to call Terraform. For more information, see [How authorized networks work](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/network-isolation#how_authorized_networks_work).
   * For the extended\_reservation field, use one of the following, depending on whether you want to target specific [blocks](https://docs.cloud.google.com/ai-hypercomputer/docs/terminology#block) in a reservation when provisioning the node pool:
     * To place the node pool anywhere in the reservation, provide the name of your reservation (RESERVATION_NAME).
     * To target a specific block within your reservation, use the reservation and block names in the following format:

            ```text
            RESERVATION_NAME/reservationBlocks/BLOCK_NAME
            ```

   * If you don't know which blocks are available in your reservation, see [View a reservation topology](https://docs.cloud.google.com/ai-hypercomputer/docs/view-reserved-capacity#view-capacity-topology).
   * Set the boot disk sizes for each node of the system and A4X Max node pools. The disk size that you need depends on your use case. For example, if you use the disk as a cache to reduce the latency of pulling an image repeatedly, you can set a larger disk size to accommodate your framework, model, or container image:
     * SYSTEM_NODE_POOL_DISK_SIZE_GB: the size of the boot disk for each node of the system node pool. The smallest allowed disk size is 10. The default value is 200.
     * A4X_MAX_NODE_POOL_DISK_SIZE_GB: the size of the boot disk for each node of the A4X Max node pool. The smallest allowed disk size is 10. The default value is 100.

7. To modify advanced settings, edit the [examples/gke-a4x-max-bm/gke-a4x-max-bm.yaml](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples/gke-a4x-max-bm/gke-a4x-max-bm.yaml) file.

8. [Generate Application Default Credentials (ADC)](https://docs.cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform. If you're using Cloud Shell, you can run the following command:

    ```bash
    gcloud auth application-default login
    ```

9. Deploy the blueprint to provision the GKE infrastructure using A4X Max machine types:

    ```bash
    cd ~/cluster-toolkit
    ./gcluster deploy -d \
    examples/gke-a4x-max-bm/gke-a4x-max-bm-deployment.yaml \
    examples/gke-a4x-max-bm/gke-a4x-max-bm.yaml
    ```

10. When prompted, select **(A)pply** to deploy the blueprint.

* The blueprint creates VPC networks, a GPU RDMA VPC network, service accounts, a cluster, and a node pool.
* To support the fio-bench-job-template job template in the blueprint, Google Cloud buckets, network storage, and persistent volumes resources are created.

### Run NCCL on GKE clusters

This section describes how to run [NCCL/gIB](https://docs.cloud.google.com/ai-hypercomputer/docs/nccl/overview) tests on GKE clusters:

1. Connect to your cluster:

    ```bash
    gcloud container clusters get-credentials CLUSTER_NAME \
        --location=COMPUTE_REGION
    ```

    Replace the following variables:

   * CLUSTER_NAME: the name of your cluster, which, for the clusters created with Cluster Toolkit, is based on the DEPLOYMENT_NAME.
   * COMPUTE_REGION: the name of the compute region.

2. Deploy an all-gather NCCL performance test by using the [gke-a4x-max-bm/nccl-jobset-example.yaml](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples/gke-a4x-max-bm/nccl-jobset-example.yaml) file:
    1. The test uses a certain number of nodes by default (2). If you want to change the number of nodes, modify the YAML file to change the following values to your required number of nodes:
        1. numNodes
        2. parallelism
        3. completions
        4. N_NODES
    2. Create the resources to run the test:

        ```bash
        kubectl create -f ~/cluster-toolkit/examples/gke-a4x-max-bm/nccl-jobset-example.yaml
        ```

3. Confirm that all nccl-test Pods have reached the Completed state:

    ```bash
    kubectl get pods
    ```

4. Find a Pod name matching the pattern nccl-all-worker-0-0-\*. The logs of this Pod contain the results of the NCCL test. To fetch the logs for this Pod, run the following command:

    ```bash
    kubectl logs $(kubectl get pods -o go-template='{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}' | grep nccl-all-worker-0-0)
    ```

### Clean up resources created by Cluster Toolkit

To avoid recurring charges for the resources used, clean up the resources provisioned by Cluster Toolkit, including the VPC networks and GKE cluster:

```bash
cd ~/cluster-toolkit
./gcluster destroy CLUSTER_NAME
```

Replace CLUSTER_NAME with the name of your cluster. For the clusters created with Cluster Toolkit, the cluster name is based on the DEPLOYMENT_NAME.
