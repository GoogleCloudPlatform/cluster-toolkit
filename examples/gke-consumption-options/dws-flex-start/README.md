# DWS Flex Start Consumption Option

[Dynamic Workload Scheduler (DWS)](https://cloud.google.com/blog/products/compute/introducing-dynamic-workload-scheduler) is a resource management and job scheduling platform designed for AI Hypercomputer. Dynamic Workload Scheduler improves your access to AI/ML resources, helps you optimize your spend, and can improve the experience of workloads such as training and fine-tuning jobs, by scheduling all the accelerators needed simultaneously. Dynamic Workload Scheduler supports TPUs and NVIDIA GPUs, and brings scheduling advancements from Google ML fleet to Google Cloud customers.

Note the `enable_flex_start` variable in the yaml files.

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

    Replace the following variables:\
    BUCKET_NAME: the name of the new Cloud Storage bucket.\
    PROJECT_ID: ID of the project where the bucket is being created.\
    COMPUTE_REGION: the compute region where you want to store the state of the Terraform deployment.

1. In the `examples/gke-consumption-options/dws-flex-start/gke-a3-ultragpu-deployment.yaml` file, fill in the following settings in the terraform_backend_defaults and vars sections to match the specific values for your deployment:

    `bucket`: the name of the Cloud Storage bucket you created in the previous step.\
    `deployment_name`: the name of the deployment.\
    `project_id`: your Google Cloud project ID.\
    `region`: the compute region for the cluster.\
    `zone`: the compute zone for the node pool of A3 Ultra machines.\
    **`enable_flex_start`**: enable DWS Flex Start.\
    `authorized_cidr`: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine to call Terraform.\
    `system_node_pool_disk_size_gb`: the size of disk for each node of the system node pool. Default value is 100.\
    `a3ultra_node_pool_disk_size_gb`: the size of disk for each node of the A3 Ultra node pool. Default value is 100.\
    To modify advanced settings, edit `examples/gke-consumption-options/dws-flex-start/gke-a3-ultragpu.yaml`.

1. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

    ```sh
    gcloud auth application-default login
    ```

1. Deploy the blueprint to provision the GKE infrastructure using A3 Ultra machine types:

    ```sh
    cd ~/cluster-toolkit
    ./gcluster deploy -d \
    examples/gke-consumption-options/dws-flex-start/gke-a3-ultragpu-deployment.yaml \
    examples/gke-consumption-options/dws-flex-start/gke-a3-ultragpu.yaml
    ```

1. When prompted, select (A)pply to deploy the blueprint.
   * The blueprint creates VPC networks, a GPU RDMA VPC network, service accounts, a cluster, and a nodepool.

## Run a job

The dws-flex-start example provides a `dws-flex-start.yaml` file that runs this example: https://cloud.google.com/kubernetes-engine/docs/how-to/dws-flex-start-training

1. Connect to the GKE cluster using gcloud command.

    ```sh
    gcloud container clusters get-credentials <cluster-name> --location <location> --project <project-name>
    ```

1. Run the jobs.

    ```sh
    kubectl apply -f examples/gke-consumption-options/dws-flex-start/dws-flex-start.yaml
    ```

1. Consider using `kubectl get jobs` and `kubectl describe job <job-name>` to get information about the jobs.\
You can also use `kubectl get pods` and `kubectl describe pod <pod-name>` to get pod information.

## Note
* DWS Flex Start does not work with static nodes. So, static_node_count cannot be set.
* To use DWS Flex Start, `auto_repair` should be set to `false`.
