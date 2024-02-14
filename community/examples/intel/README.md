# Intel Solutions for the HPC Toolkit

<!-- TOC generated with: md_toc github community/examples/intel/README.md | sed -e "s/\s-\s/ * /"-->
<!-- TOC -->

- [Intel Solutions for the HPC Toolkit](#intel-solutions-for-the-hpc-toolkit)
  - [Intel-Optimized Slurm Cluster](#intel-optimized-slurm-cluster)
    - [Initial Setup for the Intel-Optimized Slurm Cluster](#initial-setup-for-the-intel-optimized-slurm-cluster)
    - [Deploy the Slurm Cluster](#deploy-the-slurm-cluster)
    - [Connect to the login node](#connect-to-the-login-node)
    - [Access the cluster and provision an example job](#access-the-cluster-and-provision-an-example-job)
    - [Delete the infrastructure when not in use](#delete-the-infrastructure-when-not-in-use)
  - [DAOS Cluster](#daos-cluster)
    - [Initial Setup for DAOS Cluster](#initial-setup-for-daos-cluster)
    - [Deploy the DAOS Cluster](#deploy-the-daos-cluster)
    - [Connect to a client node](#connect-to-a-client-node)
    - [Verify the DAOS storage system](#verify-the-daos-storage-system)
    - [Create a DAOS Pool and Container](#create-a-daos-pool-and-container)
      - [About the DAOS Command Line Tools](#about-the-daos-command-line-tools)
      - [Determine Free Space](#determine-free-space)
      - [Create a Pool](#create-a-pool)
      - [Create a Container](#create-a-container)
      - [Mount the DAOS Container](#mount-the-daos-container)
    - [Use DAOS Storage](#use-daos-storage)
    - [Unmount the DAOS Container](#unmount-the-daos-container)
    - [Delete the DAOS infrastructure when not in use](#delete-the-daos-infrastructure-when-not-in-use)
  - [DAOS Server with Slurm cluster](#daos-server-with-slurm-cluster)
    - [Initial Setup for the DAOS/Slurm cluster](#initial-setup-for-the-daosslurm-cluster)
    - [Deploy the DAOS/Slurm Cluster](#deploy-the-daosslurm-cluster)
    - [Connect to the DAOS/Slurm Cluster login node](#connect-to-the-daosslurm-cluster-login-node)
    - [Create and Mount a DAOS Container](#create-and-mount-a-daos-container)
    - [Run a Job that uses the DAOS Container](#run-a-job-that-uses-the-daos-container)
    - [Unmount the Container](#unmount-the-container)
    - [Delete the DAOS/Slurm Cluster infrastructure when not in use](#delete-the-daosslurm-cluster-infrastructure-when-not-in-use)

## Intel-Optimized Slurm Cluster

This document is adapted from a [Cloud Shell tutorial][tutorial] developed to
demonstrate Intel Select Solutions within the Toolkit. It expands upon that
tutorial by building custom images that save provisioning time and improve
reliability when scaling up compute nodes.

The Google Cloud [HPC VM Image][hpcvmimage] has a built-in feature enabling it
to install a Google Cloud-tested release of Intel compilers and libraries that
are known to achieve optimal performance on Google Cloud.

[tutorial]: ../../../docs/tutorials/intel-select/intel-select.md
[hpcvmimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm

Identify a project to work in and substitute its unique id wherever you see
`<<PROJECT_ID>>` in the instructions below.

### Initial Setup for the Intel-Optimized Slurm Cluster

Before provisioning any infrastructure in this project you should follow the
Toolkit guidance to enable [APIs][apis] and establish minimum resource
[quotas][quotas]. In particular, the following APIs should be enabled

- [file.googleapis.com](https://cloud.google.com/filestore/docs/reference/rest) (Cloud Filestore)
- [compute.googleapis.com](https://cloud.google.com/compute/docs/reference/rest/v1#service:-compute.googleapis.com) (Google Compute Engine)

[apis]: ../../../README.md#enable-gcp-apis
[quotas]: ../../../README.md#gcp-quotas

And the following available quota is required in the region used by the cluster:

- Filestore: 2560GB
- C2 CPUs: 4 (login node)
- C2 CPUs: up to 6000 (fully-scaled "compute" partition)
  - This quota is not necessary at initial deployment, but will be required to
    successfully scale the partition to its maximum size

### Deploy the Slurm Cluster

Use `ghpc` to provision the blueprint, supplying your project ID

```text
ghpc create --vars project_id=<<PROJECT_ID>> community/examples/intel/hpc-intel-select-slurm.yaml
```

This will create a set of directories containing Terraform modules and Packer
templates. **Please ignore the printed instructions** in favor of the following:

1. Provision the network and startup scripts that install Intel software

    ```shell
    terraform -chdir=hpc-intel-select/primary init
    terraform -chdir=hpc-intel-select/primary validate
    terraform -chdir=hpc-intel-select/primary apply
    ```

2. Capture the startup scripts to files that will be used by Packer to build the
   images

    ```shell
    terraform -chdir=hpc-intel-select/primary output \
      -raw startup_script_startup_controller > \
      hpc-intel-select/build1/controller-image/startup_script.sh

    terraform -chdir=hpc-intel-select/primary output \
      -raw startup_script_startup_compute > \
      hpc-intel-select/build2/compute-image/startup_script.sh
    ```

3. Build the custom Slurm controller image. While this step is executing, you
   may begin the next step in parallel.

    ```shell
    cd hpc-intel-select/build1/controller-image
    packer init .
    packer validate .
    packer build -var startup_script_file=startup_script.sh .
    ```

4. Build the custom Slurm image for login and compute nodes

    ```shell
    cd -
    cd hpc-intel-select/build2/compute-image
    packer init .
    packer validate .
    packer build -var startup_script_file=startup_script.sh .
    ```

5. Provision the Slurm cluster

    ```shell
    cd -
    terraform -chdir=hpc-intel-select/cluster init
    terraform -chdir=hpc-intel-select/cluster validate
    terraform -chdir=hpc-intel-select/cluster apply
    ```

### Connect to the login node

Once the startup script has completed and Slurm reports readiness, connect to the login node.

1. Open the following URL in a new tab.

    https://console.cloud.google.com/compute

    This will take you to **Compute Engine > VM instances** in the Google Cloud Console

    Ensure that you select the project in which you are provisioning the cluster.

2. Click on the **SSH** button associated with the `slurm-hpc-intel-select-login0`
   instance.

    This will open a separate pop up window with a terminal into our newly created
    Slurm login VM.

### Access the cluster and provision an example job

   **The commands below should be run on the login node.**

1. Create a default ssh key to be able to ssh between nodes

    ```shell
    ssh-keygen -q -N '' -f ~/.ssh/id_rsa
    cp ~/.ssh/id_rsa.pub ~/.ssh/authorized_keys
    chmod 0600 ~/.ssh/authorized_keys
    ```

1. Submit an example job

    ```shell
    cp /var/tmp/dgemm_job.sh .
    sbatch dgemm_job.sh
    ```

### Delete the infrastructure when not in use

> **_NOTE:_** If the Slurm controller is shut down before the auto-scale nodes
> are destroyed then they will be left running.

Open your browser to the VM instances page and ensure that nodes named "compute"
have been shutdown and deleted by the Slurm autoscaler. Delete the remaining
infrastructure in reverse order of creation:

```shell
terraform -chdir=hpc-intel-select/cluster destroy
terraform -chdir=hpc-intel-select/primary destroy
```

## DAOS Cluster

The [pfs-daos.yaml](pfs-daos.yaml) blueprint describes an environment with
- Two DAOS server instances
- Two DAOS client instances

The [pfs-daos.yaml](pfs-daos.yaml) blueprint uses a Packer template and
Terraform modules from the [Google Cloud DAOS][google-cloud-daos] repository.
Please review the [introduction to image building](../../../docs/image-building.md)
for general information on building custom images using the Toolkit.

Identify a project to work in and substitute its unique id wherever you see
`<<PROJECT_ID>>` in the instructions below.

### Initial Setup for DAOS Cluster

Before provisioning the DAOS cluster you must follow the steps listed in the [Google Cloud DAOS Pre-deployment Guide][pre-deployment_guide].

Skip the "Build DAOS Images" step at the end of the [Pre-deployment Guide][pre-deployment_guide]. The [pfs-daos.yaml](pfs-daos.yaml) blueprint will build the images as part of the deployment.

The Pre-deployment Guide provides instructions for enabling service accounts, APIs, establishing minimum resource quotas and other necessary steps to prepare your project.

[google-cloud-daos]: https://github.com/daos-stack/google-cloud-daos
[pre-deployment_guide]: https://github.com/daos-stack/google-cloud-daos/blob/main/docs/pre-deployment_guide.md

### Deploy the DAOS Cluster

After completing the steps in the [Pre-deployment Guide][pre-deployment_guide] use `ghpc` to provision the blueprint

```text
ghpc create community/examples/intel/pfs-daos.yaml  \
  --vars project_id=<<PROJECT_ID>> \
  [--backend-config bucket=<GCS tf backend bucket>]
```

This will create the deployment directory containing Terraform modules and
Packer templates. The `--backend-config` option is not required but recommended.
It will save the terraform state in a pre-existing [Google Cloud Storage
bucket][bucket]. For more information see [Setting up a remote terraform
state][backend]. Use `ghpc deploy` to provision your DAOS storage cluster:

```text
ghpc deploy pfs-daos --auto-approve
```

[backend]: ../../../examples/README.md#optional-setting-up-a-remote-terraform-state
[bucket]: https://cloud.google.com/storage/docs/creating-buckets

### Connect to a client node

1. Open the following URL in a new tab.

   https://console.cloud.google.com/compute

   This will take you to **Compute Engine > VM instances** in the Google Cloud Console.

   Select the project in which the DAOS cluster will be provisioned.

2. Click on the **SSH** button associated with the **daos-client-0001**
   instance to open a window with a terminal into the first DAOS client instance.

### Verify the DAOS storage system

The `community/examples/intel/pfs-daos.yaml` blueprint does not contain configuration for DAOS pools and containers. Therefore, pools and containers will need to be created manually.

Before pools and containers can be created the storage system must be formatted. Formatting the storage is done automatically by the startup script that runs on the *daos-server-0001* instance. The startup script will run the [dmg storage format](https://docs.daos.io/v2.2/admin/deployment/?h=dmg+storage#storage-formatting) command. It may take a few minutes for all daos server instances to join.

Verify that the storage system has been formatted and that the daos-server instances have joined.

```bash
sudo dmg system query -v
```

The command will not return output until the system is ready.

The output will look similar to

```text
Rank UUID                                 Control Address   Fault Domain      State  Reason
---- ----                                 ---------------   ------------      -----  ------
0    225a0a51-d4ed-4ac3-b1a5-04b31c08b559 10.128.0.51:10001 /daos-server-0001 Joined
1    553ab1dc-99af-460e-a57c-3350611d1d09 10.128.0.43:10001 /daos-server-0002 Joined
```

Both daos-server instances should show a state of *Joined*.

### Create a DAOS Pool and Container

#### About the DAOS Command Line Tools

The DAOS Management tool `dmg` is used by System Administrators to manage the DAOS storage [system](https://docs.daos.io/v2.2/overview/architecture/#daos-system) and DAOS [pools](https://docs.daos.io/v2.2/overview/storage/#daos-pool). Therefore, `sudo` must be used when running `dmg`.

The DAOS CLI `daos` is used by both users and System Administrators to create and manage [containers](https://docs.daos.io/v2.2/overview/storage/#daos-container). It is not necessary to use `sudo` with the `daos` command.

#### Determine Free Space

Determine how much free space is available.

```bash
sudo dmg storage query usage
```

The result will look similar to

```text
Hosts            SCM-Total SCM-Free SCM-Used NVMe-Total NVMe-Free NVMe-Used
-----            --------- -------- -------- ---------- --------- ---------
daos-server-0001 215 GB    215 GB   0 %      6.4 TB     6.4 TB    0 %
daos-server-0002 215 GB    215 GB   0 %      6.4 TB     6.4 TB    0 %
```

In the example output above we see that there is a total of 12.8TB NVME-Free.

#### Create a Pool

Create a single pool owned by root which uses all available free space.

```bash
sudo dmg pool create -z 12.8TB -t 3 -u root --label=pool1
```

Set ACLs to allow any user to create a container in *pool1*.

```bash
sudo dmg pool update-acl -e A::EVERYONE@:rcta pool1
```

See the [Pool Operations](https://docs.daos.io/v2.2/admin/pool_operations) section of the of the DAOS Administration Guide for more information about creating pools.

#### Create a Container

At this point it is necessary to determine who will need to access the container
and how it will be used. The ACLs will need to be set properly to allow users and/or groups to access the container.

For the purpose of this demo create the container without specifying ACLs. The container will be owned by your user account and you will have full access to the container.

```bash
daos cont create pool1 \
  --label cont1 \
  --type POSIX \
  --properties rf:0
```

See the [Container Management](https://docs.daos.io/v2.2/user/container) section of the of the DAOS User Guide for more information about creating containers.

#### Mount the DAOS Container

Mount the container with dfuse (DAOS Fuse)

```bash
mkdir -p ${HOME}/daos/cont1
dfuse --singlethread \
  --pool=pool1 \
  --container=cont1 \
  --mountpoint=${HOME}/daos/cont1
```

Verify that the container is mounted

```bash
df -h -t fuse.daos
```

### Use DAOS Storage

The `cont1` container is now mounted on `${HOME}/daos/cont1`

Create a 20GiB file which will be stored in the DAOS filesystem.

```bash
time LD_PRELOAD=/usr/lib64/libioil.so \
dd if=/dev/zero of="${HOME}/daos/cont1/test20GiB.img" iflag=fullblock bs=1G count=20
```

See the [File System](https://docs.daos.io/v2.2/user/filesystem/) section of the DAOS User Guide for more information about DFuse.

### Unmount the DAOS Container

The container will need to by unmounted before you log out.  If this is not done it can leave open file handles and prevent the container from being mounted when you log in again.

```bash
fusermount3 -u ${HOME}/daos/cont1
```

Verify that the container is unmounted

```bash
df -h -t fuse.daos
```

See the [DFuse (DAOS FUSE)](https://docs.daos.io/v2.2/user/filesystem/?h=dfuse#dfuse-daos-fuse) section of the DAOS User Guide for more information about mounting POSIX containers.

### Delete the DAOS infrastructure when not in use

> **_NOTE:_** All the DAOS data will be permanently lost after cluster deletion.

Delete the remaining infrastructure

```shell
ghpc destroy pfs-daos --auto-approve
```

## DAOS Server with Slurm cluster

The [hpc-slurm-daos.yaml](hpc-slurm-daos.yaml) blueprint describes an environment with a Slurm cluster and four DAOS server instances. The compute nodes are configured as DAOS clients and have the ability to use the DAOS filesystem on the DAOS server instances.

The blueprint uses modules from
- [google-cloud-daos][google-cloud-daos]
- [community/modules/scheduler/SchedMD-slurm-on-gcp-controller][SchedMD-slurm-on-gcp-controller]
- [community/modules/scheduler/SchedMD-slurm-on-gcp-login-node][SchedMD-slurm-on-gcp-login-node]
- [community/modules/compute/SchedMD-slurm-on-gcp-partition][SchedMD-slurm-on-gcp-partition]

The blueprint also uses a Packer template from the [Google Cloud
DAOS][google-cloud-daos] repository. Please review the [introduction to image
building](../../../docs/image-building.md) for general information on building
custom images using the Toolkit.

Identify a project to work in and substitute its unique id wherever you see
`<<PROJECT_ID>>` in the instructions below.

### Initial Setup for the DAOS/Slurm cluster

Before provisioning the DAOS cluster you must follow the steps listed in the [Google Cloud DAOS Pre-deployment Guide][pre-deployment_guide].

Skip the "Build DAOS Images" step at the end of the [Pre-deployment Guide][pre-deployment_guide]. The [hpc-slurm-daos.yaml](hpc-slurm-daos.yaml) blueprint will build the DAOS server image as part of the deployment.

The Pre-deployment Guide provides instructions for enabling service accounts, APIs, establishing minimum resource quotas and other necessary steps to prepare your project for DAOS server deployment.

[google-cloud-daos]: https://github.com/daos-stack/google-cloud-daos
[pre-deployment_guide]: https://github.com/daos-stack/google-cloud-daos/blob/main/docs/pre-deployment_guide.md

[packer-template]: https://github.com/daos-stack/google-cloud-daos/blob/main/images/daos.pkr.hcl
[apis]: ../../../README.md#enable-gcp-apis
[SchedMD-slurm-on-gcp-controller]: ../../modules/scheduler/SchedMD-slurm-on-gcp-controller
[SchedMD-slurm-on-gcp-login-node]: ../../modules/scheduler/SchedMD-slurm-on-gcp-login-node
[SchedMD-slurm-on-gcp-partition]: ../../modules/compute/SchedMD-slurm-on-gcp-partition

Follow the Toolkit guidance to enable [APIs][apis] and establish minimum resource [quotas][quotas] for Slurm.

[apis]: ../../../README.md#enable-gcp-apis
[quotas]: ../../../README.md#gcp-quotas

The following available quota is required in the region used by Slurm:

- Filestore: 2560GB
- C2 CPUs: 6000 (fully-scaled "compute" partition)
  - This quota is not necessary at initial deployment, but will be required to
    successfully scale the partition to its maximum size
- C2 CPUs: 4 (login node)

### Deploy the DAOS/Slurm Cluster

Use `ghpc` to provision the blueprint, supplying your project ID

```text
ghpc create community/examples/intel/hpc-slurm-daos.yaml \
  --vars project_id=<<PROJECT_ID>> \
  [--backend-config bucket=<GCS tf backend bucket>]
```

This will create a set of directories containing Terraform modules and Packer
templates.

The `--backend-config` option is not required but recommended. It will save the terraform state in a pre-existing [Google Cloud Storage bucket][bucket]. For more information see [Setting up a remote terraform state][backend].

Follow `ghpc` instructions to deploy the environment

```text
ghpc deploy daos-slurm --auto-approve
```

[backend]: ../../../examples/README.md#optional-setting-up-a-remote-terraform-state
[bucket]: https://cloud.google.com/storage/docs/creating-buckets

### Connect to the DAOS/Slurm Cluster login node

Once the startup script has completed and Slurm reports readiness, connect to the login node.

1. Open the following URL in a new tab.

   https://console.cloud.google.com/compute

   This will take you to **Compute Engine > VM instances** in the Google Cloud Console

   Select the project in which the cluster will be provisionsd.

2. Click on the `SSH` button associated with the `slurm-daos-slurm-login0`
   instance.

   This will open a separate pop up window with a terminal into our newly created
   Slurm login VM.

### Create and Mount a DAOS Container

The [community/examples/intel/hpc-slurm-daos.yaml](hpc-slurm-daos.yaml) blueprint defines a single DAOS pool named `pool1`. The pool will be created when the *daos-server* instances are provisioned.

You will need to create your own DAOS container in the pool that can be used by your Slurm jobs.

While logged into the login node create a container named `cont1` in the `pool1` pool:

```bash
daos cont create --type=POSIX --properties=rf:0 --label=cont1 pool1
```

Since the `cont1` container is owned by your account, your Slurm jobs will need to run as your user account in order to access the container.

Create a mount point for the container and mount it with dfuse (DAOS Fuse)

```bash
mkdir -p ${HOME}/daos/cont1

dfuse --singlethread \
--pool=pool1 \
--container=cont1 \
--mountpoint=${HOME}/daos/cont1
```

Verify that the container is mounted

```bash
df -h -t fuse.daos
```

### Run a Job that uses the DAOS Container

On the login node create a `daos_job.sh` file with the following content

```bash
#!/bin/bash
JOB_HOSTNAME="$(hostname)"
TIMESTAMP="$(date '+%Y%m%d%H%M%S')"

echo "Timestamp         = ${TIMESTAMP}"
echo "Date              = $(date)"
echo "Hostname          = $(hostname)"
echo "User              = $(whoami)"
echo "Working Directory = $(pwd)"
echo ""
echo "Number of Nodes Allocated = $SLURM_JOB_NUM_NODES"
echo "Number of Tasks Allocated = $SLURM_NTASKS"

MOUNT_DIR="${HOME}/daos/cont1"
LOG_FILE="${MOUNT_DIR}/${JOB_HOSTNAME}.log"

echo "${JOB_HOSTNAME} : Creating directory: ${MOUNT_DIR}"
mkdir -p "${MOUNT_DIR}"

echo "${JOB_HOSTNAME} : Mounting with dfuse"
dfuse --singlethread --pool=pool1 --container=cont1 --mountpoint="${MOUNT_DIR}"
sleep 5

echo "${JOB_HOSTNAME} : Creating log file"
echo "Job ${SLURM_JOB_ID} running on ${JOB_HOSTNAME}" | tee "${MOUNT_DIR}/${TIMESTAMP}_${JOB_HOSTNAME}.log"

echo "${JOB_HOSTNAME} : Unmounting dfuse"
fusermount3 -u "${MOUNT_DIR}"
```

Run the `daos_job.sh` script in an interactive Slurm job on 4 nodes

```bash
srun --nodes=4 \
  --ntasks-per-node=1 \
  --time=00:10:00 \
  --job-name=daos \
  --output=srunjob_%j.log \
  --partition=compute \
  daos_job.sh &
```

Run `squeue` to see the status of the job. The `daos_job.sh` script will run once on each of the 4 nodes. Each time it runs it creates a log file which is stored in the `cont1` DAOS container.

Wait for the job to complete and then view the files that were created in the `cont1` DAOS container mounted on `${HOME}/daos/cont1`.

```bash
ls -l ${HOME}/daos/cont1/*.log
cat ${HOME}/daos/cont1/*.log
```

### Unmount the Container

The container will need to by unmounted before you log out.  If this is not done it can leave open file handles and prevent the container from being mounted when you log in again.

```bash
fusermount3 -u ${HOME}/daos/cont1
```

Verify that the container is unmounted

```bash
df -h -t fuse.daos
```

See the [DFuse (DAOS FUSE)](https://docs.daos.io/v2.2/user/filesystem/?h=dfuse#dfuse-daos-fuse) section of the DAOS User Guide for more information about mounting POSIX containers.

### Delete the DAOS/Slurm Cluster infrastructure when not in use

> **_NOTE:_** All the DAOS data will be permanently lost after cluster deletion.

<!-- -->

> **_NOTE:_** If the Slurm controller is shut down before the auto-scale nodes
> are destroyed then they will be left running.

Open your browser to the VM instances page and ensure that nodes named "compute"
have been shutdown and deleted by the Slurm autoscaler. Delete the remaining
infrastructure with `terraform`:

```shell
ghpc destroy daos-slurm --auto-approve
```
