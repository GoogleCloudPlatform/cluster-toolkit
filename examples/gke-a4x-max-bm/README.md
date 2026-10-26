### Requirements

The following requirements apply to an AI-optimized A4X-Max Bare Metal GKE cluster:

1. Your project must be allowlisted to use A4X-Max machine type. Please work with your account team to get your project allowlisted.
2. The recommended GKE version for A4X-Max support is 1.34.3-gke.1318000. The GB300 GPUs in A4X-Max require a minimum of the 580.95.05 GPU driver version. GKE, by default, automatically installs this driver version on all A4X-Max nodes that run the required minimum version for A4X-Max, which is 1.34.3-gke.1318000

### Creation of cluster

1. [Launch Cloud Shell](https://docs.cloud.google.com/shell/docs/launching-cloud-shell). You can use a different environment; however, we recommend Cloud Shell because the dependencies are already pre-installed for Cluster Toolkit. If you don't want to use Cloud Shell, follow the instructions to [install dependencies](https://docs.cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) to prepare a different environment, and enable the required GCP APIs:

    ```bash
    gcloud services enable \
      compute.googleapis.com \
      container.googleapis.com \
      iam.googleapis.com \
      storage.googleapis.com \
      --project=<PROJECT_ID>
    ```

2. From the root of the [`cluster-toolkit`](https://github.com/GoogleCloudPlatform/cluster-toolkit) repository, build `./gcluster` (if not already built):

    ```bash
    test -x ./gcluster || make
    ```

3. Create a Cloud Storage bucket to store the state of the Terraform deployment:

    ```bash
    gcloud storage buckets create gs://BUCKET_NAME \
        --default-storage-class=STANDARD \
        --project=PROJECT_ID \
        --location=COMPUTE_REGION_TERRAFORM_STATE \
        --uniform-bucket-level-access
    gcloud storage buckets update gs://BUCKET_NAME --versioning
    ```

4. Replace the following variables:
   * BUCKET_NAME: the name of the new Cloud Storage bucket.
   * PROJECT_ID: your Google Cloud project ID.
   * COMPUTE_REGION_TERRAFORM_STATE: the compute region where you want to store the state of the Terraform deployment.

5. In [`examples/gke-a4x-max-bm/gke-a4x-max-bm-deployment.yaml`](gke-a4x-max-bm-deployment.yaml), fill in the `terraform_backend_defaults` and `vars` sections to match your deployment:

    ```yaml
    terraform_backend_defaults:
      type: gcs
      configuration:
        bucket: <BUCKET_NAME>

    vars:
      project_id: <PROJECT_ID>
      deployment_name: gke-a4x-max-bm-dep
      region: <COMPUTE_REGION>
      zone: <COMPUTE_ZONE>
      static_node_count: 18
      authorized_cidr: <IP_ADDRESS>/<SUFFIX>
      reservation: <RESERVATION_NAME>
      reservation_project_id: <PROJECT_ID>
      num_node_pools: 1
    ```

   * DEPLOYMENT_NAME (`deployment_name`): a unique name for the deployment, which must be between 6 and 30 characters in length. If the deployment name isn't unique within a project, cluster creation fails. The default value is `gke-a4x-max-bm-dep`.
   * BUCKET_NAME (`bucket`): the name of the Cloud Storage bucket you created in the previous step.
   * PROJECT_ID (`project_id` / `reservation_project_id`): your Google Cloud project ID (and the project ID hosting the reservation).
   * COMPUTE_REGION (`region`): the compute region for the cluster.
   * COMPUTE_ZONE (`zone`): the compute zone for the node pool of A4X Max machines. Note that this zone should match the zone where machines are available in your reservation.
   * NODE_COUNT (`static_node_count`): the number of A4X Max nodes in your cluster's node pool, which must be 18 nodes or less. We recommend using 18 nodes to obtain the GPU topology of 1x72 in one subblock using an NVLink domain.
   * IP_ADDRESS/SUFFIX (`authorized_cidr`): the IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine that you want to use to call Terraform. For more information, see [How authorized networks work](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/network-isolation#how_authorized_networks_work).
   * For the `reservation` field, use one of the following, depending on whether you want to target specific [blocks](https://docs.cloud.google.com/ai-hypercomputer/docs/terminology#block) in a reservation when provisioning the node pool:
     * To place the node pool anywhere in the reservation, provide the name of your reservation (RESERVATION_NAME).
     * To target a specific block within your reservation, use the reservation and block names in the following format:

            ```text
            RESERVATION_NAME/reservationBlocks/BLOCK_NAME
            ```

   * If you don't know which blocks are available in your reservation, see [View a reservation topology](https://docs.cloud.google.com/ai-hypercomputer/docs/view-reserved-capacity#view-capacity-topology).
   * Set the boot disk sizes for each node of the system and A4X Max node pools. The disk size that you need depends on your use case. For example, if you use the disk as a cache to reduce the latency of pulling an image repeatedly, you can set a larger disk size to accommodate your framework, model, or container image:
     * SYSTEM_NODE_POOL_DISK_SIZE_GB: the size of the boot disk for each node of the system node pool. The smallest allowed disk size is 10. The default value is 200.
     * A4X_MAX_NODE_POOL_DISK_SIZE_GB: the size of the boot disk for each node of the A4X Max node pool. The smallest allowed disk size is 10. The default value is 100.

6. To modify advanced settings, edit a copy of the [`examples/gke-a4x-max-bm/gke-a4x-max-bm.yaml`](gke-a4x-max-bm.yaml) file.

7. [Generate Application Default Credentials (ADC)](https://docs.cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform. If you're using Cloud Shell, you can run the following command:

    ```bash
    gcloud auth application-default login
    ```

8. Deploy the blueprint to provision the GKE infrastructure using A4X Max machine types:

    ```bash
    ./gcluster deploy -d \
    examples/gke-a4x-max-bm/gke-a4x-max-bm-deployment.yaml \
    examples/gke-a4x-max-bm/gke-a4x-max-bm.yaml
    ```

9. When prompted, select **(A)pply** to deploy the blueprint.

* The blueprint creates VPC networks, a GPU RDMA VPC network, service accounts, a cluster, and a node pool.
* To support the fio-bench-job-template job template in the blueprint, Google Cloud buckets, network storage, and persistent volumes resources are created.

### Run NCCL on GKE clusters

This section describes how to run [NCCL/gIB](https://docs.cloud.google.com/ai-hypercomputer/docs/nccl/overview) tests on GKE clusters:

1. Connect to your cluster:

    ```bash
    gcloud container clusters get-credentials DEPLOYMENT_NAME \
        --location=COMPUTE_REGION
    ```

    Replace the following variables:

   * DEPLOYMENT_NAME: the `deployment_name` value from your deployment file (the GKE cluster name matches `deployment_name`).
   * COMPUTE_REGION: the name of the compute region.

2. Deploy an all-gather NCCL performance test by using the [gke-a4x-max-bm/nccl-jobset-example.yaml](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples/gke-a4x-max-bm/nccl-jobset-example.yaml) file:
    1. The test uses a certain number of nodes by default (2). If you want to change the number of nodes, modify the YAML file to change the following values to your required number of nodes:
        1. numNodes
        2. parallelism
        3. completions
        4. N_NODES
    2. Create the resources to run the test:

        ```bash
        kubectl create -f examples/gke-a4x-max-bm/nccl-jobset-example.yaml
        ```

3. Confirm that all nccl-test Pods have reached the Completed state:

    ```bash
    kubectl get pods
    ```

4. Find a Pod name matching the pattern nccl-all-worker-0-0-\*. The logs of this Pod contain the results of the NCCL test. To fetch the logs for this Pod, run the following command:

    ```bash
    kubectl logs $(kubectl get pods -o go-template='{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}' | grep nccl-all-worker-0-0)
    ```

### Install and Run MPI Operator on GKE Cluster

The Kubeflow MPI Operator manages distributed MPI workloads on GKE.

1. **Deploy MPI Operator (v0.8.2):**

   * **Automated (During Cluster Creation via Blueprint YAML):** Include the MPI Operator manifest in `apply_manifests` under `kubectl-apply` in your blueprint YAML (`gke-a4x-max-bm.yaml`):

     ```yaml
       - id: kubectl-apply
         source: modules/management/kubectl-apply
         use: [a4x-max-cluster]
         settings:
           apply_manifests:
           - name: mpi-operator
             source: https://raw.githubusercontent.com/kubeflow/mpi-operator/v0.8.2/deploy/v2beta1/mpi-operator.yaml
     ```

   * **Manual (After Cluster Deployment via `kubectl`):** Once the cluster is deployed, run the following command against your cluster:

     ```bash
     kubectl apply --server-side -f https://raw.githubusercontent.com/kubeflow/mpi-operator/v0.8.2/deploy/v2beta1/mpi-operator.yaml
     ```

2. **Verify Installation:**

   ```bash
   kubectl get crd | grep mpijob
   kubectl get pods -n mpi-operator
   ```

3. **Run a Sample MPIJob Test:**
   Create a test manifest `sample-mpijob.yaml`:

   ```yaml
   apiVersion: kubeflow.org/v2beta1
   kind: MPIJob
   metadata:
     name: sample-mpi-job
     namespace: default
   spec:
     slotsPerWorker: 1
     runPolicy:
       cleanPodPolicy: Running
     mpiReplicaSpecs:
       Launcher:
         replicas: 1
         template:
           spec:
             containers:
             - name: mpi-launcher
               image: mpioperator/mpi-pi:v0.8.2-openmpi
               command:
               - mpirun
               - --allow-run-as-root
               - -n
               - "2"
               - --hostfile
               - /etc/mpi/hostfile
               - echo
               - "Hello World from MPI worker!"
       Worker:
         replicas: 2
         template:
           spec:
             containers:
             - name: mpi-worker
               image: mpioperator/mpi-pi:v0.8.2-openmpi
   ```

   Submit the job and inspect launcher logs:

   ```bash
   kubectl apply -f sample-mpijob.yaml
   kubectl logs -l training.kubeflow.org/job-role=launcher
   kubectl delete -f sample-mpijob.yaml
   ```

### Clean up resources created by Cluster Toolkit

To avoid recurring charges for the resources used, clean up the resources provisioned by Cluster Toolkit, including the VPC networks and GKE cluster:

```bash
./gcluster destroy DEPLOYMENT_NAME
```

Replace `DEPLOYMENT_NAME` with the `deployment_name` value from your deployment file (the deployment directory created by `gcluster`).

**Note:** GCS buckets created for Terraform state are not deleted by the `./gcluster destroy` command and must be deleted manually.
