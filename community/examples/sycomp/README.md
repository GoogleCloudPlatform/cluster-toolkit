# Sycomp Intelligent Data Storage Platform Blueprint

This document provides instructions on how to use a blueprint to deploy or expand a `Sycomp Intelligent Data Storage Platform` cluster and enable a `SLURM` cluster to access the data via `NFS` on Google Cloud Platform (GCP)
using the Google Cluster Toolkit.

The directory contains four example blueprints that can be used to deploy or expand a `Sycomp Storage` cluster:

- sycomp-storage.yaml

  Blueprint used to deploy a `Sycomp Storage` cluster consisting of 3 storage servers.

- sycomp-storage-expansion.yaml

  Blueprint used to expand the `Sycomp Storage` cluster to 4 storage servers.

- sycomp-storage-ece.yaml

  Blueprint used to deploy a `Sycomp Storage` cluster consisting of 7 storage servers using `ECE` (Erasure Code Edition) software RAID.

- sycomp-storage-slurm.yaml

  Blueprint used to deploy a `SLURM` cluster and a `Sycomp Storage` cluster with three servers. The `SLURM` compute nodes are configured as `NFS` clients and have the ability to use the Sycomp Storage filesystem.

## Prerequisites

1. Google Cloud SDK is installed and configured.
2. The Google Cloud Cluster Toolkit is set up [Set up Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).
3. You have a Google Cloud project and the required permissions to create VPCs, and Compute Engine instances.
4. The following APIs must be enabled in your project:
   - Compute Engine API (`compute.googleapis.com`)
5. You have an SSH key pair (`~/.ssh/id_rsa` and `~/.ssh/id_rsa.pub` or similar). If you don't have one, you can generate it using `ssh-keygen -t rsa`.
6. You have a valid **Customer Token** provided by Sycomp.
7. You have a valid **Project access token** provided by Sycomp.

> **_NOTE:_** To get a customer and project access token, contact Sycomp (sycompstorage@sycomp.com).
>
> **_NOTE:_** To avoid repeatedly entering passwords, you can set `credential.helper` in git.

```shell
git config credential.helper cache
```

## Configuration

Before deploying you need to edit the blueprint yaml files and replace the placeholder values.

Required parameter updates for each blueprint:

- **`vars` block:**
  - `project_id`: Your Google Cloud project ID.
  - `deployment_name`: A unique name for this deployment (e.g., `sycomp-storage`).
  - `region`: The region where you want to deploy the cluster.
  - `zone`: The zone where you want to deploy the cluster.

- **`network1` module settings:**
  - `source`: `modules/network/vpc` is the default and deploys the cluster in a newly created network. To use an existing network, change `source` to `modules/network/pre-existing-vpc`.
  - `network_name`: A name for the new VPC network (e.g., `sycomp-net`).
  - `subnetwork_name`: A name for the new subnetwork (e.g., `sycomp-subnet`).
  - `allowed_ssh_ip_ranges`: A list of IP address ranges in CIDR format that
     are allowed to connect via SSH. **You must include the IP address of the
     machine you are running the deployment from.** For example: `["1.2.3.0/24"]`.

- **`sycomp-scale-gcp` module settings:**
  - `security.ssh.ssh_user_name`: The username for SSH access to the management node (e.g., `sycomp`).
  - `security.ssh.private_key`: The file path to your SSH private key (e.g., `~/.ssh/id_rsa`).
  - `security.ssh.public_key`: The file path to your SSH public key (e.g., `~/.ssh/id_rsa.pub`).
  - `security.customer_token.token`: Your Sycomp Customer Token. To get a customer token, contact Sycomp (`sycompstorage@sycomp.com`).
  - `name_prefix` (Optional): By default, `sycomp-scale-gcp` sets the value of `name_prefix` based on the `deployment_name`. Since `name_prefix` only accepts **alphanumeric** characters (letters and numbers), `sycomp-scale-gcp` automatically **removes any non-alphanumeric characters** (such as hyphens and underscores) before assigning the value. For example, a `deployment_name` of `sycomp-storage-1` would result in a `name_prefix` of `sycompstorage1`.
  - **`scale_config`** (Optional):
    - `scale_node_count`: The number of nodes in the Sycomp Storage cluster. Default is `3`.
    - `scale_volumes`: Configuration for the data disks. Default is 4 disks of 250GiB each per storage node.

- **`sycomp-scale-expansion` module settings:**
  - `name_prefix`: Must match the `name_prefix` of the cluster to be expanded. Note: if not set explicitly, `name_prefix` is derived from `deployment_name` by removing non-alphanumeric characters. You can find the correct value for an existing cluster by checking its Terraform outputs or by inspecting resource names in the Google Cloud Console.
  - `add_scale_nodes`: The number of nodes to be added.

## Deployment

Once the blueprint file (e.g., `sycomp-storage.yaml`) is configured, you can deploy the cluster from the blueprint by following these steps from your terminal.

1. **Authenticate with Google Cloud:**

   ```bash
   gcloud auth login
   gcloud auth application-default login
   ```

2. **Create the deployment directory:**

   ```bash
   # Replace <blueprint-filename> with the specific blueprint filename (e.g., sycomp-storage.yaml).
   gcluster create community/examples/sycomp/<blueprint-filename>
   ```

   This command will create a new directory named after your `deployment_name` (e.g., `sycomp-storage`).

3. **Deploy the resources:**

   ```bash
   # The terraform init command will prompt for a username and password.
   # Enter any username and use the project access token from Sycomp as the password.
   # To get a project access token, contact Sycomp (sycompstorage@sycomp.com).
   #
   # Replace <deployment_name> with the `deployment_name` from your blueprint (e.g., "sycomp-storage").
   terraform -chdir=<deployment_name>/primary init
   gcluster deploy <deployment_name>
   ```

   This process will take several minutes as it provisions the Sycomp Storage cluster nodes.

## Cleanup

To remove all resources created by this blueprint, run the following command:

```bash
gcluster destroy <deployment_name>
```
