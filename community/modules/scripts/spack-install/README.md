## Description

This module can be used to install spack on a VM. This includes:

1. Cloning spack into a predefined directory
1. Checking out a specific version of spack
1. Configuring compilers within spack
1. Installing application licenses that spack packages might depend on
1. Installing various spack specs.

The output of this module is a startup script that is intended to be attached
to either the login or controller node of a scheduler, or a
[vm-instance](../../../../modules/compute/vm-instance/README.md). The
resulting installation of spack can then be mounted across many other VMs to
share a software stack.

> **_NOTE:_** This module currently is capable of re-running to install
> additional packages, but cannot be used to uninstall packages from the VM.
>
> **_NOTE:_** Currently, license installation is performed by copying a
> license file from a GCS bucket to a specific directory on the target VM.
>
> **_NOTE:_** When populating a buildcache with packages, the VM this
> spack module is running on requires the following scope:
> https://www.googleapis.com/auth/devstorage.read_write

## Example

As an example, the below is a possible definition of a spack installation. To
see this module used in a full blueprint, see the [hpc-slurm-gromacs.yaml] example.

```yaml
  - id: spack
    source: community/modules/scripts/spack-install
    settings:
      install_dir: /sw/spack
      spack_url: https://github.com/spack/spack
      spack_ref: v0.19.0
      spack_cache_url:
      - mirror_name: 'gcs_cache'
        mirror_url: gs://example-buildcache/linux-centos7
      configs:
      - type: single-config
        scope: defaults
        value: "config:build_stage:/sw/spack/spack-stage"
      - type: file
        scope: defaults
        value: |
          modules:
            default:
              tcl:
                hash_length: 0
                all:
                  conflict:
                    - '{name}'
                projections:
                  all: '{name}/{version}-{compiler.name}-{compiler.version}'
      compilers:
      - gcc@10.3.0 target=x86_64
      packages:
      - cmake%gcc@10.3.0 target=x86_64
      environments:
      - name: main-env
        packages:
        - intel-mkl%gcc@10.3.0 target=skylake
        - intel-mpi@2018.4.274%gcc@10.3.0 target=skylake
        - fftw%intel@18.0.5 target=skylake ^intel-mpi@2018.4.274%intel@18.0.5 target=x86_64
      - name: explicit-env
        content: |
          spack:
            definitions:
            - compilers:
              - gcc@10.3.0
            - mpis:
              - intel-mpi@2018.4.274
            - packages:
              - intel-mkl
            - mpi_packages:
              - fftw
            specs:
            - matrix:
              - - $packages
              - - $%compilers
            - matrix:
              - - $mpis
              - - $%compilers
            - matrix:
              - - $mpi_packages
              - - $%compilers
              - - $^mpis
```

Following the above description of this module, it can be added to a Slurm
deployment via the following:

```yaml
- id: slurm_controller
  source: community/modules/scheduler/SchedMD-slurm-on-gcp-controller
  use: [spack]
  settings:
    subnetwork_name: ((module.network1.primary_subnetwork.name))
    login_node_count: 1
    partitions:
    - $(compute_partition.partition)
```

Alternatively, it can be added as a startup script via:

```yaml
  - id: startup
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(spack.install_spack_deps_runner)
      - $(spack.install_spack_runner)
```

[hpc-slurm-gromacs.yaml]: ../../../examples/hpc-slurm-gromacs.yaml

## Environment Setup

### Activating Spack

[Spack installation] produces a setup script that adds `spack` to your `PATH` as
well as some other command-line integration tools. This script can be found at
`<install path>/share/spack/setup-env.sh`. This script will be automatically
added to bash startup by the `install_spack_runner`. In the case that you are
using Spack on a different machine than the one where Spack was installed, you
can use the `setup_spack_runner` to make sure Spack is also available on that
machine.

[Spack installation]: https://spack-tutorial.readthedocs.io/en/latest/tutorial_basics.html#installing-spack

### Example using `setup_spack_runner`

The following examples assumes that a different machine is running
`$(spack.install_spack_runner)` and the Slurm login node has access to the Spack
instal through a shared file system.

```yaml
  - id: spack
    source: community/modules/scripts/spack-install
    ...

  - id: spack-setup
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(spack.setup_spack_runner)

  - id: slurm_login
    source: community/modules/scheduler/SchedMD-slurm-on-gcp-login-node
    use: [spack-setup, ...]
```

### Managing Spack Python dependencies

Spack is configured with [SPACK_PYTHON] to ensure that Spack itself uses a
Python virtual environment with a supported copy of Python with the package
`google-cloud-storage` pre-installed. This enables Spack to use mirrors and
[build caches][builds] on Google Cloud Storage. It does not configure Python
packages *inside* Spack virtual environments. If you need to add more Python
dependencies for Spack itself, use the `spack python` command:

```shell
sudo -i spack python -m pip install package-name
```

