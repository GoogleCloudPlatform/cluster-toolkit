## Description
This module is a wrapper around the [slurm-controller-hybrid] module by SchedMD
as part of the [slurm-gcp] github repository. The hybrid module serves to create
the configurations needed to extend an on-premise slurm cluster to one with one
or more Google Cloud bursting partitions. These partitions will create the
requested nodes in a GCP project on-demand and scale after a period of not being
used, in the same way as the [schedmd-slurm-gcp-v5-controller] module
auto-scales VMs.

Further documentation on how to use this module when deploying a hybrid Slurm
cluster can be found in our [docs](../../../../docs/hybrid-slurm-cluster/). There, you can
find two tutorials. The [first] tutorial walks you through deploying a test
environment entirely in GCP that is designed to demonstrate the capabilities
without needing to make any changes to your local slurm cluster. The [second]
tutorial goes through the process of deploying the hybrid configuration onto a
on-premise slurm cluster.

> **_NOTE:_** This is an experimental module and the functionality and
> documentation will likely be updated in the near future. This module has only
> been tested in limited capacity with the HPC Toolkit. On Premise
> Slurm configurations can vary significantly, this module should
> be used as a starting point, not a complete solution.

[schedmd-slurm-gcp-v5-controller]: ../schedmd-slurm-gcp-v5-controller/
[first]: ../../../../docs/hybrid-slurm-cluster/README.md#demo-with-cloud-controller-instructionsmd
[second]: ../../../../docs/hybrid-slurm-cluster/README.md#on-prem-instructionsmd

### Usage
The [slurm-controller-hybrid] is intended to be run on the controller of the on
premise slurm cluster, meaning executing `terraform init/apply` against the
deployment directory. This allows the module to infer settings such as the
slurm user and user ID when setting permissions for the created configurations.

If unable to install terraform and other dependencies on the controller
directly, it is possible to deploy the hybrid module in a separate build
environment and copy the created configurations to the on premise controller
manually. This will require addition configuration and verification of
permissions. For more information see the [hybrid.md] documentation on
[slurm-gcp].

[slurm-controller-hybrid]: https://github.com/SchedMD/slurm-gcp/tree/5.9.1/terraform/slurm_cluster/modules/slurm_controller_hybrid

> **_NOTE:_** The hybrid module requires the following dependencies to be
> installed on the system deploying the module:
>
> * [terraform]
> * [addict]
> * [httplib2]
> * [pyyaml]
> * [google-api-python-client]
> * [google-cloud-pubsub]
> * A full list of recommended python packages is available in a
>   [requirements.txt] file in the [slurm-gcp] repo.

[terraform]: https://learn.hashicorp.com/tutorials/terraform/install-cli
[addict]: https://pypi.org/project/addict/
[httplib2]: https://pypi.org/project/httplib2/
[pyyaml]: https://pypi.org/project/PyYAML/
[google-api-python-client]: https://pypi.org/project/google-api-python-client/
[google-cloud-pubsub]: https://pypi.org/project/google-cloud-pubsub/
[requirements.txt]: https://github.com/SchedMD/slurm-gcp/blob/5.9.1/scripts/requirements.txt

### Manual Configuration
This module *does not* complete the installation of hybrid partitions on your
slurm cluster. After deploying, you must follow the steps listed out in the
[hybrid.md] documentation under [manual steps].

[hybrid.md]: https://github.com/SchedMD/slurm-gcp/blob/5.9.1/docs/hybrid.md
[manual steps]: https://github.com/SchedMD/slurm-gcp/blob/5.9.1/docs/hybrid.md#manual-configurations

### Example Usage
The hybrid module can be added to a blueprint as follows:

```yaml
- id: slurm-controller
  source: ./community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid
  use:
  - debug-partition
  - compute-partition
  - pre-existing-storage
  settings:
    output_dir: ./hybrid
    slurm_bin_dir: /usr/local/bin
    slurm_control_host: static-controller
```

This defines a HPC module that create a hybrid configuration with the following
attributes:

