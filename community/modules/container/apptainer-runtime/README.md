## Description

This module emits `startup-script` runners that prepare the shared Apptainer
runtime layout under a caller-chosen mounted path.

It owns the shared directory structure and the shell profile snippet that adds
the shared modulefile tree to `MODULEPATH`. It does not stage any application
images by itself.

This module is intentionally narrow so it can be composed with:

- `community/modules/container/apptainer-app` for per-application staging
- site-specific startup scripts that install distro-specific packages or desktop
  tooling

## Usage

### Compose runtime and app runners

```yaml
- id: appsfs
  source: community/modules/file-system/nfs-server
  settings:
    local_mounts:
    - /shared

- id: apptainer-runtime
  source: community/modules/container/apptainer-runtime
  use: [appsfs]

- id: shared-app
  source: community/modules/container/apptainer-app
  use: [appsfs]
  settings:
    app_id: openfoam
    display_name: OpenFOAM
    image_ref: us-central1-docker.pkg.dev/my-project/apps/openfoam@sha256:0123456789abcdef

- id: shared-app-startup
  source: modules/scripts/startup-script
  settings:
    runners: $(flatten([apptainer-runtime.startup_runners, shared-app.startup_runners]))
```

If `install_root` is omitted, the module follows the standard Cluster Toolkit
`network_storage` contract and resolves it from the selected
`network_storage.local_mount`.

## Notes

- callers still choose where the startup script runs
- callers can append site-specific runners after `startup_runners`
- this module does not install Apptainer packages; keep existing image- or
  site-specific install logic in a separate runner to avoid duplicating distro
  policy in multiple modules

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [terraform_data.module_ready](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_bin_subdir"></a> [bin\_subdir](#input\_bin\_subdir) | Relative subdirectory under install\_root where wrapper commands are written. | `string` | `"bin"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Deployment name associated with the generated artifacts. | `string` | n/a | yes |
| <a name="input_install_root"></a> [install\_root](#input\_install\_root) | Absolute mounted path where shared Apptainer assets will be written. If unset, resolve the path from network\_storage. | `string` | `null` | no |
| <a name="input_manifest_subdir"></a> [manifest\_subdir](#input\_manifest\_subdir) | Relative subdirectory under install\_root where generated app manifests are written. | `string` | `"app-manifests"` | no |
| <a name="input_module_init_path"></a> [module\_init\_path](#input\_module\_init\_path) | Absolute path of the shell profile snippet that should add the shared modulefile tree to MODULEPATH. | `string` | `"/etc/profile.d/shared-modules.sh"` | no |
| <a name="input_modulefile_subdir"></a> [modulefile\_subdir](#input\_modulefile\_subdir) | Relative subdirectory under install\_root where Tcl modulefiles are written. | `string` | `"modulefiles"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | Optional list of network storage mounts, following the standard Cluster<br/>Toolkit network\_storage contract (e.g. from a storage module used via<br/>`use:`). When install\_root is unset, the module resolves install\_root from<br/>the selected entry's local\_mount. | <pre>list(object({<br/>    server_ip               = string<br/>    remote_mount            = string<br/>    local_mount             = string<br/>    local_mount_owner       = optional(string)<br/>    local_mount_permissions = optional(string)<br/>    fs_type                 = string<br/>    mount_options           = string<br/>    client_install_runner   = optional(map(string))<br/>    mount_runner            = optional(map(string))<br/>  }))</pre> | `[]` | no |
| <a name="input_network_storage_index"></a> [network\_storage\_index](#input\_network\_storage\_index) | Index to select when network\_storage is provided as a list and install\_root is unset. | `number` | `0` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID associated with the deployment. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region associated with the deployment. | `string` | n/a | yes |
| <a name="input_sif_subdir"></a> [sif\_subdir](#input\_sif\_subdir) | Relative subdirectory under install\_root where SIF images are staged. | `string` | `"containers"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_bin_dir"></a> [bin\_dir](#output\_bin\_dir) | Resolved directory where wrapper commands should be written. |
| <a name="output_install_root_resolved"></a> [install\_root\_resolved](#output\_install\_root\_resolved) | Resolved install root, either from install\_root or the selected network\_storage local\_mount. |
| <a name="output_layout_runner"></a> [layout\_runner](#output\_layout\_runner) | Shell runner that creates the shared Apptainer layout directories. |
| <a name="output_manifest_dir"></a> [manifest\_dir](#output\_manifest\_dir) | Resolved directory where generated app manifests should be written. |
| <a name="output_module_init_runner"></a> [module\_init\_runner](#output\_module\_init\_runner) | Shell runner that writes the profile snippet used to expose the shared modulefile tree. |
| <a name="output_modulefile_dir"></a> [modulefile\_dir](#output\_modulefile\_dir) | Resolved directory where Tcl modulefiles should be written. |
| <a name="output_sif_dir"></a> [sif\_dir](#output\_sif\_dir) | Resolved directory where SIF images should be staged. |
| <a name="output_startup_runners"></a> [startup\_runners](#output\_startup\_runners) | Ordered list of runners suitable for direct use with modules/scripts/startup-script. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
