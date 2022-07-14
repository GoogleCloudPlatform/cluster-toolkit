## Description

**THIS MODULE IS PRE-RELEASE AND DOES NOT YET SUPPORT A FULLY FUNCTIONAL
HTCONDOR POOL**

This module creates a Toolkit runner that will install HTCondor on RedHat 7 or
derivative operating systems such as the CentOS 7 release in the [HPC VM
Image][hpcvmimage].

It also exports a list of Google Cloud APIs which must be enabled prior to
provisioning an HTCondor Pool.

[hpcvmimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm

## Important note

This module enables Linux firewall rules that block access to the instance
metadata server for any POSIX user that is not `root` or `condor`. This prevents
user jobs from being able to escalate privileges to act as the VM. System
services and HTCondor itself can continue to do so, such as writing to Cloud
Logging. This [feature can be disabled](#input_block_metadata_server).

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

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_block_metadata_server"></a> [block\_metadata\_server](#input\_block\_metadata\_server) | Use Linux firewall to block the instance metadata server for users other than root and HTCondor daemons | `bool` | `true` | no |
| <a name="input_enable_docker"></a> [enable\_docker](#input\_enable\_docker) | Install and enable docker daemon alongside HTCondor | `bool` | `true` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_gcp_service_list"></a> [gcp\_service\_list](#output\_gcp\_service\_list) | Google Cloud APIs required by HTCondor |
| <a name="output_install_autoscaler_deps_runner"></a> [install\_autoscaler\_deps\_runner](#output\_install\_autoscaler\_deps\_runner) | Toolkit Runner to install HTCondor autoscaler dependencies |
| <a name="output_install_autoscaler_runner"></a> [install\_autoscaler\_runner](#output\_install\_autoscaler\_runner) | Toolkit Runner to install HTCondor autoscaler |
| <a name="output_install_htcondor_runner"></a> [install\_htcondor\_runner](#output\_install\_htcondor\_runner) | Runner to install HTCondor using startup-scripts |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
