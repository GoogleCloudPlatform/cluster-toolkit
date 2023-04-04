## Description

This module allows creating an instance of Distributed Asynchronous Object Storage ([DAOS](https://docs.daos.io/)) on Google Cloud Platform ([GCP](https://cloud.google.com/)).

For more information, please refer to the [Google Cloud DAOS repo on GitHub](https://github.com/daos-stack/google-cloud-daos).

For more information on this and other network storage options in the Cloud HPC
Toolkit, see the extended [Network Storage documentation](../../../../docs/network_storage.md).

> **_NOTE:_** DAOS on GCP does not require an HPC Toolkit wrapper and, therefore, sources directly from GitHub. It will not work as a [local or embedded module](../../../../modules/README.md#embedded-modules).

## Examples

Working examples of a DAOS deployment and how it can be used in conjunction with Slurm [can be found in the community examples folder](../../../examples/intel/).

A full list of server module parameters can be found at [the DAOS Server module README](https://github.com/daos-stack/google-cloud-daos/tree/main/terraform/modules/daos_server).

### DAOS Server Images

In order to use the DAOS server terraform module a DAOS server image must be created as instructed in the *images* directory [here](https://github.com/daos-stack/google-cloud-daos/tree/main/images).

DAOS server images must be built from the same tagged version of the [google-cloud-daos](https://github.com/daos-stack/google-cloud-daos) repository that is specified in the `source:` attribute for modules used in the [community examples](../../../examples/intel/).

For example, in the following snippet taken from the [community/example/intel/daos-cluster.yml](../../../examples/intel/daos-cluster.yaml) the `source:` attribute specifies v0.3.0 of the  daos_server terraform module

```yaml
  - id: daos-server
    source: github.com/daos-stack/google-cloud-daos.git//terraform/modules/daos_server?ref=v0.3.0
    use: [network1]
    settings:
      number_of_instances: 2
      labels: {ghpc_role: file-system}
```

In order to use the daos_server module v0.3.0 , you need to

1. Clone the [google-cloud-daos](https://github.com/daos-stack/google-cloud-daos) repo and check out v0.3.0
2. Follow the instructions in the images/README.md directory to build a DAOS server image

## Recommended settings

By default, the DAOS system is created with 4 servers will be configured for best cost per GB (TCO, see below), the system will be formated at the server side using [`dmg format`](https://github.com/daos-stack/google-cloud-daos/tree/develop/terraform/examples/daos_cluster#format-storage) but no pool or containers will be created.

The following settings will configure this [system for TCO](https://github.com/daos-stack/google-cloud-daos/tree/main/terraform/examples/daos_cluster#the-terraformtfvarstcoexample-file) (default):

```yaml
  - id: daos-server
    source: github.com/daos-stack/google-cloud-daos.git//terraform/modules/daos_server?ref=v0.3.0
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
  - id: daos-server
    source: github.com/daos-stack/google-cloud-daos.git//terraform/modules/daos_server?ref=v0.3.0
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

## Support

Content in the [google-cloud-daos](https://github.com/daos-stack/google-cloud-daos) repository is licensed under the [Apache License Version 2.0](https://github.com/daos-stack/google-cloud-daos/blob/main/LICENSE) open-source license.

[DAOS](https://github.com/daos-stack/daos) is being distributed under the BSD-2-Clause-Patent open-source license.

Intel Corporation provides several ways for the users to get technical support:

1. Community support is available to everybody through Jira and via the DAOS channel for the Google Cloud users on Slack.

   To access Jira, please follow these steps:

   - Navigate to https://daosio.atlassian.net/jira/software/c/projects/DAOS/issues/

   - You will need to request access to DAOS Jira to be able to create and update tickets. An Atlassian account is required for this type of access. Read-only access is available without an account.
   - If you do not have an Atlassian account, follow the steps at https://support.atlassian.com/atlassian-account/docs/create-an-atlassian-account/ to create one.

   To access the Slack channel for DAOS on Google Cloud, please follow this link https://daos-stack.slack.com/archives/C03GLTLHA59

   > This type of support is provided on a best-effort basis, and it does not have any SLA attached.

2. Commercial L3 support is available on an on-demand basis. Please get in touch with Intel Corporation to obtain more information.

   - You may inquire about the L3 support via the Slack channel (https://daos-stack.slack.com/archives/C03GLTLHA59)

[here](https://github.com/daos-stack/google-cloud-daos/tree/main/images)
