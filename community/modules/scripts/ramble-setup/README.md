## Description

This module will create a set of startup-script runners that will setup Ramble,
and install Ramble’s dependencies.

Ramble is a multi-platform experimentation framework capable of driving
software installation, acquiring input files, configuring experiments, and
extracting results. For more information about ramble, see:
https://github.com/GoogleCloudPlatform/ramble

This module outputs two startup script runners, which can be added to startup
scripts to setup, ramble and its dependencies.

For this module to be completely functionaly, it depends on a spack
installation. For more information, see HPC-Toolkit’s Spack module.

> **_NOTE:_** This is an experimental module and the functionality and
> documentation will likely be updated in the near future. This module has only
> been tested in limited capacity.

# Examples

## Basic Example

```yaml
- id: ramble-setup
  source: community/modules/scripts/ramble-setup
```

This example simply installs ramble on a VM.

## Full Example

```yaml
- id: ramble-setup
  source: community/modules/scripts/ramble-setup
  settings:
    install_dir: /ramble
    ramble_url: https://github.com/GoogleCloudPlatform/ramble
    ramble_ref: v0.2.1
    log_file: /var/log/ramble.log
    chown_owner: “owner”
    chgrp_group: “user_group”
    chmod_mode: “a+r”
```

This example simply installs ramble into a VM at the location `/ramble`, checks
out the v0.2.1 tag, changes the owner and group to “owner” and “user_group”,
and chmod’s the clone to make it world readable.

Also see a more complete [Ramble example blueprint](../../../examples/ramble.yaml).

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.42 |
| <a name="requirement_local"></a> [local](#requirement\_local) | >= 2.0.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.42 |
| <a name="provider_local"></a> [local](#provider\_local) | >= 2.0.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_startup_script"></a> [startup\_script](#module\_startup\_script) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script | 50644b2 |

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket.bucket](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket) | resource |
| [local_file.debug_file_shell_install](https://registry.terraform.io/providers/hashicorp/local/latest/docs/resources/file) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_chgrp_group"></a> [chgrp\_group](#input\_chgrp\_group) | Group to chgrp the Ramble clone to. Default will not modify the clone. | `string` | `null` | no |
| <a name="input_chmod_mode"></a> [chmod\_mode](#input\_chmod\_mode) | Mode to chmod the Ramble clone to. Defaults to null (i.e. do not modify).<br>For usage information see:<br>https://docs.ansible.com/ansible/latest/collections/ansible/builtin/file_module.html#parameter-mode | `string` | `null` | no |
| <a name="input_chown_owner"></a> [chown\_owner](#input\_chown\_owner) | Owner to chown the Ramble clone to. Default will not modify the clone. | `string` | `null` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of deployment, used to name bucket containing startup script. | `string` | n/a | yes |
| <a name="input_install_dir"></a> [install\_dir](#input\_install\_dir) | Destination directory of installation of Ramble. | `string` | `"/apps/ramble"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Key-value pairs of labels to be added to created resources. | `map(string)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created. | `string` | n/a | yes |
| <a name="input_ramble_profile_script_path"></a> [ramble\_profile\_script\_path](#input\_ramble\_profile\_script\_path) | Path to the Ramble profile.d script. Created by this module | `string` | `"/etc/profile.d/ramble.sh"` | no |
| <a name="input_ramble_ref"></a> [ramble\_ref](#input\_ramble\_ref) | Git ref to checkout for Ramble. | `string` | `"develop"` | no |
| <a name="input_ramble_url"></a> [ramble\_url](#input\_ramble\_url) | URL for Ramble repository to clone. | `string` | `"https://github.com/GoogleCloudPlatform/ramble"` | no |
| <a name="input_ramble_virtualenv_path"></a> [ramble\_virtualenv\_path](#input\_ramble\_virtualenv\_path) | Virtual environment path in which to install Ramble Python interpreter and other dependencies | `string` | `"/usr/local/ramble-python"` | no |
| <a name="input_region"></a> [region](#input\_region) | Region to place bucket containing startup script. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_controller_startup_script"></a> [controller\_startup\_script](#output\_controller\_startup\_script) | Ramble installation script, duplicate for SLURM controller. |
| <a name="output_gcs_bucket_path"></a> [gcs\_bucket\_path](#output\_gcs\_bucket\_path) | Bucket containing the startup scripts for Ramble, to be reused by ramble-execute module. |
| <a name="output_ramble_path"></a> [ramble\_path](#output\_ramble\_path) | Location ramble is installed into. |
| <a name="output_ramble_profile_script_path"></a> [ramble\_profile\_script\_path](#output\_ramble\_profile\_script\_path) | Path to Ramble profile script. |
| <a name="output_ramble_ref"></a> [ramble\_ref](#output\_ramble\_ref) | Git ref the ramble install is checked out to use |
| <a name="output_ramble_runner"></a> [ramble\_runner](#output\_ramble\_runner) | Runner to be used with startup-script module or passed to ramble-execute module.<br>- installs Ramble dependencies<br>- installs Ramble<br>- generates profile.d script to enable access to Ramble<br>This is safe to run in parallel by multiple machines. |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | Ramble installation script. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
