# TPU 7x Blueprints

## TPU 7x (`tpu7x-standard-4t`) Slurm Cluster Deployment

This directory provides two Cluster Toolkit blueprints and a deployment configuration ([`tpu7x-slurm-deployment.yaml`](tpu7x-slurm-deployment.yaml)) for provisioning a Slurm 26.05 cluster with GCE-native TPU 7x (`tpu7x-standard-4t`) compute nodes, comprising both a **static** TPU partition (`tpu`) and an on-demand **dynamic** TPU partition (`tpu-dyn`):

1. **[`tpu7x-slurm-blueprint.yaml`](tpu7x-slurm-blueprint.yaml)**: Uses a pre-built Slurm 26.05 TPU image (`aci-tpu-u2404-slurm-2605-amd64`) and Google Cloud Managed Lustre mounted at `/home` (`cluster-env` and `cluster` groups).
2. **[`tpu7x-slurm-packer-blueprint.yaml`](tpu7x-slurm-packer-blueprint.yaml)**: Builds a custom Slurm 26.05 TPU image (`slurm-tpu-v7x-ubuntu2404`) from `ubuntu-accel-2404-amd64-tpu-tpu7x` using Packer (`image-env` and `image` groups) and deploys the cluster (`cluster-env` and `cluster` groups) with Google Cloud Filestore mounted at `/home`.

Selective deployment and teardown for multi-group blueprints (`image-env`, `image`, `cluster-env`, and `cluster` groups) are documented centrally. See [examples/machine-learning/README.md](../README.md) for full details.

### Build the Cluster Toolkit `gcluster` binary

Follow the instructions [here](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment) to set up your Cluster Toolkit environment, including enabling required APIs and IAM permissions.

### Configure the deployment file

Edit [`tpu7x-slurm-deployment.yaml`](tpu7x-slurm-deployment.yaml) with your Terraform state bucket name, project, region, zone, reservation, and TPU topology settings:

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <TF_STATE_BUCKET_NAME>

vars:
  deployment_name: tpu7x-slurm
  project_id: <PROJECT_ID>
  region: <REGION>
  zone: <ZONE>
  tpu_cluster_size: 16
  tpu_accelerator_topology: 4x4x4
  tpu_dynamic_max_nodes: 16
  tpu_reservation_name: <RESERVATION_NAME>
```

> **Note:**
>
> - If the GCS bucket specified in `terraform_backend_defaults.configuration.bucket` does not exist yet, `./gcluster deploy` (or `./gcluster create`) will create it automatically in your `project_id` and `region` (prompting for confirmation unless `--auto-approve` is passed).
> - Each `tpu7x-standard-4t` VM provides 4 physical TPU chips (`tpus_per_node = 4`, exposing 8 TensorCores), so for the static partition (`tpu`), the total chips in `tpu_accelerator_topology` divided by 4 must match `tpu_cluster_size` (for example, `2x2x1` = 4 chips = 1 VM; `2x2x2` = 8 chips = 2 VMs; `2x4x4` = 32 chips = 8 VMs; `4x4x4` = 64 chips = 16 VMs).

### Additional ways to provision

Cluster Toolkit also supports DWS Flex-Start and Spot VMs in addition to reservations:

- [For more information on DWS Flex-Start in Slurm](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/docs/slurm-dws-flex.md)
- [For more information on Spot VMs](https://cloud.google.com/compute/docs/instances/spot)

To use one of these alternative models, modify the `vars` section in `tpu7x-slurm-deployment.yaml` and replace `tpu_reservation_name` with one of the following:

- `tpu_enable_spot_vm: true` (for Spot VMs)
- `tpu_dws_flex_enabled: true` (for DWS Flex-Start)

### Deploy the Slurm Cluster

#### Option 1: Using the Pre-Built Image (`tpu7x-slurm-blueprint.yaml`)

```bash
./gcluster deploy \
  -d examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-deployment.yaml \
  examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-blueprint.yaml \
  --auto-approve
```

#### Option 2: Building a Custom Image with Packer (`tpu7x-slurm-packer-blueprint.yaml`)

```bash
./gcluster deploy \
  -d examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-deployment.yaml \
  examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-packer-blueprint.yaml \
  --auto-approve
```

If you have already built the custom Slurm image (`slurm-tpu-v7x-ubuntu2404`) in your project, you can deploy only the `cluster-env` and `cluster` groups by skipping `image-env` and `image`:

```bash
./gcluster deploy \
  -d examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-deployment.yaml \
  examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-packer-blueprint.yaml \
  --only cluster-env,cluster \
  --auto-approve
```

### Running TPU Workloads

Once logged into the Slurm login node (`<deployment_name>-slurm-login-001`), you can run jobs on either the **static** partition (`-p tpu`, always-warm slice matching `tpu_accelerator_topology`) or the **dynamic** partition (`-p tpu-dyn`, which provisions an on-demand slice matching your `--layout` flag and powers down when idle):

```bash
# 1. Create a shared JAX TPU virtualenv on /home:
python3 -m venv ~/jax-env
~/jax-env/bin/pip install -U pip "jax[tpu]" -f https://storage.googleapis.com/jax-releases/libtpu_releases.html

# 2. Run a distributed JAX job across the static TPU slice (e.g., 8 nodes = 2x4x4):
srun -N 8 -p tpu --gres=tpu:tpu7x:4 --layout=tpu7x=2x4x4 ~/jax-env/bin/python3 -c \
  "import jax; jax.distributed.initialize(); print(f'Rank {jax.process_index()}: {len(jax.devices())} total TPU cores ({len(jax.local_devices())} local)')"

# 3. Run an on-demand job on the dynamic TPU partition (e.g., 2 nodes = 2x2x2):
srun -N 2 -p tpu-dyn --gres=tpu:tpu7x:4 --layout=tpu7x=2x2x2 ~/jax-env/bin/python3 -c \
  "import jax; jax.distributed.initialize(); print(f'Rank {jax.process_index()}: {len(jax.devices())} total TPU cores ({len(jax.local_devices())} local)')"
```

## Clean Up

To destroy all resources created by the blueprint, run:

```bash
./gcluster destroy <DEPLOYMENT_NAME>
```

Replace `<DEPLOYMENT_NAME>` with the `deployment_name` specified in your deployment file.

**Note:** GCS buckets created for Terraform state are not deleted by the `./gcluster destroy` command and must be deleted manually.
