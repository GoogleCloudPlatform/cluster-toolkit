# TPU 7x Blueprints

## TPU 7x (`tpu7x-standard-4t`) Slurm Cluster Deployment

This directory provides a Cluster Toolkit blueprint ([`tpu7x-slurm-blueprint.yaml`](tpu7x-slurm-blueprint.yaml)) and deployment configuration ([`tpu7x-slurm-deployment.yaml`](tpu7x-slurm-deployment.yaml)) for provisioning a Slurm cluster with TPU 7x (`tpu7x-standard-4t`) compute nodes. It sets up a **static** TPU partition (`tpu`), an on-demand **dynamic** TPU partition (`tpu-dyn`), and a Google Cloud Managed Lustre instance mounted at `/home`.

Selective deployment and teardown for multi-group blueprints (`cluster-env` and `cluster` groups) are documented centrally. See [examples/machine-learning/README.md](../README.md) for full details.

### Build the Cluster Toolkit `gcluster` binary

Follow the instructions [here](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment) to set up your Cluster Toolkit environment, including enabling required APIs and IAM permissions.

### Managed Lustre Capacity

This blueprint provisions a Google Cloud Managed Lustre instance mounted at `/home` across the cluster. By default, it is configured with `36000` GiB (`36` TiB) capacity and `500` MBps/TiB throughput.

- Managed Lustre is supported in specific regions and zones. See [Managed Lustre supported locations](https://cloud.google.com/managed-lustre/docs/locations).
- Storage capacity (`lustre_size_gib`) and throughput (`per_unit_storage_throughput`) are correlated. See [Managed Lustre performance tiers](https://cloud.google.com/managed-lustre/docs/create-instance#performance-tiers) if you want to adjust the size in [`tpu7x-slurm-blueprint.yaml`](tpu7x-slurm-blueprint.yaml).

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
  tpu_static_nodes: 8
  tpu_dynamic_max_nodes: 4
  tpu_accelerator_topology: 2x4x4
  tpu_reservation_name: <RESERVATION_NAME>
```

> **Note:**
>
> - If the GCS bucket specified in `terraform_backend_defaults.configuration.bucket` does not exist yet, `./gcluster deploy` (or `./gcluster create`) will create it automatically in your `project_id` and `region` (prompting for confirmation unless `--auto-approve` is passed).
> - Each `tpu7x-standard-4t` VM has 4 TPU chips (8 cores). For the static partition (`tpu`), `tpu_accelerator_topology` sets the 3D chip topology per slice (for example, `2x2x1` = 1 VM, `2x2x2` = 2 VMs, `2x2x4` = 4 VMs, `2x4x4` = 8 VMs, `4x4x4` = 16 VMs), and `tpu_static_nodes` must be a multiple of the number of VMs per slice (for example, `tpu_static_nodes: 8` with `2x2x4` provisions **two** 4-VM `2x2x4` slices, whereas with `2x4x4` it provisions **one** 8-VM `2x4x4` slice).

### Additional ways to provision

Cluster Toolkit also supports DWS Flex-Start and Spot VMs in addition to reservations:

- [For more information on DWS Flex-Start in Slurm](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/docs/slurm-dws-flex.md)
- [For more information on Spot VMs](https://cloud.google.com/compute/docs/instances/spot)

To use one of these alternative models, modify the `vars` section in `tpu7x-slurm-deployment.yaml` and replace `tpu_reservation_name` with one of the following:

- `tpu_enable_spot_vm: true` (for Spot VMs)
- `tpu_dws_flex_enabled: true` (for DWS Flex-Start)

### Deploy the Slurm Cluster

```bash
./gcluster deploy \
  -d examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-deployment.yaml \
  examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-blueprint.yaml \
  --auto-approve
```

To re-deploy only the `cluster` group while keeping `cluster-env` (networking and Managed Lustre) intact:

```bash
./gcluster deploy \
  -d examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-deployment.yaml \
  examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-blueprint.yaml \
  --only cluster \
  --auto-approve -w
```

### Running TPU Workloads

Once logged into the Slurm login node (`<deployment_name>-slurm-login-001`), you can run jobs on either the **static** partition (`-p tpu`) or the **dynamic** partition (`-p tpu-dyn`) using `--layout=tpu7x=<topology>`:

- **Static partition (`-p tpu`):** When `--layout` is specified, it must match the configured `tpu_accelerator_topology` (and `-N` must match the slice VM count). If `--layout` is omitted, Slurm schedules the job as a standard non-accelerator job.
- **Dynamic partition (`-p tpu-dyn`):** `--layout` is required so Slurm can provision a TPU slice with the requested topology on demand. Jobs missing `--layout` or specifying an invalid topology or mismatched node count are rejected.

```bash
# 1. Create a shared JAX TPU virtualenv on /home:
sudo mkdir -p "$HOME" && sudo chown "$(id -u):$(id -g)" "$HOME"
python3 -m venv ~/jax-env
~/jax-env/bin/pip install -U pip "jax[tpu]" -f https://storage.googleapis.com/jax-releases/libtpu_releases.html

# 2. Run a JAX job on the static TPU partition (must match tpu_accelerator_topology, e.g., 8 nodes = 2x4x4):
srun -N 8 -p tpu --layout=tpu7x=2x4x4 ~/jax-env/bin/python -c \
  "import jax; jax.distributed.initialize(); print(f'Host {jax.process_index()}: total={jax.device_count()}, local={jax.local_device_count()}')"

# 3. Run an on-demand job on the dynamic TPU partition (e.g., 2 nodes = 2x2x2):
srun -N 2 -p tpu-dyn --layout=tpu7x=2x2x2 ~/jax-env/bin/python -c \
  "import jax; jax.distributed.initialize(); print(f'Host {jax.process_index()}: total={jax.device_count()}, local={jax.local_device_count()}')"
```

## Clean Up

To destroy all resources created by the blueprint, run:

```bash
./gcluster destroy <DEPLOYMENT_NAME>
```

Replace `<DEPLOYMENT_NAME>` with the `deployment_name` specified in your deployment file.

**Note:** GCS buckets created for Terraform state are not deleted by the `./gcluster destroy` command and must be deleted manually.
