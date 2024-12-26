# Building customized VM images

This page describes how to build custom VM images for your HPC workloads.
[Images][images] are most commonly used as boot disks of VMs containing the
operating system and your HPC applications. A typical custom image workflow is:

1. Use [Packer](https://packer.io) to boot a VM with a [standard operating
  system][standard-os] "source" image published by Google or one of our partners
  (e.g. [Slurm images][slurm-images]).
2. Packer will execute a script or series of scripts to add your application, its
  dependencies, and modify configuration settings.
3. Once complete, Packer will shutdown the VM and convert its boot disk to a
  "custom image" in your project.
4. New VMs are provisioned using the custom image to execute your HPC workload

[images]: https://cloud.google.com/compute/docs/images
[standard-os]: https://cloud.google.com/compute/docs/images/os-details
[slurm-images]: https://github.com/GoogleCloudPlatform/slurm-gcp/blob/master/docs/images.md#public-image

## Examples

Nearly all HPC schedulers will require you to either (1) install your
application using a source image that already contains the scheduler daemon
or (2) install both the scheduler and your custom application. The examples
below demonstrate each approach:

- [Customizing a Slurm cluster (Hello, World)](../examples/README.md#image-builderyaml-)
- [Customizing a Slurm cluster (AI/ML applications)](../examples/README.md#ml-slurmyaml-)
- [Provisioning an HTCondor pool (installing scheduler)](../examples/README.md#htc-htcondoryaml--)

## Why build an image?

Building a custom image will increase the security, reliability, and scalability
of your workload by:

- Reducing or eliminating the time at boot taken to install your application
  and other customizations.
- Reducing or eliminating the risk that your workload will fail at boot due to
  an outage of an external service (_e.g._, a package repository) or an external
  service that cannot scale to serve your workload.
- Reducing or eliminating your workload's runtime dependencies on access to the
  public internet.
- Eliminating disparities in your application environment introduced by
  the maintainers of the public sources of your applications.

## Requirements

### Outbound connection to internet

Most image customizations scripts will depend upon **outbound** access to the
public internet. For example, the installation of a Python package may invoke
`pip` to  install packages from the [Python Package Index (pypi)][pypi]. There
are 2 mechanisms we provide for outbound internet access:

- Recommended: use a network configured with a [Cloud NAT][nat] for your
  region.
- Alternative: use a VM with a public IP address.

The [Toolkit VPC module][vpc] automatically creates a VPC with a NAT configured
for your primary region. Alternatively, most Packer modules expose a variable
that attaches a public IP address to your builder VM. In the [googlecompute]
Packer plugin, this setting is called `omit_external_ip`. Setting this value to
`false` enables a public IP address.

[googlecompute]: https://developer.hashicorp.com/packer/plugins/builders/googlecompute
[nat]: https://cloud.google.com/nat/docs/overview
[pypi]: https://pypi.org/
[vpc]: ../modules/network/vpc/README.md

### Inbound connections

The Toolkit-recommended solution for Packer on Google Cloud is to use
[startup-script] metadata to execute a shell script at boot. This is a simple
and secure approach because it requires no extra configuration to enable inbound
VM access. It is also the same mechanism used for boot-time configuration and
can be freely swapped back and forth during development.

Most publicly available examples of Packer usage execute customizations using
[provisioners]. Examples of provisioners include shell scripts and Ansible
playbooks. To upload these files, inbound firewall rules must be added to allow

- Port 22 (SSH) for Linux
- Port 3389 (WinRM) for Windows machines

The IP range for this rule must include at least the machine running Packer.
Alternatively, you can use [Identity-Aware Proxy (IAP)][iap] to form a [secure
TCP tunnel][iap-tcp] from your machine to the VM via a static IP range managed
by Google Cloud. The [vpc] module automatically enables the IAP firewall rule
for SSH. Read its documentation to enable IAP firewall rules for WinRM, Windows
Remote Desktop, and arbitrary TCP ports.

Please read the [IAP TCP Tunneling][iap-tcp] overview for further details,
including the IAM roles needed for the account invoking Packer.

[iap]: https://cloud.google.com/iap/docs/concepts-overview
[iap-tcp]: https://cloud.google.com/iap/docs/using-tcp-forwarding
[provisioners]: https://developer.hashicorp.com/packer/docs/provisioners
[startup-script]: https://cloud.google.com/compute/docs/instances/startup-scripts

## Toolkit-supported approaches

Toolkit supports Packer modules as distinct deployment groups each containing
only 1 Packer module that must be identified using `kind`. For example:

```yaml
- group: packer
  modules:
  - id: custom-image
    source: modules/packer/custom-image
    kind: packer
    use: []
    settings: {}
```

### Toolkit Packer module

The Toolkit includes a [Packer module](../modules/packer/custom-image/README.md)
for building custom images in blueprints. This module has been designed with
requirements and best practices in mind. For example, it works effectively with
startup scripts and, by default, will enable IAP tunnels for provisioners that
need them.

#### Windows support

The Toolkit Packer module provides limited support for building custom VM images
based on the [Windows images][windows-images] published by Google. The custom VM
images can be used in blueprints so long as the underlying scheduler and
workload supports Windows. Windows solutions do not receive the same level of
testing as Linux solutions so you should anticipate that there will not be
functionality parity. Please file [issues] when encountering specific problems
and [feature requests][features] when requesting new functionality.

[windows-images]: https://cloud.google.com/compute/docs/images/os-details#windows_server
[issues]: https://github.com/GoogleCloudPlatform/hpc-toolkit/issues
[features]: https://github.com/GoogleCloudPlatform/hpc-toolkit/discussions/categories/ideas-and-feature-requests

### External Packer modules

The Toolkit supports Packer modules developed by 3rd parties -- including ones
that you have developed! -- hosted via git or GitHub. We recommend reading the
module documentation on:

- [GitHub-hosted modules and packages](https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/main/modules/README.md#github-hosted-modules-and-packages)
- [GitHub-hosted Packer modules](https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/main/modules/README.md#github-hosted-packer-modules)

In particular, the Toolkit recommends using double-slash (`//`) notation to
identify the root of the git repository. Doing so will ensure that the Packer
module has access to all files within the repository even if it is located in
a subdirectory.

> **_NOTE:_** If you fail to use `//` notatation when referring to the [SchedMD
> packer module][schedmd-packer], it will fail to execute because it refers
> to Ansible playbooks by a relative path (`../ansible`) that will not be
> downloaded.

[schedmd-packer]: https://github.com/GoogleCloudPlatform/slurm-gcp/tree/master/packer#readme

For example, to address the issue noted above:

```yaml
deployment_groups:
- group: primary
  modules:
  - id: network1
    source: modules/network/vpc

- group: packer
  modules:
  - id: custom-image
    source: github.com/GoogleCloudPlatform/slurm-gcp//packer?ref=5.12.2&depth=1
    kind: packer
    settings:
      use_iap: true
      subnetwork: $(network1.subnetwork_self_link)
      ...
```
