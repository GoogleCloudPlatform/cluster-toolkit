# Deploying with Accelerators in the Cloud HPC Toolkit

## Supported modules

* [vm-instance] and therefore any module that relies on `vm-instance` including
  * HTCondor modules including [htcondor-install], [htcondor-configure] and
    [htcondor-execute-point].
  * [omnia-install]
* Slurm on GCP modules where applicable, both version 4 and version 5
  * `schedmd-slurm-gcp-v5-*`
  * `SchedMD-slurm-on-gcp-*`
* PBS Pro modules (`pbspro-*`)

## Accelerator definition automation

The schedmd-slurm-gcp-v5 modules ([node-group], [controller] and [login]),
the [vm-instance] module and any module relying on [vm-instance] support
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
[htcondor-configure]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/scheduler/htcondor-configure
[htcondor-execute-point]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/compute/htcondor-execute-point

## Troubleshooting and tips

* To list accelerator types and availability by region, run
  `gcloud compute accelerator-types list`. The information is also available in
  the Google Cloud documentation [here](https://cloud.google.com/compute/docs/gpus/gpu-regions-zones).
* Deployment time of VMs with many guest accelerators can take longer.

### Slurm on GCP

* When deploying a Slurm cluster with GPUs, we highly recommend using the
  modules based on Slurm on GCP version 5 (`schedmd-slurm-gcp-v5-*`). The
  interface is more consistent with HPC Toolkit standards and more functionality
  is available to support, debug and workaround any issues related to GPU
  resources.
* `SchedMD-slurm-on-gcp-*` modules have a different interface for defining
  attached accelerators, `gpu_type` and `gpu_count`. These must be set even if
  the machine type implies GPUs.
* Timeouts when launching compute nodes
  * Some GPUs will fail in Slurm on GCP v4 HPC Toolkit modules
    (`SchedMD-slurm-on-gcp-*`) due to a timeout that cannot be customized.
  * Slurm on GCP v5 HPC Toolkit modules (`schedmd-slurm-gcp-v5-*`) allow the
    Slurm configuration timeouts to customized to work around this via the
    `cloud_parameters` variable on the [controller].

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

* The Slurm on GCP v5 HPC Toolkit modules (`schedmd-slurm-gcp-v5-*`) have two
  variables that can be used to define attached GPUs, `guest_accelerators` for
  HPC Toolkit consistency and `gpus` for consistency with the underlying
  terraform modules from the [Slurm on GCP repo][slurm-gcp].
* In order to utilize the GPUs in the compute VMs deployed by Slurm, the GPU
  count must be specified when starting a job with `srun` or `sbatch`:
  
  ```shell
  srun -N 1 -p gpu_partition --gpus 8 nvidia-smi
  ```

  ```bash
  #!/bin/bash

  #SBATCH --nodes=1
  #SBATCH --partition=gpu_partition
  #SBATCH --gpus=8
  
  nvidia-smi
  ```

[slurm-gcp]: https://github.com/SchedMD/slurm-gcp

## Further Reading

* [Cloud GPU Documentation](https://cloud.google.com/compute/docs/gpus/)
* [GPU Information](https://cloud.google.com/compute/docs/gpus/about-gpus): More
  generalized information about GPUs in the Google Cloud Platform.
* [GPU Region and Zone Availability](https://cloud.google.com/compute/docs/gpus/gpu-regions-zones)
* [GPU Pricing](https://cloud.google.com/compute/gpus-pricing)
