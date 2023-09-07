# VM Images

* [Specifying Blueprint Images](#specifying-blueprint-image)
  * [Instance Image](#instance-images)
  * [Pinning Specific Images](#pinning-specifics-images)
* [HPC Toolkit Supported Images](#hpc-toolkit-supported-images)
  * [HPC CentOS 7](#hpc-centos-7)
  * [HPC Rocky Linux 8](#hpc-rocky-linux-8)
  * [Debian 11](#debian-11)
  * [Ubuntu 20.04 LTS](#ubuntu-2004-lts)
  * [Windows](#windows)
  * [Other Images](#other-images)
  * [Slurm on GCP Custom Images](#slurm-on-gcp-custom-images)

For information on customizing VM images with extra software and configuration
settings, see [Building Images](image-building.md).

Please see the [blueprint catalog](https://cloud.google.com/hpc-toolkit/docs/setup/hpc-blueprint-catalog) for examples.

For Slurm images, please see [SchedMD's GitHub repository](https://github.com/SchedMD/slurm-gcp/blob/master/docs/images.md#public-image).

## Specifying Blueprint Image

### Instance Images

> [!NOTE]
> This information is applicable for most source modules, but there are some
> modules that have their own image specification. Please read the
> documentation for any module utilized.

When an HPC Toolkit blueprint points to a predefined source module (e.g.
`community/modules/compute/schedmd-slurm-gcp-v5-node-group`), generally the
module has a default image defined. In order to override this default image, a
user may specify the `instance_image` setting in the yaml blueprint, within
either the specific module definition or the global variables. The
`instance_image` setting is defined by three parameters within the blueprint:

```yaml
instance_image:
  project: centos-cloud
  family: centos-v7        # If family is defined, omit name
  name: centos-7-v20230809 # If name is defined, omit family
```

The `project` setting defines the space where the image will be found. Either
this is set to a known project where HPC images are hosted (e.g
`cloud-hpc-image-public`, `schedmd-slurm-public`, etc.) or a private project
owned by you or your team.

The `family` setting defines a group of images built with the same label, and
generally with some underlying similarities, usually an OS version or a software
version installed on top of the OS. When this is specified, instances will be
created with the latest image within the family. This will keep software more up
to date, but will be less deterministic.

The `name` setting defines a specific static image. While these images are less
likely to be modified, it cannot be guaranteed. It is possible that an image
publisher may choose to delete and re-publish images with the same name.

> [!NOTE]
> The `name` setting is not always available, depending on the source module.
> In these cases, please default back to the family setting.

### Pinning Specifics Images

Users may want to be able to guarantee that an image has not been changed across
multiple HPC deployments. One way to guarantee that the same image is used,
would be to either create a custom image
([Image Building](docs/image-building.md)), or to copy an image to a personal or
team project and reference that.

The following command will copy a specified image from a source project to your
own:

```shell
# Copy image from one project to another
gcloud compute images create <new_image_name> --project=<your project> --source-image=<source_image_name> --source-image-project=<source_project>
```

Alternatively, a user can specify a family of images you wish to pull from (i.e.
`--source-image-family` instead of `--source-image`). See more on
[gcloud compute images create](gcloud-compute-images).

Once the image has been created or copied, the user can specify their own
project and the new image name in the `instance_image` field discussed in
[Instance Images](#instance-images)

## HPC Toolkit Supported Images

### HPC CentOS 7

The HPC Toolkit has officially supported the [HPC CentOS 7 VM Image][hpcimage]
as the primary VM image for HPC workloads on Google Cloud since it's release.
Since the [HPC CentOS 7 VM Image][hpcimage] comes pre-tuned for optimal
performance on typical HPC workloads, it is the default VM image in our modules,
unless there is specific requirement for a different OS distribution.

[hpcimage]: https://cloud.google.com/blog/topics/hpc/introducing-hpc-vm-images

### HPC Rocky Linux 8

HPC Rocky Linux 8 is planned to become the primary supported VM image for HPC
workloads on Google Cloud from 2024.

### Debian 11

The HPC Toolkit officially supports Debian 11 based VM images in the majority of
our modules, with a couple of exceptions.

### Ubuntu 20.04 LTS

The HPC Toolkit officially supports Ubuntu 20.04 LTS based VM images in the
majority of our modules, with a couple of exceptions.

### Windows

See [building Windows images](image-building.md#windows-support) for a
description of our support for Windows images.

### Supported features

<table>
<tr>
  <th>Deployment Type/Scheduler</th>
  <th>Feature</th>
  <th></th>
  <th>CentOS 7</th><th>Debian 11</th><th>Rocky Linux 8</th><th>Ubuntu 20.04</th>
</tr>
<tr>
  <td></td><td></td><td></td><td></td><td></td><td></td><td></td>
</tr>

<tr>
  <th rowspan="3">Cloud Batch</th>
  <th>Lustre</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-lustre.yaml">✓</a></td>
  <td></td>
  <td></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-lustre.yaml">✓</a></td>
</tr>
<tr>
  <th>Shared filestore</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-lustre.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-lustre.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-lustre.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-lustre.yaml">✓</a></td>
</tr>
<tr>
  <th>Startup script</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-startup.yaml">✓</a></td>
</tr>

<tr>
  <th rowspan="4">Slurm</th>
  <th>Chrome Remote Desktop</th>
  <th></th>
  <td></td>
  <td><a href="../community/examples/hpc-slurm-chromedesktop.yaml">✓</a></td>
  <td></td>
  <td></td>
</tr>
<tr>
  <th>Lustre</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/slurm-lustre.yaml">✓</a></td>
  <td></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/slurm-lustre.yaml">✓</a></td>
  <td></td>
</tr>
<tr>
  <th>Shared filestore</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/slurm-filestore.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/slurm-filestore.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/slurm-filestore.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/slurm-filestore.yaml">✓</a></td>
</tr>
<tr>
  <th>Startup script</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/slurm-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/slurm-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/slurm-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/slurm-startup.yaml">✓</a></td>
</tr>

<tr>
  <th rowspan="4">VM Instance</th>
  <th>Chrome Remote Desktop</th>
  <th></th>
  <td></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-crd.yaml">✓</a></td>
  <td></td>
  <td><sup><b>*</b></sup></td>
</tr>
<tr>
  <th>Lustre</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-lustre.yaml">✓</a></td>
  <td></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-lustre.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-lustre.yaml">✓</a></td>
</tr>
<tr>
  <th>Shared filestore</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-filestore.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-filestore.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-filestore.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-filestore.yaml">✓</a></td>
</tr>
<tr>
  <th>Startup script</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-startup.yaml">✓</a></td>
</tr>

<tr>
  <th rowspan="1">HTCondor</th>
  <th></th>
  <th></th>
  <td>✓</td><td></td><td>✓</td><td></td>
</tr>

<tr>
  <th rowspan="1">Omnia</th>
  <th></th>
  <th></th>
  <td></td><td></td><td>✓</td><td></td>
</tr>
</table>

<sup><b>*</b></sup> Chrome Remote desktop does not support Ubuntu 20.04, but it does support Ubuntu 22.04.

### Other Images

The HPC Toolkit strives to provide flexibility wherever possible. It is possible
to set a VM image in many HPC Toolkit modules. While we do not officially
support images not listed here, other public and custom images should work with
the majority of modules with or without further customization, such as custom
startup-scripts.

### Slurm on GCP Custom Images

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

[vm-instance]: ../modules/compute/vm-instance
[hpc-toolkit-packer]: ../modules/packer/custom-image
[schedmd-slurm-gcp-v5-controller]: ../community/modules/scheduler/schedmd-slurm-gcp-v5-controller
[schedmd-slurm-gcp-v5-login]: ../community/modules/scheduler/schedmd-slurm-gcp-v5-login
[schedmd-slurm-gcp-v5-node-group]: ../community/modules/compute/schedmd-slurm-gcp-v5-node-group
[batch-job]: ../modules/scheduler/batch-job-template
[batch-login]: ../modules/scheduler/batch-login-node
[htcondor-setup]: ../community/modules/scheduler/htcondor-setup
[omnia-install]: ../community/modules/scripts/omnia-install
[hpc-slurm-ubuntu2004.yaml]: ../community/examples/hpc-slurm-ubuntu2004.yaml

[htc-htcondor.yaml]: ../community/examples/htc-htcondor.yaml
[omnia-cluster.yaml]: ../community/examples/omnia-cluster.yaml
[vm-startup.yaml]: ../tools/validate_configs/os_compatibility_tests/vm-startup.yaml
[vm-crd.yaml]: ../tools/validate_configs/os_compatibility_tests/vm-crd.yaml
[vm-filestore.yaml]: ../tools/validate_configs/os_compatibility_tests/vm-filestore.yaml
[vm-lustre.yaml]: ../tools/validate_configs/os_compatibility_tests/vm-lustre.yaml
[slurm-startup.yaml]: ../tools/validate_configs/os_compatibility_tests/slurm-startup.yaml
[hpc-slurm-chromedesktop.yaml]: ../community/examples/hpc-slurm-chromedesktop.yaml
[slurm-filestore.yaml]: ../tools/validate_configs/os_compatibility_tests/slurm-filestore.yaml
[slurm-lustre.yaml]: ../tools/validate_configs/os_compatibility_tests/slurm-lustre.yaml
[batch-startup.yaml]: ../tools/validate_configs/os_compatibility_tests/batch-startup.yaml
[batch-filestore.yaml]: ../tools/validate_configs/os_compatibility_tests/batch-filestore.yaml
