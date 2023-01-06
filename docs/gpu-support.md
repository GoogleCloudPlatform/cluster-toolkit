# Deploying with Accelerators in the Cloud HPC Toolkit

## Supported modules

* vm-instance
* schedmd-slurm-gcp-v5-*
* SchedMD-slurm-on-gcp-*
* pbspro-*

## Accelerator definition automation

The [vm-instance] and [schedmd-slurm-gcp-v5] modules support contain automation
for defining the `guest_accelerator` config. If the user supplies any value for
this setting, the automation will be bypassed.

The automation is handled primary in the `gpu_definition.tf` files in these
modules. This file assumes the existence of two input variables in the module:

* guest_accelerator: A list of terraform objects with the attributes type and
  count.
* machine_type: Defines the machine type of the VM being created.

`gpu_definition.yaml` works by checking the `machine_type` to associate it with
a GPU type and to extract the expected GPU count. The result will always
define the maximum number of GPUs for the machine_type. If this is not desired,
the `guest_accelerator` can be defined manually instead.

This automation also only supports machine type `a2`. Machine type `n1` and
others can also have guest accelerators attached, however the type and count
cannot be determined automatically like with `a2`.

## Troubleshooting and tips

* To list accelerator types and availability by region, run
  `gcloud compute accelerator-types list`. The information is also available in
  the Google Cloud documentation [here](https://cloud.google.com/compute/docs/gpus/gpu-regions-zones).
* Deployment time of VMs with guest accelerators can be longer than a simple VM.

### Slurm on GCP

* When deploying a Slurm cluster with GPUs, we highly recommend using the
  modules based on Slurm on GCP version 5 (schedmd-slurm-gcp-v5-*). The
  interface is more consistent with HPC Toolkit standards and more functionality
  is available to support, debug and workaround any issues related to GPU
  resources.
* SchedMD-slurm-on-gcp-v5-* modules have a different interface for defining
  attached accelerators, `gpu_type` and `gpu_count`. These must be set even if
  the machine type implies GPUs.
* Some GPUs will fail on SchedMD-slurm-on-gcp due to a timeout
* schedmd-slurm-gcp-v5 modules have a timeout variables that can be adjusted to
  work around this.
* schedmd-slurm-gcp-v5 modules have two variables that can be used to define
  attached GPUs, `guest_accelerators` for HPC Toolkit consistency and `gpus` for
  consistency with the underlying modules from Slurm on GCP.

## Further Reading

* [Cloud GPU Documentation](https://cloud.google.com/compute/docs/gpus/)
* [GPU Information](https://cloud.google.com/compute/docs/gpus/about-gpus): More
  generalized information about GPUs in the Google Cloud Platform.
* [GPU Region and Zone Availability](https://cloud.google.com/compute/docs/gpus/gpu-regions-zones)
* [GPU Pricing](https://cloud.google.com/compute/gpus-pricing)
