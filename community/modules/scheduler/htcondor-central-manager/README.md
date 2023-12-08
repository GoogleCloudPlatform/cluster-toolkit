## Description

This module provisions a highly available HTCondor central manager using a [Managed
Instance Group (MIG)][mig] with auto-healing.

[mig]: https://cloud.google.com/compute/docs/instance-groups

## Usage

This module provisions an HTCondor central manager with a standard
configuration. For the node to function correctly, you must supply the input
variable described below:

- [var.central_manager_runner](#input_central_manager_runner)
  - Runner must download a POOL password / signing key and create an [IDTOKEN]
  with no scopes (full authorization).

A reference implementation is included in the Toolkit module
[htcondor-pool-secrets]. You may substitute implementations so long as they
duplicate the functionality in the references. Usage is demonstrated in the
[HTCondor example][htc-example].

[htc-example]: ../../../../examples/README.md#htc-htcondoryaml--
[htcondor-pool-secrets]: ../htcondor-pool-secrets/README.md
[IDTOKEN]: https://htcondor.readthedocs.io/en/latest/admin-manual/security.html#introducing-idtokens

## Behavior of Managed Instance Group (MIG)

A regional [MIG][mig] is used to provision the central manager, although only
1 node will ever be active at a time. By default, the node will be provisioned
in any of the zones available in that region, however, it can be constrained to
run in fewer zones (or a single zone) using [var.zones](#input_zones).

The VM replacement policy is set to [opportunistic]. In practice, this means
that an active VM will not be replaced by Terraform actions, but may be
replaced when either:

- intentionally by issuing an update via Cloud Console or using gcloud (below)
- the VM becomes unhealthy or is otherwise automatically replaced (e.g. regular
  Google Cloud maintenance)

For example, to manually update all instances in a MIG:

```text
gcloud compute instance-groups managed update-instances \
   <<NAME-OF-MIG>> --all-instances --region <<REGION>> \
   --project <<PROJECT_ID>> --minimal-action replace
```

[opportunistic]: https://cloud.google.com/compute/docs/instance-groups/rolling-out-updates-to-managed-instance-groups#type

## Limiting inter-zone egress

Because all the elements of the HTCondor pool use regional MIGs, they may be
subject to [interzone egress fees][network-pricing]. The primary traffic between
nodes of an HTCondor pool running embarrassingly parallel jobs is expected to
be limited to API traffic for job scheduling and monitoring. Please review the
[network pricing][network-pricing] documentation and determine if this cost is
a concern. If it is, use [var.zones](#input_zones) to constrain each node within
your HTCondor pool to operate within a single zone.

[network-pricing]: https://cloud.google.com/vpc/network-pricing

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.1.0 |
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
| <a name="module_central_manager_instance_template"></a> [central\_manager\_instance\_template](#module\_central\_manager\_instance\_template) | github.com/terraform-google-modules/terraform-google-vm//modules/instance_template | 84d7959 |
| <a name="module_htcondor_cm"></a> [htcondor\_cm](#module\_htcondor\_cm) | github.com/terraform-google-modules/terraform-google-vm//modules/mig | aea74d1 |
| <a name="module_startup_script"></a> [startup\_script](#module\_startup\_script) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script | 50644b2 |

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket_object.cm_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [time_sleep.mig_warmup](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/sleep) | resource |
| [google_compute_image.htcondor](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_image) | data source |
| [google_compute_instance.cm](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_instance) | data source |
| [google_compute_region_instance_group.cm](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_region_instance_group) | data source |
| [google_compute_zones.available](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_zones) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_central_manager_runner"></a> [central\_manager\_runner](#input\_central\_manager\_runner) | A list of Toolkit runners for configuring an HTCondor central manager | `list(map(string))` | `[]` | no |
| <a name="input_central_manager_service_account_email"></a> [central\_manager\_service\_account\_email](#input\_central\_manager\_service\_account\_email) | Service account e-mail for central manager (can be supplied by htcondor-setup module) | `string` | n/a | yes |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. HTCondor cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Boot disk size in GB | `number` | `20` | no |
| <a name="input_distribution_policy_target_shape"></a> [distribution\_policy\_target\_shape](#input\_distribution\_policy\_target\_shape) | Target shape for instance group managing high availability of central manager | `string` | `"BALANCED"` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enable or Disable OS Login with "ENABLE" or "DISABLE". Set to "INHERIT" to inherit project OS Login setting. | `string` | `"ENABLE"` | no |
| <a name="input_enable_shielded_vm"></a> [enable\_shielded\_vm](#input\_enable\_shielded\_vm) | Enable the Shielded VM configuration (var.shielded\_instance\_config). | `bool` | `false` | no |
| <a name="input_htcondor_bucket_name"></a> [htcondor\_bucket\_name](#input\_htcondor\_bucket\_name) | Name of HTCondor configuration bucket | `string` | n/a | yes |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Custom VM image with HTCondor installed using the htcondor-install module."<br><br>Expected Fields:<br>name: The name of the image. Mutually exclusive with family.<br>family: The image family to use. Mutually exclusive with name.<br>project: The project where the image is hosted. | `map(string)` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to resources. List key, value pairs. | `map(string)` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for HTCondor central managers | `string` | `"c2-standard-4"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata to add to HTCondor central managers | `map(string)` | `{}` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network in which the HTCondor central manager will be created. | `string` | `null` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which HTCondor central manager will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Default region for creating resources | `string` | n/a | yes |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Scopes by which to limit service account attached to central manager. | `set(string)` | <pre>[<br>  "https://www.googleapis.com/auth/cloud-platform"<br>]</pre> | no |
| <a name="input_shielded_instance_config"></a> [shielded\_instance\_config](#input\_shielded\_instance\_config) | Shielded VM configuration for the instance (must set var.enabled\_shielded\_vm) | <pre>object({<br>    enable_secure_boot          = bool<br>    enable_vtpm                 = bool<br>    enable_integrity_monitoring = bool<br>  })</pre> | <pre>{<br>  "enable_integrity_monitoring": true,<br>  "enable_secure_boot": true,<br>  "enable_vtpm": true<br>}</pre> | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork in which the HTCondor central manager will be created. | `string` | `null` | no |
| <a name="input_zones"></a> [zones](#input\_zones) | Zone(s) in which central manager may be created. If not supplied, will default to all zones in var.region. | `list(string)` | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_central_manager_ips"></a> [central\_manager\_ips](#output\_central\_manager\_ips) | IP addresses of the central managers provisioned by this module |
| <a name="output_central_manager_name"></a> [central\_manager\_name](#output\_central\_manager\_name) | Name of the central managers provisioned by this module |
| <a name="output_list_instances_command"></a> [list\_instances\_command](#output\_list\_instances\_command) | Command to list central managers provisioned by this module |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