[SPACK_PYTHON]: https://spack.readthedocs.io/en/latest/getting_started.html#shell-support
[builds]: https://spack.readthedocs.io/en/latest/binary_caches.html
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

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_caches_to_populate"></a> [caches\_to\_populate](#input\_caches\_to\_populate) | Defines caches which will be populated with the installed packages.<br>  Each cache must specify a type (either directory, or mirror).<br>  Each cache must also specify a path. For directory caches, this path<br>  must be on a local file system (i.e. file:///path/to/cache). For<br>  mirror paths, this can be any valid URL that spack accepts.<br><br>  NOTE: GPG Keys should be installed before trying to populate a cache<br>  with packages.<br><br>  NOTE: The gpg\_keys variable can be used to install existing GPG keys<br>  and create new GPG keys, both of which are acceptable for populating a<br>  cache. | `list(map(any))` | `[]` | no |
| <a name="input_compilers"></a> [compilers](#input\_compilers) | Defines compilers for spack to install before installing packages. | `list(string)` | `[]` | no |
| <a name="input_concretize_flags"></a> [concretize\_flags](#input\_concretize\_flags) | Defines the flags to pass into `spack concretize` | `string` | `""` | no |
| <a name="input_configs"></a> [configs](#input\_configs) | List of configuration options to set within spack.<br>    Configs can be of type 'single-config' or 'file'.<br>    All configs must specify content, and a<br>    a scope. | `list(map(any))` | `[]` | no |
| <a name="input_environments"></a> [environments](#input\_environments) | Defines spack environments to configure, given as a list.<br>  Each environment must define a name.<br>  Additional optional attributes are 'content' and 'packages'.<br>  'content' must be a string, defining the content of the Spack Environment YAML file.<br>  'packages' must be a list of strings, defining the spack specs to install.<br>  If both 'content' and 'packages' are defined, 'content' is processed first. | `any` | `[]` | no |
| <a name="input_gpg_keys"></a> [gpg\_keys](#input\_gpg\_keys) | GPG Keys to trust within spack.<br>  Each key must define a type. Valid types are 'file' and 'new'.<br>  Keys of type 'file' must define a path to the key that<br>  should be trusted.<br>  Keys of type 'new' must define a 'name' and 'email' to create<br>  the key with. | `list(map(any))` | `[]` | no |
| <a name="input_install_dir"></a> [install\_dir](#input\_install\_dir) | Directory to install spack into. | `string` | `"/sw/spack"` | no |
| <a name="input_install_flags"></a> [install\_flags](#input\_install\_flags) | Defines the flags to pass into `spack install` | `string` | `""` | no |
| <a name="input_licenses"></a> [licenses](#input\_licenses) | List of software licenses to install within spack. | <pre>list(object({<br>    source = string<br>    dest   = string<br>  }))</pre> | `null` | no |
| <a name="input_log_file"></a> [log\_file](#input\_log\_file) | Defines the logfile that script output will be written to | `string` | `"/var/log/spack.log"` | no |
| <a name="input_packages"></a> [packages](#input\_packages) | Defines root packages for spack to install (in order). | `list(string)` | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created. | `string` | n/a | yes |
| <a name="input_spack_cache_url"></a> [spack\_cache\_url](#input\_spack\_cache\_url) | List of buildcaches for spack. | <pre>list(object({<br>    mirror_name = string<br>    mirror_url  = string<br>  }))</pre> | `null` | no |
| <a name="input_spack_ref"></a> [spack\_ref](#input\_spack\_ref) | Git ref to checkout for spack. | `string` | `"v0.20.0"` | no |
| <a name="input_spack_url"></a> [spack\_url](#input\_spack\_url) | URL to clone the spack repo from. | `string` | `"https://github.com/spack/spack"` | no |
| <a name="input_spack_virtualenv_path"></a> [spack\_virtualenv\_path](#input\_spack\_virtualenv\_path) | Virtual environment path in which to install Spack Python interpreter and other dependencies | `string` | `"/usr/local/spack-python"` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | The GCP zone where the instance is running. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_controller_startup_script"></a> [controller\_startup\_script](#output\_controller\_startup\_script) | Path to the Spack installation script, duplicate for SLURM controller. |
| <a name="output_install_spack_deps_runner"></a> [install\_spack\_deps\_runner](#output\_install\_spack\_deps\_runner) | Runner to install dependencies for spack using an ansible playbook. The<br>startup-script module will automatically handle installation of ansible.<br>- id: example-startup-script<br>  source: modules/scripts/startup-script<br>  settings:<br>    runners:<br>    - $(your-spack-id.install\_spack\_deps\_runner)<br>... |
| <a name="output_install_spack_runner"></a> [install\_spack\_runner](#output\_install\_spack\_runner) | Runner to install Spack using the startup-script module |
| <a name="output_setup_spack_runner"></a> [setup\_spack\_runner](#output\_setup\_spack\_runner) | Adds Spack setup-env.sh script to /etc/profile.d so that it is called at shell startup. Among other things this adds Spack binary to user PATH. |
| <a name="output_spack_path"></a> [spack\_path](#output\_spack\_path) | Path to the root of the spack installation |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | Path to the Spack installation script. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
