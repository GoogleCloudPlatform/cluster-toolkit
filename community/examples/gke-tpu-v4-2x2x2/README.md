# GKE TPU V4 2x2x2 blueprint

This example shows how a TPU cluster with v4 machines and topology 2x2x2 can be created. The example also includes a `tpu-available-chips.yaml` that creates a kubernetes service and job. The job includes commands to install `jax` and run a simple command using jax, on the TPU.

Key parameters when working with TPUs:

* `num_slices`: Number of TPU slices required. A slice is a collection of chips all located inside the same TPU Pod connected by high-speed inter chip interconnects (ICI). Slices are described in terms of chips or TensorCores, depending on the TPU version.
* `tpu_topology`: The TPU topology desired. Topology is the number and physical arrangement of the TPU chips in a TPU slice.

## Before you begin

Before you start, make sure you have performed the following tasks:

* Enable the Google Kubernetes Engine API.

* If you want to use the Google Cloud CLI for this task, [install](https://cloud.google.com/sdk/docs/install) and then [initialize](https://cloud.google.com/sdk/docs/initializing) the gcloud CLI. If you previously installed the gcloud CLI, get the latest version by running gcloud components update.
  > **NOTE:** For existing gcloud CLI installations, make sure to set the compute/region and compute/zone properties. By setting default locations, you can avoid errors in gcloud CLI like the following: One of [--zone, --region] must be supplied: Please specify location.

* Ensure that you have enough quota for TPUs.

* Ensure that you have the following roles enabled:
  * `roles/editor`
  * `roles/container.clusterAdmin`
  * `roles/iam.serviceAccountAdmin`

## Create a cluster using Cluster Toolkit

This section guides you through the cluster creation process, ensuring that your project follows best practices.

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

1. In the [`community/examples/gke-t4-2x2x2/gke-t4-2x2x2-deployment.yaml`](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/develop/community/examples/gke-t4-2x2x2/gke-t4-2x2x2-deployment.yaml) file, replace the following variables in the `terraform_backend_defaults` and `vars` sections to match the specific values for your deployment:

   * `bucket`: the name of the Cloud Storage bucket you created in the previous step.
   * `project_id`: your Google Cloud project ID.
   * `region`: the compute region for the cluster.
   * `zone`: the compute zone for the TPUs.
   * `authorized_cidr`: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine to call Terraform.
   * `extended_reservation`: the name of your reservation in the form of <project>/<reservation-name>
   * `static_node_count`: the number of TPU nodes in your cluster.

  To modify advanced settings, edit
  `community/examples/gke-t4-2x2x2/gke-t4-2x2x2.yaml`.

1. Generate [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/provide-credentials-adc#google-idp) to provide access to Terraform.

1. Deploy the blueprint to provision the GKE  infrastructure
    using TPU v4 machine types:

   ```sh
    cd ~/cluster-toolkit
    ./gcluster deploy -d \
    community/examples/gke-t4-2x2x2/gke-t4-2x2x2-deployment.yaml \
    community/examples/gke-t4-2x2x2/gke-t4-2x2x2.yaml
   ```

## Run the sample job

The `tpu-available-chips.yaml`(https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/develop/community/examples/gke-t4-2x2x2/tpu-available-chips.yaml) file creates a service and a job resource in kubernetes. It is based on https://cloud.google.com/kubernetes-engine/docs/how-to/tpus#tpu-chips-node-pool. The  workload returns the number of TPU chips across all of the nodes in a multi-host TPU slice.

1. Connect to your cluster:

    ```sh
    gcloud container clusters get-credentials gke-t4-2x2x2
    ```

1. Create the resources:

    ```sh
    kubectl create -f ~/cluster-toolkit/community/examples/gke-t4-2x2x2/tpu-available-chips.yaml
    ```

    This command returns a service and a job name.

    The output should be:

    ```sh
    service/headless-svc created
    job.batch/tpu-available-chips created
    ```

1. Obtain list of pods using:

    ```sh
    kubectl get pods
    ```

    Identify two pods with prefix `tpu-available-chips`.

1. Display logs of either of the pods using:

    ```sh
    kubectl logs <pod-name>
    ```

    This should display `TPU cores: 8` at the end of the logs which is the number of TPU chips across all of the nodes in a multi-host TPU slice.

## Clean up

To avoid recurring charges for the resources used on this page, clean up the resources provisioned by Cluster Toolkit, including the VPC networks and GKE cluster:

   ```sh
   ./gcluster destroy gke-tpu-v4-2x2x2/
   ```

Useful TPU links:
1. [TPU architecture](https://cloud.google.com/tpu/docs/system-architecture-tpu-vm)
2. [TPU v4](https://cloud.google.com/tpu/docs/v4)
