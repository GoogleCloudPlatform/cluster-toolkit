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

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_gcp_service_list"></a> [gcp\_service\_list](#output\_gcp\_service\_list) | Google Cloud APIs required by HTCondor |
| <a name="output_install_htcondor_runner"></a> [install\_htcondor\_runner](#output\_install\_htcondor\_runner) | Runner to install HTCondor using startup-scripts |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
