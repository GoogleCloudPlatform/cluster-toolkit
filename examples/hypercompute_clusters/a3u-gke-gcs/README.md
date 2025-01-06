# A3-Ultra GKE + GCS Reference Design

This reference design provides a high-performance and scalable architecture for
deploying AI/ML workloads on Google Kubernetes Engine (GKE) with Google Cloud
Storage (GCS).

## Key Features

* **Multi-VPC Design:** Utilizes three VPCs: two for GKE nodes and one dedicated
  for GPU RDMA networks.
* **Cloud Storage Fuse Integration:** Enables seamless access to GCS buckets
  from within your containers using the Cloud Storage Fuse CSI Driver. Cloud
  Storage Fuse is configured to utilize the 12 TB of Local SSD
* **Hierarchical Namespace Buckets:** Leverages GCS buckets with Hierarchical
  Namespace enabled, optimizing performance for checkpointing and restarting
  workloads. (Requires GKE 1.31 or later).
* **Kueue for Workload Scheduling:** Provides a robust and flexible system for
  managing your AI/ML training jobs.
* **Jobset API for Tightly Coupled Workloads:** Facilitates running tightly
  coupled AI/ML training jobs efficiently.

## Deployment Steps

1. **Build the Cluster Toolkit `gcluster` binary:**

   Follow the instructions [here](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).

2. **(Optional) Create a GCS Bucket for Terraform State:**

   This step is recommended for storing your Terraform state. Use the
   following commands, replacing placeholders with your project details:

   ```bash
   BUCKET_NAME=<your-bucket-name>
   PROJECT_ID=<your-gcp-project>
   REGION=<your-preferred-region>

   gcloud storage buckets create gs://${BUCKET_NAME} \
       --project=${PROJECT_ID} \
       --default-storage-class=STANDARD \
       --location=${REGION} \
       --uniform-bucket-level-access

   gcloud storage buckets update gs://${BUCKET_NAME} --versioning
   ```

3. **Create and Configure GCS Buckets:**

   * Create separate GCS buckets for training data and checkpoint/restart data:

     ```bash
     PROJECT_ID=<your-gcp-project>
     REGION=<your-preferred-region>
     TRAINING_BUCKET_NAME=<training-bucket-name>
     CHECKPOINT_BUCKET_NAME=<checkpoint-bucket-name>
     PROJECT_NUMBER=<your-project-number>

     gcloud storage buckets create gs://${TRAINING_BUCKET_NAME} \
         --location=${REGION} \
         --uniform-bucket-level-access \
         --enable-hierarchical-namespace

     gcloud storage buckets create gs://${CHECKPOINT_BUCKET_NAME} \
         --location=${REGION} \
         --uniform-bucket-level-access \
         --enable-hierarchical-namespace
     ```

   * Grant workload identity service accounts (WI SAs) access to the buckets:

     ```bash

     gcloud storage buckets add-iam-policy-binding gs://${TRAINING_BUCKET_NAME} \
         --member "principal://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${PROJECT_ID}.svc.id.goog/subject/ns/default/sa/default" \
         --role roles/storage.objectUser

     gcloud storage buckets add-iam-policy-binding gs://${CHECKPOINT_BUCKET_NAME} \
         --member "principal://iam.googleapis.com/projects/$PROJECT_NUMBER}/locations/global/workloadIdentityPools/${PROJECT_ID}.svc.id.goog/subject/ns/default/sa/default" \
         --role roles/storage.objectUser
     ```

4. **Customize Deployment Configuration:**

   Modify the `deployment.yaml` file to suit your needs. This will include
   region/zone, nodepool sizes, reservation name, and checkpoint/training bucket
   names.

5. **Deploy the Cluster:**

   Use the `gcluster` tool to deploy your GKE cluster with the desired configuration:

   ```bash
   gcluster deploy -d deployment.yaml a3u-gke-gcs.yaml
   ```

## Example Workload Job

Once the cluster has been deployed, there will be instructions on how to get
credentials for the cluster, as well as how to deploy an example workload. This
example workload uses [fio](https://github.com/axboe/fio) to run a series of
benchmarks against the LocalSSD and GCSFuse mounts.

The instructions will look something like:

```bash
Use the following commands to:
Submit your job:
  kubectl create -f <PATH/TO/DEPLOYMENT_DIR>/primary/my-job-<some-id>.yaml
```
