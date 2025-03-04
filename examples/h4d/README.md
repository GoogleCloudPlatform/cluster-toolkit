# H4D Blueprints

This document outlines the deployment steps for provisioning H4D
`h4d-highmem-192-lssd` VMs in addition to VMs that use Slurm
as an orchestrator.

## Deployment Instructions

### Build the Cluster Toolkit gcluster binary

Follow instructions
[here](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment)

### (Optional, but recommended) Create a GCS Bucket for storing terraform state

```bash
#!/bin/bash
TF_STATE_BUCKET_NAME=<your-bucket>
PROJECT_ID=<your-gcp-project>
REGION=<your-preferred-region>

gcloud storage buckets create gs://${TF_STATE_BUCKET_NAME} \
    --project=${PROJECT_ID} \
    --default-storage-class=STANDARD --location=${REGION} \
    --uniform-bucket-level-access
gcloud storage buckets update gs://${TF_STATE_BUCKET_NAME} --versioning
```

### Deploy the cluster

```bash
#!/bin/bash
gcluster deploy /examples/h4d/h4d-vm.yaml (or hpc-slurm-h4d.yaml)
```

### RDMA CM Kernel patches
[The kernel patch](https://patchwork.kernel.org/project/linux-rdma/patch/20250220175612.2763122-1-jmoroni@google.com/) should be put in `examples/h4d/`. The modified kernel file will be placed onto VMs during startup and the RDMA modules will be reloaded automatically.
