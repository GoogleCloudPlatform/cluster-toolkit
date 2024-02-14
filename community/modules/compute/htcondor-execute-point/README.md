## Description

This module performs the following tasks:

- create an instance template from which execute points will be created
- create a managed instance group (MIG) for execute points
- create a Toolkit runner to configure the autoscaler to scale the MIG

It is expected to be used with the [htcondor-install] and [htcondor-setup]
modules.

[htcondor-install]: ../../scripts/htcondor-install/README.md
[htcondor-setup]: ../../scheduler/htcondor-setup/README.md

### Known limitations

This module may be used multiple times in a blueprint to create sets of
execute points in an HTCondor pool. If used more than 1 time, the setting
[name_prefix](#input_name_prefix) must be set to a value that is unique across
all uses of the htcondor-execute-point module. If you do not follow this
constraint, you will likely receive an error while running `terraform apply`
similar to that shown below.

```text
Error: Invalid value for variable

  on modules/embedded/community/modules/scheduler/htcondor-access-point/main.tf line 136, in module "startup_script":
 136:   runners = local.all_runners
    ├────────────────
    │ var.runners is list of map of string with 5 elements

All startup-script runners must have a unique destination.
```

### How to configure jobs to select execute points

HTCondor access points provisioned by the Toolkit are specially configured to
honor an attribute named `RequireId` in each [Job ClassAd][jobad]. This value
must be set to the ID of a MIG created by an instance of this module. The
[htcondor-access-point] module includes a setting `var.default_mig_id` that will
set this value automatically to the MIG ID corresponding to the module's
execute points. If this setting is left unset each job must specify `+RequireId`
explicitly. In all cases, the default value can be overridden explicitly as shown
below:

```text
universe       = vanilla
executable     = /bin/echo
arguments      = "Hello, World!"
output         = out.$(ClusterId).$(ProcId)
error          = err.$(ClusterId).$(ProcId)
log            = log.$(ClusterId).$(ProcId)
request_cpus   = 1
request_memory = 100MB
+RequireId     = "htcondor-pool-ep-mig"
queue
```

[htcondor-access-point]: ../../scheduler/htcondor-access-point/README.md
[jobad]: https://htcondor.readthedocs.io/en/latest/users-manual/matchmaking-with-classads.html

### Example

A full example can be found in the [examples README][htc-example].

[htc-example]: ../../../../examples/README.md#htc-htcondoryaml--

The following code snippet creates a pool with 2 sets of HTCondor execute
points, one using On-demand pricing and the other using Spot pricing. They use
a startup script and network created in previous steps.

```yaml
- id: htcondor_execute_point
  source: community/modules/compute/htcondor-execute-point
  use:
  - network1
  - htcondor_secrets
  - htcondor_setup
  - htcondor_cm
  settings:
    instance_image:
      project: $(vars.project_id)
      family: $(vars.new_image_family)
    min_idle: 2

- id: htcondor_execute_point_spot
  source: community/modules/compute/htcondor-execute-point
  use:
  - network1
  - htcondor_secrets
  - htcondor_setup
  - htcondor_cm
  settings:
    instance_image:
      project: $(vars.project_id)
      family: $(vars.new_image_family)
    spot: true

- id: htcondor_access
  source: community/modules/scheduler/htcondor-access-point
  use:
  - network1
  - htcondor_secrets
  - htcondor_setup
  - htcondor_cm
  - htcondor_execute_point
  - htcondor_execute_point_spot
  settings:
    default_mig_id: $(htcondor_execute_point.mig_id)
    enable_public_ips: true
    instance_image:
      project: $(vars.project_id)
      family: $(vars.new_image_family)
  outputs:
  - access_point_ips
  - access_point_name
```

## Support

HTCondor is maintained by the [Center for High Throughput Computing][chtc] at
the University of Wisconsin-Madison. Support for HTCondor is available via:

- [Discussion lists](https://htcondor.org/mail-lists/)
- [HTCondor on GitHub](https://github.com/htcondor/htcondor/)
- [HTCondor manual](https://htcondor.readthedocs.io/en/latest/)

[chtc]: https://chtc.cs.wisc.edu/

## Known Issues

When using OS Login with "external users" (outside of the Google Cloud
organization), then Docker universe jobs will fail and cause the Docker daemon
to crash. This stems from the use of POSIX user ids (uid) outside the range
supported by Docker. Please consider disabling OS Login if this atypical
situation applies.

```yaml
vars:
  # add setting below to existing deployment variables
  enable_oslogin: DISABLE
```

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.1 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_execute_point_instance_template"></a> [execute\_point\_instance\_template](#module\_execute\_point\_instance\_template) | terraform-google-modules/vm/google//modules/instance_template | ~> 8.0 |
| <a name="module_mig"></a> [mig](#module\_mig) | github.com/terraform-google-modules/terraform-google-vm//modules/mig | aea74d1 |
| <a name="module_startup_script"></a> [startup\_script](#module\_startup\_script) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script | 50644b2 |

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket_object.execute_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_compute_image.htcondor](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_image) | data source |
| [google_compute_zones.available](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_zones) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_central_manager_ips"></a> [central\_manager\_ips](#input\_central\_manager\_ips) | List of IP addresses of HTCondor Central Managers | `list(string)` | n/a | yes |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. HTCondor cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Boot disk size in GB | `number` | `100` | no |
| <a name="input_distribution_policy_target_shape"></a> [distribution\_policy\_target\_shape](#input\_distribution\_policy\_target\_shape) | Target shape across zones for instance group managing execute points | `string` | `"ANY"` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enable or Disable OS Login with "ENABLE" or "DISABLE". Set to "INHERIT" to inherit project OS Login setting. | `string` | `"ENABLE"` | no |
| <a name="input_enable_shielded_vm"></a> [enable\_shielded\_vm](#input\_enable\_shielded\_vm) | Enable the Shielded VM configuration (var.shielded\_instance\_config). | `bool` | `false` | no |
| <a name="input_execute_point_runner"></a> [execute\_point\_runner](#input\_execute\_point\_runner) | A list of Toolkit runners for configuring an HTCondor execute point | `list(map(string))` | `[]` | no |
| <a name="input_execute_point_service_account_email"></a> [execute\_point\_service\_account\_email](#input\_execute\_point\_service\_account\_email) | Service account for HTCondor execute point (e-mail format) | `string` | n/a | yes |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br>    type  = string,<br>    count = number<br>  }))</pre> | `[]` | no |
| <a name="input_htcondor_bucket_name"></a> [htcondor\_bucket\_name](#input\_htcondor\_bucket\_name) | Name of HTCondor configuration bucket | `string` | n/a | yes |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | HTCondor execute point VM image<br><br>Expected Fields:<br>name: The name of the image. Mutually exclusive with family.<br>family: The image family to use. Mutually exclusive with name.<br>project: The project where the image is hosted. | `map(string)` | <pre>{<br>  "family": "hpc-rocky-linux-8",<br>  "project": "cloud-hpc-image-public"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to HTConodr execute points | `map(string)` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for HTCondor execute points | `string` | `"n2-standard-4"` | no |
| <a name="input_max_size"></a> [max\_size](#input\_max\_size) | Maximum size of the HTCondor execute point pool. | `number` | `100` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata to add to HTCondor execute points | `map(string)` | `{}` | no |
| <a name="input_min_idle"></a> [min\_idle](#input\_min\_idle) | Minimum number of idle VMs in the HTCondor pool (if pool reaches var.max\_size, this minimum is not guaranteed); set to ensure jobs beginning run more quickly. | `number` | `0` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Name prefix given to hostnames in this group of execute points; must be unique across all instances of this module | `string` | n/a | yes |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network HTCondor execute points will join | `string` | `"default"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HTCondor execute points will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region in which HTCondor execute points will be created | `string` | n/a | yes |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Scopes by which to limit service account attached to central manager. | `set(string)` | <pre>[<br>  "https://www.googleapis.com/auth/cloud-platform"<br>]</pre> | no |
| <a name="input_shielded_instance_config"></a> [shielded\_instance\_config](#input\_shielded\_instance\_config) | Shielded VM configuration for the instance (must set var.enabled\_shielded\_vm) | <pre>object({<br>    enable_secure_boot          = bool<br>    enable_vtpm                 = bool<br>    enable_integrity_monitoring = bool<br>  })</pre> | <pre>{<br>  "enable_integrity_monitoring": true,<br>  "enable_secure_boot": true,<br>  "enable_vtpm": true<br>}</pre> | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision VMs using discounted Spot pricing, allowing for preemption | `bool` | `false` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork HTCondor execute points will join | `string` | `null` | no |
| <a name="input_target_size"></a> [target\_size](#input\_target\_size) | Initial size of the HTCondor execute point pool; set to null (default) to avoid Terraform management of size. | `number` | `null` | no |
| <a name="input_windows_startup_ps1"></a> [windows\_startup\_ps1](#input\_windows\_startup\_ps1) | Startup script to run at boot-time for Windows-based HTCondor execute points | `list(string)` | `[]` | no |
| <a name="input_zones"></a> [zones](#input\_zones) | Zone(s) in which execute points may be created. If not supplied, will default to all zones in var.region. | `list(string)` | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_autoscaler_runner"></a> [autoscaler\_runner](#output\_autoscaler\_runner) | Toolkit runner to configure the HTCondor autoscaler |
| <a name="output_mig_id"></a> [mig\_id](#output\_mig\_id) | ID of the managed instance group containing the execute points |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
