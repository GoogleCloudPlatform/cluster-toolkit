# Setup AF3 with Simple Ipynb Launcher - PART 1: specific cluster settings
This guide explains the additional steps needed to deploy the AF3 solution with the Slurm REST-API required by the [Simple Ipynb launcher](./README.md).

> [!NOTE]
> **Important:** The steps described here are intended to be followed **before** deploying the SLURM cluster (as referenced [here](../../README.md#deploy-slurm-cluster)).  
> If you have **already deployed** the cluster, you should tear down the cluster and redeploy after the steps outlined here. Once your cluster is deployed, you will have to follow the steps outlined in [Setup AF3 with Simple Ipynb Launcher - PART 2: IPython notebook setup](./Setup-post-cluster-deployment.md)

## Prerequisites

### Set up Bucket for IPython Notebook
If you want to use the Simple ipynb launcher, you need to create an additional bucket for the IPython Notebook that is provided by Google Vertex AI Workbench. It should be located in the region where you stand up your cluster:

```bash
#!/bin/bash

UNIQUE_AF3IPYNB_BUCKET=<your-bucket>
PROJECT_ID=<your-gcp-project>
REGION=<your-preferred-region>

gcloud storage buckets create gs://${UNIQUE_JOB_BUCKET} \
    --project=${PROJECT_ID} \
    --default-storage-class=STANDARD --location=${REGION} \
    --uniform-bucket-level-access
```

### Configure Secret Manager
The IPython notebook will need a secure connection with the Slurm REST API servers. We are using the Google Cloud Secret Manager to manage the necessary credentials in a secure way. Please follow the guideline [here](https://cloud.google.com/secret-manager/docs/create-secret-quickstart) to create a Secret Manager.

You do not need to manually add any data to the secret; the SLURM token will be automatically populated by the system.

## Modify the af3-slurm-deployment.yaml
Set the `slurm_rest_server_activate` value to `true` in the `af3-slurm-deployment.yaml` file to enable the SLURM REST API server on the controller node of the `af3-slurm.yaml` cluster. This is required for the Jupyter Notebook environment to send requests —such as job submissions— to the SLURM scheduler via the REST API.

```yaml
slurm_rest_server_activate: true
```

Set the `slurm_rest_token_secret_name` value in the `af3-slurm-deployment.yaml` with the name of the secret you created as part of the [Prerequisites](#prerequisites).

```yaml
slurm_rest_token_secret_name: "<your-secret-name>"
```

Set the `af3ipynb_bucket` value in the `af3-slurm-deployment.yaml` with the name of the secret you created as part of the [Prerequisites](#prerequisites).

```yaml
af3ipynb_bucket: "<your-pre-existing-bucket-name>"
```

## Continue to cluster deployment
Go back to [Deploy Slurm Cluster](../../README.md#deploy-slurm-cluster) and deploy the cluster.
