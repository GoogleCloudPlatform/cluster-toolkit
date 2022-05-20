# Intel Solutions for the HPC Toolkit

<!-- TOC generated with: md_toc github community/examples/intel/README.md | sed -e "s/\s-\s/ * /"-->
<!-- TOC -->

* [Intel-Optimized Slurm Cluster](#intel-optimized-slurm-cluster)
  * [Provisioning the Intel-optimized Slurm cluster](#provisioning-the-intel-optimized-slurm-cluster)
  * [Initial Setup for the Intel-Optimized Slurm Cluster](#initial-setup-for-the-intel-optimized-slurm-cluster)
  * [Deploying the Slurm Cluster](#deploying-the-slurm-cluster)
  * [Connecting to the login node](#connecting-to-the-login-node)
  * [Access the cluster and provision an example job](#access-the-cluster-and-provision-an-example-job)
  * [Delete the infrastructure when not in use](#delete-the-infrastructure-when-not-in-use)
* [DAOS Cluster](#daos-cluster)
  * [Provisioning the DAOS cluster](#provisioning-the-daos-cluster)
  * [Initial Setup for DAOS Cluster](#initial-setup-for-daos-cluster)
  * [Deploying the DAOS Cluster](#deploying-the-daos-cluster)
  * [Connecting to a client node](#connecting-to-a-client-node)
  * [Create pools and partitions](#create-pools-and-partitions)
  * [Delete the DAOS infrastructure when not in use](#delete-the-daos-infrastructure-when-not-in-use)
* [DAOS Server with Slurm cluster](#daos-server-with-slurm-cluster)
  * [Provisioning the DAOS/Slurm cluster](#provisioning-the-daosslurm-cluster)
  * [Initial Setup for the DAOS/Slurm cluster](#initial-setup-for-the-daosslurm-cluster)
  * [Deploying the DAOS/Slurm Cluster](#deploying-the-daosslurm-cluster)
  * [Connecting to the DAOS/Slurm Cluster login node](#connecting-to-the-daosslurm-cluster-login-node)
  * [Create DAOS/Slurm Cluster pools and partitions](#create-daosslurm-cluster-pools-and-partitions)
  * [Delete the DAOS/Slurm Cluster infrastructure when not in use](#delete-the-daosslurm-cluster-infrastructure-when-not-in-use)

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
   `VM instances` in the Google Cloud Console:

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

1. Create a default ssh key to be able to ssh between nodes:

    ```shell
    ssh-keygen -q -N '' -f ~/.ssh/id_rsa
    cp ~/.ssh/id_rsa.pub ~/.ssh/authorized_keys
    chmod 0600 ~/.ssh/authorized_keys
    ```

1. Submit an example job:

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

Use `ghpc` to provision the blueprint, supplying your project ID:

```text
ghpc create community/examples/intel/daos-cluster.yaml  \
  --vars project_id=<<PROJECT_ID>> \
  [--backend-config bucket=<GCS tf backend bucket>]
```

It will create a set of directories containing Terraform modules and Packer
templates. Please notice how you may provide an optional, but recommended, [back-end configuration][backend]. This will save the terraform state in a pre-existing [Google Cloud Storage bucket][bucket].

Please follow `ghpc` instructions to deploy the environment:

  ```shell
  terraform -chdir=daos-cluster/primary init
  terraform -chdir=daos-cluster/primary validate
  terraform -chdir=daos-cluster/primary apply
  ```

[backend]: ../../../examples/README.md#optional-setting-up-a-remote-terraform-state
[bucket]: https://cloud.google.com/storage/docs/creating-buckets
### Connecting to a client node

1. Open the following URL in a new tab. This will take you to `Compute Engine` >
   `VM instances` in the Google Cloud Console:

    ```text
    https://console.cloud.google.com/compute
    ```

    Ensure that you select the project in which you are provisioning the cluster.

1. Click on the `SSH` button associated with the `daos-client-0001`
   instance.

   This will open a separate pop up window with a terminal into our newly created
   DAOS client VM.

### Create pools and partitions

In this example, no pool creation is specified, and therefore, DAOS server only automatically issues a [`dmg format`](https://github.com/daos-stack/google-cloud-daos/tree/develop/terraform/examples/daos_cluster#format-storage).

After connecting to the client VM follow the necessary DAOS administration tasks to [create a pool][create-pool], and [a container][create-container] with the appropriate permissions and mount it.

[create-pool]: https://github.com/daos-stack/google-cloud-daos/tree/develop/terraform/examples/daos_cluster#create-a-pool
[create-container]: https://github.com/daos-stack/google-cloud-daos/tree/develop/terraform/examples/daos_cluster#create-a-container

### Delete the DAOS infrastructure when not in use

> **_NOTE:_** All the DAOS data will be permanently lost after cluster deletion.

To delete the remaining infrastructure:

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

Use `ghpc` to provision the blueprint, supplying your project ID:

```text
ghpc create community/examples/intel/daos-slurm.yaml \
  --vars project_id=<<PROJECT_ID>> \
  [--backend-config bucket=<GCS tf backend bucket>]
```

It will create a set of directories containing Terraform modules and Packer
templates. Please notice how you may provide an optional, but recommended, [back-end configuration][backend]. This will save the terraform state in a pre-existing [Google Cloud Storage bucket][bucket].

Please follow `ghpc` instructions to deploy the environment:

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
   `VM instances` in the Google Cloud Console:

      ```text
      https://console.cloud.google.com/compute
      ```

    Ensure that you select the project in which you are provisioning the cluster.

1. Click on the `SSH` button associated with the `slurm-daos-slurm-login0`
   instance.

   This will open a separate pop up window with a terminal into our newly created
   Slurm login VM.

### Create DAOS/Slurm Cluster pools and partitions

In this example, no pool creation is specified, and therefore, DAOS server only automatically issues a [`dmg format`](https://github.com/daos-stack/google-cloud-daos/tree/develop/terraform/examples/daos_cluster#format-storage).

After connecting to the client VM follow the necessary DAOS administration tasks to [create a pool][create-pool], and [a container][create-container] with the appropriate permissions and mount it.

<!--
These are defined above:
[create-pool]: https://github.com/daos-stack/google-cloud-daos/tree/develop/terraform/examples/daos_cluster#create-a-pool
[create-container]: https://github.com/daos-stack/google-cloud-daos/tree/develop/terraform/examples/daos_cluster#create-a-container
-->

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
