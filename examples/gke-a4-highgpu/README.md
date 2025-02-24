# Create a GKE Cluster with A4 High nodes

This example shows how to create your own [Hypercompute Cluster](https://cloud.google.com/ai-hypercomputer/docs/hypercompute-cluster) with Google Kubernetes Engine (GKE) to support your AI and ML workloads, using A4 High GPUs.

GKE is the open, portable, extensible, and highly scalable platform for Hypercompute Cluster. GKE provides a single platform surface to run a diverse set of workloads for your organization's needs. This includes high performance distributed pre-training, model fine-tuning, model inference, application serving, and supporting services. GKE reduces the operational burden of managing multiple platforms.

The following instructions use [Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/overview), which lets you create your GKE cluster quickly while incorporating best practices. Through Cluster Toolkit, you have access to reference design blueprints that codify the Hypercompute Cluster environment on GKE including compute, storage, and networking resources. Additionally, Cluster Toolkit sets up the cluster to use GPUDirect RDMA-over-Converged-Ethernet (RoCE) for distributed AI workloads.

## Before you begin

Before you start, make sure you have performed the following tasks:

* The user has the following roles: `roles/editor`, `roles/container.clusterAdmin`, and `roles/iam.serviceAccountAdmin`.

* Enable the Google Kubernetes Engine API.

* If you want to use the Google Cloud CLI for this task, [install](https://cloud.google.com/sdk/docs/install) and then [initialize](https://cloud.google.com/sdk/docs/initializing) the gcloud CLI. If you previously installed the gcloud CLI, get the latest version by running gcloud components update.
  > **NOTE:** For existing gcloud CLI installations, make sure to set the compute/region and compute/zone properties. By setting default locations, you can avoid errors in gcloud CLI like the following: One of [--zone, --region] must be supplied: Please specify location.
Ensure that you have enough quota for A4 High GPUs. To request more quota, follow the instructions in GPU quota. To ensure that your cluster has capacity, you can follow the instructions to reserve capacity.

* Ensure that you have enough quota for A4 High GPUs. To request more quota,
  follow the instructions in [GPU quota](https://cloud.google.com/compute/resource-usage#gpu_quota). To ensure that your cluster has capacity, you can follow the instructions to [reserve capacity](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#reserve-capacity).

### Requirements

The following requirements apply to GKE Hypercompute Cluster:

* The B200 GPUs in A4 High VMs require a minimum of 570 GPU driver version, which is available in GKE 1.32 as `LATEST` driver version. For A4 High, you must set `gpu_driver_version: "LATEST"` with GKE 1.32.
* To use GPUDirect RDMA, use GKE patch version 1.32.1-gke.1420000 or higher.
* To use GPUDirect RDMA, the GKE nodes must use a Container-Optimized OS node image. Ubuntu and Windows node images are not supported.

## Reserve capacity

To ensure that your workloads have the A4 High GPU resources required for these instructions, you can create a [future reservation request](https://cloud.google.com/compute/docs/instances/future-reservations-overview). With this request, you can reserve blocks of capacity for a defined duration in the future. At that date and time in the future, Compute Engine automatically provisions the blocks of capacity by creating on-demand reservations that you can immediately consume by provisioning node pools for this cluster.

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

This section guides you through the cluster creation process, ensuring that your project follows best practices and meets the [requirements](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute#requirements) for GKE Hypercompute Cluster.

> **NOTE:** Modify the deployment name to update the names of other infra resources automatically.

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

1. In the [`examples/gke-a4-highgpu/gke-a4-highgpu-deployment.yaml`](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/develop/examples/gke-a4-highgpu/gke-a4-highgpu-deployment.yaml) file, replace the following variables in the `terraform_backend_defaults` and `vars` sections to match the specific values for your deployment:

   * `BUCKET_NAME`: the name of the Cloud Storage bucket you created in the previous step.
   * `PROJECT_ID`: your Google Cloud project ID.
   * `COMPUTE_REGION`: the compute region for the cluster.
   * `COMPUTE_ZONE`: the compute zone for the node pool of A4 High machines.
   * `IP_ADDRESS/SUFFIX`: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine to call Terraform.
   * `RESERVATION_NAME`: the name of your reservation.
   * `BLOCK_NAME`: the name of a specific block within the reservation.
   * `NODE_COUNT`: the number of A4 High nodes in your cluster.

  To modify advanced settings, edit
  `examples/gke-a4-highgpu/gke-a4-highgpu.yaml`.

1. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

1. Deploy the blueprint to provision the GKE  infrastructure
    using A4 High machine types:

   ```sh
   cd ~/cluster-toolkit
   ./gcluster deploy -d \
    examples/gke-a4-highgpu/gke-a4-highgpu-deployment.yaml \
    examples/gke-a4-highgpu/gke-a4-highgpu.yaml
   ```

## Deploy and run NCCL test with Topology Aware Scheduling (TAS)

To validate the functionality of the provisioned cluster, you can run a [NCCL test](https://github.com/NVIDIA/nccl-tests). To run a NCCL test with [Topology Aware Scheduling](https://kueue.sigs.k8s.io/docs/concepts/topology_aware_scheduling/),
complete the following steps.

1. Connect to your cluster:

    ```sh
    gcloud container clusters get-credentials gke-a4-high
    ```

1. Deploy an all-gather NCCL performance test with Topology Aware Scheduling
    enabled by using the [nccl-jobset-example.yaml](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/develop/examples/gke-a4-highgpu/nccl-jobset-example.yaml) file.

    By default, this test uses four nodes. To change the number of nodes,
    modify the YAML file to change the following values from `4` to your required
    number of nodes:

   * `parallelism`
   * `completions`
   * `N_NODES`

    Create the resources to run the test:

    ```sh
    kubectl create -f ~/cluster-toolkit/examples/gke-a4-highgpu/nccl-jobset-example.yaml
    ```

    This command returns a JobSet name.

    The output should be similar to the following:

    ```none {:.devsite-disable-click-to-copy}
    jobset.jobset.x-k8s.io/all-gather8t7dt created
    ```

1. To view the results of the NCCL test, run this command to view all of the
    running Pods:

    ```sh
    kubectl get pods
    ```

    The output should be similar to the following:

    ```none {:.devsite-disable-click-to-copy}
    NAME                          READY   STATUS      RESTARTS   AGE
    all-gather8t7dt-w-0-0-n9s6j   0/1     Completed   0          9m34s
    all-gather8t7dt-w-0-1-rsf7r   0/1     Completed   0          9m34s
    ```

1. Find a Pod name matching the pattern `jobset-name-w-0-0-*`. The logs of this
    Pod contain the results of the NCCL test.

    To fetch the logs for this Pod, run this command:

    ```sh
    kubectl logs all-gather8t7dt-w-0-0-n9s6j
    ```

    The output should be similar to the following:

    ```none {:.devsite-disable-click-to-copy}
    #       size         count      type   redop    root     time   algbw   busbw #wrong     time   algbw   busbw #wrong
    #        (B)    (elements)                               (us)  (GB/s)  (GB/s)            (us)  (GB/s)  (GB/s)
            1024            16     float    none      -1    54.07    0.02    0.02      0    55.80    0.02    0.02      0
            2048            32     float    none      -1    55.46    0.04    0.03      0    55.31    0.04    0.03      0
            4096            64     float    none      -1    55.59    0.07    0.07      0    55.38    0.07    0.07      0
            8192           128     float    none      -1    56.05    0.15    0.14      0    55.92    0.15    0.14      0
           16384           256     float    none      -1    57.08    0.29    0.27      0    57.75    0.28    0.27      0
           32768           512     float    none      -1    57.49    0.57    0.53      0    57.22    0.57    0.54      0
           65536          1024     float    none      -1    59.20    1.11    1.04      0    59.20    1.11    1.04      0
          131072          2048     float    none      -1    59.58    2.20    2.06      0    63.57    2.06    1.93      0
          262144          4096     float    none      -1    63.87    4.10    3.85      0    63.61    4.12    3.86      0
          524288          8192     float    none      -1    64.83    8.09    7.58      0    64.40    8.14    7.63      0
         1048576         16384     float    none      -1    79.74   13.15   12.33      0    76.66   13.68   12.82      0
         2097152         32768     float    none      -1    78.41   26.74   25.07      0    79.05   26.53   24.87      0
         4194304         65536     float    none      -1    83.21   50.41   47.26      0    81.25   51.62   48.39      0
         8388608        131072     float    none      -1    94.35   88.91   83.35      0    99.07   84.68   79.38      0
        16777216        262144     float    none      -1    122.9  136.55  128.02      0    121.7  137.83  129.21      0
        33554432        524288     float    none      -1    184.2  182.19  170.80      0    178.1  188.38  176.60      0
        67108864       1048576     float    none      -1    294.7  227.75  213.51      0    277.7  241.62  226.52      0
       134217728       2097152     float    none      -1    495.4  270.94  254.00      0    488.8  274.60  257.43      0
       268435456       4194304     float    none      -1    877.5  305.92  286.80      0    861.3  311.65  292.17      0
       536870912       8388608     float    none      -1   1589.8  337.71  316.60      0   1576.2  340.61  319.33      0
      1073741824      16777216     float    none      -1   3105.7  345.74  324.13      0   3069.2  349.85  327.98      0
      2147483648      33554432     float    none      -1   6161.7  348.52  326.74      0   6070.7  353.75  331.64      0
      4294967296      67108864     float    none      -1    12305  349.03  327.22      0    12053  356.35  334.08      0
      8589934592     134217728     float    none      -1    24489  350.77  328.85      0    23991  358.05  335.67      0
    # Out of bounds values : 0 OK
    # Avg bus bandwidth    : 120.248
    ```

## Clean up

To avoid recurring charges for the resources used on this page, clean up the resources provisioned by Cluster Toolkit, including the VPC networks and GKE cluster:

   ```sh
   ./gcluster destroy gke-a4-high/
   ```
