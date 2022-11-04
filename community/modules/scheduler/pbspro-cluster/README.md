## Description

This module provisions an entire PBS Professional cluster as a simple alternative to using the [pbspro-server], [pbspro-client], and [pbspro-execution] modules directly.
The following requirements must be observed:

- one must have an existing Altair license server with sufficient licenses to
  run PBS Pro
- jobs should be submitted from a network filesystem mounted on all hosts to
  faciliate file transfers for jobs and their logs

[pbspro-server]: ../pbspro-server/README.md
[pbspro-client]: ../pbspro-client/README.md
[pbspro-execution]: ../../../modules/compute/pbspro-execution/README.md

### Example

The following example snippet demonstrates use of the cluster module in concert
with the [pre-existing-vpc], [pbspro-preinstall], and [filestore] modules.

```yaml
  - id: pbspro_cluster
    source: community/modules/scheduler/pbspro-cluster
    use:
    - network1
    - homefs
    - pbspro_setup
    settings:
      pbs_license_server:  ## IP address or resolvable DNS name of license server
```

[pre-existing-vpc]: ../../../../modules/network/pre-existing-vpc/README.md
[pbspro-preinstall]: ../../scripts/pbspro-preinstall/README.md
[filestore]: ../../../../modules/file-system/filestore/README.md

## Support

PBS Professional is licensed and supported by [Altair][pbspro]. This module is
maintained and supported by the HPC Toolkit team in collaboration with Altair.

[pbspro]: https://www.altair.com/pbs-professional

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
| <a name="module_pbs_client"></a> [pbs\_client](#module\_pbs\_client) | github.com/GoogleCloudPlatform/hpc-toolkit//community/modules/scheduler/pbspro-client | 7206f3b1 |
| <a name="module_pbs_execution"></a> [pbs\_execution](#module\_pbs\_execution) | github.com/GoogleCloudPlatform/hpc-toolkit//community/modules/compute/pbspro-execution | 7206f3b1 |
| <a name="module_pbs_server"></a> [pbs\_server](#module\_pbs\_server) | github.com/GoogleCloudPlatform/hpc-toolkit//community/modules/scheduler/pbspro-server | 7206f3b1 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_client_host_settings"></a> [client\_host\_settings](#input\_client\_host\_settings) | Deploy 0 or more client hosts using vm-instance parameters (https://goo.gle/hpc-toolkit-vm-instance) | `any` | `{}` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. Cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_execution_host_settings"></a> [execution\_host\_settings](#input\_execution\_host\_settings) | Deploy 0 or more execution hosts using vm-instance parameters (https://goo.gle/hpc-toolkit-vm-instance) | `any` | `{}` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. List key, value pairs. | `any` | n/a | yes |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network to attach the VM. | `string` | `"default"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_pbs_client_rpm_url"></a> [pbs\_client\_rpm\_url](#input\_pbs\_client\_rpm\_url) | Path to PBS Pro Client Host RPM file | `string` | n/a | yes |
| <a name="input_pbs_data_service_user"></a> [pbs\_data\_service\_user](#input\_pbs\_data\_service\_user) | PBS Data Service POSIX user | `string` | `"pbsdata"` | no |
| <a name="input_pbs_exec"></a> [pbs\_exec](#input\_pbs\_exec) | Root path in which to install PBS | `string` | `"/opt/pbs"` | no |
| <a name="input_pbs_execution_rpm_url"></a> [pbs\_execution\_rpm\_url](#input\_pbs\_execution\_rpm\_url) | Path to PBS Pro Execution Host RPM file | `string` | n/a | yes |
| <a name="input_pbs_home"></a> [pbs\_home](#input\_pbs\_home) | PBS working directory | `string` | `"/var/spool/pbs"` | no |
| <a name="input_pbs_license_server"></a> [pbs\_license\_server](#input\_pbs\_license\_server) | IP address or DNS name of PBS license server | `string` | n/a | yes |
| <a name="input_pbs_license_server_port"></a> [pbs\_license\_server\_port](#input\_pbs\_license\_server\_port) | Netowrking port of PBS license server | `number` | `6200` | no |
| <a name="input_pbs_server_rpm_url"></a> [pbs\_server\_rpm\_url](#input\_pbs\_server\_rpm\_url) | Path to PBS Pro Server Host RPM file | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which Google Cloud Storage bucket will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Default region for creating resources | `string` | n/a | yes |
| <a name="input_server_conf"></a> [server\_conf](#input\_server\_conf) | A sequence of qmgr commands in format as generated by qmgr -c 'print server' | `string` | `"# empty qmgr configuration file"` | no |
| <a name="input_server_host_settings"></a> [server\_host\_settings](#input\_server\_host\_settings) | Deploy 1 or more server hosts using vm-instance parameters (https://goo.gle/hpc-toolkit-vm-instance) | `any` | `{}` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork to attach the VM. | `string` | `null` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Default zone for creating resources | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
