# GKE TPU v5e Blueprint

This example shows how a TPU cluster with v5e machines can be created. The example also includes a `tpu-singleslice.yaml` that creates a Kubernetes job. The job includes commands to run JAX and print the TPU device count.

Key parameters when working with TPUs:

* `num_slices`: Number of TPU slices required. A slice is a collection of chips all located inside the same TPU Pod connected by high-speed inter-chip interconnects (ICI).
* `tpu_topology`: The TPU topology desired. Topology is the number and physical arrangement of the TPU chips in a TPU slice.

## Before you begin

Before you start, make sure you have performed the following tasks:

* Enable the Google Kubernetes Engine API.
* If you want to use the Google Cloud CLI for this task, [install](https://cloud.google.com/sdk/docs/install) and then [initialize](https://cloud.google.com/sdk/docs/initializing) the gcloud CLI.
* Ensure that you have enough quota for TPUs (`tpu-v5-lite-podslice` quota in your region).
* Ensure that you have the following roles enabled on your deployment identity:
  * `roles/editor`
  * `roles/container.clusterAdmin`
  * `roles/iam.serviceAccountAdmin`

## Create a cluster using Cluster Toolkit

This section guides you through the cluster creation process, ensuring that your project follows best practices.

> **NOTE:** If you would like to create more than one cluster in a project, make sure you update the deployment name.

1. [Launch Cloud Shell](https://cloud.google.com/shell/docs/launching-cloud-shell). You can use a different environment; however, we recommend Cloud Shell because the dependencies are already pre-installed for Cluster Toolkit. If you don't want to use Cloud Shell, follow the instructions to [install dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) to prepare a different environment.

1. Clone the Cluster Toolkit from the git repository:

    ```sh
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

1. In the `examples/gke-tpu-v5e/gke-tpu-v5e-deployment.yaml` file, replace the following variables in the `terraform_backend_defaults` and `vars` sections to match the specific values for your deployment:

   * `bucket`: the name of the Cloud Storage bucket you created in the previous step.
   * `project_id`: your Google Cloud project ID.
   * `region`: the compute region for the cluster.
   * `zone`: the compute zone for the TPUs.
   * `num_slices`: the number of TPU slices to create.
   * `machine_type`: the machine type of the TPU (e.g., `ct5lp-hightpu-8t`).
   * `tpu_topology`: the TPU placement topology for the pod slice node pool (e.g., `2x4`).
   * `authorized_cidr`: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine running Terraform.
   * `reservation`: the name of the Compute Engine reservation of TPU v5e nodes (if any).

    > **Note:** The `static_node_count` is automatically calculated from `machine_type`, `num_slices` and `tpu_topology`. It is derived using the formula: `(total_chips_in_topology / chips_per_machine)`.

   To modify advanced settings, edit `examples/gke-tpu-v5e/gke-tpu-v5e.yaml`.

1. To use on-demand capacity, you can remove the reservation usage by making the following changes.
   1. Remove the `reservation` variable from the `gke-tpu-v5e-deployment.yaml`.
   1. Remove the `reservation_affinity` block from the nodepool module.

1. To utilize spot instances, remove the reservation variable from `gke-tpu-v5e-deployment.yaml` and add `spot: true`. In `gke-tpu-v5e.yaml`, replace the reservation_affinity block under `gke-tpu-v5e-pool` module with `spot: $(vars.spot)`.

1. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

1. Deploy the blueprint to provision the GKE infrastructure:

    ```sh
    ./gcluster deploy -d \
      examples/gke-tpu-v5e/gke-tpu-v5e-deployment.yaml \
      examples/gke-tpu-v5e/gke-tpu-v5e.yaml
    ```

## Advanced Scheduling with Kueue

This blueprint installs and configures [Kueue](https://kueue.sigs.k8s.io/) by default to manage quotas and job queuing.

1. **Quota:** The blueprint automatically calculates and sets a `google.com/tpu` quota in the `ClusterQueue`. The node count is automatically derived from your `machine_type` and `tpu_topology`, and the quota is calculated as: `num_slices` × `(total_chips_in_topology / chips_per_machine)` × `chips_per_machine`.

1. **Submit a Job:** To submit a job to the queue, add the label `kueue.x-k8s.io/queue-name: user-queue` to your Job or JobSet manifest.

   A sample job file is provided: `kueue-job-sample.yaml`.

   ```sh
   kubectl create -f examples/gke-tpu-v5e/kueue-job-sample.yaml
   ```

1. **Validation:** Check the status of your workload:

   ```sh
   kubectl get workloads
   ```

## Run the sample job

The `tpu-singleslice.yaml` file creates a Kubernetes Job workload. It is designed to verify TPU connectivity and JAX functionality on the host.

1. Connect to your cluster:

    ```sh
    gcloud container clusters get-credentials gke-tpu-v5e --region=REGION --project=PROJECT_ID
    ```

    Replace `REGION` and `PROJECT_ID` with your deployment region and project ID.

1. Update the nodeSelector under the template spec of `tpu-singleslice.yaml` file. The values depend on the tpu accelerator and tpu topology used in the blueprint.

    ```yaml
    nodeSelector:
        cloud.google.com/gke-tpu-accelerator: tpu-v5-lite-podslice
        cloud.google.com/gke-tpu-topology: 2x2
    ```

1. Run the workload:

    ```sh
    kubectl apply -f examples/gke-tpu-v5e/tpu-singleslice.yaml
    ```

1. Monitor the job status:

    ```sh
    kubectl get jobs
    kubectl get pods
    ```

1. Check the logs of the pod to verify JAX detected the TPU devices correctly:

    ```sh
    kubectl logs -l job-name=tpu-singleslice-job -c jax-tpu
    ```

    You should see output similar to:

    ```sh
    Global device count: 4
    ```

    (Depending on your topology, e.g., 4 chips for `2x2`).

## Tear down the cluster

To avoid recurring charges for the resources used on this page, clean up the resources provisioned by Cluster Toolkit, including the VPC networks and GKE cluster:

   ```sh
   ./gcluster destroy DEPLOYMENT_NAME
   ```
