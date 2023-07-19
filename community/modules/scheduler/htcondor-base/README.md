## Description

This module creates the basic security infrastructure of an HTCondor pool in
Google Cloud.

> **_NOTE:_** This module was previously named htcondor-configure. The interface
> and responsibilities of this module have changed significantly. Please review
> the [example](#example) and modify your blueprints accordingly.

## Security setup

This module will take the following actions:

- store an HTCondor Pool password in Google Cloud Secret Manager
  - will generate a new password if one is not supplied
- create service accounts for an HTCondor Access Point and Central Manager

It is expected to be used with the [htcondor-install] and
[htcondor-execute-point] modules.

[hpcvmimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm
[htcondor-install]: ../../scripts/htcondor-base/README.md
[htcondor-execute-point]: ../../compute/htcondor-execute-point/README.md

[htcrole]: https://htcondor.readthedocs.io/en/latest/getting-htcondor/admin-quick-start.html#what-get-htcondor-does-to-configure-a-role

### Example

The following code snippet uses this module to create a startup script that
installs HTCondor software and configures an HTCondor Central Manager. A full
example can be found in the [examples README][htc-example].

[htc-example]: ../../../../examples/README.md#htc-htcondoryaml--

```yaml
- id: network1
  source: modules/network/pre-existing-vpc

- id: htcondor_install
  source: community/modules/scripts/htcondor-install

- id: htcondor_configure
  source: community/modules/scheduler/htcondor-base
  use:
  - network1

- id: htcondor_central_manager_startup
  source: modules/scripts/startup-script
  settings:
    runners:
    - $(htcondor_install.install_htcondor_runner)
    - $(htcondor_configure.central_manager_runner)

- id: htcondor_cm
  source: modules/compute/vm-instance
  use:
  - network1
  - htcondor_central_manager_startup
  settings:
    name_prefix: cm0
    machine_type: c2-standard-4
    disable_public_ips: true
    service_account:
      email: $(htcondor_configure.central_manager_service_account)
      scopes:
      - cloud-platform
    network_interfaces:
    - network: null
      subnetwork: $(network1.subnetwork_self_link)
      subnetwork_project: $(vars.project_id)
      network_ip: $(htcondor_configure.central_manager_internal_ip)
      stack_type: null
      access_config: []
      ipv6_access_config: []
      alias_ip_range: []
      nic_type: VIRTIO_NET
      queue_count: null
  outputs:
  - internal_ip
```

## High Availability

This module supports high availability modes of the HTCondor Central Manager and
of the Access Points. In these modes, the services can be resiliant against
zonal failures by distributing the services across two zones. Modify the above
example by setting `central_manager_high_availability` to `true` and adding a
new deployment variable `zone_secondary` set to another zone in the same region.
The 2 VMs can use the same startup script, but should differ by setting:

- primary and secondary zones defined in deployment variables
- primary and secondary IP addresses created by this module
- differing name prefixes

```yaml
vars:
  # add typical settings (deployment_name, project_id, etc.)
  # select a region and 2 different zones within the region
  region: us-central1
  zone: us-central1-c
  zone_secondary: us-central1-f

- id: htcondor_configure
  source: community/modules/scheduler/htcondor-base
  use:
  - network1
  settings:
    central_manager_high_availability: true

- id: htcondor_cm_primary
  source: modules/compute/vm-instance
  use:
  - network1
  - htcondor_central_manager_startup
  settings:
    name_prefix: cm0
    machine_type: c2-standard-4
    disable_public_ips: true
    service_account:
      email: $(htcondor_configure.central_manager_service_account)
      scopes:
      - cloud-platform
    network_interfaces:
    - network: null
      subnetwork: $(network1.subnetwork_self_link)
      subnetwork_project: $(vars.project_id)
      network_ip: $(htcondor_configure.central_manager_internal_ip)
      stack_type: null
      access_config: []
      ipv6_access_config: []
      alias_ip_range: []
      nic_type: VIRTIO_NET
      queue_count: null
  outputs:
  - internal_ip

- id: htcondor_cm_secondary
  source: modules/compute/vm-instance
  use:
  - network1
  - htcondor_central_manager_startup
  settings:
    name_prefix: cm1
    machine_type: c2-standard-4
    zone: $(vars.zone_secondary)
    disable_public_ips: true
    service_account:
      email: $(htcondor_configure.central_manager_service_account)
      scopes:
      - cloud-platform
    network_interfaces:
    - network: null
      subnetwork: $(network1.subnetwork_self_link)
      subnetwork_project: $(vars.project_id)
      network_ip: $(htcondor_configure.central_manager_secondary_internal_ip)
      stack_type: null
      access_config: []
      ipv6_access_config: []
      alias_ip_range: []
      nic_type: VIRTIO_NET
      queue_count: null
  outputs:
  - internal_ip

```

Access Point high availability is impacted by known issues [HTCONDOR-1590] and
[HTCONDOR-1594]. These are anticipated to be resolved in LTS release 10.0.3 and
above or feature release 10.4 and above. Please see [HTCondor version
numbering][htcver] and [release notes][htcnotes] for details.

[htcver]: https://htcondor.readthedocs.io/en/latest/version-history/introduction-version-history.html#types-of-releases
[htcnotes]: https://htcondor.readthedocs.io/en/latest/version-history/index.html
[HTCONDOR-1590]: https://opensciencegrid.atlassian.net/jira/software/c/projects/HTCONDOR/issues/HTCONDOR-1590
[HTCONDOR-1594]: https://opensciencegrid.atlassian.net/jira/software/c/projects/HTCONDOR/issues/HTCONDOR-1594

## Support

HTCondor is maintained by the [Center for High Throughput Computing][chtc] at
the University of Wisconsin-Madison. Support for HTCondor is available via:

- [Discussion lists](https://htcondor.org/mail-lists/)
- [HTCondor on GitHub](https://github.com/htcondor/htcondor/)
- [HTCondor manual](https://htcondor.readthedocs.io/en/latest/)

[chtc]: https://chtc.cs.wisc.edu/

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_access_point_service_account"></a> [access\_point\_service\_account](#module\_access\_point\_service\_account) | terraform-google-modules/service-accounts/google | ~> 4.2 |
| <a name="module_central_manager_service_account"></a> [central\_manager\_service\_account](#module\_central\_manager\_service\_account) | terraform-google-modules/service-accounts/google | ~> 4.2 |
| <a name="module_execute_point_service_account"></a> [execute\_point\_service\_account](#module\_execute\_point\_service\_account) | terraform-google-modules/service-accounts/google | ~> 4.2 |
| <a name="module_health_check_firewall_rule"></a> [health\_check\_firewall\_rule](#module\_health\_check\_firewall\_rule) | terraform-google-modules/network/google//modules/firewall-rules | ~> 6.0 |
| <a name="module_htcondor_bucket"></a> [htcondor\_bucket](#module\_htcondor\_bucket) | terraform-google-modules/cloud-storage/google | ~> 4.0 |

## Resources

| Name | Type |
|------|------|
| [google_compute_subnetwork.htcondor](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_access_point_roles"></a> [access\_point\_roles](#input\_access\_point\_roles) | Project-wide roles for HTCondor Access Point service account | `list(string)` | <pre>[<br>  "roles/compute.instanceAdmin",<br>  "roles/monitoring.metricWriter",<br>  "roles/logging.logWriter",<br>  "roles/storage.objectViewer"<br>]</pre> | no |
| <a name="input_central_manager_roles"></a> [central\_manager\_roles](#input\_central\_manager\_roles) | Project-wide roles for HTCondor Central Manager service account | `list(string)` | <pre>[<br>  "roles/monitoring.metricWriter",<br>  "roles/logging.logWriter",<br>  "roles/storage.objectViewer"<br>]</pre> | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. HTCondor cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_execute_point_roles"></a> [execute\_point\_roles](#input\_execute\_point\_roles) | Project-wide roles for HTCondor Execute Point service account | `list(string)` | <pre>[<br>  "roles/monitoring.metricWriter",<br>  "roles/logging.logWriter",<br>  "roles/storage.objectViewer"<br>]</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to resources. List key, value pairs. | `map(string)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which HTCondor pool will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Default region for creating resources | `string` | n/a | yes |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork in which Central Managers will be placed. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_access_point_service_account_email"></a> [access\_point\_service\_account\_email](#output\_access\_point\_service\_account\_email) | HTCondor Access Point Service Account (e-mail format) |
| <a name="output_central_manager_service_account_email"></a> [central\_manager\_service\_account\_email](#output\_central\_manager\_service\_account\_email) | HTCondor Central Manager Service Account (e-mail format) |
| <a name="output_execute_point_service_account_email"></a> [execute\_point\_service\_account\_email](#output\_execute\_point\_service\_account\_email) | HTCondor Execute Point Service Account (e-mail format) |
| <a name="output_htcondor_bucket_name"></a> [htcondor\_bucket\_name](#output\_htcondor\_bucket\_name) | Name of the HTCondor configuration bucket |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
