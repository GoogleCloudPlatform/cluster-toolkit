# Collaborative Deployment Verification Instructions

These commands will help you verify the new "Collaborative Deployment" feature.

## Prerequisites

1.  **GCS Bucket**: You need a GCS bucket where artifacts will be stored.
    *   Example: `gs://my-cluster-state-bucket`
    *   *If you don't have one, create it:* `gcloud storage buckets create gs://my-cluster-state-bucket`

2.  **Authentication**: Ensure you are authenticated with Google Cloud.
    *   `gcloud auth application-default login`

## Step 1: Create a Blueprint with GCS Backend

Create a file named `collab-test.yaml` with the following content.
**Replace `YOUR_PROJECT_ID`** with your actual Project ID.
**Replace `YOUR_BUCKET_NAME`** with your GCS bucket name.

```yaml
blueprint_name: collab-test
vars:
  project_id: YOUR_PROJECT_ID
  deployment_name: collab-test-v1
  region: us-central1
deployment_groups:
- group: primary
  modules:
  - source: modules/network/pre-existing-vpc
    kind: terraform
    id: network
    settings:
      network_name: default
      subnetwork_name: default
      project_id: $(vars.project_id)
      region: $(vars.region)
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: YOUR_BUCKET_NAME
```

## Step 2: Create the Deployment (User A)

Run `gcluster create` to generate the deployment and trigger the automatic upload.

```bash
./gcluster create collab-test.yaml --vars="project_id=YOUR_PROJECT_ID"
```

*Check the output logs:* You should see a message confirming the upload:
`INFO: Successfully uploaded expanded_blueprint.yaml to gs://YOUR_BUCKET_NAME/collab-test-v1/artifacts/expanded_blueprint.yaml`

## Step 3: Deploy the Infrastructure (User A)

Run `gcluster deploy` to actually create the resources and initialize the Terraform state in the GCS backend.

```bash
./gcluster deploy collab-test-v1
```

*This step ensures the state file is created in your GCS bucket.*

## Step 4: Simulate "User B" (Clean Agent)

To verify the `pull` command, remove the local deployment directory to simulate a fresh workstation.

```bash
rm -rf collab-test-v1
```

## Step 5: Pull the Deployment (User B)

Run `gcluster pull` using the GCS URI of the deployment root (bucket + deployment name).

```bash
./gcluster pull gs://YOUR_BUCKET_NAME/collab-test-v1
```

*Expected Output:*
*   `INFO: Downloading blueprint from ...`
*   `INFO: Successfully pulled deployment to collab-test-v1`

## Step 6: Deploy (User B) - Reusing Existing State

Run `gcluster deploy` again. This step is necessary to:
1.  Initialize Terraform locally (`terraform init`).
2.  Download the state from your GCS bucket.
3.  Generate any local outputs needed to connect to the cluster.

Since the resources were already created by User A, this command will **not create new resources**. It will simply sync your local environment with the existing cluster.

```bash
./gcluster deploy collab-test-v1
```

*Terraform should detect that the resources already exist and match the configuration ("No changes").*

**Note:** This step will be **much faster** (a few seconds/minutes) compared to the initial creation (Step 3). It only validates the state and does not re-provision the resources.
