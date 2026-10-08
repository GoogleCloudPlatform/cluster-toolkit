# Objective

This document will guide you to successfully provisioning a Slurm cluster with
a3-highgpu-8g compute nodes running NVIDIA H100 GPUs.

## Before starting

> [!NOTE]
> **TCPXO Availability:** The next-generation TCPXO (FasTrak) networking stack is now enabled by default for `a3-highgpu-8g` (`a3high-slurm-blueprint.yaml`).
> TCPXO offers improved routing and can currently be used with Spot VMs or On-Demand instances (it is **not supported with reservations**).

## Upgrading from the "legacy" solution
There is no direct path for upgrading the a3-highgpu-8g legacy solution.
The recommended path requires temporarily bringing down your cluster and
replacing it with the solution described in this document.

We recommend using `gcluster destroy` to destroy the deployments provisioned by the legacy blueprints:

- ![deprecated-badge] [Legacy v5 image building blueprint](v5-legacy/ml-slurm-a3-1-image-v5-legacy.yaml)
- ![deprecated-badge] [Legacy v5 cluster provisioning blueprint](v5-legacy/ml-slurm-a3-2-cluster-v5-legacy.yaml)
- ![deprecated-badge] [Legacy base provisioning blueprint](/examples/machine-learning/a3-highgpu-8g/ml-slurm-a3-0-base.yaml)
- ![deprecated-badge] [Legacy image provisioning blueprint](/examples/machine-learning/a3-highgpu-8g/ml-slurm-a3-1-image.yaml)
- ![deprecated-badge] [Legacy cluster provisioning blueprint](/examples/machine-learning/a3-highgpu-8g/ml-slurm-a3-2-cluster.yaml)

Then follow the instructions below.

## Required setup

Please follow the initial instructions for:

- Installing Cluster Toolkit [dependencies][tkdeps] (Go, Terraform, Packer)
- Installing the Cluster [Toolkit][tkinstall]

Verify that your release of the Cluster Toolkit is 1.37.0 or later.

```shell
gcluster --version
```

## Top-Level Design of Solution

The blueprint is split into 3 deployment groups:

1. Group 1 provisions the system network, gpu network and 1 Managed Lustre instance for mounting `/home`
across the cluster.
2. Group 2 builds a custom image installing Slurm on an Ubuntu 24.04 accelerator image. The image
includes GPUDirect-TCPXO (`dmabuf-import-helper` via DKMS) and Managed Lustre client support
so that the managed Lustre `/home` directory can mount successfully when the nodes boot.
3. Group 3 provisions Slurm cluster and a3-highgpu-8g nodes using the custom image.

## First time considerations

> [!IMPORTANT]
> These steps do not need to be repeated when a cluster is re-provisioned. They
> are initial setup steps in a project.

### Saving Terraform state
Create a bucket with versioning enabled to store Terraform state:

```shell
export PROJECT_ID=customer-project-id
export BUCKET=customer-bucket
gcloud storage buckets create gs://${BUCKET} --project=${PROJECT_ID} \
    --default-storage-class=STANDARD --location=${REGION} \
    --uniform-bucket-level-access
gcloud storage buckets update gs://${BUCKET} --versioning
```

Modify the blueprints to configure the new bucket to serve as a Terraform
remote backend:

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: customer-bucket # modify to bucket created above
```

### Deployment variables

Set the values in a3high-slurm-deployment.yaml for your deployment

```yaml
  deployment_name: unique-name
  project_id: customer-project
  region: customer-region
  zone: customer-zone
```

#### Reservation created by Google

> [!IMPORTANT]
> If you have ***not*** received a VM reservation from Google Cloud staff, then
> skip this step and proceed to [manual reservation creation](#manual-creation-of-reservation).

Set the deployment variable `a3_reservation_name` to the reservation name provided by Google.

```yaml
  # a3_reservation_name must be specified; if Google staff have provided you
  # with a reservation name, use it. Otherwise supply user-created reservation.
  a3_reservation_name: reservation-name-provided-by-google
```

#### Manual creation of reservation

> [!IMPORTANT]
> If you received a VM reservation from Google Cloud staff, then skip this step.

We recommend creating a reservation to ensure reliable access to re-create VMs
if you need to redeploy or otherwise maintain your cluster.

```shell
gcloud compute reservations create ${A3_RESERVATION_NAME} \
    --project=${PROJECT_ID} \
    --machine-type=a3-highgpu-8g \
    --vm-count=${N_VMS} \
    --zone=${ZONE} \
    --require-specific-reservation \
    --log-http
```

This reservation be must be specified when creating VMs with matching parameters
(e.g. a3-highgpu-8g VM in configured zone). Ensure that the reservation name in the
deployment file matches the name of the user-created reservation.

```yaml
  # a3_reservation_name must be specified; if Google staff have provided you
  # with a reservation name, use it. Otherwise supply user-created reservation.
  a3_reservation_name: 
```

#### Using Spot VM or DWS Flex

> [!IMPORTANT]
> Select one of the provisioning models : either spot vm , dws flex or reservation

In order to make use of DWS Flex Start mode with SlurmGCP, set the `a3_dws_flex_enabled` variable as shown below

```yaml
  vars: 
    a3_dws_flex_enabled: true             # enabling dws flex by setting the variable to true
    # the rest of the variables
