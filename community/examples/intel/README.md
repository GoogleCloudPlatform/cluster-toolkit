# Intel Solutions for the HPC Toolkit

<!-- TOC generated with: md_toc github community/examples/intel/README.md | sed -e "s/\s-\s/ * /"-->
<!-- TOC -->

- [Intel Solutions for the HPC Toolkit](#intel-solutions-for-the-hpc-toolkit)
  - [Intel-Optimized Slurm Cluster](#intel-optimized-slurm-cluster)
    - [Provisioning the Intel-optimized Slurm cluster](#provisioning-the-intel-optimized-slurm-cluster)
    - [Initial Setup for the Intel-Optimized Slurm Cluster](#initial-setup-for-the-intel-optimized-slurm-cluster)
    - [Deploying the Slurm Cluster](#deploying-the-slurm-cluster)
    - [Connecting to the login node](#connecting-to-the-login-node)
    - [Access the cluster and provision an example job](#access-the-cluster-and-provision-an-example-job)
    - [Delete the infrastructure when not in use](#delete-the-infrastructure-when-not-in-use)
  - [DAOS Cluster](#daos-cluster)
    - [Provisioning the DAOS cluster](#provisioning-the-daos-cluster)
    - [Initial Setup for DAOS Cluster](#initial-setup-for-daos-cluster)
    - [Deploying the DAOS Cluster](#deploying-the-daos-cluster)
    - [Connecting to a client node](#connecting-to-a-client-node)
    - [Verifying the DAOS storage system](#verifying-the-daos-storage-system)
    - [Creating a DAOS Pool and Container](#creating-a-daos-pool-and-container)
      - [About the DAOS Command Line Tools](#about-the-daos-command-line-tools)
      - [Determining Free Space](#determining-free-space)
      - [Creating a Pool](#creating-a-pool)
      - [Creating a Container](#creating-a-container)
    - [Mounting the DAOS Container](#mounting-the-daos-container)
    - [Unmounting the DAOS Container](#unmounting-the-daos-container)
    - [Delete the DAOS infrastructure when not in use](#delete-the-daos-infrastructure-when-not-in-use)
  - [DAOS Server with Slurm cluster](#daos-server-with-slurm-cluster)
    - [Provisioning the DAOS/Slurm cluster](#provisioning-the-daosslurm-cluster)
    - [Initial Setup for the DAOS/Slurm cluster](#initial-setup-for-the-daosslurm-cluster)
    - [Deploying the DAOS/Slurm Cluster](#deploying-the-daosslurm-cluster)
    - [Connecting to the DAOS/Slurm Cluster login node](#connecting-to-the-daosslurm-cluster-login-node)
    - [Creating and Mounting a DAOS Container](#creating-and-mounting-a-daos-container)
    - [Running a Job that uses the DAOS Container](#running-a-job-that-uses-the-daos-container)
    - [Unmounting the Container](#unmounting-the-container)
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

### Provisioning the Intel-optimized Slurm cluster

Identify a project to work in and substitute its unique id wherever you see
`<<PROJECT_ID>>` in the instructions below.

### Initial Setup for the Intel-Optimized Slurm Cluster

Before provisioning any infrastructure in this project you should follow the
Toolkit guidance to enable [APIs][apis] and establish minimum resource
[quotas][quotas]. In particular, the following APIs should be enabled

* [file.googleapis.com](https://cloud.google.com/filestore/docs/reference/rest) (Cloud Filestore)
* [compute.googleapis.com](https://cloud.google.com/compute/docs/reference/rest/v1#service:-compute.googleapis.com) (Google Compute Engine)

[apis]: ../../../README.md#enable-gcp-apis
[quotas]: ../../../README.md#gcp-quotas

And the following available quota is required in the region used by the cluster:

* Filestore: 2560GB
* C2 CPUs: 4 (login node)
* C2 CPUs: up to 6000 (fully-scaled "compute" partition)
  * This quota is not necessary at initial deployment, but will be required to
    successfully scale the partition to its maximum size

### Deploying the Slurm Cluster

Use `ghpc` to provision the blueprint, supplying your project ID:

```text
ghpc create --vars project_id=<<PROJECT_ID>> community/examples/intel/hpc-cluster-intel-select.yaml
```

This will create a set of directories containing Terraform modules and Packer
templates. **Please ignore the printed instructions** in favor of the following:

1. Provision the network and startup scripts that install Intel software.

    ```shell
    terraform -chdir=hpc-intel-select/primary init
    terraform -chdir=hpc-intel-select/primary validate
    terraform -chdir=hpc-intel-select/primary apply
    ```

1. Capture the startup scripts to files that will be used by Packer to build the
   images.

    ```shell
    terraform -chdir=hpc-intel-select/primary output \
      -raw startup_script_startup_controller > \
      hpc-intel-select/packer/controller-image/startup_script.sh

    terraform -chdir=hpc-intel-select/primary output \
      -raw startup_script_startup_compute > \
      hpc-intel-select/packer/compute-image/startup_script.sh
    ```

1. Build the custom Slurm controller image. While this step is executing, you
   may begin the next step in parallel.

    ```shell
    cd hpc-intel-select/packer/controller-image
    packer init .
    packer validate .
    packer build -var startup_script_file=startup_script.sh .
    ```

1. Build the custom Slurm image for login and compute nodes

    ```shell
    cd -
    cd hpc-intel-select/packer/compute-image
    packer init .
    packer validate .
    packer build -var startup_script_file=startup_script.sh .
    ```

1. Provision the Slurm cluster

    ```shell
    cd -
    terraform -chdir=hpc-intel-select/cluster init
    terraform -chdir=hpc-intel-select/cluster validate
    terraform -chdir=hpc-intel-select/cluster apply
    ```

### Connecting to the login node

Once the startup script has completed and Slurm reports readiness, connect to the login node.

1. Open the following URL in a new tab. This will take you to `Compute Engine` >
   `VM instances` in the Google Cloud Console

    ```text
    https://console.cloud.google.com/compute
    ```

    Ensure that you select the project in which you are provisioning the cluster.

1. Click on the `SSH` button associated with the `slurm-hpc-intel-select-login0`
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

The file [daos-cluster.yaml](daos-cluster.yaml) describes an environment with a 4-node DAOS server and a [managed instance group][mig] with two DAOS Clients.

For more information, please refer to the [Google Cloud DAOS repo on GitHub][google-cloud-daos].

> **_NOTE:_** You MUST first create [client and server DAOS images][daos-images] for this example to work.

[mig]: https://cloud.google.com/compute/docs/instance-groups
[google-cloud-daos]: https://github.com/daos-stack/google-cloud-daos
[daos-images]: https://github.com/daos-stack/google-cloud-daos/tree/main/images

### Provisioning the DAOS cluster

Identify a project to work in and substitute its unique id wherever you see
`<<PROJECT_ID>>` in the instructions below.

### Initial Setup for DAOS Cluster

Before provisioning any infrastructure in this project you should follow the
Toolkit guidance to enable [APIs][apis] and establish minimum resource
[quotas][quotas]. In particular, the following APIs should be enabled

* [compute.googleapis.com](https://cloud.google.com/compute/docs/reference/rest/v1#service:-compute.googleapis.com) (Google Compute Engine)
* [secretmanager.googleapis.com](https://cloud.google.com/secret-manager/docs/reference/rest#service:-secretmanager.googleapis.com) (Secret manager, for secure mode)

[apis]: ../../../README.md#enable-gcp-apis
[quotas]: ../../../README.md#gcp-quotas

And the following available quota is required in the region used by the cluster:

* C2 CPUs: 32 (16 per client node)
* N2 CPUs: 144 (36 per server node)
* PD-SSD: 120GB (20GB per client and server)
* Local SSD: 4 \* 16 \* 375 = 24,000GB (6TB per server)

### Deploying the DAOS Cluster

Use `ghpc` to provision the blueprint, supplying your project ID

```text
ghpc create community/examples/intel/daos-cluster.yaml  \
  --vars project_id=<<PROJECT_ID>> \
  [--backend-config bucket=<GCS tf backend bucket>]
```

It will create a set of directories containing Terraform modules and Packer
templates. Notice how you may provide an optional, but recommended, [back-end configuration][backend]. This will save the terraform state in a pre-existing [Google Cloud Storage bucket][bucket].

Follow `ghpc` instructions to deploy the environment

  ```shell
  terraform -chdir=daos-cluster/primary init
  terraform -chdir=daos-cluster/primary validate
  terraform -chdir=daos-cluster/primary apply
  ```

[backend]: ../../../examples/README.md#optional-setting-up-a-remote-terraform-state
[bucket]: https://cloud.google.com/storage/docs/creating-buckets
### Connecting to a client node

1. Open the following URL in a new tab. This will take you to **Compute Engine > VM instances** in the Google Cloud Console

    ```text
    https://console.cloud.google.com/compute
    ```

    Ensure that you select the project in which you are provisioning the cluster.

2. Click on the **SSH** button associated with the **daos-client-0001**
   instance.

   This will open a window with a terminal into our newly created DAOS client VM.

### Verifying the DAOS storage system

The `community/examples/intel/daos-cluster.yaml` blueprint does not contain configuration for DAOS pools and containers. Therefore, pools and containers will need to be created manually.

Before pools and containers can be created the storage system must be formatted. Formatting the storage is done automatically by the startup script that runs on the *daos-server-0001* instance. The startup script will run the [dmg storage format](https://docs.daos.io/v2.0/admin/deployment/?h=dmg+storage#storage-formatting) command. It may take a few minutes for all daos server instances to join.

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

### Creating a DAOS Pool and Container

#### About the DAOS Command Line Tools

The DAOS Management tool `dmg` is used by System Administrators to manange the DAOS storage [system](https://docs.daos.io/v2.0/overview/architecture/#daos-system) and DAOS [pools](https://docs.daos.io/v2.0/overview/storage/#daos-pool). Therefore, `sudo` must be used when running `dmg`.

The DAOS CLI `daos` is used by both users and System Administrators to create and manage [containers](https://docs.daos.io/v2.0/overview/storage/#daos-container). It is not necessary to use `sudo` with the `daos` command.

#### Determining Free Space

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

#### Creating a Pool

Create a single pool owned by root which uses all available free space.

```bash
sudo dmg pool create -z 12.8TB -t 3 -u root --label=pool1
```

Set ACLs to allow any user to create a container in *pool1*.

```bash
sudo dmg pool update-acl -e A::EVERYONE@:rcta pool1
```

See the [Pool Operations](https://docs.daos.io/v2.0/admin/pool_operations) section of the of the DAOS Administration Guide for more information about creating pools.

#### Creating a Container

At this point it is necessary to determine who will need to access the container
and how it will be used. The ACLs will need to be set properly to allow users and/or groups to access the container.

For the purpose of this demo create the container without specifying ACLs. The container will be owned by your user account and you will have full access to the container.

```bash
daos cont create pool1 \
  --label cont1 \
  --type POSIX \
  --properties rf:1
```

See the [Container Management](https://docs.daos.io/v2.0/admin/pool_operations) section of the of the DAOS User Guide for more information about creating containers.

### Mounting the DAOS Container

Create a mount point for the container

```bash
mkdir -p /home/$USER/daos/cont1
```

Mount the container with dfuse (DAOS Fuse)

```bash
dfuse --singlethread \
  --pool=pool1 \
  --container=cont1 \
  --mountpoint=/home/$USER/daos/cont1
```

Verify that the container is mounted

```bash
df -h -t fuse.daos
```

Create a file in the container

```bash
echo "Hello World" > /home/$USER/daos/cont1/hello.txt
```

See the [File System](https://docs.daos.io/v2.0/user/filesystem/) section of the DAOS User Guide for more information about DFuse.

### Unmounting the DAOS Container

The container will need to by unmounted before you log out.  If this is not done it can leave open file handles and prevent the container from being mounted when you log in again.

```bash
fusermount3 -u /home/$USER/daos/cont1
```

Verify that the container is unmounted

```bash
df -h -t fuse.daos
```

See the [DFuse (DAOS FUSE)](https://docs.daos.io/v2.0/user/filesystem/?h=dfuse#dfuse-daos-fuse) section of the DAOS User Guide for more information about mounting POSIX containers.

### Delete the DAOS infrastructure when not in use

> **_NOTE:_** All the DAOS data will be permanently lost after cluster deletion.

Delete the remaining infrastructure

```shell
terraform -chdir=daos-cluster/primary destroy
```

## DAOS Server with Slurm cluster

The file [daos-slurm.yaml](daos-slurm.yaml) describes an environment with a 4-nodes DAOS server and a slurm cluster configured to be able to access this file system.

For more information, please refer to the [Google Cloud DAOS repo on GitHub][google-cloud-daos].

> **_NOTE:_** You MUST first create [client and server DAOS images][daos-images] for this example to work.

[mig]: https://cloud.google.com/compute/docs/instance-groups
[google-cloud-daos]: https://github.com/daos-stack/google-cloud-daos
[daos-images]: https://github.com/daos-stack/google-cloud-daos/tree/main/images

### Provisioning the DAOS/Slurm cluster

Identify a project to work in and substitute its unique id wherever you see
`<<PROJECT_ID>>` in the instructions below.

### Initial Setup for the DAOS/Slurm cluster

Before provisioning any infrastructure in this project you should follow the
Toolkit guidance to enable [APIs][apis] and establish minimum resource
[quotas][quotas]. In particular, the following APIs should be enabled

* [compute.googleapis.com](https://cloud.google.com/compute/docs/reference/rest/v1#service:-compute.googleapis.com) (Google Compute Engine)
* [secretmanager.googleapis.com](https://cloud.google.com/secret-manager/docs/reference/rest#service:-secretmanager.googleapis.com) (Secret manager, for secure mode)

[apis]: ../../../README.md#enable-gcp-apis
[quotas]: ../../../README.md#gcp-quotas

And the following available quota is required in the region used by the cluster:

For DAOS:
* N2 CPUs: 64 (16 per server node)
* PD-SSD: 80GB (20GB per server)
* Local SSD: 4 \* 4 \* 375 = 6,000GB (1.5TB per server)

For Slurm:
* Filestore: 2560GB
* C2 CPUs: 6000 (fully-scaled "compute" partition)
  * This quota is not necessary at initial deployment, but will be required to
    successfully scale the partition to its maximum size
* C2 CPUs: 4 (login node)

### Deploying the DAOS/Slurm Cluster

Use `ghpc` to provision the blueprint, supplying your project ID

```text
ghpc create community/examples/intel/daos-slurm.yaml \
  --vars project_id=<<PROJECT_ID>> \
  [--backend-config bucket=<GCS tf backend bucket>]
```

It will create a set of directories containing Terraform modules and Packer
templates. Please notice how you may provide an optional, but recommended, [back-end configuration][backend]. This will save the terraform state in a pre-existing [Google Cloud Storage bucket][bucket].

Follow `ghpc` instructions to deploy the environment

  ```shell
  terraform -chdir=daos-slurm/primary init
  terraform -chdir=daos-slurm/primary validate
  terraform -chdir=daos-slurm/primary apply
  ```

[backend]: ../../../examples/README.md#optional-setting-up-a-remote-terraform-state
[bucket]: https://cloud.google.com/storage/docs/creating-buckets

### Connecting to the DAOS/Slurm Cluster login node

Once the startup script has completed and Slurm reports readiness, connect to the login node.

1. Open the following URL in a new tab. This will take you to `Compute Engine` >
   `VM instances` in the Google Cloud Console

      ```text
      https://console.cloud.google.com/compute
      ```

    Ensure that you select the project in which you are provisioning the cluster.

1. Click on the `SSH` button associated with the `slurm-daos-slurm-login0`
   instance.

   This will open a separate pop up window with a terminal into our newly created
   Slurm login VM.

### Creating and Mounting a DAOS Container

The `community/examples/intel/daos-slurm.yaml` blueprint contains configuration that will create one DAOS pool named `pool1`.

You will need to create your own DAOS container that can be used by your Slurm jobs.

While logged into the login node create a container named `cont1` in the `pool1` pool:

```bash
daos cont create pool1 \
  --label cont1 \
  --type POSIX \
  --properties rf:0
```

The `cont1` container is owned by your account and therefore your SLURM jobs will need to run with your user account in order to access the container.

Create a mount point for the container and mount it with dfuse (DAOS Fuse)

```bash
mkdir -p /home/$USER/daos/cont1

dfuse --singlethread \
--pool=pool1 \
--container=cont1 \
--mountpoint=/home/$USER/daos/cont1
```

Verify that the container is mounted

```bash
df -h -t fuse.daos
```

### Running a Job that uses the DAOS Container

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

MOUNT_DIR="/home/${USER}/daos/cont1"
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

Run the `daos_job.sh` script in an interactive SLURM job on 4 nodes

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

Wait for the job to complete and then view the files that were created in the `cont1` DAOS container mounted on `/home/${USER}/daos/cont1`.

```bash
ls -l /home/${USER}/daos/cont1/*.log
cat /home/${USER}/daos/cont1/*.log
```

### Unmounting the Container

The container will need to by unmounted before you log out.  If this is not done it can leave open file handles and prevent the container from being mounted when you log in again.

```bash
fusermount3 -u /home/${USER}/daos/cont1
```

Verify that the container is unmounted

```bash
df -h -t fuse.daos
```

See the [DFuse (DAOS FUSE)](https://docs.daos.io/v2.0/user/filesystem/?h=dfuse#dfuse-daos-fuse) section of the DAOS User Guide for more information about mounting POSIX containers.

### Delete the DAOS/Slurm Cluster infrastructure when not in use

> **_NOTE:_** All the DAOS data will be permanently lost after cluster deletion.

<!-- -->

> **_NOTE:_** If the Slurm controller is shut down before the auto-scale nodes
> are destroyed then they will be left running.

Open your browser to the VM instances page and ensure that nodes named "compute"
have been shutdown and deleted by the Slurm autoscaler. Delete the remaining
infrastructure with `terraform`:

```shell
terraform -chdir=daos-cluster/primary destroy
```
