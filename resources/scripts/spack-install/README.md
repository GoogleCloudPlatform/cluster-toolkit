## Description

This resource can be used to install spack on a VM. This includes:
 1. Cloning spack into a predefined directory
 2. Checking out a specific version of spack
 3. Configuring compilers within spack
 4. Installing application licenses that spack packages might depend on
 5. Installing various spack specs.

**Please note**: This resource currently is capable of re-running to install
additional packages, but cannot be used to uninstall packages from the VM.

**Please note**: Currently, license installation is performed by copying a
license file from a GCS bucket to a specific directory on the target VM.

## Example

```
  - source: ./resources/scripts/spack-install
    kind: terraform
    id: spack
    settings:
      install_dir: /apps/spack
      spack_url: https://github.com/spack/spack
      spack_ref: v0.17.0
      spack_cache_url:
       - mirror_name: 'gcs_cache'
         mirror_url: gs://example-buildcache/linux-centos7
      compilers:
        - gcc@10.3.0 target=x86_64
      packages:
        - cmake%gcc@10.3.0 target=x86_64
        - intel-mkl%gcc@10.3.0 target=skylake
        - intel-mpi@2018.4.274%gcc@10.3.0 target=skylake
        - fftw%intel@18.0.5 target=skylake ^intel-mpi@2018.4.274%intel@18.0.5 target=x86_64
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
| <a name="input_compilers"></a> [compilers](#input\_compilers) | Defines compilers for spack to install before installing packages. | `list(string)` | `[]` | no |
| <a name="input_install_dir"></a> [install\_dir](#input\_install\_dir) | Directory to install spack into | `string` | `"/apps/spack"` | no |
| <a name="input_licenses"></a> [licenses](#input\_licenses) | List of software licenses to install within spack. | <pre>list(object({<br>    source = string<br>    dest   = string<br>  }))</pre> | `null` | no |
| <a name="input_packages"></a> [packages](#input\_packages) | Defines packages for spack to install (in order) | `list(string)` | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_spack_cache_url"></a> [spack\_cache\_url](#input\_spack\_cache\_url) | List of buildcaches for spack | <pre>list(object({<br>    mirror_name = string<br>    mirror_url  = string<br>  }))</pre> | `null` | no |
| <a name="input_spack_ref"></a> [spack\_ref](#input\_spack\_ref) | Git ref to checkout for spack | `string` | `"develop"` | no |
| <a name="input_spack_url"></a> [spack\_url](#input\_spack\_url) | URL to clone the spack repo from | `string` | `"https://github.com/spack/spack"` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | The GCP zone where the instance is running | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_controller_startup_script"></a> [controller\_startup\_script](#output\_controller\_startup\_script) | Path to the Spack installation script |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | Path to the Spack installation script |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
