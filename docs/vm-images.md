# Supported and Tested VM Images

* [HPC CentOS 7 VM Image](#hpc-centos-7-vm-image)
* [Ubuntu](#ubuntu)
* [Windows](#windows)
* [Other Images](#other-images)

## HPC CentOS 7 VM Image
The HPC Toolkit has officially supported the [HPC CentOS 7 VM Image][hpcimage] as the
primary VM image for HPC workloads on Google Cloud since it's release. Since the
[HPC CentOS 7 VM Image][hpcimage] comes pre-tuned for optimal performance on
typical HPC workloads, it is the default VM image in our modules, unless there
is specific requirement for a different OS distribution.

Exceptions:

* [DDN-EXAScaler]: The underlying terraform module has a limitted set of
  supported images [documented][exascalerimages] in the exascaler-cloud-terraform
  github repo. **This requirement only applies to the servers, not the clients.**
* [omnia-install]: Only provides support for Rocky 8.

[hpcimage]: https://cloud.google.com/blog/topics/hpc/introducing-hpc-vm-images

## Ubuntu
The HPC Toolkit officially supports Ubuntu based VM images in the majority of
our modules, with a couple exceptions:

* [htcondor-configure]: Only provides support for the HPC CentOS 7 image.
* [nfs-server]: Only provides support for CentOS 7 images for the server itself.
* [DDN-EXAScaler]: The underlying terraform module has a limitted set of
  supported images [documented][exascalerimages] in the exascaler-cloud-terraform
  github repo. **This requirement only applies to the servers, not the clients.**
* [omnia-install]: Only provides support for Rocky 8.

HPC Toolkit modules and startup scripts
have been evaulated against the `ubuntu-2004-lts` image family. For more
information about the Ubuntu Google Cloud images, see the Canonical
[documentation](https://ubuntu.com/server/docs/cloud-images/google-cloud-engine).

To use the Ubuntu images with the `schedmd-slurm-gcp-v5` modules, follow
the pattern used in the [hpc-slurm-ubuntu2004.yaml] example.

In most other modules that provide the option to set a VM image, you can set it
to use the Ubuntu image with the following:

```yaml
...
settings:
  instance_image:
    family: ubuntu-2004-lts
    project: ubuntu-os-cloud
```

[htcondor-configure]: ../community/modules/scheduler/htcondor-configure/README.md
[nfs-server]: ../community/modules/file-system/nfs-server/README.md
[DDN-EXAScaler]: ../community/modules/file-system/DDN-EXAScaler/README.md
[exascalerimages]: https://github.com/DDNStorage/exascaler-cloud-terraform/blob/master/gcp/README.md#boot-image-options
[omnia-install]: ../community/modules/scripts/omnia-install/README.md
[hpc-slurm-ubuntu2004.yaml]: ../community/examples/hpc-slurm-ubuntu2004.yaml

## Windows

The HPC Toolkit provides limited support for building custom VM images based on
the [Windows images][windows-images] published by Google. The custom VM images
can be used in blueprints so long as the underlying scheduler and workload
supports Windows. Windows solutions do not receive the same level of testing as
Linux solutions so you should anticipate that there will not be functionality
parity. Please file [issues] when encountering specific problems and [feature
requests][features] when requesting new functionality.

[windows-images]: https://cloud.google.com/compute/docs/images/os-details#windows_server
[issues]: https://github.com/GoogleCloudPlatform/hpc-toolkit/issues
[features]: https://github.com/GoogleCloudPlatform/hpc-toolkit/discussions/categories/ideas-and-feature-requests

## Other Images

The HPC Toolkit strives to provide flexibility wherever possible. It is possible
to set a VM image in many HPC Toolkit modules. While we do not officially
support images not listed here, other public and custom images should work with
the majority of modules with or without further customization, such as custom
startup-scripts.

## Slurm on GCP Custom Images

HPC Toolkit modules based on terraform modules in [Slurm on GCP][slurm-gcp]
allow custom images via custom instance templates and directly through the
`instance_image` variable, but they have explicit requirements to function
correctly with the Slurm cluster. We recommend one of two options for creating a
custom image for these modules:

1. Use the [packer templates][slurm-gcp-packer] hosted in the
   [Slurm on GCP][slurm-gcp] github repository directly. The
   `example.pkrvars.hcl` file can be customized to your needs, by supplying a
   different base image or through the `extra_ansible_provisioners` variable.
1. Create a custom image with a HPC Toolkit [packer module][hpc-toolkit-packer]
   using one of the Slurm on GCP images as the base image. The image can be
   customized via `shell_scripts`, `ansible_playbooks` or a provided
   `startup_script`.

For more information on the Slurm on GCP public images, see their
[documentation][slurm-gcp-images]. From there, you can see which public images
are available, which software is installed on them and more information on how
to customize them using option 1 listed above.

These instructions apply to the following modules:

* [schedmd-slurm-gcp-v5-controller]
* [schedmd-slurm-gcp-v5-login]
* [schedmd-slurm-gcp-v5-node-group]

[slurm-gcp]: https://github.com/SchedMD/slurm-gcp
[slurm-gcp-packer]: https://github.com/SchedMD/slurm-gcp/tree/master/packer
[slurm-gcp-images]: https://github.com/SchedMD/slurm-gcp/blob/master/docs/images.md
[hpc-toolkit-packer]: ../modules/packer/custom-image
[schedmd-slurm-gcp-v5-controller]: ../community/modules/scheduler/schedmd-slurm-gcp-v5-controller
[schedmd-slurm-gcp-v5-login]: ../community/modules/scheduler/schedmd-slurm-gcp-v5-login
[schedmd-slurm-gcp-v5-node-group]: ../community/modules/compute/schedmd-slurm-gcp-v5-node-group
