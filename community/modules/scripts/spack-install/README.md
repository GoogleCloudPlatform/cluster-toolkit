## Description

This module will create a set of startup-script runners that will install Spack,
and execute an arbitrary number of Spack commands.

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
| <a name="input_caches_to_populate"></a> [caches\_to\_populate](#input\_caches\_to\_populate) | DEPRECATED | `any` | `null` | no |
| <a name="input_chgrp_group"></a> [chgrp\_group](#input\_chgrp\_group) | Group to chgrp the Spack clone to | `string` | `"root"` | no |
| <a name="input_chmod_mode"></a> [chmod\_mode](#input\_chmod\_mode) | Mode to chmod the Spack clone to. | `string` | `""` | no |
| <a name="input_chown_owner"></a> [chown\_owner](#input\_chown\_owner) | Owner to chown the Spack clone to | `string` | `"root"` | no |
| <a name="input_commands"></a> [commands](#input\_commands) | Commands to execute within spack | `list(string)` | `[]` | no |
| <a name="input_compilers"></a> [compilers](#input\_compilers) | Defines compilers for spack to install before installing packages. | `list(string)` | `[]` | no |
| <a name="input_concretize_flags"></a> [concretize\_flags](#input\_concretize\_flags) | DEPRECATED | `any` | `null` | no |
| <a name="input_configs"></a> [configs](#input\_configs) | DEPRECATED | `any` | `null` | no |
| <a name="input_environments"></a> [environments](#input\_environments) | DEPRECATED | `any` | `null` | no |
| <a name="input_gpg_keys"></a> [gpg\_keys](#input\_gpg\_keys) | DEPRECATED | `any` | `null` | no |
| <a name="input_install_dir"></a> [install\_dir](#input\_install\_dir) | Destination directory of installation of Spack | `string` | `"/apps/ramble"` | no |
| <a name="input_install_flags"></a> [install\_flags](#input\_install\_flags) | DEPRECATED | `any` | `null` | no |
| <a name="input_licenses"></a> [licenses](#input\_licenses) | DEPRECATED | `any` | `null` | no |
| <a name="input_log_file"></a> [log\_file](#input\_log\_file) | Log file to write output from spack steps into | `string` | `"/var/log/spack.log"` | no |
| <a name="input_packages"></a> [packages](#input\_packages) | Defines root packages for spack to install (in order). | `list(string)` | `[]` | no |
| <a name="input_spack_cache_url"></a> [spack\_cache\_url](#input\_spack\_cache\_url) | DEPRECATED | `any` | `null` | no |
| <a name="input_spack_ref"></a> [spack\_ref](#input\_spack\_ref) | Git ref to checkout for Spack | `string` | `"v0.19.0"` | no |
| <a name="input_spack_url"></a> [spack\_url](#input\_spack\_url) | URL for Spack repository to clone | `string` | `"https://github.com/spack/spack"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_install_spack_deps_runner"></a> [install\_spack\_deps\_runner](#output\_install\_spack\_deps\_runner) | Runner to install dependencies for spack using an ansible playbook. The<br>startup-script module will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.install\_spack\_deps\_runner)<br>... |
| <a name="output_install_spack_runner"></a> [install\_spack\_runner](#output\_install\_spack\_runner) | Runner to both install Spack and execute arbitrary Spack commands.<br>Provided to maintain compatibility with older spack modules. The<br>startup-script modules will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.install\_spack\_runner)<br>... |
| <a name="output_setup_spack_runner"></a> [setup\_spack\_runner](#output\_setup\_spack\_runner) | Adds Spack setup-env.sh script to /etc/profile.d so that it is called at shell startup. Among other things this adds Spack binary to user PATH. |
| <a name="output_spack_commands_runner"></a> [spack\_commands\_runner](#output\_spack\_commands\_runner) | Runner to run Spack commands using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.spack\_commands\_runner)<br>... |
| <a name="output_spack_compilers_runner"></a> [spack\_compilers\_runner](#output\_spack\_compilers\_runner) | Runner to install and configure compilers using Spack using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.spack\_compilers\_runner)<br>... |
| <a name="output_spack_install_runner"></a> [spack\_install\_runner](#output\_spack\_install\_runner) | Runner to install Spack using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.spack\_install\_runner)<br>... |
| <a name="output_spack_packages_runner"></a> [spack\_packages\_runner](#output\_spack\_packages\_runner) | Runner to install Spack packages using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.spack\_packages\_runner)<br>... |
| <a name="output_spack_path"></a> [spack\_path](#output\_spack\_path) | Location spack is installed into. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
