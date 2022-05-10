## Description

This module can be used to install spack on a VM. This includes:

1. Cloning spack into a predefined directory
2. Checking out a specific version of spack
3. Configuring compilers within spack
4. Installing application licenses that spack packages might depend on
5. Installing various spack specs.

The output of this module is a startup script that is intended to be attached
to either the controller node of a scheduler, or a single VM. The resulting
installation of spack can then be mounted across many other VMs to share a
software stack.

Two output variables are defined by this module:

- `startup_script` - Can be used to chain this with
  `modules/scripts/startup_script` as a runner
- `controller_startup_script` - Can be added to a scheduler by simply adding a
  `use: [spack]` option to the contoller node

**Please note**: This module currently is capable of re-running to install
additional packages, but cannot be used to uninstall packages from the VM.

**Please note**: Currently, license installation is performed by copying a
license file from a GCS bucket to a specific directory on the target VM.

**Please note**: When populating a buildcache with packages, the VM this
spack module is running on requires the following scope:
https://www.googleapis.com/auth/devstorage.read_write

## Example

As an example, the below is a possible definition of a spack installation.

```yaml
  - source: community/modules/scripts/spack-install
    kind: terraform
    id: spack
    settings:
      install_dir: /sw/spack
      spack_url: https://github.com/spack/spack
      spack_ref: v0.17.0
      spack_cache_url:
      - mirror_name: 'gcs_cache'
        mirror_url: gs://example-buildcache/linux-centos7
      configs:
      - type: 'single-config'
        value: 'config:install_tree:/sw/spack/opt'
        scope: 'site'
      - type: 'file'
        scope: 'site'
        value: |
          config:
            build_stage:
              - /sw/spack/stage
      - type: 'file'
        scope: 'site'
        value: |
          modules:
            tcl:
              hash_length: 0
              whitelist:
                - gcc
              blacklist:
                - '%gcc@4.8.5'
              all:
                conflict:
                  - '{name}'
                filter:
                  environment_blacklist:
                    - "C_INCLUDE_PATH"
                    - "CPLUS_INCLUDE_PATH"
                    - "LIBRARY_PATH"
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
```

Following the above description of this module, it can be added to a Slurm
deployment via the following:

```yaml
- source: community/modules/scheduler/SchedMD-slurm-on-gcp-controller
    kind: terraform
    id: slurm_controller
    use: [spack]
    settings:
      subnetwork_name: ((module.network1.primary_subnetwork.name))
      login_node_count: 1
      partitions:
      - $(compute_partition.partition)

```

Alternatively, it can be added as a startup script via:

```yaml
  - source: modules/scripts/startup-script
    kind: terraform
    id: startup
    settings:
      runners:
      - type: ansible-local
        source: modules/spack-install/scripts/install_spack_deps.yml
        destination: install_spack_deps.yml
      - type: shell
        content: $(spack.startup_script)
        destination: "/sw/spack-install.sh"
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2021 Google LLC

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
| <a name="input_configs"></a> [configs](#input\_configs) | List of configuration options to set within spack.<br>    Configs can be of type 'single-config' or 'file'.<br>    All configs must specify a value, and a<br>    a scope. | `list(map(any))` | `[]` | no |
| <a name="input_environments"></a> [environments](#input\_environments) | Defines a spack environment to configure. | <pre>list(object({<br>    name     = string<br>    packages = list(string)<br>  }))</pre> | `null` | no |
| <a name="input_gpg_keys"></a> [gpg\_keys](#input\_gpg\_keys) | GPG Keys to trust within spack.<br>  Each key must define a type. Valid types are 'file' and 'new'.<br>  Keys of type 'file' must define a path to the key that<br>  should be trusted.<br>  Keys of type 'new' must define a 'name' and 'email' to create<br>  the key with. | `list(map(any))` | `[]` | no |
| <a name="input_install_dir"></a> [install\_dir](#input\_install\_dir) | Directory to install spack into. | `string` | `"/sw/spack"` | no |
| <a name="input_licenses"></a> [licenses](#input\_licenses) | List of software licenses to install within spack. | <pre>list(object({<br>    source = string<br>    dest   = string<br>  }))</pre> | `null` | no |
| <a name="input_log_file"></a> [log\_file](#input\_log\_file) | Defines the logfile that script output will be written to | `string` | `"/dev/null"` | no |
| <a name="input_packages"></a> [packages](#input\_packages) | Defines root packages for spack to install (in order). | `list(string)` | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created. | `string` | n/a | yes |
| <a name="input_spack_cache_url"></a> [spack\_cache\_url](#input\_spack\_cache\_url) | List of buildcaches for spack. | <pre>list(object({<br>    mirror_name = string<br>    mirror_url  = string<br>  }))</pre> | `null` | no |
| <a name="input_spack_ref"></a> [spack\_ref](#input\_spack\_ref) | Git ref to checkout for spack. | `string` | `"develop"` | no |
| <a name="input_spack_url"></a> [spack\_url](#input\_spack\_url) | URL to clone the spack repo from. | `string` | `"https://github.com/spack/spack"` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | The GCP zone where the instance is running. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_controller_startup_script"></a> [controller\_startup\_script](#output\_controller\_startup\_script) | Path to the Spack installation script, duplicate for SLURM controller. |
| <a name="output_install_spack_deps_runner"></a> [install\_spack\_deps\_runner](#output\_install\_spack\_deps\_runner) | Runner to install dependencies for spack using the startup-script module<br>This runner requires ansible to be installed. This can be achieved using the<br>install\_ansible.sh script as a prior runner in the startup-script module:<br>runners:<br>- type: shell<br>  source: modules/startup-script/examples/install\_ansible.sh<br>  destination: install\_ansible.sh<br>- $(spack.install\_spack\_deps\_runner)<br>... |
| <a name="output_install_spack_runner"></a> [install\_spack\_runner](#output\_install\_spack\_runner) | Runner to install Spack using the startup-script module |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | Path to the Spack installation script. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
