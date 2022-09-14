## Description

This module creates one or more
[compute VM instances](https://cloud.google.com/compute/docs/instances).

### Example

```yaml
- id: compute
  source: modules/compute/vm-instance
  kind: terraform
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
      vm_count: 4  # Note: should match instance count
      collocation: "COLLOCATED"
      availability_domain_count: null
```

> **Warning** When creating a compact placement with more than 10 VMs, you must
> add `-parallelism=<n>` argument on apply. For example if you have 15 VMs in a
> placement group: `terraform apply -parallelism=15`. This is because terraform
> self limits to 10 parallel requests by default but the create instance
> requests will not succeed until all VMs in the placement group have been
> requested, forming a deadlock.

Use the following settings for spread placement:

```yaml
  ...
  settings:
    instance_count: 4
    machine_type: n2-standard-4
    placement_policy:
      vm_count: null
      collocation: null
      availability_domain_count: 2
```

> **_NOTE:_** Due to
> [this open issue](https://github.com/hashicorp/terraform-provider-google/issues/11483),
> it may be required to specify the `vm_count`. Once this issue is resolved,
> `vm_count` will no longer be mandatory.

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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 4.12 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 4.12 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_compute_instance.compute_vm](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_compute_instance) | resource |
| [google_compute_disk.boot_disk](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_disk) | resource |
| [google_compute_resource_policy.placement_policy](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_resource_policy) | resource |
| [google_compute_image.compute_image](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_image) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_bandwidth_tier"></a> [bandwidth\_tier](#input\_bandwidth\_tier) | Tier 1 bandwidth increases the maximum egress bandwidth for VMs.<br>  Using the `tier_1_enabled` setting will enable both gVNIC and TIER\_1 higher bandwidth networking.<br>  Using the `gvnic_enabled` setting will only enable gVNIC and will not enable TIER\_1.<br>  Note that TIER\_1 only works with specific machine families & shapes and must be using an image that supports gVNIC. See [official docs](https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration) for more details. | `string` | `"not_enabled"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, used to name the cluster | `string` | n/a | yes |
| <a name="input_disable_public_ips"></a> [disable\_public\_ips](#input\_disable\_public\_ips) | If set to true, instances will not have public IPs | `bool` | `false` | no |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of disk for instances. | `number` | `200` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Disk type for instances. | `string` | `"pd-standard"` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enable or Disable OS Login with "ENABLE" or "DISABLE". Set to "INHERIT" to inherit project OS Login setting. | `string` | `"ENABLE"` | no |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br>    type  = string,<br>    count = number<br>  }))</pre> | `null` | no |
| <a name="input_instance_count"></a> [instance\_count](#input\_instance\_count) | Number of instances | `number` | `1` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Instance Image | <pre>object({<br>    family  = string,<br>    project = string<br>  })</pre> | <pre>{<br>  "family": "hpc-centos-7",<br>  "project": "cloud-hpc-image-public"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. List key, value pairs. | `any` | n/a | yes |
| <a name="input_local_ssd_count"></a> [local\_ssd\_count](#input\_local\_ssd\_count) | The number of local SSDs to attach to each VM. See https://cloud.google.com/compute/docs/disks/local-ssd. | `number` | `0` | no |
| <a name="input_local_ssd_interface"></a> [local\_ssd\_interface](#input\_local\_ssd\_interface) | Interface to be used with local SSDs. Can be either 'NVME' or 'SCSI'. No effect unless `local_ssd_count` is also set. | `string` | `"NVME"` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for the instance creation | `string` | `"c2-standard-60"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata, provided as a map | `map(string)` | `{}` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Name Prefix | `string` | `null` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network to attach the VM. | `string` | `"default"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. | <pre>list(object({<br>    server_ip     = string,<br>    remote_mount  = string,<br>    local_mount   = string,<br>    fs_type       = string,<br>    mount_options = string<br>  }))</pre> | `[]` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except for when `placement_policy`, spot provisioning, or GPUs require it to be `TERMINATE` | `string` | `null` | no |
| <a name="input_placement_policy"></a> [placement\_policy](#input\_placement\_policy) | Control where your VM instances are physically located relative to each other within a zone. | <pre>object({<br>    vm_count                  = number,<br>    availability_domain_count = number,<br>    collocation               = string,<br>  })</pre> | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to the instance. See https://www.terraform.io/docs/providers/google/r/compute_instance_template.html#service_account. | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | <pre>{<br>  "email": null,<br>  "scopes": [<br>    "https://www.googleapis.com/auth/devstorage.read_only",<br>    "https://www.googleapis.com/auth/logging.write",<br>    "https://www.googleapis.com/auth/monitoring.write",<br>    "https://www.googleapis.com/auth/servicecontrol",<br>    "https://www.googleapis.com/auth/service.management.readonly",<br>    "https://www.googleapis.com/auth/trace.append"<br>  ]<br>}</pre> | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision VMs using discounted Spot pricing, allowing for preemption | `bool` | `false` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script used on the instance | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork to attach the VM. | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Network tags, provided as a list | `list(string)` | `[]` | no |
| <a name="input_threads_per_core"></a> [threads\_per\_core](#input\_threads\_per\_core) | Sets the number of threads per physical core. By setting threads\_per\_core<br>to 2, Simultaneous Multithreading (SMT) is enabled extending the total number<br>of virtual cores. For example, a machine of type c2-standard-60 will have 60<br>virtual cores with threads\_per\_core equal to 2. With threads\_per\_core equal<br>to 1 (SMT turned off), only the 30 physical cores will be available on the VM.<br><br>The default value of \"0\" will turn off SMT for supported machine types, and<br>will fall back to GCE defaults for unsupported machine types (t2d, shared-core <br>instances, or instances with less than 2 vCPU). <br><br>Disabling SMT can be more performant in many HPC workloads, therefore it is<br>disabled by default where compatible.<br><br>null = SMT configuration will use the GCE defaults for the machine type<br>0 = SMT will be disabled where compatible (default)<br>1 = SMT will always be disabled (will fail on incompatible machine types)<br>2 = SMT will always be enabled (will fail on incompatible machine types) | `number` | `0` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Compute Platform zone | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_external_ip"></a> [external\_ip](#output\_external\_ip) | External IP of the instances (if enabled) |
| <a name="output_internal_ip"></a> [internal\_ip](#output\_internal\_ip) | Internal IP of the instances |
| <a name="output_name"></a> [name](#output\_name) | Name of any instance created |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
