# DWS Flex Start with Compact Placement (Workload Policy)

[Dynamic Workload Scheduler (DWS)](https://cloud.google.com/blog/products/compute/introducing-dynamic-workload-scheduler) is a resource management and job scheduling platform designed for AI Hypercomputer. Dynamic Workload Scheduler improves your access to AI/ML resources, helps you optimize your spend, and can improve the experience of workloads such as training and fine-tuning jobs, by scheduling all the accelerators needed simultaneously. Dynamic Workload Scheduler supports TPUs and NVIDIA GPUs, and brings scheduling advancements from Google ML fleet to Google Cloud customers.

This directory contains the GKE blueprint, deployment variables, and test job for running DWS Flex Start combined with a custom **Compute Engine Workload Policy** to enforce physical compact placement on the provisioned nodes.

1. **Workload Policy definition:** The blueprint defines a custom GCE resource policy using the `workload_policy` module:

    ```yaml
    - id: workload_policy
      source: modules/compute/resource-policy
      settings:
        name: "h4d-workload-policy"
        workload_policy:
          type: "HIGH_THROUGHPUT"
          # Optional: physical boundary constraint for compaction.
          # Supported values: "SUBBLOCK", "BLOCK", or "CLUSTER" (default)
          max_topology_distance: "CLUSTER"
    ```

2. **Mapping to Node Pool:** The policy is then mapped directly to the GKE node pool MIG. This binds the physical `HIGH_THROUGHPUT` constraint to the pool along with the physical boundary limit (`max_topology_distance`) so that when the GKE Autoscaler requests nodes, they are guaranteed to sit in a physically collocated cluster matching the selected topology constraint.

## Create a cluster
These steps guide you through the cluster creation process.

Note: If you create multiple clusters using these same cluster blueprints, ensure that all VPCs and subnet names are unique per project to prevent errors.

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

    Replace the following variables:\
    BUCKET_NAME: the name of the new Cloud Storage bucket.\
    PROJECT_ID: ID of the project where the bucket is being created.\
    COMPUTE_REGION: the compute region where you want to store the state of the Terraform deployment.

5. In the `examples/gke-consumption-options/dws-flex-start-compact-placement/gke-h4d/gke-h4d-deployment.yaml` file, fill in the following settings in the `terraform_backend_defaults` and `vars` sections to match the specific values for your deployment:

    `bucket`: the name of the Cloud Storage bucket you created in the previous step.\
    `deployment_name`: the name of the deployment.\
    `project_id`: your Google Cloud project ID.\
    `region`: the compute region for the cluster.\
    `zone`: the compute zone for the node pool of H4D machines.\
    `authorized_cidr`: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine to call Terraform.\
    **`enable_flex_start`**: enable DWS Flex Start.\
    To modify advanced settings, edit `examples/gke-consumption-options/dws-flex-start-compact-placement/gke-h4d/gke-h4d.yaml`.

6. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

    ```sh
    gcloud auth application-default login
    ```

7. Deploy the blueprint to provision the GKE infrastructure:

    ```sh
    cd ~/cluster-toolkit
    ./gcluster deploy -d \
    examples/gke-consumption-options/dws-flex-start-compact-placement/gke-h4d/gke-h4d-deployment.yaml \
    examples/gke-consumption-options/dws-flex-start-compact-placement/gke-h4d/gke-h4d.yaml
    ```

8. When prompted, select (A)pply to deploy the blueprint.
   * The blueprint creates VPC networks, an RDMA VPC network, service accounts, a cluster, and a nodepool mapping the custom workload policy.

## Note
* DWS Flex Start does not work with static nodes. So, `static_node_count` cannot be set.
* To use DWS Flex Start, `auto_repair` should be set to `false`.
* Compact placement using custom GCE workload policies (like `HIGH_THROUGHPUT` topology constraints) with DWS Flex Start is currently supported for **A4**, **A3 Ultra**, and **H4D** machine types.
* TPUs are natively scheduled as compact physical slices via the `tpu_topology` parameter. So, for TPUs (incl. Flex-Start) users must specify `tpu_topology`. This is already supported for Flex-start because it’s mandatory for TPU Slice scheduling.

## Run a job

The example folder provides a sample job test-job.yaml that runs a sleep container for `10s`.

Any job applied to this node pool must meet the following requirements:
* **Tolerations**: Because the `h4d-pool` node pool is tainted (`node-type=h4d:NoSchedule`) to prevent generic scheduling, any job you apply **must** include the matching toleration under `spec.template.spec.tolerations`

  ```yaml
  tolerations:
  - key: "node-type"
    operator: "Equal"
    value: "h4d"
    effect: "NoSchedule"
  ```

1. Connect to the GKE cluster using gcloud command.

    ```sh
    gcloud container clusters get-credentials <cluster-name> --location <location> --project <project-id>
    ```

    Replace `<cluster-name>` with the name of your cluster, `<location>` with the name of the compute region, and `<project-id>` with the ID of the project.

2. Submit the sample test job:

    ```sh
    kubectl apply -f examples/gke-consumption-options/dws-flex-start-compact-placement/dws-flex-start-h4d.yaml
    ```

3. Monitor the scale-up and execution timeline:
    ***Check Pod Status**: Immediately after submitting, the pods will be `Pending` because the H4D node pool is at size `0`:

      ```sh
      kubectl get pods -w
      ```

      Output:

      ```text
      NAME              READY   STATUS    RESTARTS   AGE
      h4d-job-1-q2ksv   0/1     Pending   0          10s
      h4d-job-2-j9wla   0/1     Pending   0          10s
      ```

    ***Inspect Autoscaler Events**: Describe one of the pending pods or list events to verify GKE is triggering a scale-up for the H4D partition:

      ```sh
      kubectl describe pod h4d-job-1-q2ksv
      ```

      Look for the `TriggeredScaleUp` event:

      ```text
      Events:
        Type    Reason            Age   From                Message
        ----    ------            ---   ----                -------
        Normal  TriggeredScaleUp  15s   cluster-autoscaler  pod triggered scale-up by cluster-autoscaler: group h4d-pool-xxxx
      ```

    ***Track Node Readiness**: After sometime, the physical VM will boot and register. The pods will transition to `Running` and then `Completed`.

      ```sh
      kubectl get nodes -w
      ```

      Output:

      ```text
      NAME                      STATUS   ROLES    AGE   VERSION
      gke-h4d-pool-8hwg         Ready    <none>   10s   v1.32.1-gke.1001000
      ```

    ***Observe Completion**: Once the sleep workload finishes, the pods will move to `Completed`.

      ```sh
      kubectl get pods
      ```

      Output:

      ```text
      NAME              READY   STATUS      RESTARTS   AGE
      h4d-job-1-q2ksv   0/1     Completed   0          2m
      h4d-job-2-j9wla   0/1     Completed   0          2m
      ```

*NOTE:* Since the node pool is configured with default `max_run_duration: 900` (15 minutes), the nodes will be automatically deleted by GKE after running for 15 minutes, or when the jobs complete and GKE Autoscaler scales down the idle nodes.

## Verification via Google Cloud Console

To verify the workload policy configuration using the Google Cloud Console UI:

1. In the Google Cloud Console, navigate to the **Kubernetes Engine > Clusters** page.
2. Click on the name of your GKE cluster.
3. Select the **Nodes** tab.
4. Scroll down to the node pool details and click on the link to the Managed Instance Group (MIG) under the **Instance groups** column.
5. On the Managed Instance Group page, select the **Details** tab.
6. Scroll down to locate the **Workload policy** section.
7. Verify that the attached policy configuration shows the correct type and topology distance.
