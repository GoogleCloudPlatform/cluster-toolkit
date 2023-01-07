## Description

This module will create a set of startup-script runners that will install Ramble,
and execute an arbitrary number of Ramble commands.

For more information about ramble, see: https://github.com/GoogleCloudPlatform/ramble

This module outputs four startup script runners, which can be added to startup
scripts to install, configure, and execute ramble benchmarks.

For this module to be completely functionaly, it depends on a spack
installation. For more information, see HPC-Toolkit’s Spack module.

# Examples

## Simple Example

```yaml
- id: spack
  source: community/modules/scripts/spack
- id: ramble
  source: community/modules/scripts/ramble
  use: [spack] # Depends on a spack module name
  settings:
    commands:
    - ramble workspace create foo
- id: startup
  source: modules/scripts/startup-script
  settings:
    runners:
    - $(spack.spack_deps_runner))
    - $(spack.spack_full_install_runner)
    - $(ramble.ramble_deps_runner))
    - $(ramble.ramble_install_runner)
    - $(ramble.ramble_commands_runner)
- id: workstation
  source: modules/compute/vm-instance
  use: [startup]
```

In this example, both spack and ramble are installed on the resulting
workstation. Ramble is installed through the use of the
`ramble.ramble_install_runner`, and ramble specific commands are executed via
the `ramble.ramble_commands_runner`. In order to ensure ￼some basic
dependencies such as git and the `google-cloud-storage` pip package ￼have been
installed, the `ramble.ramble_deps_runner` is included.

This set of two startup script runners will be executed at startup on the
￼`workstation` VM instance as the `startup` module is supplied to it via `use`.

## Full Example

```yaml
- id: spack
  source: community/modules/scripts/spack
- id: ramble
  source: community/modules/scripts/ramble
  use: [spack] # Depends on a spack module name
  settings:
    commands:
    - ramble workspace create hostname_test -c /apps/hostname_experiments.yaml -t /apps/hostname_execute.tpl
    - ramble -w hostname_test workspace setup
    - ramble -w hostname_test on
    - ramble -w hostname_test workspace analyze
- id: startup
  source: modules/scripts/startup-script
  settings:
    runners:
    - type: ‘data’
      destination: /apps/hostname_experiments.yaml
      content: |
        ramble:
          applications:
            hostname:
              workloads:
                serial:
                  experiments:
                    run_hostname:
                      n_nodes: 1
                      n_ranks: 1
                      processes_per_node: 1
    - type: ‘data’
      destination: /apps/hostname_execute.tpl
      content: |
        #!/bin/bash
        {command}
    - $(spack.spack_deps_runner))
    - $(spack.spack_full_install_runner)
    - $(ramble.ramble_deps_runner))
    - $(ramble.ramble_install_runner)
    - $(ramble.ramble_commands_runner)
- id: workstation
  source: modules/compute/vm-instance
  use: [startup]
```

This example builds off of the previous example by adding two data runners to
generate ramble configuration and template files. These files are then used
within the ramble module to create, setup, execute, and analyze experiments
within a workspace.

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
| <a name="input_chgrp_group"></a> [chgrp\_group](#input\_chgrp\_group) | Group to chgrp the Ramble clone to | `string` | `"root"` | no |
| <a name="input_chmod_mode"></a> [chmod\_mode](#input\_chmod\_mode) | Mode to chmod the Ramble clone to. | `string` | `""` | no |
| <a name="input_chown_owner"></a> [chown\_owner](#input\_chown\_owner) | Owner to chown the Ramble clone to | `string` | `"root"` | no |
| <a name="input_commands"></a> [commands](#input\_commands) | Commands to execute within ramble | `list(string)` | `[]` | no |
| <a name="input_install_dir"></a> [install\_dir](#input\_install\_dir) | Destination directory of installation of Ramble | `string` | `"/apps/ramble"` | no |
| <a name="input_log_file"></a> [log\_file](#input\_log\_file) | Log file to write output from ramble steps into | `string` | `"/var/log/ramble.log"` | no |
| <a name="input_ramble_ref"></a> [ramble\_ref](#input\_ramble\_ref) | Git ref to checkout for Ramble | `string` | `"develop"` | no |
| <a name="input_ramble_url"></a> [ramble\_url](#input\_ramble\_url) | URL for Ramble repository to clone | `string` | `"https://github.com/GoogleCloudPlatform/ramble"` | no |
| <a name="input_spack_path"></a> [spack\_path](#input\_spack\_path) | Path to the spack installation | `string` | `""` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_ramble_commands_runner"></a> [ramble\_commands\_runner](#output\_ramble\_commands\_runner) | Runner to run Ramble commands using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-ramble-id.ramble\_commands\_runner)<br>... |
| <a name="output_ramble_deps_runner"></a> [ramble\_deps\_runner](#output\_ramble\_deps\_runner) | Runner to install dependencies for ramble using an ansible playbook. The<br>startup-script module will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-ramble-id.ramble\_deps\_runner)<br>... |
| <a name="output_ramble_install_runner"></a> [ramble\_install\_runner](#output\_ramble\_install\_runner) | Runner to install Ramble using an ansible playbook. The startup-script module<br>will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-ramble-id.ramble\_install\_runner)<br>... |
| <a name="output_ramble_path"></a> [ramble\_path](#output\_ramble\_path) | Location ramble is installed into. |
| <a name="output_ramble_setup_runner"></a> [ramble\_setup\_runner](#output\_ramble\_setup\_runner) | Adds Ramble setup-env.sh script to /etc/profile.d so that it is called at shell startup. Among other things this adds Ramble binary to user PATH. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
