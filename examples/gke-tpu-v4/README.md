# GKE TPU v4 Blueprint

This example shows how a TPU cluster with v4 machines can be created. The example also includes a consolidated `tpu-kueue-jax-sample.yaml` that creates a Kueue-managed Kubernetes JobSet to run JAX and print the active TPU device count.

Key parameters when working with TPUs:

* `num_slices`: Number of TPU slices required. A slice is a collection of chips all located inside the same TPU Pod connected by high-speed inter-chip interconnects (ICI).
* `tpu_topology`: The TPU topology desired. Topology is the number and physical arrangement of the TPU chips in a TPU slice.

## Before you begin

Before you start, make sure you have performed the following tasks:

* Enable the Google Kubernetes Engine API.
* If you want to use the Google Cloud CLI for this task, [install](https://cloud.google.com/sdk/docs/install) and then [initialize](https://cloud.google.com/sdk/docs/initializing) the gcloud CLI.
* Ensure that you have enough quota for TPUs (`tpu-v4-podslice` quota in your region/zone).
* Ensure that you have the following roles enabled on your deployment identity:
  * `roles/editor`
  * `roles/container.clusterAdmin`
  * `roles/iam.serviceAccountAdmin`

## Create a cluster using Cluster Toolkit

This section guides you through the cluster creation process, ensuring that your project follows best practices.

> **NOTE:** If you would like to create more than one cluster in a project, make sure you update the deployment name.

1. [Launch Cloud Shell](https://cloud.google.com/shell/docs/launching-cloud-shell). You can use a different environment; however, we recommend Cloud Shell because the dependencies are already pre-installed for Cluster Toolkit. If you don't want to use Cloud Shell, follow the instructions to [install dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) to prepare a different environment.

1. Install the Cluster Toolkit: Download and extract the pre-built `gcluster` binary bundle. Follow the download commands for your operating system in the [Install the Cluster Toolkit](https://docs.cloud.google.com/cluster-toolkit/docs/setup/configure-environment#install) section of the setup guide.

1. Once extracted, verify that the installation is ready:

      ```sh
      ./gcluster --version
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
   * `COMPUTE_REGION`: the compute region where you want to store the state of the Terraform deployment (e.g. `us-central2` for TPU v4).

1. In the `examples/gke-tpu-v4/gke-tpu-v4-deployment.yaml` file, replace the following variables in the `terraform_backend_defaults` and `vars` sections to match the specific values for your deployment:

   * `bucket`: the name of the Cloud Storage bucket you created in the previous step.
   * `project_id`: your Google Cloud project ID.
   * `region`: the compute region for the cluster (e.g., `us-central2`).
   * `zone`: the compute zone for the TPUs (e.g., `us-central2-b`).
   * `num_slices`: the number of TPU slices to create.
   * `machine_type`: the machine type of the TPU (e.g., `ct4p-hightpu-4t`).
   * `tpu_topology`: the TPU placement topology for the pod slice node pool (e.g., `2x2x2`).
   * `authorized_cidr`: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine running Terraform.
   * `user_namespace`: The Kubernetes service account namespace where your TPU workloads will run (defaults to `default`).

    > **Note:** The `static_node_count` is automatically calculated from `machine_type`, `num_slices` and `tpu_topology`. It is derived using the formula: `(total_chips_in_topology / chips_per_machine)`. For `2x2x2` (8 chips) on `ct4p-hightpu-4t` (4 chips/node), it will create 2 nodes.

   To modify advanced settings, edit `examples/gke-tpu-v4/gke-tpu-v4.yaml`.

1. To utilize spot instances, add `spot: true` in `gke-tpu-v4-deployment.yaml`.

1. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

1. Deploy the blueprint to provision the GKE infrastructure:

    ```sh
    ./gcluster deploy examples/gke-tpu-v4/gke-tpu-v4.yaml \
      -d examples/gke-tpu-v4/gke-tpu-v4-deployment.yaml
    ```

## Kueue Scheduling & Running a Sample JAX Job

This blueprint installs and configures [Kueue](https://kueue.sigs.k8s.io/) by default to manage TPU quotas and queue job submissions. The provided `tpu-kueue-jax-sample.yaml` file creates a Kubernetes JobSet that integrates both Kueue queue routing and JAX TPU device count validation.

1. **Connect to your cluster:**

    ```sh
    gcloud container clusters get-credentials gke-tpu-v4 --region=REGION --project=PROJECT_ID
    ```

    Replace `REGION` and `PROJECT_ID` with your deployment region and project ID.

2. **Update the Node Selector (if needed):** Open `examples/gke-tpu-v4/tpu-kueue-jax-sample.yaml` and verify that the `nodeSelector` values match the TPU accelerator and topology configured in your deployment:

    ```yaml
    nodeSelector:
        cloud.google.com/gke-tpu-accelerator: tpu-v4-podslice
        cloud.google.com/gke-tpu-topology: 2x2x2
    ```

3. **Submit the Job:** Submit the workload to GKE. The job contains the label `kueue.x-k8s.io/queue-name: user-queue` which automatically routes it through Kueue:

    ```sh
    kubectl apply -f examples/gke-tpu-v4/tpu-kueue-jax-sample.yaml
    ```

4. **Verify Workload Admission & Status:** Check if Kueue successfully admitted and queued the workload:

    ```sh
    kubectl get workloads
    ```

    Monitor the job and pod execution:

    ```sh
    kubectl get jobset
    kubectl get pods -l jobset.sigs.k8s.io/jobset-name=tpu-v4-kueue-jax-sample
    ```

5. **Verify JAX Logs:** Print the logs of the pods to confirm JAX successfully detected the TPU devices:

    ```sh
    kubectl logs -l jobset.sigs.k8s.io/jobset-name=tpu-v4-kueue-jax-sample
    ```

    A successful execution will output logs from both pods showing:

    ```sh
    Global device count: 8
    ```

## Running Pathways Workloads

This blueprint supports [**Pathways-on-Cloud**](https://github.com/AI-Hypercomputer/pathways-utils/) orchestration, allowing you to run JAX workloads distributed across remote TPU workers coordinated by a CPU-based head node.

*NOTE*: Refer to the [GCluster Job Submission Guide](../../docs/gcluster_job_guide.md) for detailed instructions on job submission.

### 1. Enable Pathways in the Blueprint

Before deploying, ensure Pathways is enabled in `examples/gke-tpu-v4/gke-tpu-v4.yaml`:

```yaml
vars:
  # Enable Pathways for TPUs (provisions CPU node pool and configures Kueue quotas)
  enable_pathways_for_tpus: true
```

### 2. Submit the Live Job

Submit the job to your live GKE cluster:

```sh
./gcluster job submit \
  --name pathways-job-v4 \
  --compute-type v4-8 \
  --pathways \
  --pathways-gcs-location gs://YOUR_COORDINATION_BUCKET/pathways-scratch \
  --image us-docker.pkg.dev/cloud-tpu-images/jax-ai-image/tpu:latest \
  --command "pip install pathwaysutils && python -c 'import pathwaysutils; pathwaysutils.initialize(); import jax; print(\"JAX Device count:\", jax.device_count())'"
```

### 3. Monitor and Manage the Job

1. **Monitor and Check Logs:** Use the `gcluster` CLI to track the execution status and view the workload logs:

   ```sh
   gcluster job logs pathways-job-v4
   ```

    A successful execution will output logs ending with:

   ```sh
   JAX Device count: 8
   ```

2. **Cancel and Clean Up Job:** To terminate a running job or clean up resources once finished, run:

   ```sh
   gcluster job cancel pathways-job-v4
   ```

## Tear down the cluster

To avoid recurring charges for the resources used on this page, clean up the resources provisioned by Cluster Toolkit:

   ```sh
   ./gcluster destroy DEPLOYMENT_NAME
   ```
