# Description

This module creates a Batch Node Pool configured for running HPC workloads with
Intel MPI. The number of VMs in the pool, the machine type of those VMs, the
boot image used, and the maximum allowed idle time of the pool (before it is automatically
deleted) can all be customized. An NFS share can also be specified, which will be mounted
automatically by each node in the pool.

After the pool is created subsequent Batch jobs can be execute on the pool's nodes. Batch
automatically divides the nodes of the pool amongst the jobs targetting it and manages the
queue of jobs targetting a pool when the pool is not large enough to run all its jobs at
the same time.

## Example

```yaml
- id: batch-mpi-pool
  source: modules/scheduler/batch-mpi-pool
  ...
```

## Authentication

The module submits a Batch job to create a node pool using the gcloud CLI on the
workstation deploying the workspace. Google Cloud credentials for an account with
the `Batch Job Administrator` role must be available on the workstation.

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

## Resources

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, also used for the job\_id | `string` | n/a | yes |
| <a name="project_id"></a> [project\_id](#input\_project\_id) | The project to create the node pool | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_zone) | The region in which to create the node pool | `string` | n/a | yes |
| <a name="pool_size"></a> [pool\_size](#input\_pool\_size) | The number of nodes to place in the pool | `number` | n/a | yes |
| <a name="pool_duration"></a> [pool\_duration](#input\_pool\_duration) | The allowed idle lifetime of the node pool | `string` | `1h` | no |
| <a name="machine_type"></a> [machine\_type](#input\_machine\_type) | The type of VM to add to the pool | `string` | `c2-standard-60` | no |
| <a name="boot_image"></a> [boot\_image](#input\_boot\_image) | The boot image to use for VMs added to the pool | `string` | `batch-hpc-centos` | no |
| <a name="nfs_share.server\_ip"></a> [nfs\_share.server_ip](#input\_nfs\_share.server_ip) | The IP address of an NFS server for the pool VMs to mount | `string` | n/a | no |
| <a name="nfs_share.remote\_path"></a> [nfs\_share.remote_path](#input\_nfs\_share.remote_path) | The remote path of the NFS server to mount (e.g. /share) | `string` | n/a | no |
| <a name="nfs_share.mount\_path"></a> [nfs\_share.mount_path](#input\_nfs\_share.mount_path) | The path on each VM in the pool to mount the NFS share | `string` | n/a | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions for submitting Batch jobs to the node pool |
| <a name="output_batch_run_mpi_workload"></a> [batch_run_mpi_workload](#output\_batch_run_mpi_workload) | Sample job configuration for an MPI job to run in the node pool |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
