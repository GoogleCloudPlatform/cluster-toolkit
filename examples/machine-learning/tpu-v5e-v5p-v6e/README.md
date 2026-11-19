# TPU v5e, v5p, and v6e Blueprints

This directory provides a Cluster Toolkit blueprint ([`tpu-slurm-blueprint.yaml`](tpu-slurm-blueprint.yaml)) and deployment configuration ([`tpu-slurm-deployment.yaml`](tpu-slurm-deployment.yaml)) for provisioning a Slurm cluster with TPU (`ct6e-standard-4t`, `ct5p-hightpu-4t`, `ct5lp-hightpu-4t`) compute nodes. It sets up a static TPU partition (`tpu`), an on-demand dynamic TPU partition (`tpu-dyn`), and a Google Cloud Managed Lustre instance mounted at `/home`.

Selective deployment and teardown for multi-group blueprints (`cluster-env` and `cluster` groups) are documented centrally. See [examples/machine-learning/README.md](../README.md) for full details.

## Supported TPU Machine Types

Set `tpu_machine_type` in [`tpu-slurm-deployment.yaml`](tpu-slurm-deployment.yaml) to select the TPU generation for your cluster. Each `*-4t` TPU VM provides 4 physical TPU chips:

| TPU Generation | `tpu_machine_type` | Slurm `--layout` Type | Topology Format | Example Slice Topologies (`1` / `2` / `4` / `8` VMs) |
| :--- | :--- | :--- | :--- | :--- |
| **TPU v6e** | `ct6e-standard-4t` | `v6e` | 2D (`AxB`) | `2x2` / `2x4` / `4x4` / `4x8` |
| **TPU v5p** | `ct5p-hightpu-4t` | `v5p` | 3D (`AxBxC`) | `2x2x1` / `2x2x2` / `2x2x4` / `2x4x4` |
| **TPU v5e** | `ct5lp-hightpu-4t` | `v5e` | 2D (`AxB`) | `2x2` / `2x4` / `4x4` / `4x8` |

## Prerequisites & Storage Setup

### Build the Cluster Toolkit `gcluster` binary

Follow the instructions [here](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment) to set up your Cluster Toolkit environment, including enabling required APIs and IAM permissions.

### Managed Lustre Capacity

This blueprint provisions a Google Cloud Managed Lustre instance mounted at `/home` across the cluster. By default, it is configured with `36000` GiB (`36` TiB) capacity and `500` MBps/TiB throughput.

