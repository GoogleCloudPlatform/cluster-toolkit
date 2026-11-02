# Deploying with Accelerators in the Cluster Toolkit

## Supported modules

* [vm-instance] and therefore any module that relies on `vm-instance` including:
  * HTCondor modules including [htcondor-install], [htcondor-setup] and
    [htcondor-execute-point].
* Slurm on GCP modules version 6
  * `schedmd-slurm-gcp-v6-*`
* Cloud Batch modules through custom instance templates

## Accelerator definition automation

The [nodeset] (`schedmd-slurm-gcp-v6-nodeset`), [vm-instance] and [gke-node-pool]
modules support automation for defining the `guest_accelerator` config via the
shared internal `modules/internal/gpu-definition` module and `machine_configs`
injection ([controller], [login] and [htcondor-execute-point] do not receive
`machine_configs`, so set `guest_accelerator` explicitly on them). If the
user supplies an explicit value for `guest_accelerator`, the automation is
bypassed.

The `modules/internal/gpu-definition` module looks up `machine_configs.gpus[var.machine_type]`
(populated by `gcluster` at expand/create time from the Compute Engine `machineTypes.get` API)
and automatically attaches the default GPU type and count for any machine type that
bundles accelerators. For example:
* `a2-highgpu-4g` -> `type: nvidia-tesla-a100`, `count: 4`
* `a2-ultragpu-8g` -> `type: nvidia-a100-80gb`, `count: 8`
* `g2-standard-4` -> `type: nvidia-l4`, `count: 1`

Machine type `n1` can also have guest accelerators attached (`nvidia-tesla-t4`, `nvidia-tesla-v100`, etc.), which must be specified explicitly via `guest_accelerator`.

[vm-instance]: ../modules/compute/vm-instance/README.md
[gke-node-pool]: ../modules/compute/gke-node-pool/README.md
[nodeset]: ../community/modules/compute/schedmd-slurm-gcp-v6-nodeset/README.md
[controller]: ../community/modules/scheduler/schedmd-slurm-gcp-v6-controller/README.md
[login]: ../community/modules/scheduler/schedmd-slurm-gcp-v6-login/README.md
[htcondor-install]: ../community/modules/scripts/htcondor-install/README.md
[htcondor-setup]: ../community/modules/scheduler/htcondor-setup/README.md
[htcondor-execute-point]: ../community/modules/compute/htcondor-execute-point/README.md

## Troubleshooting and tips

* To list accelerator types and availability by region, run
  `gcloud compute accelerator-types list`. The information is also available in
  the Google Cloud documentation [here](https://cloud.google.com/compute/docs/gpus/gpu-regions-zones).
* Deployment time of VMs with many guest accelerators can take longer. See the
  [Timeouts when deploying a compute VM](#timeouts-when-deploying-a-compute-vm)
  section below if you experience timeouts because of this.

### Slurm on GCP

When deploying a Slurm cluster with GPUs, use the Slurm on GCP v6 modules (`schedmd-slurm-gcp-v6-*`). See [`examples/machine-learning/`](../examples/machine-learning/README.md) for production-ready A3 High, A3 Mega, A3 Ultra, A4, A4X, A4X Max, and TPU 7x Slurm blueprints.

#### Interface Considerations

The Slurm on GCP v6 Cluster Toolkit modules (`schedmd-slurm-gcp-v6-*`) and `vm-instance` use the singular `guest_accelerator` variable (a list of `{ type = string, count = number }` objects) to define attached GPUs when overriding the automatic `machine_type` mapping.

#### Timeouts when deploying a compute VM

As mentioned above, VMs with many guest accelerators can take longer to deploy.
Slurm sets timeouts for creating VMs, and it's possible for high GPU
configurations to push past the default timeout. We recommend using the Slurm on
GCP v6 modules.

The v6 Toolkit modules (`schedmd-slurm-gcp-v6-*`) allow Slurm configuration
timeouts to customized via the [cloud_parameters] variable on the [controller].
See the example below which increases the `resume_timeout` from the default of
300s to 600s:

```yaml
- id: slurm_controller
  source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
  use: [network1]
  settings:
    cloud_parameters:
      resume_rate: 0
      resume_timeout: 600  # Update this value, default is 300
      suspend_rate: 0
      suspend_timeout: 300
      no_comma_params: false
```

#### Launching Slurm jobs with GPUs

In order to utilize the GPUs in the compute VMs deployed by Slurm, the GPU
count must be specified when submitting a job with `srun` or `sbatch`. For
instance, the following `srun` command launches a job that runs nvidia-smi in a
partition called `gpu_partition` (`-p gpu_partition`) on a full node (`-N 1`)
with 8 GPUs (`--gpus 8`):

```shell
srun -N 1 -p gpu_partition --gpus 8 nvidia-smi
```

An equivalent `sbatch` script:

```bash
#!/bin/bash

#SBATCH --nodes=1
#SBATCH --partition=gpu_partition
#SBATCH --gpus=8

nvidia-smi
```

Both commands support further customization of GPU resources. For more
information, see the SchedMD documentation:
* [Generic Resource Scheduling](https://slurm.schedmd.com/gres.html#Running_Jobs)
* [srun Documentation](https://slurm.schedmd.com/srun.html)
* [sbatch Documentation](https://slurm.schedmd.com/sbatch.html)

[slurm-gcp]: https://github.com/GoogleCloudPlatform/slurm-gcp
[cloud_parameters]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/scheduler/schedmd-slurm-gcp-v6-controller#input_cloud_parameters

## Further Reading

* [Cloud GPU Documentation](https://cloud.google.com/compute/docs/gpus/)
* [GPU Information](https://cloud.google.com/compute/docs/gpus/about-gpus): More
  generalized information about GPUs in the Google Cloud Platform.
* [GPU Region and Zone Availability](https://cloud.google.com/compute/docs/gpus/gpu-regions-zones)
* [GPU Pricing](https://cloud.google.com/compute/gpus-pricing)
