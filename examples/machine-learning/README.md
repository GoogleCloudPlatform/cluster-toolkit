# Objective

This document will guide you to successfully provisioning a Slurm cluster with
A3 VM family compute nodes running NVIDIA H100 GPUs.

## Required initial setup

Please follow the initial instructions for:

- Installing Cloud HPC Toolkit [dependencies][tkdeps] (Go, Terraform, Packer)
- Installing the Cloud HPC [Toolkit][tkinstall]

[tkdeps]: https://cloud.google.com/hpc-toolkit/docs/setup/install-dependencies
[tkinstall]: https://github.com/GoogleCloudPlatform/hpc-toolkit/#quickstart

Verify that your release of the HPC Toolkit is 1.29.0 or later.

```shell
ghpc --version
```

Additional features of the solution require several Python packages to be
installed in your execution environment. We recommend using a Python virtual
environment to manage them as shown below

```shell
python3 -m venv toolkit-a3
source toolkit-a3/bin/activate
pip3 install -r \
    https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/5.10.6/scripts/requirements.txt
```

**Always** activate the environment before running any ghpc commands such as
deploy or destroy.

```shell
source /absolute/path/to/toolkit-a3/bin/activate
```

Install the beta gcloud components to enable beta APIs:

```shell
gcloud components install beta
```

## Top-Level Design of Solution

The solution is split into 3 HPC Toolkit blueprints:

1. Provision 5 VPCs (1 system network, 4 GPU networks) and 1 Filestore for
mounting /home across the cluster
2. Build a custom image installing Slurm in an Ubuntu 20.04 image. The image
runs a kernel patched with performance enhancements for the A3 VM family.
3. Provision a Slurm cluster using the custom image

The 1st and 2nd blueprints should be provisioned once and rarely need further
modification. This approach separates the lifecycle of a Filestore instance from
the lifecycle of the cluster, allowing the cluster to be deleted while retaining
access to data and home directories. The 3rd cluster blueprint may be more
frequently updated and re-provisioned as discussed below.

## First time considerations

_These steps do not need to be repeated when a cluster is re-provisioned. They
are initial setup steps in a project._

The following steps create a compact placement policy and VM reservation that
improves the performance of tightly-coupled workloads and ensures capacity for
A3 VMs. Values in the blueprint must be modified as shown below.

Replace the values for `PROJECT_ID`, `REGION`, and `ZONE` with the project,
region, and zone in which you have an A3 VM family allocation. The value for
`BUCKET` must be unique and will be used to create a new bucket. After replacing
the values, execute them so that they automatically populate parameters in the
commands shown below.

```shell
export PROJECT_ID=customer-project-id
export BUCKET=customer-bucket
export REGION=customer-region
export ZONE=customer-zone
export N_VMS=32
export POLICY=policy-compact-0
export RESERVATION=slurm-res-0
```

Each A3 VM (`N_VMS`) contains 8 NVIDIA H100 GPUs.  Create a bucket with
versioning enabled to store Terraform state:

```shell
gcloud storage buckets create gs://${BUCKET} --project=${PROJECT_ID} \
    --default-storage-class=STANDARD --location=${REGION} \
    --uniform-bucket-level-access
gcloud storage buckets update gs://${BUCKET} --versioning
```

If Google Cloud staff have created a reservation for you, then skip this step
and use the reservation name visible in the Cloud Console.  Otherwise, create
a compact placement policy attached to a reservation of A3 VMs.

First, create a compact placement policy:

```shell
gcloud beta compute resource-policies create group-placement ${POLICY} \
    --region=${REGION} \
    --collocation=COLLOCATED \
    --project=${PROJECT_ID}
```

And then create the reservation with the policy attached:

```shell
gcloud compute reservations create ${RESERVATION} \
    --project=${PROJECT_ID} \
    --machine-type=a3-highgpu-8g \
    --vm-count=${N_VMS} \
    --zone=${ZONE} \
    --require-specific-reservation \
    --resource-policies=placement=${POLICY} \
    --log-http
```

Modify the the blueprint deployment variables `project_id`, `region`, `zone`,
and `zones` in the `vars` block of `ml-slurm-a3-0-base.yaml`,
`ml-slurm-a3-1-image.yaml` and `ml-slurm-h100-2-cluster.yaml`:

```yaml
  project_id: customer-project
  region: customer-region
  zone: customer-zone
```

Obtain values for `source_image_project_id` and `source_image` from your Google
Cloud representative. Set them at approximately lines 33 and 34 of
`ml-slurm-a3-1-image.yaml`. Set the reservation name in the blueprint
deployment variable `a3_reservation_name` at approximately line 39 of
`ml-slurm-a3-2-cluster.yaml`. If the reservation has been supplied to you
by Google Cloud staff, use its name below.

```yaml
  a3_reservation_name: slurm-res-0
```

## Cluster creation

