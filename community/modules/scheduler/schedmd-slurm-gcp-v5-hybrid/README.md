## Description
This module is a wrapper around the [hybrid-controller] module by SchedMD as
part of the [slurm-gcp] github repository. The hybrid module serves to create
the configurations needed to extend an on-premise slurm cluster to one with one
or more Google Cloud bursting partitions. These partitions will create the
requested nodes in a GCP project on-demand and scale after a period of not being
used, in the same way as the [schedmd-slurm-gcp-v5-controller] module
auto-scales VMs.

### Usages

### Setup

### Manual Configuration

### Example Blueprint

## License
<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2022 Google LLC

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

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_slurm_controller_instance"></a> [slurm\_controller\_instance](#module\_slurm\_controller\_instance) | github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_controller_hybrid | v5.0.3 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_cloud_parameters"></a> [cloud\_parameters](#input\_cloud\_parameters) | cloud.conf options. | <pre>object({<br>    no_comma_params = bool<br>    resume_rate     = number<br>    resume_timeout  = number<br>    suspend_rate    = number<br>    suspend_timeout = number<br>  })</pre> | <pre>{<br>  "no_comma_params": false,<br>  "resume_rate": 0,<br>  "resume_timeout": 300,<br>  "suspend_rate": 0,<br>  "suspend_timeout": 300<br>}</pre> | no |
| <a name="input_compute_startup_script"></a> [compute\_startup\_script](#input\_compute\_startup\_script) | Startup script used by the compute VMs. | `string` | `""` | no |
| <a name="input_disable_default_mounts"></a> [disable\_default\_mounts](#input\_disable\_default\_mounts) | Disable default global network storage from the controller<br>- /usr/local/etc/slurm<br>- /etc/munge<br>- /home<br>- /apps<br>If these are disabled, the slurm etc and munge dirs must be added manually,<br>or some other mechanism must be used to synchronize the slurm conf files<br>and the munge key across the cluster. | `bool` | `false` | no |
| <a name="input_enable_bigquery_load"></a> [enable\_bigquery\_load](#input\_enable\_bigquery\_load) | Enables loading of cluster job usage into big query.<br>NOTE: Requires Google Bigquery API. | `bool` | `false` | no |
| <a name="input_enable_cleanup_compute"></a> [enable\_cleanup\_compute](#input\_enable\_cleanup\_compute) | Enables automatic cleanup of compute nodes and resource policies (e.g.<br>placement groups) managed by this module, when cluster is destroyed.<br>NOTE: Requires Python and script dependencies.<br>*WARNING*: Toggling this may impact the running workload. Deployed compute nodes<br>may be destroyed and their jobs will be requeued. | `bool` | `false` | no |
| <a name="input_enable_cleanup_subscriptions"></a> [enable\_cleanup\_subscriptions](#input\_enable\_cleanup\_subscriptions) | Enables automatic cleanup of pub/sub subscriptions managed by this module, when<br>cluster is destroyed.<br>NOTE: Requires Python and script dependencies.<br>*WARNING*: Toggling this may temporarily impact var.enable\_reconfigure behavior. | `bool` | `false` | no |
| <a name="input_enable_devel"></a> [enable\_devel](#input\_enable\_devel) | Enables development mode. Not for production use. | `bool` | `false` | no |
| <a name="input_enable_reconfigure"></a> [enable\_reconfigure](#input\_enable\_reconfigure) | Enables automatic Slurm reconfigure on when Slurm configuration changes (e.g.<br>slurm.conf.tpl, partition details). Compute instances and resource policies<br>(e.g. placement groups) will be destroyed to align with new configuration.<br>NOTE: Requires Python and Google Pub/Sub API.<br>*WARNING*: Toggling this will impact the running workload. Deployed compute nodes<br>will be destroyed and their jobs will be requeued. | `bool` | `false` | no |
| <a name="input_epilog_scripts"></a> [epilog\_scripts](#input\_epilog\_scripts) | List of scripts to be used for Epilog. Programs for the slurmd to execute<br>on every node when a user's job completes.<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_Epilog. | <pre>list(object({<br>    filename = string<br>    content  = string<br>  }))</pre> | `[]` | no |
| <a name="input_google_app_cred_path"></a> [google\_app\_cred\_path](#input\_google\_app\_cred\_path) | Path to Google Applicaiton Credentials. | `string` | `null` | no |
| <a name="input_login_network_storage"></a> [login\_network\_storage](#input\_login\_network\_storage) | Storage to mounted on login and controller instances<br>* server\_ip     : Address of the storage server.<br>* remote\_mount  : The location in the remote instance filesystem to mount from.<br>* local\_mount   : The location on the instance filesystem to mount to.<br>* fs\_type       : Filesystem type (e.g. "nfs").<br>* mount\_options : Options to mount with. | <pre>list(object({<br>    server_ip     = string<br>    remote_mount  = string<br>    local_mount   = string<br>    fs_type       = string<br>    mount_options = string<br>  }))</pre> | `[]` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | Storage to mounted on all instances.<br>- server\_ip     : Address of the storage server.<br>- remote\_mount  : The location in the remote instance filesystem to mount from.<br>- local\_mount   : The location on the instance filesystem to mount to.<br>- fs\_type       : Filesystem type (e.g. "nfs").<br>- mount\_options : Options to mount with. | <pre>list(object({<br>    server_ip     = string<br>    remote_mount  = string<br>    local_mount   = string<br>    fs_type       = string<br>    mount_options = string<br>  }))</pre> | `[]` | no |
| <a name="input_output_dir"></a> [output\_dir](#input\_output\_dir) | Directory where this module will write its files to. These files include:<br>cloud.conf; cloud\_gres.conf; config.yaml; resume.py; suspend.py; and util.py. | `string` | `null` | no |
| <a name="input_partition"></a> [partition](#input\_partition) | Cluster partitions as a list. | <pre>list(object({<br>    compute_list = list(string)<br>    partition = object({<br>      enable_job_exclusive    = bool<br>      enable_placement_groups = bool<br>      network_storage = list(object({<br>        server_ip     = string<br>        remote_mount  = string<br>        local_mount   = string<br>        fs_type       = string<br>        mount_options = string<br>      }))<br>      partition_conf = map(string)<br>      partition_name = string<br>      partition_nodes = map(object({<br>        node_count_dynamic_max = number<br>        node_count_static      = number<br>        enable_spot_vm         = bool<br>        group_name             = string<br>        instance_template      = string<br>        node_conf              = map(string)<br>        spot_instance_config = object({<br>          termination_action = string<br>        })<br>      }))<br>      subnetwork        = string<br>      zone_policy_allow = list(string)<br>      zone_policy_deny  = list(string)<br>    })<br>  }))</pre> | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_prolog_scripts"></a> [prolog\_scripts](#input\_prolog\_scripts) | List of scripts to be used for Prolog. Programs for the slurmd to execute<br>whenever it is asked to run a job step from a new job allocation.<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_Prolog. | <pre>list(object({<br>    filename = string<br>    content  = string<br>  }))</pre> | `[]` | no |
| <a name="input_slurm_bin_dir"></a> [slurm\_bin\_dir](#input\_slurm\_bin\_dir) | Path to directroy of Slurm binary commands (e.g. scontrol, sinfo). If 'null',<br>then it will be assumed that binaries are in $PATH. | `string` | `null` | no |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Cluster name, used for resource naming and slurm accounting. | `string` | n/a | yes |
| <a name="input_slurm_control_host"></a> [slurm\_control\_host](#input\_slurm\_control\_host) | The short, or long, hostname of the machine where Slurm control daemon is<br>executed (i.e. the name returned by the command "hostname -s").<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_SlurmctldHost | `string` | `null` | no |
| <a name="input_slurm_depends_on"></a> [slurm\_depends\_on](#input\_slurm\_depends\_on) | Custom terraform dependencies without replacement on delta. This is useful to<br>ensure order of resource creation.<br>NOTE: Also see terraform meta-argument 'depends\_on'. | `list(string)` | `[]` | no |
| <a name="input_slurm_log_dir"></a> [slurm\_log\_dir](#input\_slurm\_log\_dir) | Directory where Slurm logs to. | `string` | `"/var/log/slurm"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Installation instructions for the hybrid configurations created by this module. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
