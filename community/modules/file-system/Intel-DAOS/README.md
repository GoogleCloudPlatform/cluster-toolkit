## Description

This module allows creating an instance of Distributed Asynchronous Object Storage ([DAOS](https://docs.daos.io/)) on Google Cloud Platform ([GCP](https://cloud.google.com/)).

> **_NOTE:_**
> DAOS on GCP does not require an Cluster Toolkit wrapper.
> Terraform modules are sourced directly from GitHub.
> It will not work as a [local or embedded module](../../../../modules/README.md#embedded-modules).

Terraform modules for DAOS servers and clients are located in the [Google Cloud DAOS repo on GitHub](https://github.com/daos-stack/google-cloud-daos).

DAOS Terraform module parameters can be found in the README.md files in each module directory.

- [DAOS Server module](https://github.com/daos-stack/google-cloud-daos/tree/main/terraform/modules/daos_server#readme)
- [DAOS Client module](https://github.com/daos-stack/google-cloud-daos/tree/main/terraform/modules/daos_client#readme)

For more information on this and other network storage options in the Cluster Toolkit, see the extended [Network Storage documentation](../../../../docs/network_storage.md).
## Examples

The [community examples folder](../../../examples/intel/) contains two example blueprints for deploying DAOS.

- [community/examples/intel/pfs-daos.yml](../../../examples/intel/pfs-daos.yml)
  Blueprint for deploying a DAOS cluster consisting of servers and clients.
  After deploying this example the DAOS storage system will be formatted but no pools or containers will exist.
  The instructions in the [community/examples/intel/README.md](../../../examples/intel/README.md#create-a-daos-pool-and-container) describe how to

  - Deploy a DAOS cluster
  - Manage storage (create a [pool](https://docs.daos.io/v2.2/overview/storage/?h=container#daos-pool) and a [container](https://docs.daos.io/v2.2/overview/storage/?h=container#daos-container))
  - Mount a container on a client
  - Store a large file in a DAOS container

- [community/examples/intel/hpc-slurm-daos.yaml](../../../examples/intel/hpc-slurm-daos.yaml)
  Blueprint for deploying a Slurm cluster and DAOS storage with 4 servers.
  The Slurm compute nodes are configured as DAOS clients and have the ability to use the DAOS filesystem.
  The instructions in the [community/examples/intel/README.md](../../../examples/intel/README.md#deploy-the-daosslurm-cluster) describe how to deploy the Slurm cluster and run a job which uses the DAOS file system.

## Support

Content in the [google-cloud-daos](https://github.com/daos-stack/google-cloud-daos) repository is licensed under the [Apache License Version 2.0](https://github.com/daos-stack/google-cloud-daos/blob/main/LICENSE) open-source license.

[DAOS](https://github.com/daos-stack/daos) is distributed under the BSD-2-Clause-Patent open-source license.

Intel Corporation provides two options for technical support:

1. Community Support

   Community support is available to anyone through Jira and via the DAOS channel for Google Cloud users on Slack.

   JIRA: https://daosio.atlassian.net/jira/software/c/projects/DAOS/issues/

   - An Atlassian account is not needed for read only access to Jira.
   - An Atlassian account is required to create and update tickets.
     To create an account follow the steps at https://support.atlassian.com/atlassian-account/docs/create-an-atlassian-account.

   Slack: https://daos-stack.slack.com/archives/C03GLTLHA59

   Community support is provided on a best-effort basis.

2. Commercial L3 Support

   Commercial L3 support is available on an on-demand basis.

   Contact Intel Corporation to obtain more information about Commercial L3 support.

   You may inquire about L3 support via the [Slack channel](https://daos-stack.slack.com/archives/C03GLTLHA59).
