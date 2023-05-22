## Description

This module will create a set of startup-script runners that will add ramble to
profile.d, and install Ramble’s dependencies. This module will not install
Ramble. It allows VMs with access to a shared installation of
Ramble ensure they are configured to properly run Ramble.

Ramble is a multi-platform experimentation framework capable of driving
software installation, acquiring input files, configuring experiments, and
extracting results. For more information about ramble, see:
https://github.com/GoogleCloudPlatform/ramble

This module outputs a startup script runner, which will install Ramble’s
dependencies, and add its setup script to /etc/profile.d.

> **_NOTE:_** This is an experimental module and the functionality and
> documentation will likely be updated in the near future. This module has only
> been tested in limited capacity.

# Examples

## Basic Example

```yaml
- id: ramble-setup
  source: community/modules/scripts/ramble-setup

- id: ramble-access
  source: community/modules/scripts/ramble-access
  use: [ramble-setup]
```

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_ramble_path"></a> [ramble\_path](#input\_ramble\_path) | Directory where Ramble is installed. Note: This module will not actually install Ramble | `string` | `"/apps/ramble"` | no |
| <a name="input_ramble_ref"></a> [ramble\_ref](#input\_ramble\_ref) | Git ref to checkout for Ramble. | `string` | `"develop"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_ramble_runner"></a> [ramble\_runner](#output\_ramble\_runner) | Runner to setup Ramble access using an ansible playbook. The startup-script<br>module will automatically handle installation of ansible. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
