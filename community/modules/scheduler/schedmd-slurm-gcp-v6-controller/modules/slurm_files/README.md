<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright (C) SchedMD LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | ~> 1.3 |
| <a name="requirement_archive"></a> [archive](#requirement\_archive) | ~> 2.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.53 |
| <a name="requirement_local"></a> [local](#requirement\_local) | ~> 2.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_archive"></a> [archive](#provider\_archive) | ~> 2.0 |
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.53 |
| <a name="provider_local"></a> [local](#provider\_local) | ~> 2.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket_object.config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.controller_startup_scripts](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.devel](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.epilog_scripts](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.nodeset_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.nodeset_dyn_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.nodeset_startup_scripts](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.nodeset_tpu_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.prolog_scripts](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.task_epilog_scripts](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.task_prolog_scripts](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [random_uuid.cluster_id](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/uuid) | resource |
| [archive_file.slurm_gcp_devel_zip](https://registry.terraform.io/providers/hashicorp/archive/latest/docs/data-sources/file) | data source |
| [google_storage_bucket.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/storage_bucket) | data source |
| [local_file.chs_gpu_health_check](https://registry.terraform.io/providers/hashicorp/local/latest/docs/data-sources/file) | data source |
| [local_file.external_epilog](https://registry.terraform.io/providers/hashicorp/local/latest/docs/data-sources/file) | data source |
| [local_file.external_prolog](https://registry.terraform.io/providers/hashicorp/local/latest/docs/data-sources/file) | data source |
| [local_file.setup_external](https://registry.terraform.io/providers/hashicorp/local/latest/docs/data-sources/file) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_bucket_dir"></a> [bucket\_dir](#input\_bucket\_dir) | Bucket directory for cluster files to be put into. | `string` | `null` | no |
| <a name="input_bucket_name"></a> [bucket\_name](#input\_bucket\_name) | Name of GCS bucket to use. | `string` | n/a | yes |
| <a name="input_cgroup_conf_tpl"></a> [cgroup\_conf\_tpl](#input\_cgroup\_conf\_tpl) | Slurm cgroup.conf template file path. | `string` | `null` | no |
| <a name="input_cloud_parameters"></a> [cloud\_parameters](#input\_cloud\_parameters) | cloud.conf options. Default behavior defined in scripts/conf.py | <pre>object({<br/>    no_comma_params      = optional(bool, false)<br/>    private_data         = optional(list(string))<br/>    scheduler_parameters = optional(list(string))<br/>    resume_rate          = optional(number)<br/>    resume_timeout       = optional(number)<br/>    suspend_rate         = optional(number)<br/>    suspend_timeout      = optional(number)<br/>    topology_plugin      = optional(string)<br/>    topology_param       = optional(string)<br/>    tree_width           = optional(number)<br/>  })</pre> | `{}` | no |
| <a name="input_cloudsql_secret"></a> [cloudsql\_secret](#input\_cloudsql\_secret) | Secret URI to cloudsql secret. | `string` | `null` | no |
| <a name="input_compute_startup_scripts_timeout"></a> [compute\_startup\_scripts\_timeout](#input\_compute\_startup\_scripts\_timeout) | The timeout (seconds) applied to each script in compute\_startup\_scripts. If<br/>any script exceeds this timeout, then the instance setup process is considered<br/>failed and handled accordingly.<br/><br/>NOTE: When set to 0, the timeout is considered infinite and thus disabled. | `number` | `300` | no |
| <a name="input_controller_network_attachment"></a> [controller\_network\_attachment](#input\_controller\_network\_attachment) | SelfLink for NetworkAttachment to be attached to the controller, if any. | `string` | `null` | no |
| <a name="input_controller_startup_scripts"></a> [controller\_startup\_scripts](#input\_controller\_startup\_scripts) | List of scripts to be ran on controller VM startup. | <pre>list(object({<br/>    filename = string<br/>    content  = string<br/>  }))</pre> | `[]` | no |
| <a name="input_controller_startup_scripts_timeout"></a> [controller\_startup\_scripts\_timeout](#input\_controller\_startup\_scripts\_timeout) | The timeout (seconds) applied to each script in controller\_startup\_scripts. If<br/>any script exceeds this timeout, then the instance setup process is considered<br/>failed and handled accordingly.<br/><br/>NOTE: When set to 0, the timeout is considered infinite and thus disabled. | `number` | `300` | no |
| <a name="input_controller_state_disk"></a> [controller\_state\_disk](#input\_controller\_state\_disk) | A disk that will be attached to the controller instance template to save state of slurm. The disk is created and used by default.<br/>  To disable this feature, set this variable to null.<br/><br/>  NOTE: This will not save the contents at /opt/apps and /home. To preserve those, they must be saved externally. | <pre>object({<br/>    device_name = string<br/>  })</pre> | <pre>{<br/>  "device_name": null<br/>}</pre> | no |
| <a name="input_disable_default_mounts"></a> [disable\_default\_mounts](#input\_disable\_default\_mounts) | Disable default global network storage from the controller<br/>- /home<br/>- /apps | `bool` | `false` | no |
| <a name="input_enable_bigquery_load"></a> [enable\_bigquery\_load](#input\_enable\_bigquery\_load) | Enables loading of cluster job usage into big query.<br/><br/>NOTE: Requires Google Bigquery API. | `bool` | `false` | no |
| <a name="input_enable_chs_gpu_health_check_epilog"></a> [enable\_chs\_gpu\_health\_check\_epilog](#input\_enable\_chs\_gpu\_health\_check\_epilog) | Enable a Cluster Health Sacnner(CHS) GPU health check that slurmd executes as an epilog script after completing a job step from a new job allocation.<br/>Compute nodes that fail GPU health check during epilog will be marked as drained. Find more details at:<br/>https://github.com/GoogleCloudPlatform/cluster-toolkit/tree/main/docs/CHS-Slurm.md | `bool` | `false` | no |
| <a name="input_enable_chs_gpu_health_check_prolog"></a> [enable\_chs\_gpu\_health\_check\_prolog](#input\_enable\_chs\_gpu\_health\_check\_prolog) | Enable a Cluster Health Sacnner(CHS) GPU health check that slurmd executes as a prolog script whenever it is asked to run a job step from a new job allocation. Compute nodes that fail GPU health check during prolog will be marked as drained. Find more details at:<br/>https://github.com/GoogleCloudPlatform/cluster-toolkit/tree/main/docs/CHS-Slurm.md | `bool` | `false` | no |
| <a name="input_enable_debug_logging"></a> [enable\_debug\_logging](#input\_enable\_debug\_logging) | Enables debug logging mode. Not for production use. | `bool` | `false` | no |
| <a name="input_enable_external_prolog_epilog"></a> [enable\_external\_prolog\_epilog](#input\_enable\_external\_prolog\_epilog) | Automatically enable a script that will execute prolog and epilog scripts<br/>shared by NFS from the controller to compute nodes. Find more details at:<br/>https://github.com/GoogleCloudPlatform/slurm-gcp/blob/v5/tools/prologs-epilogs/README.md | `bool` | `false` | no |
| <a name="input_enable_hybrid"></a> [enable\_hybrid](#input\_enable\_hybrid) | Enables use of hybrid controller mode. When true, controller\_hybrid\_config will<br/>be used instead of controller\_instance\_config and will disable login instances. | `bool` | `false` | no |
| <a name="input_enable_slurm_auth"></a> [enable\_slurm\_auth](#input\_enable\_slurm\_auth) | Enables slurm authentication instead of munge. | `bool` | `false` | no |
| <a name="input_endpoint_versions"></a> [endpoint\_versions](#input\_endpoint\_versions) | Version of the API to use (The compute service is the only API currently supported) | <pre>object({<br/>    compute = string<br/>  })</pre> | <pre>{<br/>  "compute": null<br/>}</pre> | no |
| <a name="input_epilog_scripts"></a> [epilog\_scripts](#input\_epilog\_scripts) | List of scripts to be used for Epilog. Programs for the slurmd to execute<br/>on every node when a user's job completes.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_Epilog. | <pre>list(object({<br/>    filename = string<br/>    content  = optional(string)<br/>    source   = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_extra_logging_flags"></a> [extra\_logging\_flags](#input\_extra\_logging\_flags) | The only available flag is `trace_api` | `map(bool)` | `{}` | no |
| <a name="input_hybrid_conf"></a> [hybrid\_conf](#input\_hybrid\_conf) | The hybrid configuration | <pre>object({<br/>    slurm_bin_dir           = optional(string)<br/>    slurm_log_dir           = optional(string)<br/>    slurm_control_host      = string<br/>    slurm_control_host_port = optional(string)<br/>    slurm_control_addr      = optional(string)<br/>    output_dir              = optional(string)<br/>    install_dir             = optional(string)<br/>    slurm_uid               = optional(number)<br/>    slurm_gid               = optional(number)<br/>    service_account_email   = optional(string)<br/>    google_app_cred_path    = optional(string)<br/>  })</pre> | `null` | no |
| <a name="input_munge_mount"></a> [munge\_mount](#input\_munge\_mount) | Remote munge mount for compute and login nodes to acquire the munge.key.<br/>By default, the munge mount server will be assumed to be the<br/>`var.slurm_control_host` (or `var.slurm_control_addr` if non-null) when<br/>`server_ip=null`. | <pre>object({<br/>    server_ip     = string<br/>    remote_mount  = string<br/>    fs_type       = string<br/>    mount_options = string<br/>  })</pre> | <pre>{<br/>  "fs_type": "nfs",<br/>  "mount_options": "",<br/>  "remote_mount": "/etc/munge/",<br/>  "server_ip": null<br/>}</pre> | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | Storage to mounted on all instances.<br/>- server\_ip     : Address of the storage server.<br/>- remote\_mount  : The location in the remote instance filesystem to mount from.<br/>- local\_mount   : The location on the instance filesystem to mount to.<br/>- fs\_type       : Filesystem type (e.g. "nfs").<br/>- mount\_options : Options to mount with. | <pre>list(object({<br/>    server_ip     = string<br/>    remote_mount  = string<br/>    local_mount   = string<br/>    fs_type       = string<br/>    mount_options = string<br/>  }))</pre> | `[]` | no |
| <a name="input_nodeset"></a> [nodeset](#input\_nodeset) | Cluster nodenets, as a list. | `list(any)` | `[]` | no |
| <a name="input_nodeset_dyn"></a> [nodeset\_dyn](#input\_nodeset\_dyn) | Cluster nodenets (dynamic), as a list. | `list(any)` | `[]` | no |
| <a name="input_nodeset_startup_scripts"></a> [nodeset\_startup\_scripts](#input\_nodeset\_startup\_scripts) | List of scripts to be ran on compute VM startup in the specific nodeset. | <pre>map(list(object({<br/>    filename = string<br/>    content  = string<br/>  })))</pre> | `{}` | no |
| <a name="input_nodeset_tpu"></a> [nodeset\_tpu](#input\_nodeset\_tpu) | Cluster nodenets (TPU), as a list. | `list(any)` | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The GCP project ID. | `string` | n/a | yes |
| <a name="input_prolog_scripts"></a> [prolog\_scripts](#input\_prolog\_scripts) | List of scripts to be used for Prolog. Programs for the slurmd to execute<br/>whenever it is asked to run a job step from a new job allocation.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_Prolog. | <pre>list(object({<br/>    filename = string<br/>    content  = optional(string)<br/>    source   = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | The cluster name, used for resource naming and slurm accounting. | `string` | n/a | yes |
| <a name="input_slurm_conf_template"></a> [slurm\_conf\_template](#input\_slurm\_conf\_template) | Slurm slurm.conf template. Content of the file in 'slurm\_conf\_tpl' is used if this is not set. | `string` | `null` | no |
| <a name="input_slurm_conf_tpl"></a> [slurm\_conf\_tpl](#input\_slurm\_conf\_tpl) | Slurm slurm.conf template file path. This path is used only if raw content is not provided in 'slurm\_conf\_template'. | `string` | `null` | no |
| <a name="input_slurm_key_mount"></a> [slurm\_key\_mount](#input\_slurm\_key\_mount) | Remote mount for compute and login nodes to acquire the slurm.key. | <pre>object({<br/>    server_ip     = string<br/>    remote_mount  = string<br/>    fs_type       = string<br/>    mount_options = string<br/>  })</pre> | `null` | no |
| <a name="input_slurmdbd_conf_tpl"></a> [slurmdbd\_conf\_tpl](#input\_slurmdbd\_conf\_tpl) | Slurm slurmdbd.conf template file path. | `string` | `null` | no |
| <a name="input_task_epilog_scripts"></a> [task\_epilog\_scripts](#input\_task\_epilog\_scripts) | List of scripts to be used for TaskEpilog. Programs for the slurmd to execute<br/>as the slurm job's owner after termination of each task.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_TaskEpilog. | <pre>list(object({<br/>    filename = string<br/>    content  = optional(string)<br/>    source   = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_task_prolog_scripts"></a> [task\_prolog\_scripts](#input\_task\_prolog\_scripts) | List of scripts to be used for TaskProlog. Programs for the slurmd to execute<br/>as the slurm job's owner prior to initiation of each task.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_TaskProlog. | <pre>list(object({<br/>    filename = string<br/>    content  = optional(string)<br/>    source   = optional(string)<br/>  }))</pre> | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_bucket_dir"></a> [bucket\_dir](#output\_bucket\_dir) | Path directory within `bucket_name` for Slurm cluster file storage. |
| <a name="output_bucket_name"></a> [bucket\_name](#output\_bucket\_name) | GCS Bucket name of Slurm cluster file storage. |
| <a name="output_config"></a> [config](#output\_config) | Cluster configuration. |
| <a name="output_slurm_bucket_path"></a> [slurm\_bucket\_path](#output\_slurm\_bucket\_path) | GCS Bucket URI of Slurm cluster file storage. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
