# Create a GKE Cluster with A4 nodes

This example shows how to create your own [Hypercompute Cluster](https://cloud.google.com/ai-hypercomputer/docs/hypercompute-cluster) with Google Kubernetes Engine (GKE) to support your AI and ML workloads, using A4 GPUs.

GKE is the open, portable, extensible, and highly scalable platform for Hypercompute Cluster. GKE provides a single platform surface to run a diverse set of workloads for your organization's needs. This includes high performance distributed pre-training, model fine-tuning, model inference, application serving, and supporting services. GKE reduces the operational burden of managing multiple platforms.

The following instructions use [Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/overview), which lets you create your GKE cluster quickly while incorporating best practices. Through Cluster Toolkit, you have access to reference design blueprints that codify the Hypercompute Cluster environment on GKE including compute, storage, and networking resources. Additionally, Cluster Toolkit sets up the cluster to use GPUDirect RDMA-over-Converged-Ethernet (RoCE) for distributed AI workloads.

## Before you begin

Before you start, make sure you have performed the following tasks:

* Enable the Google Kubernetes Engine API.

* If you want to use the Google Cloud CLI for this task, [install](https://cloud.google.com/sdk/docs/install) and then [initialize](https://cloud.google.com/sdk/docs/initializing) the gcloud CLI. If you previously installed the gcloud CLI, get the latest version by running gcloud components update.
  > **NOTE:** For existing gcloud CLI installations, make sure to set the compute/region and compute/zone properties. By setting default locations, you can avoid errors in gcloud CLI like the following: One of [--zone, --region] must be supplied: Please specify location.

* Ensure that you have enough quota for A4 GPUs. To request more quota,
  follow the instructions in [GPU quota](https://cloud.google.com/compute/resource-usage#gpu_quota). To ensure that your cluster has capacity, you can follow the instructions to [reserve capacity](#reserve-capacity).

* Ensure that you have the following roles enabled:
  * `roles/editor`
  * `roles/container.clusterAdmin`
  * `roles/iam.serviceAccountAdmin`

### Requirements

The following requirements apply to GKE Hypercompute Cluster:

* The B200 GPUs in A4 VMs require a minimum of 570 GPU driver version, which is available in GKE 1.32 as `LATEST` driver version. For A4, you must set `gpu_driver_version: "LATEST"` with GKE 1.32.
* To use GPUDirect RDMA, use GKE patch version 1.32.1-gke.1420000 or higher.
* To use GPUDirect RDMA, the GKE nodes must use a Container-Optimized OS node image. Ubuntu and Windows node images are not supported.

## Reserve capacity

To ensure that your workloads have the A4 GPU resources required for these instructions, you can create a [future reservation request](https://cloud.google.com/compute/docs/instances/future-reservations-overview). With this request, you can reserve blocks of capacity for a defined duration in the future. At that date and time in the future, Compute Engine automatically provisions the blocks of capacity by creating on-demand reservations that you can immediately consume by provisioning node pools for this cluster.

Additionally, as your reserved capacity might span multiple
[blocks](https://cloud.google.com/ai-hypercomputer/docs/terminology#block), we recommend that you create GKE nodes on a specific block within your reservation.

Do the following steps to request capacity and gather the required information
to create nodes on a specific block within your reservation:

1. [Request capacity](https://cloud.google.com/ai-hypercomputer/docs/request-capacity).

1. To get the name of the blocks that are available for your reservation,
   run the following command:

   ```sh
   gcloud beta compute reservations blocks list RESERVATION_NAME \
       --zone=COMPUTE_ZONE --format "value(name)"
   ```

   Replace the following:

   * `RESERVATION_NAME`: the name of your reservation.
   * `COMPUTE_ZONE`: the compute zone of your reservation.

   The output has the following format: BLOCK_NAME.
   For example the output might be similar to the following: `example-res1-block-0001`.

1. If you want to target specific blocks within a reservation when
   provisioning GKE node pools, you must specify the full reference
   to your block as follows:

    ```none
   RESERVATION_NAME/reservationBlocks/BLOCK_NAME
   ```

   For example, using the example output in the preceding step, the full path is as follows: `example-res1/reservationBlocks/example-res1-block-0001`

## Create a cluster using Cluster Toolkit

This section guides you through the cluster creation process, ensuring that your project follows best practices and meets the [requirements](#requirements) for GKE Hypercompute Cluster.

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

1. In the [`examples/gke-a4/gke-a4-deployment.yaml`](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/develop/examples/gke-a4/gke-a4-deployment.yaml) file, replace the following variables in the `terraform_backend_defaults` and `vars` sections to match the specific values for your deployment:

   * `bucket`: the name of the Cloud Storage bucket you created in the previous step.
   * `project_id`: your Google Cloud project ID.
   * `region`: the compute region for the cluster.
   * `zone`: the compute zone for the node pool of A4 machines.
   * `authorized_cidr`: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine to call Terraform.
   * `extended_reservation`: the name of your reservation in the form of <project>/<reservation-name>/reservationBlocks/<reservation-block-name>
   * `static_node_count`: the number of A4 nodes in your cluster.

  To modify advanced settings, edit
  `examples/gke-a4/gke-a4.yaml`.

1. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

1. Deploy the blueprint to provision the GKE  infrastructure
    using A4 machine types:

   ```sh
   cd ~/cluster-toolkit
   ./gcluster deploy -d \
    examples/gke-a4/gke-a4-deployment.yaml \
    examples/gke-a4/gke-a4.yaml
   ```

## Deploy and run NCCL test with Topology Aware Scheduling (TAS)

To validate the functionality of the provisioned cluster, you can run a [NCCL test](https://github.com/NVIDIA/nccl-tests). To run a NCCL test with [Topology Aware Scheduling](https://kueue.sigs.k8s.io/docs/concepts/topology_aware_scheduling/),
complete the following steps.

1. Connect to your cluster:

    ```sh
    gcloud container clusters get-credentials gke-a4
    ```

1. Deploy an all-gather NCCL performance test with Topology Aware Scheduling
    enabled by using the [nccl-jobset-example.yaml](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/develop/examples/gke-a4/nccl-jobset-example.yaml) file.

    By default, this test uses two nodes. To change the number of nodes,
    modify the YAML file to change the following values from `2` to your required
    number of nodes:

   * `parallelism`
   * `completions`
   * `N_NODES`

    Create the resources to run the test:

    ```sh
    kubectl create -f ~/cluster-toolkit/examples/gke-a4/nccl-jobset-example.yaml
    ```

    This command returns a JobSet name.

    The output should be similar to the following:

    ```sh
    jobset.jobset.x-k8s.io/ag-4-9lkmq created
    ```

1. To view the results of the NCCL test, run this command to view all of the
    running Pods:

    ```sh
    kubectl get pods
    ```

    The output should be similar to the following:

    ```sh
    NAME                     READY   STATUS      RESTARTS   AGE
    ag-2-jnftb-w-0-0-8wrqq   0/1     Completed   0          74s
    ag-2-jnftb-w-0-1-kcxjj   0/1     Completed   0          74s
    ```

1. Find a Pod name matching the pattern `jobset-name-w-0-0-*`. The logs of this
    Pod contain the results of the NCCL test.

    To fetch the logs for this Pod, run this command:

    ```sh
    kubectl logs ag-2-jnftb-w-0-0-8wrqq
    ```

    The output should be similar to the following:

    ```sh
    #       size         count      type   redop    root     time   algbw   busbw #wrong     time   algbw   busbw #wrong
    #        (B)    (elements)                               (us)  (GB/s)  (GB/s)            (us)  (GB/s)  (GB/s)       
            1024            16     float    none      -1    39.23    0.03    0.02      0    35.16    0.03    0.03      0
            2048            32     float    none      -1    36.35    0.06    0.05      0    35.80    0.06    0.05      0
            4096            64     float    none      -1    36.21    0.11    0.11      0    35.88    0.11    0.11      0
            8192           128     float    none      -1    36.87    0.22    0.21      0    36.60    0.22    0.21      0
           16384           256     float    none      -1    37.41    0.44    0.41      0    37.16    0.44    0.41      0
           32768           512     float    none      -1    39.60    0.83    0.78      0    39.18    0.84    0.78      0
           65536          1024     float    none      -1    40.90    1.60    1.50      0    41.00    1.60    1.50      0
          131072          2048     float    none      -1    45.50    2.88    2.70      0    41.97    3.12    2.93      0
          262144          4096     float    none      -1    46.80    5.60    5.25      0    43.63    6.01    5.63      0
          524288          8192     float    none      -1    46.44   11.29   10.58      0    48.86   10.73   10.06      0
         1048576         16384     float    none      -1    81.56   12.86   12.05      0    80.30   13.06   12.24      0
         2097152         32768     float    none      -1    86.29   24.30   22.78      0    84.16   24.92   23.36      0
         4194304         65536     float    none      -1    95.18   44.07   41.31      0    89.88   46.67   43.75      0
         8388608        131072     float    none      -1    103.9   80.75   75.70      0    103.7   80.88   75.82      0
        16777216        262144     float    none      -1    132.9  126.23  118.34      0    132.4  126.72  118.80      0
        33554432        524288     float    none      -1    185.7  180.69  169.39      0    183.7  182.65  171.23      0
        67108864       1048576     float    none      -1    285.6  235.01  220.32      0    292.3  229.59  215.24      0
       134217728       2097152     float    none      -1    477.4  281.17  263.60      0    470.8  285.10  267.28      0
       268435456       4194304     float    none      -1    792.9  338.55  317.40      0    775.8  346.02  324.40      0
       536870912       8388608     float    none      -1   1456.3  368.65  345.61      0   1446.0  371.28  348.07      0
      1073741824      16777216     float    none      -1   2809.4  382.20  358.32      0   2788.3  385.08  361.02      0
      2147483648      33554432     float    none      -1   5548.2  387.06  362.87      0   5457.9  393.46  368.87      0
      4294967296      67108864     float    none      -1    11017  389.83  365.47      0    10806  397.48  372.63      0
      8589934592     134217728     float    none      -1    21986  390.71  366.29      0    21499  399.55  374.57      0
    # Out of bounds values : 0 OK
    # Avg bus bandwidth    : 128.335
    ```

## Clean up

To avoid recurring charges for the resources used on this page, clean up the resources provisioned by Cluster Toolkit, including the VPC networks and GKE cluster:

   ```sh
   ./gcluster destroy gke-a4/
   ```
