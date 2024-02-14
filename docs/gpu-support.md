# Deploying with Accelerators in the Cloud HPC Toolkit

## Supported modules

* [vm-instance] and therefore any module that relies on `vm-instance` including:
  * HTCondor modules including [htcondor-install], [htcondor-setup] and
    [htcondor-execute-point].
  * [omnia-install]
* Slurm on GCP modules where applicable, both version 5 and version 6
  * `schedmd-slurm-gcp-v5-*`
  * `schedmd-slurm-gcp-v6-*`
* PBS Pro modules (`pbspro-*`)
* Cloud Batch modules through custom instance templates

## Accelerator definition automation

The schedmd-slurm-gcp-v5 modules ([node-group], [controller] and [login]),
the [vm-instance] module and any module relying on [vm-instance] (HTCondor,
Omnia, PBS Pro) support
automation for defining the `guest_accelerator` config. If the user supplies any
value for this setting, the automation will be bypassed.

The automation is handled primary in the `gpu_definition.tf` files in these
modules. This file assumes the existence of two input variables in the module:

* `guest_accelerator`: A list of terraform objects with the attributes type and
  count.
* `machine_type`: Defines the machine type of the VM being created.

`gpu_definition.tf` works by checking the `machine_type` and associating it with
a GPU type and extracting the GPU count. For example, consider the following
machine types:
* `a2-high-gpu-4g`
  * `type` will be set to `nvidia-tesla-a100`
  * `count` will be set to 4
* `a2-ultragpu-8g`
  * `type` will be set to `nvidia-a100-80gb`
  * `count` will be set to 8.

This automation currently only supports machine type `a2`. Machine type `n1` can
also have guest accelerators attached, however the type and count
cannot be determined automatically like with `a2`.

[vm-instance]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules/compute/vm-instance
[node-group]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/compute/schedmd-slurm-gcp-v5-node-group
[controller]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/scheduler/schedmd-slurm-gcp-v5-controller
[login]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/scheduler/schedmd-slurm-gcp-v5-login
[omnia-install]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/scripts/omnia-install
[htcondor-install]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/scripts/htcondor-install
[htcondor-setup]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/scheduler/htcondor-setup
[htcondor-execute-point]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/compute/htcondor-execute-point

## Troubleshooting and tips

* To list accelerator types and availability by region, run
  `gcloud compute accelerator-types list`. The information is also available in
  the Google Cloud documentation [here](https://cloud.google.com/compute/docs/gpus/gpu-regions-zones).
* Deployment time of VMs with many guest accelerators can take longer. See the
  [Timeouts when deploying a compute VM](#timeouts-when-deploying-a-compute-vm)
  section below if you experience timeouts because of this.

### Slurm on GCP

When deploying a Slurm cluster with GPUs, we highly recommend using the
modules based on Slurm on GCP version 5 (`schedmd-slurm-gcp-v5-*`). The
interface is more consistent with HPC Toolkit standards and more functionality
is available to support, debug and workaround any issues related to GPU
resources.

#### Interface Considerations

The Slurm on GCP v5 HPC Toolkit modules (`schedmd-slurm-gcp-v5-*`) have two
variables that can be used to define attached GPUs. The variable
`guest_accelerators` is the recommended option as it is consistent with other
modules in the HPC Toolkit. The setting `gpus` can be set as well, which
provides consistency with the underlying terraform modules from the
[Slurm on GCP repo][slurm-gcp].

#### Timeouts when deploying a compute VM

As mentioned above, VMs with many guest accelerators can take longer to deploy.
Slurm sets timeouts for creating VMs, and it's possible for high GPU
configurations to push past the default timeout. We recommend using the Slurm on
GCP v5 modules.

The v5 Toolkit modules (`schedmd-slurm-gcp-v5-*`) allow Slurm configuration
timeouts to customized via the [cloud_parameters] variable on the [controller].
See the example below which increases the `resume_timeout` from the default of
300s to 600s:

```yaml
- id: slurm_controller
  source: community/modules/scheduler/schedmd-slurm-gcp-v5-controller
  use: [...]
  settings:
    cloud_parameters:
      resume_rate: 0
      resume_timeout: 600  # Update this value, default is 300
      suspend_rate: 0
      suspend_timeout: 300
      no_comma_params: false
    ...
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
[cloud_parameters]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/scheduler/schedmd-slurm-gcp-v5-controller#input_cloud_parameters

## Further Reading

* [Cloud GPU Documentation](https://cloud.google.com/compute/docs/gpus/)
* [GPU Information](https://cloud.google.com/compute/docs/gpus/about-gpus): More
  generalized information about GPUs in the Google Cloud Platform.
* [GPU Region and Zone Availability](https://cloud.google.com/compute/docs/gpus/gpu-regions-zones)
* [GPU Pricing](https://cloud.google.com/compute/gpus-pricing)