The blueprint `ml-slurm-a3-0-base.yaml` will create 5 VPCs (1 system, 4 GPU)
and a Filestore `/home` filesystem. This should take approximately 5 minutes. We
recommend the use of Cloud Storage for storing Terraform state. This should be
set directly in the blueprint:

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: customer-bucket # modify to bucket created above
```

Then run the standard Toolkit workflow at the command line:

```shell
ghpc create -l ERROR ml-slurm-a3-0-base.yaml
ghpc deploy slurm-a3-base --auto-approve
```

Several values will be output to the screen. The output will be similar to:

```hcl
network_name_sysnet = "sys-net"
network_storage_homefs = {
  "client_install_runner" = {
    "destination" = "install-nfs_home.sh"
    "source" = "modules/embedded/modules/file-system/filestore/scripts/install-nfs-client.sh"
    "type" = "shell"
  }
  "fs_type" = "nfs"
  "local_mount" = "/home"
  "mount_options" = "defaults,_netdev"
  "mount_runner" = {
    "args" = "\"10.224.153.226\" \"/nfsshare\" \"/home\" \"nfs\" \"defaults,_netdev\""
    "destination" = "mount_home.sh"
    "source" = "modules/embedded/modules/file-system/filestore/scripts/mount.sh"
    "type" = "shell"
  }
  "remote_mount" = "/nfsshare"
  "server_ip" = "10.224.153.226"
}
subnetwork_name_sysnet = "sys-subnet"
subnetwork_self_link_gpunet0 = "https://www.googleapis.com/compute/v1/projects/customer-project/regions/customer-region/subnetworks/gpu-subnet0"
subnetwork_self_link_gpunet1 = "https://www.googleapis.com/compute/v1/projects/customer-project/regions/customer-region/subnetworks/gpu-subnet1"
subnetwork_self_link_gpunet2 = "https://www.googleapis.com/compute/v1/projects/customer-project/regions/customer-region/subnetworks/gpu-subnet2"
subnetwork_self_link_gpunet3 = "https://www.googleapis.com/compute/v1/projects/customer-project/regions/customer-region/subnetworks/gpu-subnet3"
```

Build the custom image using ml-slurm-a3-1-image.yaml and the same workflow
as above. We recommend using Terraform state in Cloud Storage by modifying the
blueprint to include:

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: customer-bucket
```

And run at the command line:

```shell
ghpc create -l ERROR ml-slurm-a3-1-image.yaml
ghpc deploy slurm-a3-image --auto-approve
```

The image will take approximately 30 minutes to build. If you made no
modifications to `ml-slurm-a3-0-base.yaml` or `ml-slurm-h100-1-image.yaml`,
you must make only 1 modification to `ml-slurm-a3-2-cluster.yaml` to update
the IP address of the Filestore instance for `/home`:

```yaml
  server_ip_homefs: 10.224.153.226 # replace with IP address from output from slurm-a3-base!
```

Provision the cluster blueprint. Again, we recommend the use of Cloud Storage
for Terraform state.

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: customer-bucket
```

It will take approximately 5-10 minutes to provision the cluster.

```shell
ghpc create -w -l ERROR ml-slurm-a3-2-cluster.yaml
ghpc deploy slurm-a3-cluster --auto-approve
```

## Receive Data Path Manager (RxDM)

To achieve optimal application performance, an additional service called the
"Receive Data Path Manager" (RxDM) must run with the same lifetime as the job.
Additionally, a NCCL plugin must be installed into the execution environment of
the workload. Both the RxDM and plugin are distributed by Docker container
images.

This blueprint includes a Slurm "Prolog" and "Epilog" script that will run
before and after every job running on more than 1 A3 compute node. The Prolog
will perform the following actions:

- Install the NCCL plugin into /var/lib of the host
- Run the RxDM service
  - This is a long-lived service that runs alongside the job
  - Mounts `/var/lib/nvidia/lib64` into `/usr/lib/nvidia/lib64` of the container
  - Mount `/opt/tcpdirect_benchmark/` from the host into the container so that a
  textproto file defining the mapping from GPU to NIC is available. This file
  is present in the Deep Learning VM (DLVM) images that contain TCPDirect
  patches.
  - Mount `/run/tcpx-${SLURM_JOB_ID}` from the container into the host. This is
  set to the environment variables `${UDS_PATH}` in the script. This directory
  contains Unix socket files that implement a TCPx interface available to the
  user workload at `${UDS_PATH}`. The job must be configured to be aware of this
  path using `NCCL_GPUDIRECTTCPX_UNIX_CLIENT_PREFIX` environment variable!

The Epilog will

- Stop the RxDM service
- Prune any stopped containers (freeing up disk space)
- Remove the directory at `${UDS_PATH}`

## Jobs using the RxDM / TCPx

Jobs that are running across multiple A3 VMs will benefit from using the RxDM
and the NCCL plugin. An example containerized job is located at
`/opt/apps/scripts/run-nccl-tests.sh`. In addition to setting standard NCCL
configuration values, a job must:

- Set `NCCL_GPUDIRECTTCPX_UNIX_CLIENT_PREFIX` to `${UDS_PATH}`
- Set the `LD_LIBRARY_PATH` to include `/var/lib/tcpx/lib64` and `/usr/local/nvidia/lib64`

If job is containerized

- Mount `${UDS_PATH}` into the container at the same path
- Mount `/var/lib/tcpx/lib64` to `/var/lib/tcpx/lib64` in the container (to make the
  NCCL plugin available)
- Paths can be modified if `LD_LIBRARY_PATH` is likewise modified

## Example workload (NCCL benchmark)

The example workload below demonstrates the pattern recommended in Activating
the Receive Data Path Manager during jobs while running the standard nccl-tests
benchmark. It assumes the availability of a GPU/NIC topology file at
`/opt/tcpdirect_benchmark/gpu_rxq_configuration.textproto`. This file is built
into the DLVM images used by this solution, but may need to be provided if
using an alternative image.

### Clone the HPC Toolkit repository containing the NCCL benchmark

```shell
git clone https://github.com/GoogleCloudPlatform/hpc-toolkit
cd hpc-toolkit/examples/machine-learning/nccl-tests
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
