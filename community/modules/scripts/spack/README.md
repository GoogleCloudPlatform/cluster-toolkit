## Description

This module will create a set of startup-script runners that can be used to
install and configure [spack] on a VM.

## Examples

### Simple Example
The following snippet shows how a fairly simple spack installation runner can be
created and used:

```yaml
- id: spack
  source: community/modules/scripts/spack
  settings:
    compilers:
    - gcc@10.3.0 target=x86_64
    packages:
    - intel-mpi@2018.4.274%gcc@10.3.0
    - gromacs@2021.2 %gcc@10.3.0 ^intel-mpi@2018.4.274
- id: startup
  source: modules/scripts/startup-script
  settings:
    runners:
    - $(spack.spack_deps_runner)
    - $(spack.spack_full_install_runner)
- id: workstation
  source: modules/compute/vm-instance
  use: [startup]
```

In this example, a set of compilers and spack packages have been defined in the
`spack` module, in this case they are based on the [spack-gromacs.yaml] example.
Through the `spack_full_install_runner`, spack will be cloned onto the VM and
the compilers will be installed followed by the packages. In order to ensure
some basic dependencies such as git and the `google-cloud-storage` pip package
have been installed, the `spack_deps_runner` is included in the set of startup
script runners.

This set of two startup script runners will be executed at startup on the
`workstation` VM instance as the `startup` module is supplied to it via `use`.

### Fine Grained Example
The following example expands on the simple example above in order to show the
more flexible functionality of the spack module:

```yaml
- id: spack
  source: community/modules/scripts/spack
  settings:
    install_dir: /sw/spack
    log_file: /var/log/spack/install-runner.log
    commands:
    - spack config --scope=defaults add "config:build_stage:[/sw/spack/spack-stage]"
    - spack config --scope=defaults add -f /sw/spack_modules.yaml
    - spack mirror add gcs_cache gs://YOUR-CACHE-BUCKET-NAME
    - spack buildcache keys --install --trust
    compilers:
    - gcc@10.3.0 target=x86_64
    packages:
    - intel-mpi@2018.4.274%gcc@10.3.0
    - gromacs@2021.2 %gcc@10.3.0 ^intel-mpi@2018.4.274
- id: startup
  source: modules/scripts/startup-script
  settings:
    runners:
    - type: 'data'
     destination: '/sw/spack_modules.yaml'
     content: |
        modules:
          default:
            tcl:
              hash_length: 0
              all:
                conflict:
                - '{name}'
              projections:
                all: '{name}/{version}-{compiler.name}-{compiler.version}'
    - $(spack.spack_deps_runner)
    - $(spack.spack_clone_runner)
    - $(spack.spack_commands_runner)
    - $(spack.spack_compilers_runner)
    - $(spack.spack_packages_runner)
    - $(spack.spack_setup_runner)
- id: workstation
  source: modules/compute/vm-instance
  use: [startup]
```

In this case, we are updating the default spack config and adding a build cache
from a GCS bucket. In addition, rather than use the `spack_full_install_runner`,
each of the individual runners are added independently which provides
flexibility to execute additional startup scripts at different stages of the
spack configuration or to run them in a different order. This may be useful if
the commands provided assume compilers or packages have already been installed.

More information about each of these runners will be provided in the next
section.

## Runners

**`spack_deps_runner`:** This runner installs dependencies for the spack
installation, which include git, pip3 and the `google-cloud-storage` pip package.

**`spack_setup_runner`:** This runner updates the `profile.d` configuration to
set the spack environment on login by default. If not used, the spack
environment can be set manually by sourcing the `setup-env.sh` file stored at
`share/spack/setup-env.sh` in the `install_dir` (defaults to `/apps/spack`).

**`spack_full_install_runner`:** This runner will perform all of the actions
in the following runners in the order listed here:

1. `spack_clone_runner`
1. `spack_commands_runner`
1. `spack_compilers_runner`
1. `spack_packages_runner`

Unless your spack configuration requires significant customization, it's
recommended to use this runner rather than the ones below, as it enforces a
standard order of execution and is much less verbose.

**`spack_clone_runner`:** This runner clones the spack repo based on the
settings provided. In addition to this, it also sets permissions for the install
directory.

**`spack_commands_runner`:** This runner executes the commands provided in the
`commands` setting in order. The runner will set the environment so that `spack`
is available. It's intended to provide a mechanism for leveraging `spack` tools
directly that are not directly supported by this module.

**`spack_compilers_runner`:** This runner installs, loads and sets up the
compilers in the spack environment in the order provided.

**`spack_packages_runner`:** This runner installs the packages using spack in
the order provided.

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
| <a name="input_install_dir"></a> [install\_dir](#input\_install\_dir) | Destination directory of installation of Spack | `string` | `"/apps/spack"` | no |
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
| <a name="output_spack_clone_runner"></a> [spack\_clone\_runner](#output\_spack\_clone\_runner) | Runner to install Spack using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.spack\_clone\_runner)<br>... |
| <a name="output_spack_commands_runner"></a> [spack\_commands\_runner](#output\_spack\_commands\_runner) | Runner to run Spack commands using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.spack\_commands\_runner)<br>... |
| <a name="output_spack_compilers_runner"></a> [spack\_compilers\_runner](#output\_spack\_compilers\_runner) | Runner to install and configure compilers using Spack using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.spack\_compilers\_runner)<br>... |
| <a name="output_spack_deps_runner"></a> [spack\_deps\_runner](#output\_spack\_deps\_runner) | Runner to install dependencies for spack using an ansible playbook. The<br>startup-script module will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.spack\_deps\_runner)<br>... |
| <a name="output_spack_full_install_runner"></a> [spack\_full\_install\_runner](#output\_spack\_full\_install\_runner) | Runner that incorporates the contents of the following other runners, executed<br>in this order: spack\_clone\_runner, spack\_commands\_runner,<br>spack\_compiler\_runner, spack\_packages\_runner.<br><br>Usage:<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.spack\_full\_install\_runner)<br>... |
| <a name="output_spack_packages_runner"></a> [spack\_packages\_runner](#output\_spack\_packages\_runner) | Runner to install Spack packages using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.spack\_packages\_runner)<br>... |
| <a name="output_spack_path"></a> [spack\_path](#output\_spack\_path) | Location spack is installed into. |
| <a name="output_spack_setup_runner"></a> [spack\_setup\_runner](#output\_spack\_setup\_runner) | Adds Spack setup-env.sh script to /etc/profile.d so that it is called at<br>shell startup. Among other things this adds Spack binary to user PATH. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
