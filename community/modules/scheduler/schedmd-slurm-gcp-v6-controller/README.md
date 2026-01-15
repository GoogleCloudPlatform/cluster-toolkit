## Description

This module creates a slurm controller node via the internal
[slurm\_instance\_template] module.

More information about Slurm On GCP can be found at the
[project's GitHub page][slurm-gcp] and in the
[Slurm on Google Cloud User Guide][slurm-ug].

The [user guide][slurm-ug] provides detailed instructions on customizing and
enhancing the Slurm on GCP cluster as well as recommendations on configuring the
controller for optimal performance at different scales.

[slurm\_instance\_template]: /community/modules/internal/slurm-gcp/instance_template/README.md
[slurm-ug]: https://goo.gle/slurm-gcp-user-guide.
[enable\_cleanup\_compute]: #input\_enable\_cleanup\_compute
[enable\_cleanup\_subscriptions]: #input\_enable\_cleanup\_subscriptions
[enable\_reconfigure]: #input\_enable\_reconfigure

### Example

```yaml
- id: slurm_controller
  source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
  use:
  - network
  - homefs
  - compute_partition
  settings:
    machine_type: c2-standard-8
```

This creates a controller node with the following attributes:

* connected to the primary subnetwork of `network`
* the filesystem with the ID `homefs` (defined elsewhere in the blueprint)
  mounted
* One partition with the ID `compute_partition` (defined elsewhere in the
  blueprint)
* machine type upgraded from the default `c2-standard-4` to `c2-standard-8`

### Live Cluster Reconfiguration

The `schedmd-slurm-gcp-v6-controller` module supports the reconfiguration of
partitions and slurm configuration in a running, active cluster.

To reconfigure a running cluster:

1. Edit the blueprint with the desired configuration changes
2. Call `gcluster create <blueprint> -w` to overwrite the deployment directory
3. Follow instructions in terminal to deploy

The following are examples of updates that can be made to a running cluster:

* Add or remove a partition to the cluster
* Resize an existing partition
* Attach new network storage to an existing partition

> **NOTE**: Changing the VM `machine_type` of a partition may not work.
> It is better to create a new partition and delete the old one.

## Custom Images

For more information on creating valid custom images for the controller VM
instance or for custom instance templates, see our [vm-images.md] documentation
page.

[vm-images.md]: ../../../../docs/vm-images.md#slurm-on-gcp-custom-images

## GPU Support

More information on GPU support in Slurm on GCP and other Cluster Toolkit modules
can be found at [docs/gpu-support.md](../../../../docs/gpu-support.md)

## Reservation for Scheduled Maintenance

A [maintenance event](https://cloud.google.com/compute/docs/instances/host-maintenance-overview#maintenanceevents) is when a compute engine stops a VM to perform a hardware or
software update which is determined by the host maintenance policy. This can
also affect the running jobs if the maintenance kicks in. Now, Customers can
protect jobs from getting terminated due to maintenance using the cluster
toolkit. You can enable creation of reservation for scheduled maintenance for
your compute nodeset and Slurm will reserve your node for maintenance during the
maintenance window. If you try to schedule any jobs which overlap with the
maintenance reservation, Slurm would not schedule any job.

You can specify in your blueprint like

```yaml
  - id: compute_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network]
    settings:
      enable_maintenance_reservation: true
```

To enable creation of reservation for maintenance.

While running job on slurm cluster, you can specify total run time of the job
using [-t flag](https://slurm.schedmd.com/srun.html#OPT_time).This would only
run the job outside of the maintenance window.

```shell
srun -n1 -pcompute -t 10:00 <job.sh>
```

Currently upcoming maintenance notification is supported in ALPHA version of
compute API. You can update the API version from your blueprint,

```yaml
  - id: slurm_controller
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
    settings:
      endpoint_versions:
        compute: "alpha"
```

## Opportunistic GCP maintenance in Slurm

Customers can also enable running GCP maintenance as Slurm job opportunistically
to perform early maintenance. If a node is detected for maintenance, Slurm will
create a job to perform maintenance and put it in the job queue.

If [backfill](https://slurm.schedmd.com/sched_config.html#backfill) scheduler is
used, Slurm will backfill maintenance job if it can find any empty time window.

Customer can also choose builtin scheduler type. In this case, Slurm would run
maintenance job in strictly priority order. If the maintenance job doesn't kick
in, then forced maintenance will take place at scheduled window.

Customer can enable this feature at nodeset level by,

```yaml
  - id: debug_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network]
    settings:
      enable_opportunistic_maintenance: true
```

## Placement Max Distance

When using
[enable_placement](../../../../community/modules/compute/schedmd-slurm-gcp-v6-nodeset/README.md#input_enable_placement)
with Slurm, Google Compute Engine will attempt to place VMs as physically close
together as possible. Capacity constraints at the time of VM creation may still
force VMs to be spread across multiple racks. Google provides the `max-distance`
flag which can used to control the maximum spreading allowed. Read more about
`max-distance` in the
[official docs](https://cloud.google.com/compute/docs/instances/use-compact-placement-policies
).

You can use the `placement_max_distance` setting on the nodeset module to control the `max-distance` behavior. See the following example:

```yaml
  - id: nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [ network ]
    settings:
      machine_type: c2-standard-4
      node_count_dynamic_max: 30
      enable_placement: true
      placement_max_distance: 1

> [!NOTE]
> `schedmd-slurm-gcp-v6-nodeset.settings.enable_placement: true` must also be
> set for placement_max_distance to take effect.

In the above case using a value of 1 will restrict VM to be placed on the same
rack. You can confirm that the `max-distance` was applied by calling the
following command while jobs are running:

```shell
gcloud beta compute resource-policies list \
  --format='yaml(name,groupPlacementPolicy.maxDistance)'
```

> [!WARNING]
> If a zone lacks capacity, using a lower `max-distance` value (such as 1) is
> more likely to cause VMs creation to fail.

## TreeWidth and Node Communication

Slurm uses a fan out mechanism to communicate large groups of nodes. The shape
of this fan out tree is determined by the
[TreeWidth](https://slurm.schedmd.com/slurm.conf.html#OPT_TreeWidth)
configuration variable.

In the cloud, this fan out mechanism can become unstable when nodes restart with
new IP addresses. You can enforce that all nodes communicate directly with the
controller by setting TreeWidth to a value >= largest partition.

If the largest partition was 200 nodes, configure the blueprint as follows:

```yaml
  - id: slurm_controller
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
    ...
    settings:
      cloud_parameters:
        tree_width: 200
```

The default has been set to 128. Values above this have not been fully tested
and may cause congestion on the controller. A more scalable solution is under
way.

## ResumeRate and Node Resumption

The `ResumeRate` parameter in `slurm.conf` controls the maximum number of nodes
that Slurm attempts to resume (power up) per minute. This is particularly
important in cloud environments where auto-scaling can lead to a large number of
nodes starting concurrently.

When many nodes start simultaneously, they can place a heavy load on shared
resources, especially shared filesystems, as they all try to mount filesystems
and access configuration files at the same time. By limiting the `ResumeRate`,
you can stagger the node startup process, reducing the peak load on these shared
resources and improving overall cluster stability during scaling events.

For example, to limit the node resumption rate to 100 nodes per minute,
configure the blueprint as follows:

```yaml
  - id: slurm_controller
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
    ...
    settings:
      cloud_parameters:
        resume_rate: 100
```

Adjust this value based on the capabilities of your shared filesystem and the
expected scaling behavior of your cluster.

## Support
The Cluster Toolkit team maintains the wrapper around the [slurm-on-gcp] terraform
modules. For support with the underlying modules, see the instructions in the
[slurm-gcp README][slurm-gcp-readme].

[slurm-on-gcp]: https://github.com/GoogleCloudPlatform/slurm-gcp
[slurm-gcp-readme]: https://github.com/GoogleCloudPlatform/slurm-gcp#slurm-on-google-cloud-platform

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2023 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.41 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 6.0.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.41 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 6.0.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_bucket"></a> [bucket](#module\_bucket) | terraform-google-modules/cloud-storage/google | >= 6.1 |
| <a name="module_daos_network_storage_scripts"></a> [daos\_network\_storage\_scripts](#module\_daos\_network\_storage\_scripts) | ../../../../modules/scripts/startup-script | n/a |
| <a name="module_gpu"></a> [gpu](#module\_gpu) | ../../../../modules/internal/gpu-definition | n/a |
| <a name="module_login"></a> [login](#module\_login) | ../../internal/slurm-gcp/login | n/a |
| <a name="module_nodeset_cleanup"></a> [nodeset\_cleanup](#module\_nodeset\_cleanup) | ./modules/cleanup_compute | n/a |
| <a name="module_nodeset_cleanup_tpu"></a> [nodeset\_cleanup\_tpu](#module\_nodeset\_cleanup\_tpu) | ./modules/cleanup_tpu | n/a |
| <a name="module_slurm_controller_template"></a> [slurm\_controller\_template](#module\_slurm\_controller\_template) | ../../internal/slurm-gcp/instance_template | n/a |
| <a name="module_slurm_files"></a> [slurm\_files](#module\_slurm\_files) | ./modules/slurm_files | n/a |
| <a name="module_slurm_nodeset_template"></a> [slurm\_nodeset\_template](#module\_slurm\_nodeset\_template) | ../../internal/slurm-gcp/instance_template | n/a |
| <a name="module_slurm_nodeset_tpu"></a> [slurm\_nodeset\_tpu](#module\_slurm\_nodeset\_tpu) | ../../internal/slurm-gcp/nodeset_tpu | n/a |

## Resources

| Name | Type |
|------|------|
| [google-beta_google_compute_instance_from_template.controller](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_instance_from_template) | resource |
| [google_compute_disk.controller_disk](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk) | resource |
| [google_secret_manager_secret.cloudsql](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret) | resource |
| [google_secret_manager_secret_iam_member.cloudsql_secret_accessor](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_iam_member) | resource |
| [google_secret_manager_secret_version.cloudsql_version](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/secret_manager_secret_version) | resource |
| [google_storage_bucket_iam_member.legacy_readers](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_iam_member) | resource |
| [google_storage_bucket_iam_member.viewers](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_iam_member) | resource |
| [google_storage_bucket_object.parition_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_project.controller_project](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/project) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_additional_disks"></a> [additional\_disks](#input\_additional\_disks) | List of maps of disks. | <pre>list(object({<br/>    disk_name                  = string<br/>    device_name                = string<br/>    disk_type                  = string<br/>    disk_size_gb               = number<br/>    disk_labels                = map(string)<br/>    auto_delete                = bool<br/>    boot                       = bool<br/>    disk_resource_manager_tags = map(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_advanced_machine_features"></a> [advanced\_machine\_features](#input\_advanced\_machine\_features) | See https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance_template#nested_advanced_machine_features | <pre>object({<br/>    enable_nested_virtualization = optional(bool)<br/>    threads_per_core             = optional(number)<br/>    turbo_mode                   = optional(string)<br/>    visible_core_count           = optional(number)<br/>    performance_monitoring_unit  = optional(string)<br/>    enable_uefi_networking       = optional(bool)<br/>  })</pre> | <pre>{<br/>  "threads_per_core": 1<br/>}</pre> | no |
| <a name="input_allow_automatic_updates"></a> [allow\_automatic\_updates](#input\_allow\_automatic\_updates) | If false, disables automatic system package updates on the created instances.  This feature is<br/>only available on supported images (or images derived from them).  For more details, see<br/>https://cloud.google.com/compute/docs/instances/create-hpc-vm#disable_automatic_updates | `bool` | `true` | no |
| <a name="input_bandwidth_tier"></a> [bandwidth\_tier](#input\_bandwidth\_tier) | Configures the network interface card and the maximum egress bandwidth for VMs.<br/>  - Setting `platform_default` respects the Google Cloud Platform API default values for networking.<br/>  - Setting `virtio_enabled` explicitly selects the VirtioNet network adapter.<br/>  - Setting `gvnic_enabled` selects the gVNIC network adapter (without Tier 1 high bandwidth).<br/>  - Setting `tier_1_enabled` selects both the gVNIC adapter and Tier 1 high bandwidth networking.<br/>  - Note: both gVNIC and Tier 1 networking require a VM image with gVNIC support as well as specific VM families and shapes.<br/>  - See [official docs](https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration) for more details. | `string` | `"platform_default"` | no |
| <a name="input_bucket_dir"></a> [bucket\_dir](#input\_bucket\_dir) | Bucket directory for cluster files to be put into. If not specified, then one will be chosen based on slurm\_cluster\_name. | `string` | `null` | no |
| <a name="input_bucket_name"></a> [bucket\_name](#input\_bucket\_name) | Name of GCS bucket.<br/>Ignored when 'create\_bucket' is true. | `string` | `null` | no |
| <a name="input_can_ip_forward"></a> [can\_ip\_forward](#input\_can\_ip\_forward) | Enable IP forwarding, for NAT instances for example. | `bool` | `false` | no |
| <a name="input_cgroup_conf_tpl"></a> [cgroup\_conf\_tpl](#input\_cgroup\_conf\_tpl) | Slurm cgroup.conf template file path. | `string` | `null` | no |
| <a name="input_cloud_parameters"></a> [cloud\_parameters](#input\_cloud\_parameters) | cloud.conf options. Defaults inherited from [Slurm GCP repo](https://github.com/GoogleCloudPlatform/slurm-gcp/blob/master/terraform/slurm_cluster/modules/slurm_files/README_TF.md#input_cloud_parameters) | <pre>object({<br/>    no_comma_params         = optional(bool, false)<br/>    private_data            = optional(list(string))<br/>    scheduler_parameters    = optional(list(string))<br/>    resume_rate             = optional(number)<br/>    resume_timeout          = optional(number)<br/>    suspend_rate            = optional(number)<br/>    suspend_timeout         = optional(number)<br/>    slurmd_timeout          = optional(number)<br/>    unkillable_step_timeout = optional(number)<br/>    topology_plugin         = optional(string)<br/>    topology_param          = optional(string)<br/>    tree_width              = optional(number)<br/>    prolog_flags            = optional(string)<br/>    switch_type             = optional(string)<br/>  })</pre> | `{}` | no |
| <a name="input_cloudsql"></a> [cloudsql](#input\_cloudsql) | Use this database instead of the one on the controller.<br/>  server\_ip : Address of the database server.<br/>  user      : The user to access the database as.<br/>  password  : The password, given the user, to access the given database. (sensitive)<br/>  db\_name   : The database to access.<br/>  user\_managed\_replication : The list of location and (optional) kms\_key\_name for secret | <pre>object({<br/>    server_ip = string<br/>    user      = string<br/>    password  = string # sensitive<br/>    db_name   = string<br/>    user_managed_replication = optional(list(object({<br/>      location     = string<br/>      kms_key_name = optional(string)<br/>    })), [])<br/>  })</pre> | `null` | no |
| <a name="input_compute_startup_script"></a> [compute\_startup\_script](#input\_compute\_startup\_script) | DEPRECATED: `compute_startup_script` has been deprecated.<br/>Use `startup_script` of nodeset module instead. | `any` | `null` | no |
| <a name="input_compute_startup_scripts_timeout"></a> [compute\_startup\_scripts\_timeout](#input\_compute\_startup\_scripts\_timeout) | The timeout (seconds) applied to each startup script in compute nodes. If<br/>any script exceeds this timeout, then the instance setup process is considered<br/>failed and handled accordingly.<br/><br/>NOTE: When set to 0, the timeout is considered infinite and thus disabled. | `number` | `300` | no |
| <a name="input_controller_network_attachment"></a> [controller\_network\_attachment](#input\_controller\_network\_attachment) | SelfLink for NetworkAttachment to be attached to the controller, if any. | `string` | `null` | no |
| <a name="input_controller_project_id"></a> [controller\_project\_id](#input\_controller\_project\_id) | Optionally. Provision controller and config bucket in the different project | `string` | `null` | no |
| <a name="input_controller_startup_script"></a> [controller\_startup\_script](#input\_controller\_startup\_script) | Startup script used by the controller VM. | `string` | `"# no-op"` | no |
| <a name="input_controller_startup_scripts_timeout"></a> [controller\_startup\_scripts\_timeout](#input\_controller\_startup\_scripts\_timeout) | The timeout (seconds) applied to each script in controller\_startup\_scripts. If<br/>any script exceeds this timeout, then the instance setup process is considered<br/>failed and handled accordingly.<br/><br/>NOTE: When set to 0, the timeout is considered infinite and thus disabled. | `number` | `300` | no |
| <a name="input_controller_state_disk"></a> [controller\_state\_disk](#input\_controller\_state\_disk) | A disk that will be attached to the controller instance template to save state of slurm. The disk is created and used by default.<br/>  To disable this feature, set this variable to null.<br/><br/>  NOTE: This will not save the contents at /opt/apps and /home. To preserve those, they must be saved externally. | <pre>object({<br/>    type = string<br/>    size = number<br/>  })</pre> | <pre>{<br/>  "size": 50,<br/>  "type": "pd-ssd"<br/>}</pre> | no |
| <a name="input_create_bucket"></a> [create\_bucket](#input\_create\_bucket) | Create GCS bucket instead of using an existing one. | `bool` | `true` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment. | `string` | n/a | yes |
| <a name="input_disable_controller_public_ips"></a> [disable\_controller\_public\_ips](#input\_disable\_controller\_public\_ips) | DEPRECATED: Use `enable_controller_public_ips` instead. | `bool` | `null` | no |
| <a name="input_disable_default_mounts"></a> [disable\_default\_mounts](#input\_disable\_default\_mounts) | DEPRECATED: Use `enable_default_mounts` instead. | `bool` | `null` | no |
| <a name="input_disable_smt"></a> [disable\_smt](#input\_disable\_smt) | DEPRECATED: Use `advanced_machine_features.threads_per_core` instead. | `bool` | `null` | no |
| <a name="input_disk_auto_delete"></a> [disk\_auto\_delete](#input\_disk\_auto\_delete) | Whether or not the boot disk should be auto-deleted. | `bool` | `true` | no |
| <a name="input_disk_labels"></a> [disk\_labels](#input\_disk\_labels) | Labels specific to the boot disk. These will be merged with var.labels. | `map(string)` | `{}` | no |
| <a name="input_disk_resource_manager_tags"></a> [disk\_resource\_manager\_tags](#input\_disk\_resource\_manager\_tags) | (Optional) A set of key/value resource manager tag pairs to bind to the instance disks. Keys must be in the format tagKeys/{tag\_key\_id}, and values are in the format tagValues/456. | `map(string)` | `{}` | no |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Boot disk size in GB. | `number` | `50` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Boot disk type, can be either hyperdisk-balanced, pd-ssd, pd-standard, pd-balanced, or pd-extreme. | `string` | `"pd-ssd"` | no |
| <a name="input_enable_bigquery_load"></a> [enable\_bigquery\_load](#input\_enable\_bigquery\_load) | Enables loading of cluster job usage into big query.<br/><br/>NOTE: Requires Google Bigquery API. | `bool` | `false` | no |
| <a name="input_enable_chs_gpu_health_check_epilog"></a> [enable\_chs\_gpu\_health\_check\_epilog](#input\_enable\_chs\_gpu\_health\_check\_epilog) | Enable a Cluster Health Sacnner(CHS) GPU health check that slurmd executes as an epilog script after completing a job step from a new job allocation.<br/>Compute nodes that fail GPU health check during epilog will be marked as drained. Find more details at:<br/>https://github.com/GoogleCloudPlatform/cluster-toolkit/tree/main/docs/CHS-Slurm.md | `bool` | `false` | no |
| <a name="input_enable_chs_gpu_health_check_prolog"></a> [enable\_chs\_gpu\_health\_check\_prolog](#input\_enable\_chs\_gpu\_health\_check\_prolog) | Enable a Cluster Health Sacnner(CHS) GPU health check that slurmd executes as a prolog script whenever it is asked to run a job step from a new job allocation. Compute nodes that fail GPU health check during prolog will be marked as drained. Find more details at:<br/>https://github.com/GoogleCloudPlatform/cluster-toolkit/tree/main/docs/CHS-Slurm.md | `bool` | `false` | no |
| <a name="input_enable_cleanup_compute"></a> [enable\_cleanup\_compute](#input\_enable\_cleanup\_compute) | Enables automatic cleanup of compute nodes and resource policies (e.g.<br/>placement groups) managed by this module, when cluster is destroyed.<br/><br/>*WARNING*: Toggling this off will impact the running workload.<br/>Deployed compute nodes will be destroyed. | `bool` | `true` | no |
| <a name="input_enable_confidential_vm"></a> [enable\_confidential\_vm](#input\_enable\_confidential\_vm) | Enable the Confidential VM configuration. Note: the instance image must support option. | `bool` | `false` | no |
| <a name="input_enable_controller_public_ips"></a> [enable\_controller\_public\_ips](#input\_enable\_controller\_public\_ips) | If set to true. The controller will have a random public IP assigned to it. Ignored if access\_config is set. | `bool` | `false` | no |
| <a name="input_enable_debug_logging"></a> [enable\_debug\_logging](#input\_enable\_debug\_logging) | Enables debug logging mode. | `bool` | `false` | no |
| <a name="input_enable_default_mounts"></a> [enable\_default\_mounts](#input\_enable\_default\_mounts) | Enable default global network storage from the controller<br/>- /home<br/>- /opt/apps | `bool` | `true` | no |
| <a name="input_enable_devel"></a> [enable\_devel](#input\_enable\_devel) | DEPRECATED: `enable_devel` is always on. | `bool` | `null` | no |
| <a name="input_enable_external_prolog_epilog"></a> [enable\_external\_prolog\_epilog](#input\_enable\_external\_prolog\_epilog) | Automatically enable a script that will execute prolog and epilog scripts<br/>shared by NFS from the controller to compute nodes. Find more details at:<br/>https://github.com/GoogleCloudPlatform/slurm-gcp/blob/master/tools/prologs-epilogs/README.md | `bool` | `null` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enables Google Cloud os-login for user login and authentication for VMs.<br/>See https://cloud.google.com/compute/docs/oslogin | `bool` | `true` | no |
| <a name="input_enable_shielded_vm"></a> [enable\_shielded\_vm](#input\_enable\_shielded\_vm) | Enable the Shielded VM configuration. Note: the instance image must support option. | `bool` | `false` | no |
| <a name="input_enable_slurm_auth"></a> [enable\_slurm\_auth](#input\_enable\_slurm\_auth) | Enables slurm authentication instead of munge. | `bool` | `false` | no |
| <a name="input_enable_slurm_gcp_plugins"></a> [enable\_slurm\_gcp\_plugins](#input\_enable\_slurm\_gcp\_plugins) | DEPRECATED: Slurm GCP plugins have been deprecated.<br/>Instead of 'max\_hops' plugin please use the 'placement\_max\_distance' nodeset property.<br/>Instead of 'enable\_vpmu' plugin please use 'advanced\_machine\_features.performance\_monitoring\_unit' nodeset property. | `any` | `null` | no |
| <a name="input_enable_smt"></a> [enable\_smt](#input\_enable\_smt) | DEPRECATED: Use `advanced_machine_features.threads_per_core` instead. | `bool` | `null` | no |
| <a name="input_endpoint_versions"></a> [endpoint\_versions](#input\_endpoint\_versions) | Version of the API to use (The compute service is the only API currently supported) | <pre>object({<br/>    compute = string<br/>  })</pre> | <pre>{<br/>  "compute": "beta"<br/>}</pre> | no |
| <a name="input_epilog_scripts"></a> [epilog\_scripts](#input\_epilog\_scripts) | List of scripts to be used for Epilog. Programs for the slurmd to execute<br/>on every node when a user's job completes.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_Epilog. | <pre>list(object({<br/>    filename = string<br/>    content  = optional(string)<br/>    source   = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_extra_logging_flags"></a> [extra\_logging\_flags](#input\_extra\_logging\_flags) | The only available flag is `trace_api` | `map(bool)` | `{}` | no |
| <a name="input_gcloud_path_override"></a> [gcloud\_path\_override](#input\_gcloud\_path\_override) | Directory of the gcloud executable to be used during cleanup | `string` | `""` | no |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br/>    type  = string,<br/>    count = number<br/>  }))</pre> | `[]` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Defines the image that will be used in the Slurm controller VM instance.<br/><br/>Expected Fields:<br/>name: The name of the image. Mutually exclusive with family.<br/>family: The image family to use. Mutually exclusive with name.<br/>project: The project where the image is hosted.<br/><br/>For more information on creating custom images that comply with Slurm on GCP<br/>see the "Slurm on GCP Custom Images" section in docs/vm-images.md. | `map(string)` | <pre>{<br/>  "family": "slurm-gcp-6-11-hpc-rocky-linux-8",<br/>  "project": "schedmd-slurm-public"<br/>}</pre> | no |
| <a name="input_instance_image_custom"></a> [instance\_image\_custom](#input\_instance\_image\_custom) | A flag that designates that the user is aware that they are requesting<br/>to use a custom and potentially incompatible image for this Slurm on<br/>GCP module.<br/><br/>If the field is set to false, only the compatible families and project<br/>names will be accepted.  The deployment will fail with any other image<br/>family or name.  If set to true, no checks will be done.<br/><br/>See: https://goo.gle/hpc-slurm-images | `bool` | `false` | no |
| <a name="input_instance_template"></a> [instance\_template](#input\_instance\_template) | DEPRECATED: Instance template can not be specified for controller. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels, provided as a map. | `map(string)` | `{}` | no |
| <a name="input_login_network_storage"></a> [login\_network\_storage](#input\_login\_network\_storage) | An array of network attached storage mounts to be configured on all login nodes. | <pre>list(object({<br/>    server_ip     = string,<br/>    remote_mount  = string,<br/>    local_mount   = string,<br/>    fs_type       = string,<br/>    mount_options = string,<br/>  }))</pre> | `[]` | no |
| <a name="input_login_nodes"></a> [login\_nodes](#input\_login\_nodes) | List of slurm login instance definitions. | <pre>list(object({<br/>    group_name = string<br/>    access_config = optional(list(object({<br/>      nat_ip       = string<br/>      network_tier = string<br/>    })))<br/>    additional_disks = optional(list(object({<br/>      disk_name                  = optional(string)<br/>      device_name                = optional(string)<br/>      disk_size_gb               = optional(number)<br/>      disk_type                  = optional(string)<br/>      disk_labels                = optional(map(string), {})<br/>      auto_delete                = optional(bool, true)<br/>      boot                       = optional(bool, false)<br/>      disk_resource_manager_tags = optional(map(string), {})<br/>    })), [])<br/>    additional_networks = optional(list(object({<br/>      access_config = optional(list(object({<br/>        nat_ip       = string<br/>        network_tier = string<br/>      })), [])<br/>      alias_ip_range = optional(list(object({<br/>        ip_cidr_range         = string<br/>        subnetwork_range_name = string<br/>      })), [])<br/>      ipv6_access_config = optional(list(object({<br/>        network_tier = string<br/>      })), [])<br/>      network            = optional(string)<br/>      network_ip         = optional(string, "")<br/>      nic_type           = optional(string)<br/>      queue_count        = optional(number)<br/>      stack_type         = optional(string)<br/>      subnetwork         = optional(string)<br/>      subnetwork_project = optional(string)<br/>    })), [])<br/>    bandwidth_tier             = optional(string, "platform_default")<br/>    can_ip_forward             = optional(bool, false)<br/>    disk_auto_delete           = optional(bool, true)<br/>    disk_labels                = optional(map(string), {})<br/>    disk_resource_manager_tags = optional(map(string), {})<br/>    disk_size_gb               = optional(number)<br/>    disk_type                  = optional(string, "n1-standard-1")<br/>    enable_confidential_vm     = optional(bool, false)<br/>    enable_oslogin             = optional(bool, true)<br/>    enable_shielded_vm         = optional(bool, false)<br/>    gpu = optional(object({<br/>      count = number<br/>      type  = string<br/>    }))<br/>    labels       = optional(map(string), {})<br/>    machine_type = optional(string)<br/>    advanced_machine_features = object({<br/>      enable_nested_virtualization = optional(bool)<br/>      threads_per_core             = optional(number)<br/>      turbo_mode                   = optional(string)<br/>      visible_core_count           = optional(number)<br/>      performance_monitoring_unit  = optional(string)<br/>      enable_uefi_networking       = optional(bool)<br/>    })<br/>    metadata              = optional(map(string), {})<br/>    min_cpu_platform      = optional(string)<br/>    num_instances         = optional(number, 1)<br/>    on_host_maintenance   = optional(string)<br/>    preemptible           = optional(bool, false)<br/>    region                = optional(string)<br/>    resource_manager_tags = optional(map(string), {})<br/>    service_account = optional(object({<br/>      email  = optional(string)<br/>      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])<br/>    }))<br/>    shielded_instance_config = optional(object({<br/>      enable_integrity_monitoring = optional(bool, true)<br/>      enable_secure_boot          = optional(bool, true)<br/>      enable_vtpm                 = optional(bool, true)<br/>    }))<br/>    source_image_family  = optional(string)<br/>    source_image_project = optional(string)<br/>    source_image         = optional(string)<br/>    static_ips           = optional(list(string), [])<br/>    subnetwork           = string<br/>    spot                 = optional(bool, false)<br/>    tags                 = optional(list(string), [])<br/>    zone                 = optional(string)<br/>    termination_action   = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_login_startup_script"></a> [login\_startup\_script](#input\_login\_startup\_script) | Startup script used by the login VMs. | `string` | `"# no-op"` | no |
| <a name="input_login_startup_scripts_timeout"></a> [login\_startup\_scripts\_timeout](#input\_login\_startup\_scripts\_timeout) | The timeout (seconds) applied to each script in login\_startup\_scripts. If<br/>any script exceeds this timeout, then the instance setup process is considered<br/>failed and handled accordingly.<br/><br/>NOTE: When set to 0, the timeout is considered infinite and thus disabled. | `number` | `300` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to create. | `string` | `"c2-standard-4"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata, provided as a map. | `map(string)` | `{}` | no |
| <a name="input_min_cpu_platform"></a> [min\_cpu\_platform](#input\_min\_cpu\_platform) | Specifies a minimum CPU platform. Applicable values are the friendly names of<br/>CPU platforms, such as Intel Haswell or Intel Skylake. See the complete list:<br/>https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform | `string` | `null` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured on all instances. | <pre>list(object({<br/>    server_ip             = string,<br/>    remote_mount          = string,<br/>    local_mount           = string,<br/>    fs_type               = string,<br/>    mount_options         = string,<br/>    client_install_runner = optional(map(string))<br/>    mount_runner          = optional(map(string))<br/>  }))</pre> | `[]` | no |
| <a name="input_nodeset"></a> [nodeset](#input\_nodeset) | Define nodesets, as a list. | <pre>list(object({<br/>    node_count_static      = optional(number, 0)<br/>    node_count_dynamic_max = optional(number, 1)<br/>    node_conf              = optional(map(string), {})<br/>    nodeset_name           = string<br/>    additional_disks = optional(list(object({<br/>      disk_name                  = optional(string)<br/>      device_name                = optional(string)<br/>      disk_size_gb               = optional(number)<br/>      disk_type                  = optional(string)<br/>      disk_labels                = optional(map(string), {})<br/>      auto_delete                = optional(bool, true)<br/>      boot                       = optional(bool, false)<br/>      disk_resource_manager_tags = optional(map(string), {})<br/>    })), [])<br/>    bandwidth_tier                   = optional(string, "platform_default")<br/>    can_ip_forward                   = optional(bool, false)<br/>    disk_auto_delete                 = optional(bool, true)<br/>    disk_labels                      = optional(map(string), {})<br/>    disk_resource_manager_tags       = optional(map(string), {})<br/>    disk_size_gb                     = optional(number)<br/>    disk_type                        = optional(string)<br/>    enable_confidential_vm           = optional(bool, false)<br/>    enable_placement                 = optional(bool, false)<br/>    placement_max_distance           = optional(number, null)<br/>    enable_oslogin                   = optional(bool, true)<br/>    enable_shielded_vm               = optional(bool, false)<br/>    enable_maintenance_reservation   = optional(bool, false)<br/>    enable_opportunistic_maintenance = optional(bool, false)<br/>    gpu = optional(object({<br/>      count = number<br/>      type  = string<br/>    }))<br/>    accelerator_topology = optional(string, null)<br/>    dws_flex = object({<br/>      enabled          = bool<br/>      max_run_duration = number<br/>      use_job_duration = bool<br/>      use_bulk_insert  = bool<br/>    })<br/>    labels       = optional(map(string), {})<br/>    machine_type = optional(string)<br/>    advanced_machine_features = object({<br/>      enable_nested_virtualization = optional(bool)<br/>      threads_per_core             = optional(number)<br/>      turbo_mode                   = optional(string)<br/>      visible_core_count           = optional(number)<br/>      performance_monitoring_unit  = optional(string)<br/>      enable_uefi_networking       = optional(bool)<br/>    })<br/>    maintenance_interval     = optional(string)<br/>    instance_properties_json = string<br/>    metadata                 = optional(map(string), {})<br/>    min_cpu_platform         = optional(string)<br/>    network_tier             = optional(string, "STANDARD")<br/>    network_storage = optional(list(object({<br/>      server_ip             = string<br/>      remote_mount          = string<br/>      local_mount           = string<br/>      fs_type               = string<br/>      mount_options         = string<br/>      client_install_runner = optional(map(string))<br/>      mount_runner          = optional(map(string))<br/>    })), [])<br/>    on_host_maintenance   = optional(string)<br/>    preemptible           = optional(bool, false)<br/>    region                = optional(string)<br/>    resource_manager_tags = optional(map(string), {})<br/>    service_account = optional(object({<br/>      email  = optional(string)<br/>      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])<br/>    }))<br/>    shielded_instance_config = optional(object({<br/>      enable_integrity_monitoring = optional(bool, true)<br/>      enable_secure_boot          = optional(bool, true)<br/>      enable_vtpm                 = optional(bool, true)<br/>    }))<br/>    source_image_family  = optional(string)<br/>    source_image_project = optional(string)<br/>    source_image         = optional(string)<br/>    subnetwork_self_link = string<br/>    additional_networks = optional(list(object({<br/>      network            = string<br/>      subnetwork         = string<br/>      subnetwork_project = string<br/>      network_ip         = string<br/>      nic_type           = string<br/>      stack_type         = string<br/>      queue_count        = number<br/>      access_config = list(object({<br/>        nat_ip       = string<br/>        network_tier = string<br/>      }))<br/>      ipv6_access_config = list(object({<br/>        network_tier = string<br/>      }))<br/>      alias_ip_range = list(object({<br/>        ip_cidr_range         = string<br/>        subnetwork_range_name = string<br/>      }))<br/>    })))<br/>    access_config = optional(list(object({<br/>      nat_ip       = string<br/>      network_tier = string<br/>    })))<br/>    spot               = optional(bool, false)<br/>    tags               = optional(list(string), [])<br/>    termination_action = optional(string)<br/>    reservation_name   = optional(string)<br/>    future_reservation = string<br/>    startup_script = optional(list(object({<br/>      filename = string<br/>    content = string })), [])<br/><br/>    zone_target_shape = string<br/>    zone_policy_allow = set(string)<br/>    zone_policy_deny  = set(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_nodeset_dyn"></a> [nodeset\_dyn](#input\_nodeset\_dyn) | Defines dynamic nodesets, as a list. | <pre>list(object({<br/>    nodeset_name    = string<br/>    nodeset_feature = string<br/>  }))</pre> | `[]` | no |
| <a name="input_nodeset_tpu"></a> [nodeset\_tpu](#input\_nodeset\_tpu) | Define TPU nodesets, as a list. | <pre>list(object({<br/>    node_count_static      = optional(number, 0)<br/>    node_count_dynamic_max = optional(number, 5)<br/>    nodeset_name           = string<br/>    enable_public_ip       = optional(bool, false)<br/>    node_type              = string<br/>    accelerator_config = optional(object({<br/>      topology = string<br/>      version  = string<br/>      }), {<br/>      topology = ""<br/>      version  = ""<br/>    })<br/>    tf_version   = string<br/>    preemptible  = optional(bool, false)<br/>    preserve_tpu = optional(bool, false)<br/>    zone         = string<br/>    data_disks   = optional(list(string), [])<br/>    docker_image = optional(string, "")<br/>    network_storage = optional(list(object({<br/>      server_ip             = string<br/>      remote_mount          = string<br/>      local_mount           = string<br/>      fs_type               = string<br/>      mount_options         = string<br/>      client_install_runner = optional(map(string))<br/>      mount_runner          = optional(map(string))<br/>    })), [])<br/>    subnetwork = string<br/>    service_account = optional(object({<br/>      email  = optional(string)<br/>      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])<br/>    }))<br/>    project_id = string<br/>    reserved   = optional(string, false)<br/>  }))</pre> | `[]` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Instance availability Policy. | `string` | `"MIGRATE"` | no |
| <a name="input_partitions"></a> [partitions](#input\_partitions) | Cluster partitions as a list. See module slurm\_partition. | <pre>list(object({<br/>    partition_name        = string<br/>    partition_conf        = optional(map(string), {})<br/>    partition_nodeset     = optional(list(string), [])<br/>    partition_nodeset_dyn = optional(list(string), [])<br/>    partition_nodeset_tpu = optional(list(string), [])<br/>    enable_job_exclusive  = optional(bool, false)<br/>  }))</pre> | `[]` | no |
| <a name="input_preemptible"></a> [preemptible](#input\_preemptible) | Allow the instance to be preempted. | `bool` | `false` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_prolog_scripts"></a> [prolog\_scripts](#input\_prolog\_scripts) | List of scripts to be used for Prolog. Programs for the slurmd to execute<br/>whenever it is asked to run a job step from a new job allocation.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_Prolog. | <pre>list(object({<br/>    filename = string<br/>    content  = optional(string)<br/>    source   = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_region"></a> [region](#input\_region) | The default region to place resources in. | `string` | n/a | yes |
| <a name="input_resource_manager_tags"></a> [resource\_manager\_tags](#input\_resource\_manager\_tags) | (Optional) A set of key/value resource manager tag pairs to bind to the instances. Keys must be in the format tagKeys/{tag\_key\_id}, and values are in the format tagValues/456. | `map(string)` | `{}` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | DEPRECATED: Use `service_account_email` and `service_account_scopes` instead. | <pre>object({<br/>    email  = string<br/>    scopes = set(string)<br/>  })</pre> | `null` | no |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | Service account e-mail address to attach to the controller instance. | `string` | `null` | no |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Scopes to attach to the controller instance. | `set(string)` | <pre>[<br/>  "https://www.googleapis.com/auth/cloud-platform"<br/>]</pre> | no |
| <a name="input_shielded_instance_config"></a> [shielded\_instance\_config](#input\_shielded\_instance\_config) | Shielded VM configuration for the instance. Note: not used unless<br/>enable\_shielded\_vm is 'true'.<br/>  enable\_integrity\_monitoring : Compare the most recent boot measurements to the<br/>  integrity policy baseline and return a pair of pass/fail results depending on<br/>  whether they match or not.<br/>  enable\_secure\_boot : Verify the digital signature of all boot components, and<br/>  halt the boot process if signature verification fails.<br/>  enable\_vtpm : Use a virtualized trusted platform module, which is a<br/>  specialized computer chip you can use to encrypt objects like keys and<br/>  certificates. | <pre>object({<br/>    enable_integrity_monitoring = bool<br/>    enable_secure_boot          = bool<br/>    enable_vtpm                 = bool<br/>  })</pre> | <pre>{<br/>  "enable_integrity_monitoring": true,<br/>  "enable_secure_boot": true,<br/>  "enable_vtpm": true<br/>}</pre> | no |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Cluster name, used for resource naming and slurm accounting.<br/>If not provided it will default to the first 8 characters of the deployment name (removing any invalid characters). | `string` | `null` | no |
| <a name="input_slurm_conf_template"></a> [slurm\_conf\_template](#input\_slurm\_conf\_template) | Slurm slurm.conf template. Content of the file in 'slurm\_conf\_tpl' is used if this is not set. | `string` | `null` | no |
| <a name="input_slurm_conf_tpl"></a> [slurm\_conf\_tpl](#input\_slurm\_conf\_tpl) | Slurm slurm.conf template file path. This path is used only if raw content is not provided in 'slurm\_conf\_template'. | `string` | `null` | no |
| <a name="input_slurmdbd_conf_tpl"></a> [slurmdbd\_conf\_tpl](#input\_slurmdbd\_conf\_tpl) | Slurm slurmdbd.conf template file path. | `string` | `null` | no |
| <a name="input_static_ips"></a> [static\_ips](#input\_static\_ips) | List of static IPs for VM instances. | `list(string)` | `[]` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | Subnet to deploy to. | `string` | n/a | yes |
| <a name="input_tags"></a> [tags](#input\_tags) | Network tag list. | `list(string)` | `[]` | no |
| <a name="input_task_epilog_scripts"></a> [task\_epilog\_scripts](#input\_task\_epilog\_scripts) | List of scripts to be used for TaskEpilog. Programs for the slurmd to execute<br/>as the slurm job's owner after termination of each task.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_TaskEpilog. | <pre>list(object({<br/>    filename = string<br/>    content  = optional(string)<br/>    source   = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_task_prolog_scripts"></a> [task\_prolog\_scripts](#input\_task\_prolog\_scripts) | List of scripts to be used for TaskProlog. Programs for the slurmd to execute<br/>as the slurm job's owner prior to initiation of each task.<br/>See https://slurm.schedmd.com/slurm.conf.html#OPT_TaskProlog. | <pre>list(object({<br/>    filename = string<br/>    content  = optional(string)<br/>    source   = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_universe_domain"></a> [universe\_domain](#input\_universe\_domain) | Domain address for alternate API universe | `string` | `"googleapis.com"` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone where the instances should be created. If not specified, instances will be<br/>spread across available zones in the region. | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Post deployment instructions. |
| <a name="output_slurm_bucket"></a> [slurm\_bucket](#output\_slurm\_bucket) | GCS Bucket of Slurm cluster file storage. |
| <a name="output_slurm_bucket_dir"></a> [slurm\_bucket\_dir](#output\_slurm\_bucket\_dir) | Path directory within `bucket_name` for Slurm cluster file storage. |
| <a name="output_slurm_bucket_name"></a> [slurm\_bucket\_name](#output\_slurm\_bucket\_name) | GCS Bucket name of Slurm cluster file storage. |
| <a name="output_slurm_bucket_path"></a> [slurm\_bucket\_path](#output\_slurm\_bucket\_path) | Bucket path used by cluster. |
| <a name="output_slurm_cluster_name"></a> [slurm\_cluster\_name](#output\_slurm\_cluster\_name) | Slurm cluster name. |
| <a name="output_slurm_controller_instance"></a> [slurm\_controller\_instance](#output\_slurm\_controller\_instance) | Compute instance of controller node |
| <a name="output_slurm_login_instances"></a> [slurm\_login\_instances](#output\_slurm\_login\_instances) | Compute instances of login nodes |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
