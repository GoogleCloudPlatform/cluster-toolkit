## Description

This module creates one or more
[compute VM instances](https://cloud.google.com/compute/docs/instances).

### Example

```yaml
- id: compute
  source: modules/compute/vm-instance
  use: [network1]
  settings:
    instance_count: 8
    name_prefix: compute
    machine_type: c2-standard-60
```

This creates a cluster of 8 compute VMs that are:

* named `compute-[0-7]`
* on the network defined by the `network1` module
* of type c2-standard-60

> **_NOTE:_** Simultaneous Multithreading (SMT) is deactivated by default
> (threads_per_core=1), which means only the physical cores are visible on the
> VM. With SMT disabled, a machine of type c2-standard-60 will only have the 30
> physical cores visible. To change this, set `threads_per_core=2` under
> settings.

### VPC Networks

There are two methods for adding network connectivity to the `vm-instance`
module. The first is shown in the example above, where a `vpc` module or
`pre-existing-vpc` module is used by the `vm-instance` module. When this
happens, the `network_self_link` and `subnetwork_self_link` outputs from the
network are provided as input to the `vm-instance` and a network interface is
defined based on that. This can also be done updating the `network_self_link` and
`subnetwork_self_link` settings directly.

The alternative option can be used when more than one network needs to be added
to the `vm-instance` or further customization is needed beyond what is provided
via other variables. For this option, the `network_interfaces` variable can be
used to set up one or more network interfaces on the VM instance. The format is
consistent with the terraform `google_compute_instance` `network_interface`
block, and more information can be found in the
[terraform docs][network-interface-tf].

> **_NOTE:_** When supplying the `network_interfaces` variable, networks
> associated with the `vm-instance` via use will be ignored in favor of the
> networks added in `network_interfaces`. In addition, `bandwidth_tier` and
> `disable_public_ips` will not apply to networks defined in
> `network_interfaces`.

[network-interface-tf]: https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance#nested_network_interface

### SSH key metadata

This module will ignore all changes to the `ssh-keys` metadata field that are
typically set by [external Google Cloud tools that automate SSH access][gcpssh]
when not using OS Login. For example, clicking on the Google Cloud Console SSH
button next to VMs in the VM Instances list will temporarily modify VM metadata
to include a dynamically-generated SSH public key.

[gcpssh]: https://cloud.google.com/compute/docs/connect/add-ssh-keys#metadata

### Placement

The `placement_policy` variable can be used to control where your VM instances
are physically located relative to each other within a zone. See the official
placement [guide][guide-link] and [api][api-link] documentation.

[guide-link]: https://cloud.google.com/compute/docs/instances/define-instance-placement
[api-link]: https://cloud.google.com/sdk/gcloud/reference/compute/resource-policies/create/group-placement

Use the following settings for compact placement:

```yaml
  ...
  settings:
    instance_count: 4
    machine_type: c2-standard-60
    placement_policy:
      collocation: "COLLOCATED"
```

By default the above placement policy will always result in the most compact set
of VMs available. If you would like that provisioning failed if some level of
compactness is not obtainable, you can enforce this with the [`max_distance`
setting](https://cloud.google.com/compute/docs/instances/use-compact-placement-policies):

```yaml
  ...
  settings:
    instance_count: 4
    machine_type: c2-standard-60
    placement_policy:
      collocation: "COLLOCATED"
      max_distance: 1
```

Use the following settings for spread placement:

```yaml
  ...
  settings:
    instance_count: 4
    machine_type: n2-standard-4
    placement_policy:
      availability_domain_count: 2
```

When `vm_count` is not set, as shown in the examples above, then the VMs will be
added to the placement policy incrementally. This is the **recommended way** to
use placement policies.

If `vm_count` is specified then VMs will stay in pending state until the
specified number of VMs are created. See the warning below if using this field.

> [!WARNING]
> When creating a compact placement using `vm_count` with more than 10 VMs, you
> must add `-parallelism=<n>` argument on apply. For example if you have 15 VMs
> in a placement group: `terraform apply -parallelism=15`. This is because
> terraform self limits to 10 parallel requests by default but the create
> instance requests will not succeed until all VMs in the placement group have
> been requested, forming a deadlock.

### GPU Support

More information on GPU support in `vm-instance` and other HPC Toolkit modules
can be found at [docs/gpu-support.md](../../../docs/gpu-support.md)

## Lifecycle

The `vm-instance` module will be replaced when the `instance_image` variable is
changed and `terraform apply` is run on  the deployment group folder or
`ghpc deploy` is run. However, it will not be automatically replaced if a new
image is created in a family.

To selectively replace the vm-instance(s), consider running terraform
`apply -replace` such as:

> See https://developer.hashicorp.com/terraform/cli/commands/plan#replace-address for precise syntax terraform apply -replace=ADDRESS

```shell
terraform state list
# search for the module ID and resource
terraform apply -replace="address"
```

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.2.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.73.0 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 4.73.0 |
| <a name="requirement_null"></a> [null](#requirement\_null) | >= 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.73.0 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 4.73.0 |
| <a name="provider_null"></a> [null](#provider\_null) | >= 3.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_netstorage_startup_script"></a> [netstorage\_startup\_script](#module\_netstorage\_startup\_script) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script | 50644b2 |

## Resources

| Name | Type |
|------|------|
| [google-beta_google_compute_instance.compute_vm](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_instance) | resource |
| [google-beta_google_compute_resource_policy.placement_policy](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_resource_policy) | resource |
| [google_compute_disk.boot_disk](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk) | resource |
| [null_resource.image](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [null_resource.replace_vm_trigger_from_placement](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [google_compute_image.compute_image](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_image) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_add_deployment_name_before_prefix"></a> [add\_deployment\_name\_before\_prefix](#input\_add\_deployment\_name\_before\_prefix) | If true, the names of VMs and disks will always be prefixed with `deployment_name` to enable uniqueness across deployments.<br>See `name_prefix` for further details on resource naming behavior. | `bool` | `false` | no |
| <a name="input_auto_delete_boot_disk"></a> [auto\_delete\_boot\_disk](#input\_auto\_delete\_boot\_disk) | Controls if boot disk should be auto-deleted when instance is deleted. | `bool` | `true` | no |
| <a name="input_automatic_restart"></a> [automatic\_restart](#input\_automatic\_restart) | Specifies if the instance should be restarted if it was terminated by Compute Engine (not a user). | `bool` | `null` | no |
| <a name="input_bandwidth_tier"></a> [bandwidth\_tier](#input\_bandwidth\_tier) | Tier 1 bandwidth increases the maximum egress bandwidth for VMs.<br>  Using the `tier_1_enabled` setting will enable both gVNIC and TIER\_1 higher bandwidth networking.<br>  Using the `gvnic_enabled` setting will only enable gVNIC and will not enable TIER\_1.<br>  Note that TIER\_1 only works with specific machine families & shapes and must be using an image that supports gVNIC. See [official docs](https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration) for more details. | `string` | `"not_enabled"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, will optionally be used name resources according to `name_prefix` | `string` | n/a | yes |
| <a name="input_disable_public_ips"></a> [disable\_public\_ips](#input\_disable\_public\_ips) | If set to true, instances will not have public IPs | `bool` | `false` | no |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of disk for instances. | `number` | `200` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Disk type for instances. | `string` | `"pd-standard"` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enable or Disable OS Login with "ENABLE" or "DISABLE". Set to "INHERIT" to inherit project OS Login setting. | `string` | `"ENABLE"` | no |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br>    type  = string,<br>    count = number<br>  }))</pre> | `[]` | no |
| <a name="input_instance_count"></a> [instance\_count](#input\_instance\_count) | Number of instances | `number` | `1` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Instance Image | `map(string)` | <pre>{<br>  "family": "hpc-centos-7",<br>  "project": "cloud-hpc-image-public"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_local_ssd_count"></a> [local\_ssd\_count](#input\_local\_ssd\_count) | The number of local SSDs to attach to each VM. See https://cloud.google.com/compute/docs/disks/local-ssd. | `number` | `0` | no |
| <a name="input_local_ssd_interface"></a> [local\_ssd\_interface](#input\_local\_ssd\_interface) | Interface to be used with local SSDs. Can be either 'NVME' or 'SCSI'. No effect unless `local_ssd_count` is also set. | `string` | `"NVME"` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for the instance creation | `string` | `"c2-standard-60"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata, provided as a map | `map(string)` | `{}` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | An optional name for all VM and disk resources. <br>If not supplied, `deployment_name` will be used. <br>When `name_prefix` is supplied, and `add_deployment_name_before_prefix` is set, <br>then resources are named by "<`deployment_name`>-<`name_prefix`>-<#>". | `string` | `null` | no |
| <a name="input_network_interfaces"></a> [network\_interfaces](#input\_network\_interfaces) | A list of network interfaces. The options match that of the terraform<br>network\_interface block of google\_compute\_instance. For descriptions of the<br>subfields or more information see the documentation:<br>https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance#nested_network_interface<br><br>**\_NOTE:\_** If `network_interfaces` are set, `network_self_link` and<br>`subnetwork_self_link` will be ignored, even if they are provided through<br>the `use` field. `bandwidth_tier` and `disable_public_ips` also do not apply<br>to network interfaces defined in this variable.<br><br>Subfields:<br>network            (string, required if subnetwork is not supplied)<br>subnetwork         (string, required if network is not supplied)<br>subnetwork\_project (string, optional)<br>network\_ip         (string, optional)<br>nic\_type           (string, optional, choose from ["GVNIC", "VIRTIO\_NET"])<br>stack\_type         (string, optional, choose from ["IPV4\_ONLY", "IPV4\_IPV6"])<br>queue\_count        (number, optional)<br>access\_config      (object, optional)<br>ipv6\_access\_config (object, optional)<br>alias\_ip\_range     (list(object), optional) | <pre>list(object({<br>    network            = string,<br>    subnetwork         = string,<br>    subnetwork_project = string,<br>    network_ip         = string,<br>    nic_type           = string,<br>    stack_type         = string,<br>    queue_count        = number,<br>    access_config = list(object({<br>      nat_ip                 = string,<br>      public_ptr_domain_name = string,<br>      network_tier           = string<br>    })),<br>    ipv6_access_config = list(object({<br>      public_ptr_domain_name = string,<br>      network_tier           = string<br>    })),<br>    alias_ip_range = list(object({<br>      ip_cidr_range         = string,<br>      subnetwork_range_name = string<br>    }))<br>  }))</pre> | `[]` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network to attach the VM. Can use "default" for the default network. | `string` | `null` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except for when `placement_policy`, spot provisioning, or GPUs require it to be `TERMINATE` | `string` | `null` | no |
| <a name="input_placement_policy"></a> [placement\_policy](#input\_placement\_policy) | Control where your VM instances are physically located relative to each other within a zone.<br>See https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_resource_policy#nested_group_placement_policy | `any` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region to deploy to | `string` | n/a | yes |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the instance. See https://www.terraform.io/docs/providers/google/r/compute_instance_template.html#service_account. | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | <pre>{<br>  "email": null,<br>  "scopes": [<br>    "https://www.googleapis.com/auth/cloud-platform"<br>  ]<br>}</pre> | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision VMs using discounted Spot pricing, allowing for preemption | `bool` | `false` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script used on the instance | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork to attach the VM. | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Network tags, provided as a list | `list(string)` | `[]` | no |
| <a name="input_threads_per_core"></a> [threads\_per\_core](#input\_threads\_per\_core) | Sets the number of threads per physical core. By setting threads\_per\_core<br>to 2, Simultaneous Multithreading (SMT) is enabled extending the total number<br>of virtual cores. For example, a machine of type c2-standard-60 will have 60<br>virtual cores with threads\_per\_core equal to 2. With threads\_per\_core equal<br>to 1 (SMT turned off), only the 30 physical cores will be available on the VM.<br><br>The default value of \"0\" will turn off SMT for supported machine types, and<br>will fall back to GCE defaults for unsupported machine types (t2d, shared-core<br>instances, or instances with less than 2 vCPU).<br><br>Disabling SMT can be more performant in many HPC workloads, therefore it is<br>disabled by default where compatible.<br><br>null = SMT configuration will use the GCE defaults for the machine type<br>0 = SMT will be disabled where compatible (default)<br>1 = SMT will always be disabled (will fail on incompatible machine types)<br>2 = SMT will always be enabled (will fail on incompatible machine types) | `number` | `0` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_external_ip"></a> [external\_ip](#output\_external\_ip) | External IP of the instances (if enabled) |
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions on how to SSH into the created VM. Commands may fail depending on VM configuration and IAM permissions. |
| <a name="output_internal_ip"></a> [internal\_ip](#output\_internal\_ip) | Internal IP of the instances |
| <a name="output_name"></a> [name](#output\_name) | Name of any instance created |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
