# Objective

> [!CAUTION]
> This solution is built upon "legacy" blueprints using Slurm-GCP v5. The
> [solution using v6](../README.md) is recommended for all new deployments.
> The legacy solution is presented for customers with existing deployments. We
> recommend maintaining existing deployments with the version of the Toolkit
> originally used to deploy.

This document will guide you to successfully provisioning a Slurm cluster with
a3-highgpu-8g compute nodes running NVIDIA H100 GPUs.

## Before starting

> [!IMPORTANT]
> Before beginning, submit a request to your Google Cloud representative for
> access to the Deep Learning VM Image for a3-highgpu-8g. It is currently
> available only by Private Preview request. This image contains patches that
> significantly enhance the network performance of workloads that span multiple
> a3-highgpu-8g VMs. You will use the image ID in the steps shown below.

## Required setup

Please follow the initial instructions for:

- Installing Cluster Toolkit [dependencies][tkdeps] (Go, Terraform, Packer)
- Installing the Cluster [Toolkit][tkinstall]

Verify that your release of the Cluster Toolkit is greater than 1.31.1 and less
than or equal to 1.37.0.

```shell
gcluster --version
```

The solution requires several Python packages to be available. We recommend
installing them in a Python virtual environment:

```shell
python3 -m venv toolkit-a3
source toolkit-a3/bin/activate
pip3 install -r \
    https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/5.12.2/scripts/requirements.txt
```

**Always** activate the environment before running any gcluster commands such as
deploy or destroy.

```shell
source /absolute/path/to/toolkit-a3/bin/activate
```

## Top-Level Design of Solution

The solution is split into 3 Cluster Toolkit blueprints:

1. Provision 1 system network and 1 Filestore instance for mounting `/home`
across the cluster.
2. Build a custom image installing Slurm in an Ubuntu 20.04 image. The image
runs a kernel patched with performance enhancements for the a3-highgpu-8g VM.
3. Provision 4 GPU networks and a Slurm cluster using the custom image.

The 1st and 2nd blueprints should be provisioned once and rarely need further
modification. This approach separates the lifecycle of a Filestore instance from
the lifecycle of the cluster, allowing the cluster to be deleted while retaining
access to data and home directories. The 3rd cluster blueprint may be more
frequently updated and re-provisioned as discussed below.

## First time considerations

> [!IMPORTANT]
> These steps do not need to be repeated when a cluster is re-provisioned. They
> are initial setup steps in a project.

Replace the values for `PROJECT_ID`, `REGION`, and `ZONE` with the project,
region, and zone in which you have an a3-highgpu-8g allocation. The value for
`BUCKET` must be unique and will be used to create a new bucket. After replacing
the values, execute them so that they automatically populate parameters in the
commands shown below. Note that each a3-highgpu-8g VM (`N_VMS`) contains 8 NVIDIA
H100 GPUs.

```shell
export PROJECT_ID=customer-project-id
export BUCKET=customer-bucket
export REGION=customer-region
export ZONE=customer-zone
export N_VMS=32
```

### Saving Terraform state
Create a bucket with versioning enabled to store Terraform state:

```shell
gcloud storage buckets create gs://${BUCKET} --project=${PROJECT_ID} \
    --default-storage-class=STANDARD --location=${REGION} \
    --uniform-bucket-level-access
gcloud storage buckets update gs://${BUCKET} --versioning
```

Modify all 3 blueprints to configure the new bucket to serve as a Terraform
remote backend:

```yaml
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: customer-bucket # modify to bucket created above
```

### Set default values

Modify the the deployment variables `project_id`, `region`, `zone`, in the
`vars` block of all 3 blueprints:

```yaml
  project_id: customer-project
  region: customer-region
  zone: customer-zone
```

### Set kernel-patched OS image

Obtain values for `source_image_project_id` and `source_image` from your Google
Cloud representative. Set them at approximately lines 33 and 34 of
`ml-slurm-a3-1-image.yaml`.

```yaml
  source_image_project_id: source-image-project-id # use value supplied by Google Cloud staff
  source_image: source-image-name                  # use value supplied by Google Cloud staff
```

### Reservation created by Google