- Managed Lustre is supported in specific regions and zones. See [Managed Lustre supported locations](https://cloud.google.com/managed-lustre/docs/locations).
- Storage capacity (`lustre_size_gib`) and throughput (`per_unit_storage_throughput`) are correlated. See [Managed Lustre performance tiers](https://cloud.google.com/managed-lustre/docs/create-instance#performance-tiers) if you want to adjust the size in [`tpu-slurm-blueprint.yaml`](tpu-slurm-blueprint.yaml).

## Slurm Cluster Deployment

### Configure the deployment file

Update [`tpu-slurm-deployment.yaml`](tpu-slurm-deployment.yaml) with your environment and TPU configuration:

```yaml
---
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: <TF_STATE_BUCKET_NAME>

vars:
  deployment_name: tpu-slurm
  project_id: <PROJECT_ID>
  region: <REGION>
  zone: <ZONE>

  # TPU machine type to provision (choose one based on your TPU generation):
  #   TPU v6e : ct6e-standard-4t
  #   TPU v5p : ct5p-hightpu-4t
  #   TPU v5e : ct5lp-hightpu-4t
  tpu_machine_type: ct6e-standard-4t

  tpu_static_nodes: 4
  tpu_dynamic_max_nodes: 4
  tpu_accelerator_topology: 4x4
  tpu_reservation_name: <RESERVATION_NAME>
```

Key variables to configure:

- `bucket`: Name of the GCS bucket used to store Terraform state. If the bucket does not exist yet, `./gcluster deploy` (or `./gcluster create`) will create it automatically in your `project_id` and `region` (prompting for confirmation unless `--auto-approve` is passed).
- `deployment_name`: Unique name for this cluster deployment (e.g., `tpu-slurm`).
- `project_id`, `region`, `zone`: Target Google Cloud project, region, and zone with TPU capacity.
- `tpu_machine_type`: TPU VM machine type (`ct6e-standard-4t`, `ct5p-hightpu-4t`, or `ct5lp-hightpu-4t`).
- `tpu_accelerator_topology`: Chip topology per static TPU slice (e.g., `4x4` for a 4-VM v6e/v5e slice, or `2x2x4` for a 4-VM v5p slice; defaults to a 1-node topology). The number of VMs per slice is `total_chips / 4`.
- `tpu_static_nodes`: Total number of static TPU VM nodes in the `tpu` partition. Must be a multiple of the number of VMs per slice (for example, `tpu_static_nodes: 8` with `4x4` or `2x2x4` provisions two 4-VM slices, whereas with `4x8` or `2x4x4` it provisions one 8-VM slice).
- `tpu_dynamic_max_nodes`: Maximum number of on-demand TPU VM nodes in the `tpu-dyn` partition.
- `tpu_reservation_name`: Name of your TPU reservation when provisioning reserved capacity.

### Additional ways to provision

Cluster Toolkit also supports DWS Flex-Start and Spot VMs in addition to reservations:

- [For more information on DWS Flex-Start in Slurm](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/docs/slurm-dws-flex.md)
- [For more information on Spot VMs](https://cloud.google.com/compute/docs/instances/spot)

To use one of these alternative provisioning models, modify the `vars` section in [`tpu-slurm-deployment.yaml`](tpu-slurm-deployment.yaml) and replace `tpu_reservation_name` with one of the following:

- `tpu_enable_spot_vm: true` (for Spot VMs)
- `tpu_dws_flex_enabled: true` (for DWS Flex-Start)

### Deploy the cluster

Run `./gcluster deploy` from the root of the Cluster Toolkit repository:

```bash
./gcluster deploy \
  -d examples/machine-learning/tpu-v5e-v5p-v6e/tpu-slurm-deployment.yaml \
  examples/machine-learning/tpu-v5e-v5p-v6e/tpu-slurm-blueprint.yaml \
  --auto-approve
```

### Re-deploy the cluster

To re-deploy only the `cluster` group while keeping `cluster-env` (VPC networking and Managed Lustre) intact:

```bash
./gcluster deploy \
  -d examples/machine-learning/tpu-v5e-v5p-v6e/tpu-slurm-deployment.yaml \
  examples/machine-learning/tpu-v5e-v5p-v6e/tpu-slurm-blueprint.yaml \
  --only cluster \
  --auto-approve -w
```

## Running TPU Workloads

SSH into the Slurm login node (`<deployment_name>-slurm-login-001`) to submit jobs to either the **static** partition (`-p tpu`) or the **dynamic** partition (`-p tpu-dyn`) using `--layout=<tpu_type>=<topology>` (`v6e`, `v5p`, or `v5e`):

- **Static partition (`-p tpu`):** When `--layout` is specified, it must match the configured static slice topology and `-N` must match the slice VM count. If `--layout` is omitted, Slurm schedules the job as a standard non-accelerator job.
- **Dynamic partition (`-p tpu-dyn`):** `--layout` is required so Slurm can provision a TPU slice with the requested topology on demand. Jobs missing `--layout` or specifying an invalid topology or mismatched node count are rejected.

```bash
# 1. Create a shared JAX TPU virtualenv on /home:
sudo mkdir -p "$HOME" && sudo chown "$(id -u):$(id -g)" "$HOME"
python3 -m venv ~/jax-env
~/jax-env/bin/pip install -U pip "jax[tpu]" -f https://storage.googleapis.com/jax-releases/libtpu_releases.html

# 2. Run a distributed JAX job on the static TPU partition
#    (use --layout=v6e=4x4, --layout=v5e=4x4, or --layout=v5p=2x2x4 to match your cluster):
srun -N 4 -p tpu --layout=v6e=4x4 ~/jax-env/bin/python -c \
  "import jax; jax.distributed.initialize(); print(f'Host {jax.process_index()}: total={jax.device_count()}, local={jax.local_device_count()}')"

# 3. Run an on-demand job on the dynamic TPU partition
#    (e.g. 2 nodes: --layout=v6e=2x4, --layout=v5e=2x4, or --layout=v5p=2x2x2):
srun -N 2 -p tpu-dyn --layout=v6e=2x4 ~/jax-env/bin/python -c \
  "import jax; jax.distributed.initialize(); print(f'Host {jax.process_index()}: total={jax.device_count()}, local={jax.local_device_count()}')"
```

## Clean Up

To destroy all resources created by the blueprint, run:

```bash
./gcluster destroy <DEPLOYMENT_NAME> --auto-approve
```

Replace `<DEPLOYMENT_NAME>` with the `deployment_name` specified in your deployment file.

**Note:** GCS buckets created for Terraform state are not deleted by the `./gcluster destroy` command and must be deleted manually.
