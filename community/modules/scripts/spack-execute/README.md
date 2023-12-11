## Description

This module creates a script that defines a software build using Spack and
performs any additional customization to a Spack installation.

There are two main variable inputs that can be used to define a Spack build:
`data_files` and `commands`.

- `data_files`: Any files specified will be transferred to the machine running
  outputted script. Data file `content` can be defined inline in the blueprint
  or can point to a `source`, an absolute local path of a file. This can be used
  to transfer environment definition files, config definition files, GPG keys,
  or software licenses. `data_files` are transferred before `commands` are run.
- `commands`: A script that is run. This can be used to perform actions such as
  installation of compilers & packages, environment creation, adding a build
  cache, and modifying the spack configuration.

## Example

The `spack-execute` module should `use` a `spack-setup` module. This will
prepend the installation of Spack and its dependencies to the build. Then
`spack-execute` can be used by a module that takes `startup-script` as an input.

```yaml
  - id: spack-setup
    source: community/modules/scripts/spack-setup

  - id: spack-build
    source: community/modules/scripts/spack-execute
    use: [spack-setup]
    settings:
      commands: |
        spack install gcc@10.3.0 target=x86_64

  - id: builder-vm
    source: modules/compute/vm-instance
    use: [network1, spack-build]
```

To see a full example of this module in use, see the [hpc-slurm-gromacs.yaml] example.

[hpc-slurm-gromacs.yaml]: ../../../examples/hpc-slurm-gromacs.yaml

### Using with `startup-script` module

The `spack-runner` output can be used by the `startup-script` module.

```yaml
  - id: spack-setup
    source: community/modules/scripts/spack-setup

  - id: spack-build
    source: community/modules/scripts/spack-execute
    use: [spack-setup]
    settings:
      commands: |
        spack install gcc@10.3.0 target=x86_64

  - id: startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(spack-build.spack-runner)
      - type: shell
        destination: "my-script.sh"
        content: echo 'hello world'

  - id: workstation
    source: modules/compute/vm-instance
    use: [network1, startup-script]
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0.0 |
| <a name="requirement_local"></a> [local](#requirement\_local) | >= 2.0.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_local"></a> [local](#provider\_local) | >= 2.0.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_startup_script"></a> [startup\_script](#module\_startup\_script) | github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script | 50644b2 |

## Resources

| Name | Type |
|------|------|
| [local_file.debug_file_ansible_execute](https://registry.terraform.io/providers/hashicorp/local/latest/docs/resources/file) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_commands"></a> [commands](#input\_commands) | String of commands to run within this module | `string` | `null` | no |
| <a name="input_data_files"></a> [data\_files](#input\_data\_files) | A list of files to be transferred prior to running commands. <br>It must specify one of 'source' (absolute local file path) or 'content' (string).<br>It must specify a 'destination' with absolute path where file should be placed. | `list(map(string))` | `[]` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of deployment, used to name bucket containing spack scripts. | `string` | n/a | yes |
| <a name="input_gcs_bucket_path"></a> [gcs\_bucket\_path](#input\_gcs\_bucket\_path) | The GCS path for storage bucket and the object, starting with `gs://`. | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Key-value pairs of labels to be added to created resources. | `map(string)` | n/a | yes |
| <a name="input_log_file"></a> [log\_file](#input\_log\_file) | Defines the logfile that script output will be written to | `string` | `"/var/log/spack.log"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region to place bucket containing spack scripts. | `string` | n/a | yes |
| <a name="input_spack_profile_script_path"></a> [spack\_profile\_script\_path](#input\_spack\_profile\_script\_path) | Path to the Spack profile.d script. Created by an instance of spack-setup.<br>Can be defined explicitly, or by chaining an instance of a spack-setup module<br>through a `use` setting. | `string` | n/a | yes |
| <a name="input_spack_runner"></a> [spack\_runner](#input\_spack\_runner) | Runner from previous spack-setup or spack-execute to be chained with scripts generated by this module. | <pre>object({<br>    type        = string<br>    content     = string<br>    destination = string<br>  })</pre> | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_controller_startup_script"></a> [controller\_startup\_script](#output\_controller\_startup\_script) | Spack startup script, duplicate for SLURM controller. |
| <a name="output_gcs_bucket_path"></a> [gcs\_bucket\_path](#output\_gcs\_bucket\_path) | Bucket containing the startup scripts for spack, to be reused by spack-execute module. |
| <a name="output_spack_profile_script_path"></a> [spack\_profile\_script\_path](#output\_spack\_profile\_script\_path) | Path to the Spack profile.d script. |
| <a name="output_spack_runner"></a> [spack\_runner](#output\_spack\_runner) | Single runner that combines scripts from this module and any previously chained spack-execute or spack-setup modules. |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | Spack startup script. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
