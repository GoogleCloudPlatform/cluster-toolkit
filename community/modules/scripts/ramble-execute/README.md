## Description

This module will create startup-script runner that will execute Ramble commands.

Ramble is a multi-platform experimentation framework capable of driving
software installation, acquiring input files, configuring experiments, and
extracting results. For more information about ramble, see:
https://github.com/GoogleCloudPlatform/ramble

This module outputs a startup script runner, which can be added to a collective
startups script to execute a set of ramble commands.

For this module to be completely functionaly, it depends on a spack
installation. For more information, see HPC-Toolkit’s Spack module.

> **_NOTE:_** This is an experimental module and the functionality and
> documentation will likely be updated in the near future. This module has only
> been tested in limited capacity.

# Examples

## Basic Example

```yaml
- id: spack
  source: community/modules/scripts/spack-install
- id: ramble-setup
  source: community/modules/scripts/ramble-setup
- id: ramble-execute
  source: community/modules/scripts/ramble-execute
  use: [spack, ramble-setup]
  settings:
    commands:
    - ramble list
```

This example shows installing spack and ramble with their own modules
(spack-install and ramble-setup respectively). Then the ramble-execute module
is added to simply list all applications ramble knows about.

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_commands"></a> [commands](#input\_commands) | Commands to execute within ramble | `list(string)` | `[]` | no |
| <a name="input_log_file"></a> [log\_file](#input\_log\_file) | Log file to write output from ramble execute steps into | `string` | `"/var/log/ramble-execute.log"` | no |
| <a name="input_ramble_path"></a> [ramble\_path](#input\_ramble\_path) | Path to the ramble installation | `string` | `""` | no |
| <a name="input_ramble_runner"></a> [ramble\_runner](#input\_ramble\_runner) | Ansible based startup-script runner from a previous ramble step | <pre>object({<br>    type        = string<br>    content     = string<br>    destination = string<br>  })</pre> | `null` | no |
| <a name="input_spack_path"></a> [spack\_path](#input\_spack\_path) | Path to the spack installation | `string` | `""` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_ramble_runner"></a> [ramble\_runner](#output\_ramble\_runner) | Runner to execute Ramble commands using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-ramble-id.ramble\_execute\_runner)<br>... |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
