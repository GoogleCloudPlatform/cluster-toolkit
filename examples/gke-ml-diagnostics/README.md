# GKE ML Diagnostics with TPU v6e Blueprint

This blueprint automates the provisioning of a Google Kubernetes Engine (GKE) cluster optimized for AI/ML workloads with Google Cloud ML Diagnostics (Diagon++) pre-installed. ML Diagnostics is the recommended managed service for profiling, logging, and monitoring machine learning workloads on Google Cloud.

By leveraging the Cluster Toolkit, this blueprint eliminates the manual toil of setting up VPCs, IAM permissions, and Diagon-specific Kubernetes components, ensuring a reliable and "diagnostics-ready" environment out-of-the-box.

The automation includes:

* **Infrastructure:** Dual VPC networks, subnets, and a GKE Cluster with TPU v6e node pools.
* **IAM Security:** Creates dedicated Google Service Accounts (GSAs) for node pools and workloads, assigning necessary permissions for Cluster Director, Cloud Storage, Logging, and Artifact Registry via Workload Identity.
* **Kubernetes Orchestration:** Installs JobSet and Kueue for enhanced workload management.
* **Managed Diagnostics Suite:** Deploys Cert-Manager, the ML Diagnostics injection-webhook (for workload metadata), and connection-operator (for on-demand profiling).

## Before you begin

1. **Enable APIs:** Enable the Google Kubernetes Engine, Compute Engine, Artifact Registry, Cloud Resource Manager, and the **Cluster Director API**.
2. **gcloud CLI:** Install and initialize the [gcloud CLI](https://cloud.google.com/sdk/docs/install). Ensure components are up to date: `gcloud components update`.
3. **User IAM Roles:** Ensure the account you're using to run `gcluster` has sufficient permissions. Roles like `roles/editor` are simplest, or a combination of:
   * `roles/container.clusterAdmin`
   * `roles/compute.admin`
   * `roles/iam.serviceAccountAdmin`
   * `roles/iam.projectIamAdmin`
   * `roles/storage.admin`
   * `roles/resourcemanager.projectIamAdmin`
4. **Quota:** Verify you have sufficient TPU v6e quota in your target region.
5. **Profile Storage Bucket:** Have a Google Cloud Storage (GCS) bucket ready. This path will be provided to the ML Diagnostics SDK *within your workload code*.

## Create a cluster using Cluster Toolkit

1. **Prerequisites:**

    Ensure you have the `gcluster` binary installed. Please refer to the [main README](../../README.md#using-the-pre-built-bundle-recommended) for installation instructions.

2. **Create a GCS bucket for Terraform state:**

    ```bash
    gcloud storage buckets create gs://YOUR_STATE_BUCKET_NAME \
        --default-storage-class=STANDARD \
        --location=COMPUTE_REGION \
        --uniform-bucket-level-access
    gcloud storage buckets update gs://YOUR_STATE_BUCKET_NAME --versioning
    ```

    Replace `YOUR_STATE_BUCKET_NAME` and `COMPUTE_REGION`.

3. **Save the Blueprint:** Save the blueprint content to `~/cluster-toolkit/examples/gke-ml-diagnostics/gke-ml-diagnostics-tpu-v6e.yaml`.

4. **Create Deployment Configuration:** Create `~/cluster-toolkit/examples/gke-ml-diagnostics/gke-ml-diagnostics-tpu-v6e-deployment.yaml`:

    ```yaml
    terraform_backend_defaults:
      type: gcs
      configuration:
        bucket: "YOUR_STATE_BUCKET_NAME" # Replace

    vars:
      project_id: YOUR_PROJECT_ID # Replace
      deployment_name: gkemldiagon
      region: us-central1
      zone: us-central1-b
      # namespace: diagon # Default namespace for ML Diagnostics
    ```

5. **Deploy the blueprint:**

    ```bash
    cd ~/cluster-toolkit
    ./gcluster deploy -d \
      examples/gke-ml-diagnostics/gke-ml-diagnostics-tpu-v6e-deployment.yaml \
      examples/gke-ml-diagnostics/gke-ml-diagnostics-tpu-v6e.yaml
    ```

## Post-Deployment: Verify Diagnostics

1. **Connect to your cluster:**

    ```bash
    gcloud container clusters get-credentials <cluster-name> \
        --region <region> --project <project-id>
    ```

2. **Verify Kubernetes resources:**

    ```bash
    kubectl get all -n cert-manager
    ```

    Confirm that `cert-manager`, `mldiagnostics-webhook`, and `mldiagnostics-connection-operator` pods are running.

3. **SDK Integration:** Integrate the [ML Diagnostics SDK](https://github.com/AI-Hypercomputer/google-cloud-mldiagnostics) into your workload. For MaxText workloads, enable using the flag `managed_mldiagnostics=True`.

## Running a Sample Job

1. **Review and Update Sample Job:** Inspect `examples/gke-ml-diagnostics/ml-sample-job.yaml`. Ensure the `namespace`, `nodeSelector`, and image path are appropriate for your setup.

2. **Apply the YAML:**

    ```bash
    kubectl apply -f examples/ml-sample-job.yaml
    ```

3. **Monitor the Job:** Check pod status:

    ```bash
    kubectl get pods -n diagon -w
    ```

## Clean up

To avoid recurring charges, destroy the resources:

```bash
cd ~/cluster-toolkit
./gcluster destroy gkemldiagon
```
