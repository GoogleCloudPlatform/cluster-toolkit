# Module: Slurm Cluster

[FAQ](../../docs/faq.md) | [Troubleshooting](../../docs/troubleshooting.md) |
[Glossary](../../docs/glossary.md)

<!-- mdformat-toc start --slug=github --no-anchors --maxlevel=6 --minlevel=1 -->

- [Module: Slurm Cluster](#module-slurm-cluster)
  - [Overview](#overview)
  - [Usage](#usage)
  - [Dependencies](#dependencies)
    - [Software](#software)
      - [Required](#required)
      - [Optional](#optional)
    - [TerraformUser](#terraformuser)
      - [Required](#required-1)
      - [Optional](#optional-1)
    - [Controller SA](#controller-sa)
      - [Required](#required-2)
      - [Optional](#optional-2)
    - [Compute SA](#compute-sa)
      - [Optional](#optional-3)
    - [Login SA](#login-sa)
      - [Optional](#optional-4)
  - [Module API](#module-api)

<!-- mdformat-toc end -->

## Overview

This module creates a [Slurm](../../docs/glossary.md#slurm) cluster on
[GCP](../../docs/glossary.md#gcp). There are two modes of operation: cloud; and
hybrid. Cloud mode will create a VM controller. Hybrid mode will generate
`cloud.conf` and `cloud_gres.conf` files to be included in the on-prem
configuration files, while managing a `config.yaml` file for internal module
use.

Partitions define what compute resources are available to the controller so it
may allocate jobs. Slurm will resume/create compute instances as needed to run
allocated jobs and will suspend/terminate the instances after they are no longer
needed (e.g. IDLE for SuspendTimeout duration). Static nodes are persistent;
they are exempt from being suspended/terminated under normal conditions. Dynamic
nodes are burstable; they will scale up and down with workload.

> **WARNING:** Destroying the controller before it has suspended/terminated all
> static and dynamic node instances and supporting resources (e.g. placement
> groups, subscription) will leave those resources orphaned unless cleanup
> options are enabled (.e.g `enable_cleanup_compute`,
> `enable_cleanup_subscriptions`).

## Usage

See [examples](./examples/slurm_cluster/) directory for sample usages.

See below for a simple inclusion within your own terraform project.

```hcl
module "slurm_cluster" {
  source = "git@github.com:SchedMD/slurm-gcp.git//terraform/slurm_cluster?ref=v5.0.0"

  project_id = "<PROJECT_ID>"

  slurm_cluster_name = "<SLURM_CLUSTER_NAME>"

  # ... omitted ...
}
```

> **NOTE:** Because this module is not hosted on
> [Terraform Registry](../../docs/glossary.md#terraform-registry), the version
> must be strictly controlled via
> [revision](https://www.terraform.io/language/modules/sources#selecting-a-revision)
> syntax on the source line.

## Dependencies

### Software

Certain software must be installed on the local machine or APIs enabled in
[GCP](../../docs/glossary.md#gcp) for
[TerraformUser](../../docs/glossary.md#terraformuser) to be able to use this
module.

#### Required

- [Terraform](https://www.terraform.io/downloads.html) is installed.
- [GCP Cloud SDK](https://cloud.google.com/sdk/downloads) is installed.
- [Compute Engine API](../../docs/glossary.md#compute-engine) is enabled.

#### Optional

- [Python](../../docs/glossary.md#python) is installed.
  - Required Version: `>= 3.6.0, < 4.0.0`
  - Required when any of:
    - `enable_hybrid=true`
    - `enable_cleanup_compute=true`
    - `enable_cleanup_subscriptions=true`
    - `enable_reconfigure=true`
- [Pip](../../../docs/glossary.md#pip) packages are installed.
  - Required when any of:
    - `enable_hybrid=true`
    - `enable_cleanup_compute=true`
    - `enable_cleanup_subscriptions=true`
    - `enable_reconfigure=true`
  - `pip3 install -r ../../scripts/requirements.txt --user`
- [Private Google Access](../../docs/glossary.md#private-google-access) is
  enabled.
  - Required when any instances only have internal IPs.
- [Secret Manager API](../../docs/glossary.md#secret-manager) is enabled.
  - Required when `cloudsql != null`.
- [Pub/Sub API](../../docs/glossary.md#pubsub) is enabled.
  - Required when any of:
    - `enable_cleanup_subscriptions=true`
    - `enable_reconfigure=true`
- [Bigquery API](../../docs/glossary.md#bigquery) is enabled.
  - Required when `enable_bigquery_load=true`.

### TerraformUser

[TerraformUser](../../docs/glossary.md#terraformuser) authenticates with
credentials to [Google Cloud](../../docs/glossary.md#gcp). It is recommended to
create a principal [IAM](../../docs/glossary.md#iam) for this user and associate
[roles](../../docs/glossary.md#iam-roles) to them. Optionally, the TerraformUser
can operate through a [service account](../../docs/glossary.md#service-account).

#### Required

- Compute Instance Admin (v1) (`roles/compute.instanceAdmin.v1`)

#### Optional

- Pub/Sub Admin (`roles/pubsub.admin`)
  - Required when `enable_reconfigure=true`.
- Secret Manager Admin (`roles/secretmanager.admin`)
  - Required when `cloudsql != null`.
- Service Account User (`roles/iam.serviceAccountUser`)
  - Required when [TerraformUser](../../docs/glossary.md#terraformuser) is using
    an [service account](../../docs/glossary.md#service-account) to
    authenticate.

### Controller SA

[Service account](../../docs/glossary.md#service-account) intended to be
associated with the controller
[instance template](../../docs/glossary.md#instance-template) for
[slurm_controller_instance](../slurm_controller_instance/).

#### Required

- Compute Instance Admin (v1) (`roles/compute.instanceAdmin.v1`)
- Compute Instance Admin (beta) (`roles/compute.instanceAdmin`)
- Service Account User (`roles/iam.serviceAccountUser`)

#### Optional

- BigQuery Data Editor (`roles/bigquery.dataEditor`)
  - Required when `enable_bigquery_load=true`.
- Cloud SQL Editor (`roles/cloudsql.editor`)
  - Required when all of:
    - `cloudsql != null`
    - Communicating to CloudSQL instance
- Logs Writer (`roles/logging.logWriter`)
  - Recommended.
- Monitoring Metric Writer (`roles/monitoring.metricWriter`)
  - Recommended.
- Pub/Sub Admin (`roles/pubsub.admin`)
  - Required when `enable_reconfigure=true`.

### Compute SA

[Service account](../../docs/glossary.md#service-account) intended to be
associated with the compute
[instance templates](../../docs/glossary.md#instance-template) created by
[slurm_partition](../slurm_partition/).

#### Optional

- Logs Writer (`roles/logging.logWriter`)
  - Recommended.
- Monitoring Metric Writer (`roles/monitoring.metricWriter`)
  - Recommended.

### Login SA

[Service account](../../docs/glossary.md#service-account) intended to be
associated with the login
[instance templates](../../docs/glossary.md#instance-template) created by
[slurm_partition](../slurm_partition/).

#### Optional

- Logs Writer (`roles/logging.logWriter`)
  - Recommended.
- Monitoring Metric Writer (`roles/monitoring.metricWriter`)
  - Recommended.

## Module API

For the terraform module API reference, please see
[README_TF.md](./README_TF.md).