> [!IMPORTANT]
> If you have ***not*** received a VM reservation from Google Cloud staff, then
> skip this step and proceed to [manual reservation creation](#manual-creation-of-reservation).

Set the deployment variable `a3_reservation_name` at approximately line 38 of
`ml-slurm-a3-2-cluster.yaml` to the reservation name provided by Google. The
value for `a3_maintenance_interval` should also be set as directed by Google
staff. A common setting is `PERIODIC`, shown below, but this value must be
confirmed with Google staff.

```yaml
  # a3_reservation_name must be specified; if Google staff have provided you
  # with a reservation name, use it. Otherwise supply user-created reservation.
  a3_reservation_name: reservation-name-provided-by-google
  # a3_maintenance_interval should be empty string by default; if Google staff
  # have created a reservation, they will also provide a3_maintenance_interval
  a3_maintenance_interval: PERIODIC
```

### Manual creation of reservation

> [!IMPORTANT]
> If you received a VM reservation from Google Cloud staff, then skip this step
> after confirming that you followed the instructions in [reservation created by
> Google](#reservation-created-by-google).

We recommend creating a reservation to ensure reliable access to re-create VMs
if you need to redeploy or otherwise maintain your cluster.

```shell
gcloud compute reservations create a3-reservation-0 \
    --project=${PROJECT_ID} \
    --machine-type=a3-highgpu-8g \
    --vm-count=${N_VMS} \
    --zone=${ZONE} \
    --require-specific-reservation \
    --log-http
```

This reservation be must be specified when creating VMs with matching parameters
(e.g. a3-highgpu-8g VM in configured zone). If you executed the command above
without modification, you may leave `a3_reservation_name` and
`a3_maintenance_interval` at their default values in
`ml-slurm-a3-2-cluster.yaml`. Otherwise, ensure that the reservation name in the
blueprint matches the name of the user-created reservation.

```yaml
  # a3_reservation_name must be specified; if Google staff have provided you
  # with a reservation name, use it. Otherwise supply user-created reservation.
  a3_reservation_name: a3-reservation-0
  # a3_maintenance_interval should be empty string by default; if Google staff
  # have created a reservation, they will also provide a3_maintenance_interval
  a3_maintenance_interval: ""
```

### Set cluster size

At approximately line 37 of `ml-slurm-a3-2-cluster.yaml`, set the static cluster
size. Recall that there are 8 NVIDIA H100 GPUs per a3-highgpu-8g VM.

```yaml
  a3_static_cluster_size: 32
```

## Cluster creation

The blueprint `ml-slurm-a3-0-base.yaml` will create 1 system network and a
Filestore `/home` filesystem. Run the standard Toolkit workflow at the command
line (approx. 5 minutes):

```shell
gcluster deploy ml-slurm-a3-0-base.yaml --auto-approve
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
```

Build the custom image using ml-slurm-a3-1-image.yaml and the same workflow
as above. Run at the command line:

```shell
gcluster deploy ml-slurm-a3-1-image.yaml --auto-approve
```

The image will take approximately 30 minutes to build.

> [!IMPORTANT]
> You must modify `ml-slurm-a3-2-cluster.yaml` to update the IP address of the
> Filestore instance for `/home`. Your IP address will differ from that shown
> below and must match the output from deploying the base blueprint above:
>
> ```yaml
>   server_ip_homefs: 10.224.153.226
> ```

Provision the cluster blueprint (approximately 5-10 minutes):

```shell
gcluster deploy ml-slurm-a3-2-cluster.yaml --auto-approve
```

## Receive Data Path Manager (RxDM)

To achieve optimal application performance, an additional service called the
"Receive Data Path Manager" (RxDM) must run with the same lifetime as the job.
Additionally, a NCCL plugin must be installed into the execution environment of
the workload. Both the RxDM and plugin are distributed by Docker container
images.

This blueprint includes a Slurm "Prolog" and "Epilog" script that will run
before and after every job running on more than 1 a3-highgpu-8g compute node.
The Prolog will perform the following actions:

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

Jobs that are running across multiple a3-highgpu-8g VMs will benefit from using
the RxDM and the NCCL plugin. An example containerized job is located at
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

[consume]: https://cloud.google.com/compute/docs/instances/reservations-consume#consuming_instances_from_any_matching_reservation
[tkdeps]: https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies
[tkinstall]: https://github.com/GoogleCloudPlatform/cluster-toolkit/#quickstart
