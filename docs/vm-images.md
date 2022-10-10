# Supported and Tested VM Images

* [HPC CentOS 7 VM Image](#hpc-centos-7-vm-image)
* [Ubuntu](#ubuntu)
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
the pattern used in the [slurm-gcp-v5-ubuntu2004.yaml] example.

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
[slurm-gcp-v5-ubuntu2004.yaml]: ../community/examples/slurm-gcp-v5-ubuntu2004.yaml

## Other Images

The HPC Toolkit strives to provide flexibility wherever possible. It is possible
to set a VM image in many HPC Toolkit modules. While we do not officially
support images not listed here, other public and custom images should work with
the majority of modules with or without further customization, such as custom
startup-scripts.