* 2 partitions defined in previous modules with the IDs of `debug-partition` and
  `compute-partition`. These are the same partition modules used by
  [schedmd-slurm-gcp-v5-controller].
* Network storage to be mounted on the compute nodes when created, defined in
  `pre-existing-storage`.
* `output_directory` set to `./hybrid`. This is where the hybrid
  configurations will be created.
* `slurm_bin_dir` located at `/usr/local/bin`. Set this to wherever the slurm
  executables are installed on your system.
* `slurm_control_host`: The name of the on premise host is provided to the
  module for configuring NFS mounts and communicating with the controller after
  VM creation.

[schedmd-slurm-gcp-v5-controller]: ../schedmd-slurm-gcp-v5-controller/

### Assumptions and Limitations
**Shared directories from the controller:** By default, the following
directories are NFS mounted from the on premise controller to the created cloud
VMs:
* /home
* /opt/apps
* /etc/munge
* /usr/local/slurm/etc

The expectation is that these directories exist on the controller and that all
files required by slurmd to be in sync with the controller are in those
directories.

If this does not match your slurm cluster, these directories can be overwritten
with a custom NFS mount using [pre-existing-network-storage] or by setting the
`network_storage` variable directly in the hybrid module. **Any value in
`network_storage`, added directly or with `use`, will override the default
directories above.**

The variable `disable_default_mounts` will disregard these defaults. Note that
at a minimum, the cloud VMs require `/etc/munge` and `/usr/local/slurm/etc` to
be mounted from the controller. Those will need to be managed manually if the
`disable_default_mounts` variable is set to true.

**Power Saving Logic:** The cloud partitions will make use of the power saving
logic and the suspend and resume programs will be set. If any local partitions
also make use of these `slurm.conf` variables, a conflict will likely occur.
There is no support currently for partition level suspend and resume scripts,
therefore either the local partition will need to turn this off or the hybrid
module will not work.

**Slurm versions:** The version of slurm on the on premise cluster must match the
slurm version on the cloud VMs created by the hybrid partitions. The version
on the cloud VMs will be dictated by the version on the disk image that can be
set when defining the partitions using [schedmd-slurm-gcp-v5-partition].

If the publicly available images do not suffice, [slurm-gcp] provides
[packer templates] for creating custom disk images.

SchedMD only supports the current and last major version of slurm, therefore we
strongly advise only using versions 21 or 22 when using this module. Attempting
to use this module with any version older than 21 may lead to unexpected
results.

