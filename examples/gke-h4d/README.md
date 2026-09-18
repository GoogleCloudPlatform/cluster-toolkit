# GKE H4D Blueprint

This blueprint uses GKE to provision a Kubernetes cluster and an H4D node pool, along with networks and service accounts. Information about H4D machines can be found [here](https://cloud.google.com/blog/products/compute/new-h4d-vms-optimized-for-hpc).

> **_NOTE:_** The required GKE version for H4D support is >= 1.32.11-gke.1174000.

## Create a cluster

Follow these steps to configure and deploy the GKE-H4D cluster.

> [!NOTE]
> If you create multiple clusters using these blueprints, ensure that all VPC and subnet names are unique per project to avoid resource conflicts.

1. Launch [Cloud Shell](https://cloud.google.com/shell/docs/launching-cloud-shell). You can use a different environment; however, we recommend Cloud Shell because the dependencies are already pre-installed for Cluster Toolkit. If you don't want to use Cloud Shell, follow the [instructions to install dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) to prepare a different environment.

2. Clone the Cluster Toolkit from the git repository:

   ```sh
   cd ~
   git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
   ```

3. Install the Cluster Toolkit:

   ```sh
   cd cluster-toolkit && git checkout main && make
   ```

4. Create a Cloud Storage bucket to store the state of the Terraform deployment:

   ```sh
   gcloud storage buckets create gs://BUCKET_NAME \
       --project=PROJECT_ID \
       --default-storage-class=STANDARD \
       --location=COMPUTE_REGION \
       --uniform-bucket-level-access
   gcloud storage buckets update gs://BUCKET_NAME --versioning
   ```

5. In `examples/gke-h4d/gke-h4d-deployment.yaml`, configure the general deployment settings:
   - `bucket`: The name of the Cloud Storage bucket created in step 4.
   - `project_id`: Your Google Cloud project ID.
   - `deployment_name`: A unique name for your cluster deployment.
   - `region`: The GCP region for the cluster (e.g., `asia-southeast1`).
   - `zone`: The GCP zone for the H4D node pool (e.g., `asia-southeast1-a`).
   - `authorized_cidr`: The IP CIDR block permitted to access the Kubernetes control plane (e.g., `0.0.0.0/0` to allow all authorized users, or `<YOUR-IP-ADDRESS>/32`).

6. Select a consumption model:
   In `examples/gke-h4d/gke-h4d-deployment.yaml`, select **ONE** consumption model from the options provided. Option 1 (Specific Reservation) is uncommented by default. To use another consumption model, uncomment the desired option and comment out Option 1.

7. Generate Application Default Credentials (ADC) for Terraform:

   ```sh
   gcloud auth application-default login
   ```

8. Deploy the blueprint:

   ```sh
   ./gcluster deploy examples/gke-h4d/gke-h4d.yaml -d examples/gke-h4d/gke-h4d-deployment.yaml
   ```

   When prompted, select `(A)pply` to provision the VPC networks, Falcon IRDMA RDMA network, service accounts, GKE cluster, and H4D node pool.

---

## Running Workloads

### DWS Flex Start Test Job
When using DWS Flex Start (Option 2 or Option 3), the node pool initializes with 0 nodes and scales up on demand when matching jobs are scheduled.

A sample batch job is provided at `examples/gke-h4d/test-job-flex.yaml`.

Any job applied to this node pool must meet the following requirements:
- **Flex Start Selector**: Workloads must include `nodeSelector: cloud.google.com/gke-flex-start: "true"`.
- **Tolerations**: Because the `h4d-pool` node pool is tainted (`node-type=h4d:NoSchedule`) to prevent generic workloads from scheduling on HPC nodes, workloads **must** include the matching toleration:

  ```yaml
  tolerations:
  - key: "node-type"
    operator: "Equal"
    value: "h4d"
    effect: "NoSchedule"
  ```

#### Execution and Monitoring Steps
1. Connect to the GKE cluster:

   ```sh
   gcloud container clusters get-credentials <cluster-name> --region <region> --project <project-id>
   ```

2. Submit the sample test job:

   ```sh
   kubectl apply -f examples/gke-h4d/test-job-flex.yaml
   ```

3. Monitor the scale-up and execution lifecycle:
   - **Check Pod Status**: Initially, pods will be `Pending` because the H4D node pool is at size 0:

     ```sh
     kubectl get pods -w
     ```

     ```text
     NAME              READY   STATUS    RESTARTS   AGE
     h4d-job-1-q2ksv   0/1     Pending   0          10s
     h4d-job-2-j9wla   0/1     Pending   0          10s
     ```

   - **Inspect Autoscaler Events**: Check pod events to verify GKE Cluster Autoscaler triggered provisioning for the H4D group:

     ```sh
     kubectl describe pods -l job-name=h4d-job-1
     ```

     Look for the `TriggeredScaleUp` event:

     ```text
     Events:
       Type    Reason            Age   From                Message
       ----    ------            ---   ----                -------
       Normal  TriggeredScaleUp  15s   cluster-autoscaler  pod triggered scale-up by cluster-autoscaler: group h4d-pool-xxxx
     ```

   - **Track Node Readiness**: After the physical H4D VMs boot and register, the pods transition to `Running`:

     ```sh
     kubectl get nodes -w
     ```

   - **Observe Completion**: Once the sleep workload finishes, the pods will transition to `Completed`:

     ```sh
     kubectl get pods
     ```

     ```text
     NAME              READY   STATUS      RESTARTS   AGE
     h4d-job-1-q2ksv   0/1     Completed   0          2m
     h4d-job-2-j9wla   0/1     Completed   0          2m
     ```

4. Clean up the test job:

   ```sh
   kubectl delete -f examples/gke-h4d/test-job-flex.yaml
   ```

> [!NOTE]
> Since the node pool is configured with `max_run_duration: 900` (15 minutes), any provisioned nodes will be terminated by GKE after 15 minutes, or scaled down to 0 by Cluster Autoscaler when idle.

---

### Run a test using the MPI Operator
The MPI Operator is installed on the cluster during deployment to support distributed MPI workloads across the Falcon IRDMA interconnect.

For instructions and sample MPI jobs, refer to the [GKE HPC MPI samples](https://github.com/GoogleCloudPlatform/kubernetes-engine-samples/tree/main/hpc/mpi).

> [!NOTE]
> MPI Operator workloads require pre-provisioned nodes and are supported with static node pool options (Option 1: Specific Reservation, Option 4: Spot, and Option 5: On-Demand).

---

## Clean Up
To destroy all resources associated with the deployment, run:

```sh
./gcluster destroy CLUSTER_NAME
```

Replace `CLUSTER_NAME` with the `deployment_name` specified in your deployment file.

**Note:** GCS buckets created for Terraform state storage are not deleted by `./gcluster destroy` and must be removed manually if no longer needed.
