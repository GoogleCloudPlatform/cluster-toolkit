## Description

This module creates a bucket in which to store HTCondor configurations and
a firewall rule that allows Managed Instance Group health checks to probe the
health of HTCondor VMs.

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

- id: htcondor_service_accounts
  source: community/modules/scheduler/htcondor-service-accounts

- id: htcondor_setup
  source: community/modules/scheduler/htcondor-setup
  use:
  - network1
  - htcondor_service_accounts

- id: htcondor_secrets
  source: community/modules/scheduler/htcondor-pool-secrets
  use:
  - htcondor_service_accounts

- id: htcondor_cm
  source: community/modules/scheduler/htcondor-central-manager
  use:
  - network1
  - htcondor_secrets
  - htcondor_service_accounts
  - htcondor_setup
  settings:
    instance_image:
      project: $(vars.project_id)
      family: $(vars.new_image_family)
  outputs:
  - central_manager_name
```

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

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_health_check_firewall_rule"></a> [health\_check\_firewall\_rule](#module\_health\_check\_firewall\_rule) | ../../../../modules/network/firewall-rules | n/a |
| <a name="module_htcondor_bucket"></a> [htcondor\_bucket](#module\_htcondor\_bucket) | ../../../../community/modules/file-system/cloud-storage-bucket | n/a |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_access_point_service_account_email"></a> [access\_point\_service\_account\_email](#input\_access\_point\_service\_account\_email) | Service account e-mail for HTCondor Access Point | `string` | n/a | yes |
| <a name="input_central_manager_service_account_email"></a> [central\_manager\_service\_account\_email](#input\_central\_manager\_service\_account\_email) | Service account e-mail for HTCondor Central Manager | `string` | n/a | yes |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Cluster Toolkit deployment name. HTCondor cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_execute_point_service_account_email"></a> [execute\_point\_service\_account\_email](#input\_execute\_point\_service\_account\_email) | Service account e-mail for HTCondor Execute Points | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to resources. List key, value pairs. | `map(string)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which HTCondor pool will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Default region for creating resources | `string` | n/a | yes |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork in which Central Managers will be placed. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_htcondor_bucket_name"></a> [htcondor\_bucket\_name](#output\_htcondor\_bucket\_name) | Name of the HTCondor configuration bucket |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
