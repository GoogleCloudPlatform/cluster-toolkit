# Slurm H4d Blueprints

## Slurm clusters
For further information on deploying an H4d cluster with Slurm, please
see:

[Create an RDMA-enabled HPC Slurm cluster with H4D instances](https://cloud.google.com/compute/docs/hpc/create-a-slurm-cluster-h4d)

## Configuration

Before deploying, you need to edit the `hpc-slurm-h4d/hpc-slurm-h4d-deployment.yaml` file to match your Google Cloud environment and requirements.

Key variables to update in `vars`:

* `bucket`: Inside `terraform_backend_defaults.configuration`, replace `BUCKET_NAME` with the name of your GCS bucket for Terraform state.
* `project_id`: Your Google Cloud Project ID.
* `deployment_name`: A unique name for this deployment (e.g., `slurm-h4d-test`).
* `region`: The Google Cloud region for deployment (e.g., `us-central1`).
* `zone`: The Google Cloud zone for deployment (e.g., `us-central1-a`).
* `h4d_cluster_size`:The number of static nodes in the cluster.
* `h4d_reservation_name`: Reservation name if using reserved instances  

### Additional ways to provision
Cluster toolkit also supports DWS Flex-Start, Spot VMs, as well as reservations as ways to provision instances.

[For more information on DWS Flex-Start in Slurm](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/docs/slurm-dws-flex.md)
[For more information on Spot VMs](https://cloud.google.com/compute/docs/instances/spot)

We provide ways to enable the alternative provisioning models in the
`hpc-slurm-h4d-deployment.yaml` file.

To make use of these other models, replace `h4d_reservation_name:` in the
deployment file with the variable of choice below.

`h4d_enable_spot_vm: true` for spot or `h4d_dws_flex_enabled: true` for DWS Flex-Start.

## Deployment Steps

1. Navigate to the root directory of your Cluster Toolkit checkout or the directory containing the `hpc-slurm-h4d` folder.
2. Run the `./gcluster deploy` command, providing the `hpc-slurm-h4d-deployment.yaml` as the deployment file. The `gcluster` tool will use this file to resolve variables in the `hpc-slurm-h4d.yaml` located in the same directory.

    ```bash
    ./gcluster deploy -d examples/hpc-slurm-h4d/hpc-slurm-h4d-deployment.yaml examples/hpc-slurm-h4d/hpc-slurm-h4d.yaml -w --auto-approve
    ```

   * `-w`: Use this flag to force an overwrite of any existing deployment artifacts and configurations from a previous run *with the same deployment name*. (e.g., `slurm-h4d`).
   * `--auto-approve`: Automatically approves the Terraform apply step. Omit this if you want to review the plan first.

    The tool will create a deployment folder (e.g., `slurm-h4d`), generate Terraform files, and then apply them to create the resources in your GCP project.

## Clean Up

```bash
#!/bin/bash
./gcluster destroy [DEPLOYMENT_FOLDER] --auto-approve
```

Replace [DEPLOYMENT_FOLDER] with the appropriate value.
