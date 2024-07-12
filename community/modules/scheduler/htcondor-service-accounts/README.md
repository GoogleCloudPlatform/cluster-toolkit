## Description

This module creates the service accounts for use by the primary elements of an
[HTCondor pool][pool]:

- Central Managers
- Access Points
- Execute Points

Each service account is assigned common roles necessary for the VM to function
properly. In particular, nearly every VM requires the ability to read from Cloud
Storage buckets and write Cloud Logging entries. These roles are configurable
as described below.

[pool]: https://htcondor.readthedocs.io/en/latest/admin-manual/introduction-admin-manual.html#the-different-roles-a-machine-can-play

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
Copyright 2024 Google LLC

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
| <a name="module_access_point_service_account"></a> [access\_point\_service\_account](#module\_access\_point\_service\_account) | github.com/GoogleCloudPlatform/hpc-toolkit//community/modules/project/service-account | v1.35.0&depth=1 |
| <a name="module_central_manager_service_account"></a> [central\_manager\_service\_account](#module\_central\_manager\_service\_account) | github.com/GoogleCloudPlatform/hpc-toolkit//community/modules/project/service-account | v1.35.0&depth=1 |
| <a name="module_execute_point_service_account"></a> [execute\_point\_service\_account](#module\_execute\_point\_service\_account) | github.com/GoogleCloudPlatform/hpc-toolkit//community/modules/project/service-account | v1.35.0&depth=1 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_access_point_roles"></a> [access\_point\_roles](#input\_access\_point\_roles) | Project-wide roles for HTCondor Access Point service account | `list(string)` | <pre>[<br>  "compute.instanceAdmin.v1",<br>  "monitoring.metricWriter",<br>  "logging.logWriter",<br>  "storage.objectViewer"<br>]</pre> | no |
| <a name="input_central_manager_roles"></a> [central\_manager\_roles](#input\_central\_manager\_roles) | Project-wide roles for HTCondor Central Manager service account | `list(string)` | <pre>[<br>  "monitoring.metricWriter",<br>  "logging.logWriter",<br>  "storage.objectViewer"<br>]</pre> | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. HTCondor cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_execute_point_roles"></a> [execute\_point\_roles](#input\_execute\_point\_roles) | Project-wide roles for HTCondor Execute Point service account | `list(string)` | <pre>[<br>  "monitoring.metricWriter",<br>  "logging.logWriter",<br>  "storage.objectViewer"<br>]</pre> | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which HTCondor pool will be created | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_access_point_service_account_email"></a> [access\_point\_service\_account\_email](#output\_access\_point\_service\_account\_email) | HTCondor Access Point Service Account (e-mail format) |
| <a name="output_central_manager_service_account_email"></a> [central\_manager\_service\_account\_email](#output\_central\_manager\_service\_account\_email) | HTCondor Central Manager Service Account (e-mail format) |
| <a name="output_execute_point_service_account_email"></a> [execute\_point\_service\_account\_email](#output\_execute\_point\_service\_account\_email) | HTCondor Execute Point Service Account (e-mail format) |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