[slurm-gcp]: https://github.com/SchedMD/slurm-gcp/tree/5.9.1
[pre-existing-network-storage]: ../../../../modules/file-system/pre-existing-network-storage/
[schedmd-slurm-gcp-v5-partition]: ../../compute/schedmd-slurm-gcp-v5-partition/
[packer templates]: https://github.com/SchedMD/slurm-gcp/tree/5.9.1/packer

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
| <a name="module_slurm_controller_instance"></a> [slurm\_controller\_instance](#module\_slurm\_controller\_instance) | github.com/GoogleCloudPlatform/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_controller_hybrid | 5.9.1 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_cloud_parameters"></a> [cloud\_parameters](#input\_cloud\_parameters) | cloud.conf options. | <pre>object({<br>    no_comma_params = bool<br>    resume_rate     = number<br>    resume_timeout  = number<br>    suspend_rate    = number<br>    suspend_timeout = number<br>  })</pre> | <pre>{<br>  "no_comma_params": false,<br>  "resume_rate": 0,<br>  "resume_timeout": 300,<br>  "suspend_rate": 0,<br>  "suspend_timeout": 300<br>}</pre> | no |
| <a name="input_compute_startup_script"></a> [compute\_startup\_script](#input\_compute\_startup\_script) | Startup script used by the compute VMs. | `string` | `""` | no |
| <a name="input_compute_startup_scripts_timeout"></a> [compute\_startup\_scripts\_timeout](#input\_compute\_startup\_scripts\_timeout) | The timeout (seconds) applied to the compute\_startup\_script. If<br>any script exceeds this timeout, then the instance setup process is considered<br>failed and handled accordingly.<br><br>NOTE: When set to 0, the timeout is considered infinite and thus disabled. | `number` | `300` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment. | `string` | n/a | yes |
| <a name="input_disable_default_mounts"></a> [disable\_default\_mounts](#input\_disable\_default\_mounts) | Disable default global network storage from the controller: /usr/local/etc/slurm,<br>/etc/munge, /home, /apps.<br>If these are disabled, the slurm etc and munge dirs must be added manually,<br>or some other mechanism must be used to synchronize the slurm conf files<br>and the munge key across the cluster. | `bool` | `false` | no |
| <a name="input_enable_bigquery_load"></a> [enable\_bigquery\_load](#input\_enable\_bigquery\_load) | Enables loading of cluster job usage into big query.<br>NOTE: Requires Google Bigquery API. | `bool` | `false` | no |
| <a name="input_enable_cleanup_compute"></a> [enable\_cleanup\_compute](#input\_enable\_cleanup\_compute) | Enables automatic cleanup of compute nodes and resource policies (e.g.<br>placement groups) managed by this module, when cluster is destroyed.<br>NOTE: Requires Python and script dependencies.<br>*WARNING*: Toggling this may impact the running workload. Deployed compute nodes<br>may be destroyed and their jobs will be requeued. | `bool` | `false` | no |
| <a name="input_enable_cleanup_subscriptions"></a> [enable\_cleanup\_subscriptions](#input\_enable\_cleanup\_subscriptions) | Enables automatic cleanup of pub/sub subscriptions managed by this module, when<br>cluster is destroyed.<br>NOTE: Requires Python and script dependencies.<br>*WARNING*: Toggling this may temporarily impact var.enable\_reconfigure behavior. | `bool` | `false` | no |
| <a name="input_enable_devel"></a> [enable\_devel](#input\_enable\_devel) | Enables development mode. Not for production use. | `bool` | `false` | no |
| <a name="input_enable_reconfigure"></a> [enable\_reconfigure](#input\_enable\_reconfigure) | Enables automatic Slurm reconfigure on when Slurm configuration changes (e.g.<br>slurm.conf.tpl, partition details). Compute instances and resource policies<br>(e.g. placement groups) will be destroyed to align with new configuration.<br>NOTE: Requires Python and Google Pub/Sub API.<br>*WARNING*: Toggling this will impact the running workload. Deployed compute nodes<br>will be destroyed and their jobs will be requeued. | `bool` | `false` | no |
| <a name="input_enable_slurm_gcp_plugins"></a> [enable\_slurm\_gcp\_plugins](#input\_enable\_slurm\_gcp\_plugins) | Enables calling hooks in scripts/slurm\_gcp\_plugins during cluster resume and suspend. | `bool` | `false` | no |
| <a name="input_epilog_scripts"></a> [epilog\_scripts](#input\_epilog\_scripts) | List of scripts to be used for Epilog. Programs for the slurmd to execute<br>on every node when a user's job completes.<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_Epilog. | <pre>list(object({<br>    filename = string<br>    content  = string<br>  }))</pre> | `[]` | no |
| <a name="input_google_app_cred_path"></a> [google\_app\_cred\_path](#input\_google\_app\_cred\_path) | Path to Google Application Credentials. | `string` | `null` | no |
| <a name="input_install_dir"></a> [install\_dir](#input\_install\_dir) | Directory where the hybrid configuration directory will be installed on the<br>on-premise controller. This updates the prefix path for the resume and<br>suspend scripts in the generated `cloud.conf` file. The value defaults to<br>output\_dir if not specified. | `string` | `null` | no |
| <a name="input_munge_mount"></a> [munge\_mount](#input\_munge\_mount) | Remote munge mount for compute and login nodes to acquire the munge.key.<br><br>By default, the munge mount server will be assumed to be the<br>`var.slurm_control_host` (or `var.slurm_control_addr` if non-null) when<br>`server_ip=null`. | <pre>object({<br>    server_ip     = string<br>    remote_mount  = string<br>    fs_type       = string<br>    mount_options = string<br>  })</pre> | <pre>{<br>  "fs_type": "nfs",<br>  "mount_options": "",<br>  "remote_mount": "/etc/munge/",<br>  "server_ip": null<br>}</pre> | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on all instances. | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_output_dir"></a> [output\_dir](#input\_output\_dir) | Directory where this module will write its files to. These files include:<br>cloud.conf; cloud\_gres.conf; config.yaml; resume.py; suspend.py; and util.py.<br>If not specified explicitly, this will also be used as the default value<br>for the `install_dir` variable. | `string` | `null` | no |
| <a name="input_partition"></a> [partition](#input\_partition) | Cluster partitions as a list. | <pre>list(object({<br>    compute_list = list(string)<br>    partition = object({<br>      enable_job_exclusive    = bool<br>      enable_placement_groups = bool<br>      network_storage = list(object({<br>        server_ip     = string<br>        remote_mount  = string<br>        local_mount   = string<br>        fs_type       = string<br>        mount_options = string<br>      }))<br>      partition_conf    = map(string)<br>      partition_feature = string<br>      partition_name    = string<br>      partition_nodes = map(object({<br>        bandwidth_tier         = string<br>        node_count_dynamic_max = number<br>        node_count_static      = number<br>        enable_spot_vm         = bool<br>        group_name             = string<br>        instance_template      = string<br>        node_conf              = map(string)<br>        access_config = list(object({<br>          nat_ip       = string<br>          network_tier = string<br>        }))<br>        spot_instance_config = object({<br>          termination_action = string<br>        })<br>      }))<br>      partition_startup_scripts_timeout = number<br>      subnetwork                        = string<br>      zone_policy_allow                 = list(string)<br>      zone_policy_deny                  = list(string)<br>      zone_target_shape                 = string<br>    })<br>  }))</pre> | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_prolog_scripts"></a> [prolog\_scripts](#input\_prolog\_scripts) | List of scripts to be used for Prolog. Programs for the slurmd to execute<br>whenever it is asked to run a job step from a new job allocation.<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_Prolog. | <pre>list(object({<br>    filename = string<br>    content  = string<br>  }))</pre> | `[]` | no |
| <a name="input_slurm_bin_dir"></a> [slurm\_bin\_dir](#input\_slurm\_bin\_dir) | Path to directory of Slurm binary commands (e.g. scontrol, sinfo). If 'null',<br>then it will be assumed that binaries are in $PATH. | `string` | `null` | no |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Cluster name, used for resource naming and slurm accounting. If not provided<br>it will default to the first 8 characters of the deployment name (removing<br>any invalid characters). | `string` | `null` | no |
| <a name="input_slurm_control_addr"></a> [slurm\_control\_addr](#input\_slurm\_control\_addr) | The IP address or a name by which the address can be identified.<br>This value is passed to slurm.conf such that:<br>SlurmctldHost={var.slurm\_control\_host}\({var.slurm\_control\_addr}\)<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_SlurmctldHost | `string` | `null` | no |
| <a name="input_slurm_control_host"></a> [slurm\_control\_host](#input\_slurm\_control\_host) | The short, or long, hostname of the machine where Slurm control daemon is<br>executed (i.e. the name returned by the command "hostname -s").<br>This value is passed to slurm.conf such that:<br>SlurmctldHost={var.slurm\_control\_host}\({var.slurm\_control\_addr}\)<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_SlurmctldHost | `string` | n/a | yes |
| <a name="input_slurm_control_host_port"></a> [slurm\_control\_host\_port](#input\_slurm\_control\_host\_port) | The port number that the Slurm controller, slurmctld, listens to for work.<br>See https://slurm.schedmd.com/slurm.conf.html#OPT_SlurmctldPort | `string` | `null` | no |
| <a name="input_slurm_log_dir"></a> [slurm\_log\_dir](#input\_slurm\_log\_dir) | Directory where Slurm logs to. | `string` | `"/var/log/slurm"` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
