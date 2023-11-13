## Description

This module provisions a highly available HTCondor access point using a [Managed
Instance Group (MIG)][mig] with auto-healing.

[mig]: https://cloud.google.com/compute/docs/instance-groups

## Usage

Although this provisions an HTCondor access point with standard configuration,
for a functioning node, you must supply Toolkit runners as described below:

- [var.access_point_runner](#input_access_point_runner)
  - Runner must download or otherwise create an [IDTOKEN] with ADVERTISE_MASTER,
    ADVERTISE_SCHEDD, and DAEMON scopes
- [var.autoscaler_runner](#input_autoscaler_runner)
  - 1 runner for each set of execute points to add to the pool

Reference implementations for each are included in the Toolkit modules
[htcondor-pool-secrets] and [htcondor-execute-point]. You may substitute
implementations (e.g. alternative secret management) so long as they duplicate
the functionality in these references. Their usage is demonstrated in the
[HTCondor example][htc-example].

[htc-example]: ../../../../examples/README.md#htc-htcondoryaml--
[htcondor-execute-point]: ../../compute/htcondor-execute-point/README.md
[htcondor-pool-secrets]: ../htcondor-pool-secrets/README.md
[IDTOKEN]: https://htcondor.readthedocs.io/en/latest/admin-manual/security.html#introducing-idtokens

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.1 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_time"></a> [time](#requirement\_time) | ~> 0.9 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_time"></a> [time](#provider\_time) | ~> 0.9 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_access_point_instance_template"></a> [access\_point\_instance\_template](#module\_access\_point\_instance\_template) | github.com/terraform-google-modules/terraform-google-vm//modules/instance_template | 84d7959 |
| <a name="module_htcondor_ap"></a> [htcondor\_ap](#module\_htcondor\_ap) | github.com/terraform-google-modules/terraform-google-vm//modules/mig | aea74d1 |
| <a name="module_startup_script"></a> [startup\_script](#module\_startup\_script) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script | 50644b2 |

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket_object.ap_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [time_sleep.mig_warmup](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/sleep) | resource |
| [google_compute_image.htcondor](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_image) | data source |
| [google_compute_instance.ap](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_instance) | data source |
| [google_compute_region_instance_group.ap](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_region_instance_group) | data source |
| [google_compute_zones.available](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_zones) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_access_point_runner"></a> [access\_point\_runner](#input\_access\_point\_runner) | A list of Toolkit runners for configuring an HTCondor access point | `list(map(string))` | `[]` | no |
| <a name="input_access_point_service_account_email"></a> [access\_point\_service\_account\_email](#input\_access\_point\_service\_account\_email) | Service account for access point (e-mail format) | `string` | n/a | yes |
| <a name="input_autoscaler_runner"></a> [autoscaler\_runner](#input\_autoscaler\_runner) | A list of Toolkit runners for configuring autoscaling daemons | `list(map(string))` | `[]` | no |
| <a name="input_central_manager_ips"></a> [central\_manager\_ips](#input\_central\_manager\_ips) | List of IP addresses of HTCondor Central Managers | `list(string)` | n/a | yes |
| <a name="input_default_mig_id"></a> [default\_mig\_id](#input\_default\_mig\_id) | Default MIG ID for HTCondor jobs; if unset, jobs must specify MIG id | `string` | `""` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. HTCondor cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Boot disk size in GB | `number` | `null` | no |
| <a name="input_distribution_policy_target_shape"></a> [distribution\_policy\_target\_shape](#input\_distribution\_policy\_target\_shape) | Target shape acoss zones for instance group managing high availability of access point | `string` | `"BALANCED"` | no |
| <a name="input_enable_high_availability"></a> [enable\_high\_availability](#input\_enable\_high\_availability) | Provision HTCondor access point in high availability mode | `bool` | `false` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enable or Disable OS Login with "ENABLE" or "DISABLE". Set to "INHERIT" to inherit project OS Login setting. | `string` | `"ENABLE"` | no |
| <a name="input_enable_public_ips"></a> [enable\_public\_ips](#input\_enable\_public\_ips) | Enable Public IPs on the access points | `bool` | `false` | no |
| <a name="input_enable_shielded_vm"></a> [enable\_shielded\_vm](#input\_enable\_shielded\_vm) | Enable the Shielded VM configuration (var.shielded\_instance\_config). | `bool` | `false` | no |
| <a name="input_htcondor_bucket_name"></a> [htcondor\_bucket\_name](#input\_htcondor\_bucket\_name) | Name of HTCondor configuration bucket | `string` | n/a | yes |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Custom VM image with HTCondor and Toolkit support installed."<br><br>Expected Fields:<br>name: The name of the image. Mutually exclusive with family.<br>family: The image family to use. Mutually exclusive with name.<br>project: The project where the image is hosted. | `map(string)` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to resources. List key, value pairs. | `map(string)` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for HTCondor central managers | `string` | `"c2-standard-4"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata to add to HTCondor central managers | `map(string)` | `{}` | no |
| <a name="input_mig_id"></a> [mig\_id](#input\_mig\_id) | List of Managed Instance Group IDs containing execute points in this pool (supplied by htcondor-execute-point module) | `list(string)` | `[]` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network in which the HTCondor central manager will be created. | `string` | `null` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which HTCondor pool will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Default region for creating resources | `string` | n/a | yes |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Scopes by which to limit service account attached to central manager. | `set(string)` | <pre>[<br>  "https://www.googleapis.com/auth/cloud-platform"<br>]</pre> | no |
| <a name="input_shielded_instance_config"></a> [shielded\_instance\_config](#input\_shielded\_instance\_config) | Shielded VM configuration for the instance (must set var.enabled\_shielded\_vm) | <pre>object({<br>    enable_secure_boot          = bool<br>    enable_vtpm                 = bool<br>    enable_integrity_monitoring = bool<br>  })</pre> | <pre>{<br>  "enable_integrity_monitoring": true,<br>  "enable_secure_boot": true,<br>  "enable_vtpm": true<br>}</pre> | no |
| <a name="input_spool_parent_dir"></a> [spool\_parent\_dir](#input\_spool\_parent\_dir) | HTCondor access point configuration SPOOL will be set to subdirectory named "spool" | `string` | `"/var/lib/condor"` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork in which the HTCondor central manager will be created. | `string` | `null` | no |
| <a name="input_zones"></a> [zones](#input\_zones) | Zone(s) in which access point may be created. If not supplied, will default to all zones in var.region. | `list(string)` | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_access_point_ips"></a> [access\_point\_ips](#output\_access\_point\_ips) | IP addresses of the access points provisioned by this module |
| <a name="output_access_point_name"></a> [access\_point\_name](#output\_access\_point\_name) | Name of the access point provisioned by this module |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
