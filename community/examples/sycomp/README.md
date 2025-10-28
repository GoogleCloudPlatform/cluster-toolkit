# Sycomp Intelligent Data Storage Platform Blueprint

This document provides instructions on how to use a blueprint to deploy or expand a `Sycomp Intelligent Data Storage Platform` cluster and enable a `SLURM` cluster to access the data via `NFS` on Google Cloud Platform (GCP)
using the Google Cluster Toolkit.

The directory contains four example blueprints that can be used to deploy or expand a `Sycomp Storage` cluster:

- sycomp-storage.yaml

  Blueprint used to deploy a `Sycomp Storage` cluster consisting of 3 storage servers.

- sycomp-storage-expansion.yaml

  Blueprint used to expand a the `Sycomp Storage` cluster to 4 storage servers.

- sycomp-storage-ece.yaml

  Blueprint used to deploy a `Sycomp Storage` cluster consisting of 7 storage servers using `ECE`(Erasure Code Edition) software RAID.

- sycomp-storage-slurm.yaml

  Blueprint used to deploy a `SLURM` cluster and a `Sycomp Storage` cluster with four servers. The `SLURM` compute nodes are configured as `NFS` clients and have the ability to use the Sycomp Storage filesystem.

## Prerequisites

1. Google Cloud SDK is installed and configured.
2. The Google Cloud Cluster Toolkit is set up [Set up Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).
3. You have a Google Cloud project and the required permissions to create VPCs, and Compute Engine instances.
4. The following APIs must be enabled in your project:
   - Compute Engine API (`compute.googleapis.com`)
5. You have an SSH key pair (`~/.ssh/id_rsa` and `~/.ssh/id_rsa.pub` or similar). If you don't have one, you can generate it using `ssh-keygen -t rsa`.
6. You have a valid **Customer Token** provided by Sycomp.
7. You have a valid **Project access token** provided by Sycomp.

> **_NOTE:_** To get a customer and project access token, contact Sycomp (sycompstorage@sycomp.com). <br>
> **_NOTE:_** To avoid repeatedly entering passwords, you can set `credential.helper` in git.

```shell
git config credential.helper cache
```

## Configuration

Before deploying you need to edit the blueprint yaml files and replace the placeholder values.

Required parameter updates for each blueprint:

- **`vars` block:**
  - `project_id`: Your Google Cloud project ID.
  - `deployment_name`: A unique name for this deployment (e.g., `mystorage1`).
  - `region`: The region where you want to deploy the cluster.
  - `zone`: The zone where you want to deploy the cluster.

      **Note**: By default, the value of `deployment_name` is set by `sycomp-storage-gcp` module to its `name_prefix` variable, and `name_prefix` only accepts letters and numbers. If you want `deployment_name` to contain other characters, you need to set `name_prefix` separately for `sycomp-storage-gcp`.

- **`network1` module settings:**
  - `source`: modules/network/vpc is the default and deploys the cluster in a newly created network. To use an existing network, change source to modules/network/pre-existing-vpc.
  - `network_name`: A name for the new VPC network (e.g., `sycomp-net`).
  - `subnetwork_name`: A name for the new subnetwork (e.g., `sycomp-subnet`).
  - `allowed_ssh_ip_ranges`: A list of IP address ranges in CIDR format that
     are allowed to connect via SSH. **You must include the IP address of the
     machine you are running the deployment from.** For example: `["1.2.3.0/24"]`.

- **`sycomp-storage-gcp` module settings:**
  - `security.ssh.ssh_user_name`: The username for SSH access to the management node (e.g., `sycomp`).
  - `security.ssh.private_key`: The file path to your SSH private key (e.g., `~/.ssh/id_rsa`).
  - `security.ssh.public_key`: The file path to your SSH public key (e.g., `~/.ssh/id_rsa.pub`).
  - `security.customer_token.token`: Your Sycomp Customer Token. To getn a customer token, contact Sycomp (sycompstorage@sycomp.com).

- **(Optional) `scale_config` settings:**
  - `scale_node_count`: The number of nodes in the Sycomp Storage cluster. Default is `3`.
  - `scale_volumes`: Configuration for the data disks. Default is 4 disks of 250GiB each per storage node.

## Deployment

Once the blueprint file (e.g., `sycomp-storage.yaml`) is configured, you can deploy blueprint cluster by following these steps from your terminal.

1. **Authenticate with Google Cloud:**

   ```bash
   gcloud auth login
   gcloud auth application-default login
   ```

2. **Create the deployment directory:**

   ```bash
   gcluster create community/examples/sycomp/sycomp-storage.yaml
   ```

   This command will create a new directory named after your `deployment_name` (e.g., `mystorage1`).

3. **Deploy the resources:**

   ```bash
   # terraform will prompt you to enter the username and password.
   # Enter any username and use the project access token obtained from Sycomp as the password.
   # To get a project access token, contact Sycomp (sycompstorage@sycomp.com).
   terraform -chdir=<deployment_name>/primary init # e.g., mystorage1
   gcluster deploy <deployment_name>
   ```

   This process will take several minutes as it provisions the Sycomp Storage cluster nodes.

## Cleanup

To remove all resources created by this blueprint, run the following command from
within the deployment directory:

```bash
gcluster destroy <deployment_name>
```
