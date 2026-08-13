# DWS Flex Start with Queued Provisioning Consumption Option

[Dynamic Workload Scheduler (DWS)](https://cloud.google.com/blog/products/compute/introducing-dynamic-workload-scheduler) is a resource management and job scheduling platform designed for AI Hypercomputer. Dynamic Workload Scheduler improves your access to AI/ML resources, helps you optimize your spend, and can improve the experience of workloads such as training and fine-tuning jobs, by scheduling all the accelerators needed simultaneously. Dynamic Workload Scheduler supports TPUs and NVIDIA GPUs, and brings scheduling advancements from Google ML fleet to Google Cloud customers.

Note the `enable_flex_start` and `enable_queued_provisioning` variables in the yaml files.

## Create a cluster

These steps guide you through the cluster creation process.

Note: If you create multiple clusters using these same cluster blueprints, ensure that all VPCs and subnet names are unique per project to prevent errors.

1. Launch [Cloud Shell](https://cloud.google.com/shell/docs/launching-cloud-shell). You can use a different environment; however, we recommend Cloud Shell because the dependencies are already pre-installed for Cluster Toolkit. If you don't want to use Cloud Shell, follow the [instructions to install dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) to prepare a different environment.
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
        --project=PROJECT_ID \
        --default-storage-class=STANDARD \
        --location=COMPUTE_REGION \
        --uniform-bucket-level-access
    gcloud storage buckets update gs://BUCKET_NAME --versioning
    ```

    Replace the following variables:

   * BUCKET_NAME: the name of the new Cloud Storage bucket.
   * PROJECT_ID: ID of the project where the bucket is being created.
   * COMPUTE_REGION: the compute region where you want to store the state of the Terraform deployment.

1. In the examples/gke-consumption-options/dws-flex-start-queued-provisioning/gke-a3-ultragpu-deployment.yaml file, fill in the following settings in the terraform_backend_defaults and vars sections to match the specific values for your deployment:

    `bucket`: the name of the Cloud Storage bucket you created in the previous step.\
    `deployment_name`: the name of the deployment.\
    `project_id`: your Google Cloud project ID.\
    `region`: the compute region for the cluster.\
    `zone`: the compute zone for the node pool of A3 Ultra machines.\
    **`enable_flex_start`**: enable DWS Flex Start.\
    **`enable_queued_provisioning`**: enable queued provisioning along with DWS Flex Start.\
    `authorized_cidr`: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine to call Terraform.\
    `system_node_pool_disk_size_gb`: the size of disk for each node of the system node pool. Default value is 100.\
    `system_node_pool_disk_size_gb`: the size of disk for each node of the system node pool. Default value is 100.\
    `a3ultra_node_pool_disk_size_gb`: the size of disk for each node of the A3 Ultra node pool.\
    To modify advanced settings, edit examples/gke-consumption-options/dws-flex-start-queued-provisioning/gke-a3-ultragpu.yaml.

1. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

    ```sh
    gcloud auth application-default login
    ```

1. Deploy the blueprint to provision the GKE infrastructure using A3 Ultra machine types:

    ```sh
    cd ~/cluster-toolkit
    ./gcluster deploy -d \
    examples/gke-consumption-options/dws-flex-start-queued-provisioning/gke-a3-ultragpu-deployment.yaml \
    examples/gke-consumption-options/dws-flex-start-queued-provisioning/gke-a3-ultragpu.yaml
    ```

1. When prompted, select (A)pply to deploy the blueprint.

   * The blueprint creates VPC networks, a GPU RDMA VPC network, service accounts, a cluster, and a nodepool.

## Note

* DWS Flex Start does not work with static nodes. So, `static_node_count` cannot be set.
* To use DWS Flex Start, `auto_repair` should be set to `false`.

Along with these flex start requirements, there are a few queue-provisioning specific requirements.

* Queued provisioning does not work with `static_node_count` and requires `autoscaling_total_min_nodes` be set to `0`.

## Run a job

The dws-flex-start-queued-provisioning example provides a `sample-job.yaml` file that runs a job *similar* to this example: https://cloud.google.com/kubernetes-engine/docs/how-to/provisioningrequest

1. Connect to the GKE cluster using gcloud command.

    ```sh
    gcloud container clusters get-credentials <cluster-name> --location <location> --project <project-id>
    ```

    Replace `<cluster-name>` with the name of your cluster, `<location>` with the name of the compute region, and `<project-id>` with the ID of the project.

1. Run the jobs.

    ```sh
    kubectl apply -f examples/gke-consumption-options/dws-flex-start-queued-provisioning/sample-job.yaml
    ```

1. Consider using `kubectl get jobs` and `kubectl describe job <job-name>` to get information about the jobs.\
    You can also use `kubectl get pods` and `kubectl describe pod <pod-name>` to get pod information.

## Deploy and run NCCL test

To validate the functionality of the provisioned cluster, you can run a NCCL test.

1. Connect to your cluster:

    ```sh
    gcloud container clusters get-credentials <cluster-name> --location <location> --project <project-id>
    ```

    Replace `<cluster-name>` with the name of your cluster, `<location>` with the name of the compute region, and `<project-id>` with the ID of the project.

1. Deploy an all-gather NCCL performance test using this file `nccl-jobset-example.yaml` in the example. The tests use `2` nodes by default. To change the number of nodes, modify the YAML file to change the following values to your required number of nodes:

    parallelism
    completions
    N_NODES

    Note that the `nccl-jobset-example.yaml` file has this config under jobset metadata. These are required for using queued provisioning.

    ```yaml
      labels:
        kueue.x-k8s.io/queue-name: dws-local-queue
      annotations:
        provreq.kueue.x-k8s.io/maxRunDurationSeconds: "600"
    ```

    Create the resources to run the test.

    ```sh
    kubectl create -f ~/cluster-toolkit/examples/gke-consumption-options/dws-flex-start/nccl-jobset-example.yaml
    ```

    This command returns a JobSet name.

    The output should be similar to the following:

    ```sh
    jobset.jobset.x-k8s.io/ag-2-fz9fs created
    ```

1. To view the results of the NCCL test, run this command to view all of the running Pods:

    ```sh
    kubectl get pods
    ```

    The output should be similar to the following:

    ```sh
    NAME                     READY   STATUS      RESTARTS   AGE
    ag-2-fz9fs-w-0-0-kkd5t   0/1     Completed   0          9m34s
    ag-2-fz9fs-w-0-1-s46gz   0/1     Completed   0          9m34s
    ```

1. Find a Pod name matching the pattern jobset-name-w-0-0-*. The logs of this Pod contain the results of the NCCL test.

    To fetch the logs for this Pod, run this command:

    ```sh
    kubectl logs ag-2-fz9fs-w-0-0-kkd5t
    ```

    The output should be similar to the following:

    ```sh
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

## Hardware-Specific Guides

For detailed deployment instructions, topology requirements, and job examples, please refer to the guide for your specific hardware:

* [TPU v6e (Trillium)](gke-tpu-v6e/README.md): Optimized for `ct6e-standard-4t` clusters.
* [TPU 7x (TPU v4)](gke-tpu-7x/README.md): Optimized for `tpu7x-standard-4t` clusters.
