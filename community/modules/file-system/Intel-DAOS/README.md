## Description

This modules allows creating an instance of Distributed Asynchronous Object Storage ([DAOS](https://docs.daos.io/)) on Google Cloud Platform ([GCP](https://cloud.google.com/)).

For more information, please refer to the [Google Cloud DAOS repo on GitHub](https://github.com/daos-stack/google-cloud-daos).

> **_NOTE:_** DAOS on GCP does not require an HPC Toolkit wrapper and, therefore, sources directly from GitHub. It will not work as a [local or embedded module](../../../../modules/README.md#embedded-modules).

## Examples

Multiple fully working examples of a DAOS deployment and how it can be used in conjunction with Slurm [can be found in the community examples folder](../../../examples/intel/).

Using the DAOS server implies that one has DAOS server images created as [instructed in the images section here](https://github.com/daos-stack/google-cloud-daos/tree/main/images).

A full list of module parameters can be found at [the DAOS Server module README](https://github.com/daos-stack/google-cloud-daos/tree/main/terraform/modules/daos_server).

## Recommended settings

By default, the DAOS system is created with 4 servers will be configured for best cost per GB (TCO, see below), the system will be formated at the server side using [`dmg format`](https://github.com/daos-stack/google-cloud-daos/tree/develop/terraform/examples/daos_cluster#format-storage) but no pool or containers will be created.

The following settings will configure this [system for TCO](https://github.com/daos-stack/google-cloud-daos/tree/develop/terraform/examples/daos_cluster#the-terraformtfvarstcoexample-file) (default):

```yaml
  - source: github.com/daos-stack/google-cloud-daos.git//terraform/modules/daos_server?ref=develop
    kind: terraform
    id: daos
    use: [network1]
    settings:
      labels: {ghpc_role: file-system}
      number_of_instances : 4 # number of DAOS server instances
      machine_type        : "n2-custom-36-215040"
      os_disk_size_gb     : 20
      daos_disk_count     : 16
      daos_scm_size       : 180
```

The following settings will configure this system for [best performance](https://github.com/daos-stack/google-cloud-daos/tree/develop/terraform/examples/daos_cluster#the-terraformtfvarsperfexample-file):

```yaml
  - source: github.com/daos-stack/google-cloud-daos.git//terraform/modules/daos_server?ref=develop
    kind: terraform
    id: daos
    use: [network1]
    settings:
      labels: {ghpc_role: file-system}
      # The default DAOS settings are optimized for TCO
      # The following will tune this system for best perf
      machine_type        : "n2-standard-16"
      os_disk_size_gb     : 20
      daos_disk_count     : 4
      daos_scm_size       : 45
```