```

To learn more about DWS Flex-Start, visit https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/docs/slurm-dws-flex.md

Similarly to make use of Spot VMs with Slurm, set the `a3_enable_spot_vm` variable as shown below

```yaml
  vars: 
    a3_enable_spot_vm: true             # enabling spot vm by setting the variable to true
    # the rest of the variables
```

To learn more about Spot VM  visit: https://cloud.google.com/compute/docs/instances/spot

#### Set cluster size

Set the static cluster
size using `a3_static_cluster_size` variable. Recall that there are 8 NVIDIA H100 GPUs per a3-highgpu-8g VM.

```yaml
  a3_static_cluster_size: 32
```

## Cluster creation

> [!NOTE]
> This blueprint is not compatible with the legacy a3-highgpu-8g blueprints. We recommend bringing down the earlier cluster and redeploying using the below mentioned step.

Provision the cluster blueprint (approximately 40 minutes):

```shell
./gcluster deploy -d examples/machine-learning/a3-highgpu-8g/a3high-slurm-deployment.yaml examples/machine-learning/a3-highgpu-8g/a3high-slurm-blueprint.yaml --auto-approve
```

Building the image is time-consuming due to the installation of linux kernel, nvidia drivers, cuda toolkit, and slurm.
To significantly reduce deployment time when recreating the cluster, use the `--skip image` flag:

```shell
./gcluster deploy -d examples/machine-learning/a3-highgpu-8g/a3high-slurm-deployment.yaml examples/machine-learning/a3-highgpu-8g/a3high-slurm-blueprint.yaml --auto-approve --skip image
```

Important Restrictions:
1. This skip option is only valid when recreating the cluster; the initial deployment always requires an image build.
2. The image build cannot be skipped if the `deployment_name` is changed.

Selective deployment and teardown for this blueprint are documented centrally. See [examples/machine-learning/README.md](../README.md) for full details.

Example (deploy only the primary group for this blueprint):

```bash
./gcluster deploy -d examples/machine-learning/a3-highgpu-8g/a3high-slurm-deployment.yaml examples/machine-learning/a3-highgpu-8g/a3high-slurm-blueprint.yaml --only primary
```

## Receive Data Path Manager (RxDM)

To achieve optimal application performance, an additional service called the
"Receive Data Path Manager" (RxDM) must run with the same lifetime as the job.
Additionally, the GPUDirect-TCPXO (FasTrak) NCCL plugin must be installed into
the execution environment of the workload. Both the RxDM and plugin are
distributed by Docker container images.

This blueprint includes a Slurm "Prolog" and "Epilog" script that will run
before and after every job running on more than 1 a3-highgpu-8g compute node.
The Prolog will perform the following actions:

- Ensure the `import-helper` (`dmabuf-import-helper`) kernel module is loaded
- Install the NCCL and GPUDirect-TCPXO plugin into `/var/lib/tcpxo/lib64` of the host
- Run the RxDM service
  - This is a long-lived service that runs alongside the job
  - Mounts `/var/lib/nvidia/lib64` into `/usr/local/nvidia/lib64` of the container
  - Mounts `/dev/dmabuf_import_helper` into `/dev/dmabuf_import_helper` of the container

The Epilog will:

- Stop the RxDM service
- Prune any stopped containers (freeing up disk space)

## Jobs using TCPXO (FasTrak)

Jobs that are running across multiple a3-highgpu-8g VMs with TCPXO will benefit from the RxDM and the NCCL plugin. An example containerized job is located at `nccl-tests/run-nccl-tests.sh`. In addition to setting standard NCCL configuration values (or sourcing `/var/lib/tcpxo/lib64/nccl-env-profile.sh`), a job must:

- Set `LD_LIBRARY_PATH` to include `/var/lib/tcpxo/lib64` and `/usr/local/nvidia/lib64`
- Set `NCCL_FASTRAK_LLCM_DEVICE_DIRECTORY=/dev/aperture_devices`

If job is containerized:
- Mount `/var/lib/tcpxo/lib64` to `/var/lib/tcpxo/lib64` in the container (to make the NCCL plugin available)
- Mount `/dev/aperture_devices` to `/dev/aperture_devices` in the container

## Example workload (NCCL benchmark)

The example workload below demonstrates the pattern recommended in activating
the Receive Data Path Manager during multi-node jobs while running the standard
`nccl-tests` benchmark over GPUDirect-TCPXO (FasTrak).

### Clone the Cluster Toolkit repository containing the NCCL benchmark

```shell
git clone https://github.com/GoogleCloudPlatform/cluster-toolkit
cd cluster-toolkit/examples/machine-learning/a3-highgpu-8g/nccl-tests
```

### Import the PyTorch image from the NVIDIA Container Registry

```shell
bash import_pytorch_container.sh
```

### Build NCCL

```shell
sbatch build-nccl-tests.sh
```

### Run NCCL tests

```shell
sbatch run-nccl-tests.sh
```

## Destroy the VMs

```bash
#!/bin/bash
./gcluster destroy <DEPLOYMENT_FOLDER> --auto-approve
```

**Note:** GCS buckets created for Terraform state are not deleted by the `./gcluster destroy` command and must be deleted manually.

[consume]: https://cloud.google.com/compute/docs/instances/reservations-consume#consuming_instances_from_any_matching_reservation
[tkdeps]: https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies
[tkinstall]: https://github.com/GoogleCloudPlatform/cluster-toolkit/#quickstart
[deprecated-badge]: https://img.shields.io/badge/-deprecated-%23fea2a2?style=plastic
