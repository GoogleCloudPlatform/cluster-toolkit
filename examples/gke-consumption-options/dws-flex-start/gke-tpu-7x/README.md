## Create a TPU 7x Flex cluster
These steps guide you through the cluster creation process for TPUs using DWS Flex Start.

1. Complete the common setup steps (1-4) in the [Create a cluster](https://github.com/GoogleCloudPlatform/cluster-toolkit/tree/main/examples/gke-consumption-options/dws-flex-start#create-a-cluster) section.

1. In the `examples/gke-consumption-options/dws-flex-start/gke-tpu-7x/gke-tpu-7x-deployment.yaml` file, fill in the following settings in the terraform_backend_defaults and vars sections to match the specific values for your deployment:

    `bucket`: the name of the Cloud Storage bucket you created in the previous step.
    `deployment_name`: the name of the deployment.
    `project_id`: your Google Cloud project ID.
    `region`: the compute region for the cluster.
    `zone`: the compute zone for the node pool of TPU 7x machines.
    **`enable_flex_start`**: set to `true` to enable DWS Flex Start.
    **`autoscaling_min_node_count`**: set to `0` (required for Flex Start).
    **`autoscaling_max_node_count`**: set to the required node count for your topology (e.g., `2` for a `2x2x2` topology).
    `authorized_cidr`: The IP address range that you want to allow to connect with the cluster.
    To modify advanced settings, edit `examples/gke-consumption-options/dws-flex-start/gke-tpu-7x/gke-tpu-7x.yaml`.

1. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

    ```sh
    gcloud auth application-default login
    ```

1. Deploy the blueprint to provision the GKE infrastructure using TPU 7x machine types:

    ```sh
    cd ~/cluster-toolkit
    ./gcluster deploy -d \
    examples/gke-consumption-options/dws-flex-start/gke-tpu-7x/gke-tpu-7x-deployment.yaml \
    examples/gke-consumption-options/dws-flex-start/gke-tpu-7x/gke-tpu-7x.yaml
    ```

1. When prompted, select (A)pply to deploy the blueprint.

## Note

* DWS Flex Start does not work with static nodes. So, static_node_count cannot be set.
* To use DWS Flex Start, `auto_repair` should be set to `false`.

## Running Workloads with DWS Flex Start

The Cluster Toolkit automatically generates a pre-configured, DWS-compliant job file located in your deployment folder (e.g., `gke-tpu-7x-flex/primary/my-job-xxxx.yaml`).

If you wish to create your own custom job, ensure it includes the following critical settings:

### 1. Node Selectors & Tolerations
The job must target the TPU 7x nodes and tolerate the standard TPU taint:

```yaml
nodeSelector:
  cloud.google.com/gke-tpu-accelerator: "tpu7x"
  cloud.google.com/gke-tpu-topology: "2x2x2"
tolerations:
- key: google.com/tpu
  operator: Equal
  value: "present"
  effect: NoSchedule
```

### 2. Resource Requests

Each pod in the job should request the full amount of TPU chips available on the node (typically 4 for `tpu7x-standard-4t`):

```yaml
resources:
  limits:
    google.com/tpu: 4
  requests:
    google.com/tpu: 4
```

### 3. Parallelism

The `parallelism` and `completions` count in your Kubernetes Job (or ReplicatedJob replicas in a JobSet) must exactly match the number of nodes in your TPU slice (e.g., `2` for a `2x2x2` topology).

## Testing TPU Flex Start (Dynamic Scaling)

When using the TPU Flex Start model, the cluster begins with **0 nodes** in the TPU node pool. You can verify the dynamic scaling by following these steps:

1. **Monitor the cluster status:**
    Open two terminals. In the first, watch the pods:

    ```sh
    kubectl get pods -w
    ```

    In the second, watch the nodes:

    ```sh
    kubectl get nodes -w
    ```

2. **Submit the TPU job:**
    Submit the generated job file:

    ```sh
    kubectl apply -f <deployment_folder>/primary/my-job-xxxx.yaml
    ```

3. **Observe Scale-Up:**
   * **Initial State:** Pods will show as `Pending`.

    ```text
    NAME                READY   STATUS    RESTARTS   AGE
    my-job-2932-0-q2ksv   0/2     Pending   0          10s
    ```

   * **Autoscaling Triggered:** Check events to see the scale-up trigger: `kubectl get events`. You will see     `TriggeredScaleUp`.

   * **Nodes Joining:** After a few minutes, nodes will appear and transition to `Ready`.

    ```text
    NAME                    STATUS   ROLES    AGE   VERSION
    gke-tpu-7f1325ce-8hwg   Ready    <none>   10s   v1.34.1-gke.3971000
    ```

   * **Running:** Once nodes are ready, pods will transition to `Running`.

4. **Observe Scale-Down:**
   * **Completion:** Once the workload finishes, pods will move to `Completed`.

     ```text
     NAME                READY   STATUS      RESTARTS   AGE
     my-job-2932-0-q2ksv   0/2     Completed   0          5m
     ```

   * **Automatic Removal:** After a short idle period (typically 1-10 minutes), the Cluster Autoscaler will delete the nodes.

     ```text
     gke-tpu-7f1325ce-8hwg   NotReady,SchedulingDisabled   <none>   6m   v1.34.1-gke.3971000
     ```

   * **Final State:** The node pool will return to **0 nodes**.
