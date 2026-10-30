# VM Images

* [Specifying Blueprint Images](#specifying-blueprint-images)
  * [Instance Images](#instance-images)
  * [Pinning Specific Images](#pinning-specific-images)
* [Cluster Toolkit Supported Images](#cluster-toolkit-supported-images)
  * [Rocky Linux 9](#rocky-linux-9)
  * [Ubuntu 24.04 LTS](#ubuntu-2404-lts)
  * [Windows](#windows)
  * [Supported Features](#supported-features)
  * [Other Images](#other-images)
* [Slurm on GCP](#slurm-on-gcp)
  * [Publicly Published Slurm Images](#publicly-published-slurm-images)
  * [Custom Slurm Images](#custom-slurm-images)

For information on customizing VM images with extra software and configuration
settings, see [Building Images](image-building.md).

Please see the [blueprint catalog](https://cloud.google.com/cluster-toolkit/docs/setup/blueprint-catalog) for examples.

## Specifying Blueprint Images

### Instance Images

> [!NOTE]
> This information is applicable for most source modules, but there are some
> modules that have their own image specification. Please read the
> documentation for any module utilized.

When a Cluster Toolkit blueprint points to a predefined source module (e.g.
`community/modules/compute/schedmd-slurm-gcp-v6-nodeset`), generally the
module has a default image defined. In order to override this default image, a
user may specify the `instance_image` setting in the yaml blueprint, within
either the specific module definition or the global variables. The
`instance_image` setting is defined by three parameters within the blueprint:

```yaml
instance_image:
 family: aci-cpu-rocky-linux-9-slurm-2605-amd64
 project: advanced-compute-images                  # If family is defined, omit name
 name: aci-cpu-rocky-linux-9-slurm-2605-amd64-v20261008-1  # If name is defined, omit family
```

The `project` setting defines the space where the image will be found. Either
this is set to a known project where HPC images are hosted (e.g.
`advanced-compute-images`, `ubuntu-os-cloud`, etc.) or a private project
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

The following is a list of commonly used base images that can be used in a
blueprint:

```yaml
    settings:

      instance_image:
        family: aci-cpu-rocky-linux-9-amd64
        project: advanced-compute-images

      instance_image:
        family: aci-gpu-u2404-cuda-132-nvidia-595-amd64
        project: advanced-compute-images

      instance_image:
        family: aci-gpu-u2404-cuda-132-nvidia-595-arm64
        project: advanced-compute-images

      instance_image:
        family: ubuntu-2404-lts-amd64
        project: ubuntu-os-cloud
```

### Pinning Specific Images

Users may want to be able to guarantee that an image has not been changed across
multiple HPC deployments. One way to guarantee that the same image is used,
would be to either create a custom image
([Image Building](image-building.md)), or to copy an image to a personal or
team project and reference that.

The following command will copy a specified image from a source project to your
own:

```shell
# Copy image from one project to another
gcloud compute images create <new_image_name> --project=<your project> --source-image=<source_image_name> --source-image-project=<source_project>
```

Alternatively, a user can specify a family of images you wish to pull from (i.e.
`--source-image-family` instead of `--source-image`). See more on
[gcloud compute images create][gcloud-compute-images].

Once the image has been created or copied, the user can specify their own
project and the new image name in the `instance_image` field discussed in
[Instance Images](#instance-images)

## Cluster Toolkit Supported Images

### Rocky Linux 9

Rocky Linux 9 is the primary supported VM image for HPC workloads on Google Cloud.

### Ubuntu 24.04 LTS

The Cluster Toolkit officially supports Ubuntu 24.04 LTS based VM images in the
majority of our modules, with a couple of exceptions.

### Windows

See [building Windows images](image-building.md#windows-support) for a
description of our support for Windows images.

### Supported Features

<table>
<tr>
  <th>Deployment Type/Scheduler</th>
  <th>Feature</th>
  <th></th>
  <th>Rocky Linux 9</th><th>Ubuntu 24.04</th>
</tr>

<tr>
  <th rowspan="3">Cloud Batch</th>
  <th>Lustre</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-lustre.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-lustre.yaml">✓</a></td>
</tr>
<tr>
  <th>Shared filestore</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-filestore.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-filestore.yaml">✓</a></td>
</tr>
<tr>
  <th>Startup script</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/batch-startup.yaml">✓</a></td>
</tr>
<tr>
  <th rowspan="3">Slurm</th>
  <th>Lustre</th>
  <th></th>
  <td><a href="../examples/hpc-enterprise-slurm.yaml">✓</a></td>
  <td></td>
</tr>
<tr>
  <th>Shared filestore</th>
  <th></th>
  <td><a href="../examples/hpc-slurm.yaml">✓</a></td>
  <td></td>
</tr>
<tr>
  <th>Startup script</th>
  <th></th>
  <td><a href="../community/examples/hpc-slurm-ramble-gromacs.yaml">✓</a></td>
  <td>✓</td>
</tr>

<tr>
  <th rowspan="4">VM Instance</th>
  <th>Chrome Remote Desktop</th>
  <th></th>
  <td></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-crd.yaml">✓</a></td>
</tr>
<tr>
  <th>Lustre</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-lustre.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-lustre.yaml">✓</a></td>
</tr>
<tr>
  <th>Shared filestore</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-filestore.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-filestore.yaml">✓</a></td>
</tr>
<tr>
  <th>Startup script</th>
  <th></th>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-startup.yaml">✓</a></td>
  <td><a href="../tools/validate_configs/os_compatibility_tests/vm-startup.yaml">✓</a></td>
</tr>
<tr>
  <th rowspan="1">HTCondor</th>
  <th></th>
  <th></th>
  <td><a href="../community/examples/htc-htcondor.yaml">✓</a></td>
  <td></td>
</tr>
</table>

### Other Images

The Cluster Toolkit strives to provide flexibility wherever possible. It is possible
to set a VM image in many Cluster Toolkit modules. While we do not officially
support images not listed here, other public and custom images should work with
the majority of modules with or without further customization, such as custom
startup-scripts.

## Slurm on GCP

### Publicly Published Slurm Images

Google Cloud's Advanced Compute Images team publishes pre-built Slurm
images in the `advanced-compute-images` project, which are documented in the
[Overview of Advanced Compute Images][aci-images]. This documentation covers
which images are available, their naming convention, and what software is
pre-installed on them.

Slurm images are compatible by the minor version releases of the Terraform and
Packer modules. The default Slurm image family and vendored Slurm module
implementation used by your copy of the Toolkit in the local filesystem can be
inspected in
`community/modules/scheduler/schedmd-slurm-gcp-v6-controller/variables_controller_instance.tf`
and `community/modules/internal/slurm-gcp/`.

Commonly used Slurm image families in `advanced-compute-images` include:

* **CPU / HPC Workloads (Rocky Linux 9, AMD64):** `aci-cpu-rocky-linux-9-slurm-2605-amd64` (default for Slurm v6 modules)
* **GPU / AI/ML Workloads (Ubuntu 24.04, AMD64):** `aci-gpu-u2404-slurm-2605-cuda-132-nvidia-595-amd64`
* **GPU / AI/ML Workloads (Ubuntu 24.04, ARM64):** `aci-gpu-u2404-slurm-2605-cuda-132-nvidia-595-arm64`

<a name="slurm-on-gcp-custom-images"></a>
### Custom Slurm Images

> [!NOTE]
> Set the `instance_image_custom` to `true` in the blueprint to let terraform
> know you are aware that you are using a custom image.

See: [Image Builder](../examples/README.md#image-builderyaml-)

> [!WARNING]
> When building custom images, the Terraform and Packer modules must share the
> same version.

These instructions apply to the following modules:

* [schedmd-slurm-gcp-v6-controller]
* [schedmd-slurm-gcp-v6-login]
* [schedmd-slurm-gcp-v6-nodeset]

[aci-images]: https://cloud.google.com/compute/docs/images/workload/aci-images
[gcloud-compute-images]: https://cloud.google.com/sdk/gcloud/reference/compute/images/create
[schedmd-slurm-gcp-v6-controller]: ../community/modules/scheduler/schedmd-slurm-gcp-v6-controller
[schedmd-slurm-gcp-v6-login]: ../community/modules/scheduler/schedmd-slurm-gcp-v6-login
[schedmd-slurm-gcp-v6-nodeset]: ../community/modules/compute/schedmd-slurm-gcp-v6-nodeset
