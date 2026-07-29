## Description

This module emits `startup-script` runners that stage a Docker-format Google
Artifact Registry image as an Apptainer SIF, then write a wrapper command, Tcl
modulefile, and YAML manifest into a caller-chosen shared path.

This is a helper module only. It does not create storage, compute, or
application-specific assets by itself.

## Usage

### Feed `startup_runners` into `startup-script`

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
    bind_paths:
    - $(appsfs.network_storage[0].local_mount):$(appsfs.network_storage[0].local_mount)
    - /home:/home

- id: shared-app-startup
  source: modules/scripts/startup-script
  outputs:
  - startup_script
  settings:
    runners: $(flatten([apptainer-runtime.startup_runners, shared-app.startup_runners]))
```

Attach the resulting `startup_script` to any VM that already has the target
shared path mounted. The caller chooses where staging runs.

If `install_root` is omitted, the module follows the standard Cluster Toolkit
`network_storage` contract and resolves it from the selected
`network_storage.local_mount`. Explicit `install_root` still works and takes
precedence.

### Typical foundation wiring

For a shared storage and compute foundation, a common pattern is:

- run `community/modules/container/apptainer-runtime` once in the composed
  startup flow to own the shared layout and module-path setup
- run the generated app startup script on any host with the shared mount, such
  as a login node or shared tools VM
- resolve the shared mount from `appsfs.network_storage[0].local_mount` or an
  equivalent storage-module output
- rely on your existing modulefile initialization to expose the wrapper command

## IAM and runtime prerequisites

The staging VM service account must be able to read from Artifact Registry:

- `roles/artifactregistry.reader`

Runtime expectations:

- `gcloud` must be available on the staging VM because authentication uses
  `gcloud auth print-access-token`
- `apptainer` must already be available, or `install_apptainer` must be set and
  `apptainer_package` must be installable on the target image
- for shared installations, prefer `community/modules/container/apptainer-runtime`
  for layout/module-path setup instead of duplicating that logic in each app

## Notes

- `stage_runner` skips work when the target SIF already exists unless
  `pull_policy` is set to `always`
- the generated wrapper forwards user-provided arguments after the configured
  entrypoint and default entrypoint args
- the generated modulefile prepends the wrapper directory to `PATH`

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
| <a name="input_app_id"></a> [app\_id](#input\_app\_id) | Stable machine-readable identifier used for generated filenames. | `string` | n/a | yes |
| <a name="input_apptainer_package"></a> [apptainer\_package](#input\_apptainer\_package) | Package name used by the optional install runner. | `string` | `"apptainer"` | no |
| <a name="input_bin_subdir"></a> [bin\_subdir](#input\_bin\_subdir) | Relative subdirectory under install\_root where wrapper commands are written. | `string` | `"bin"` | no |
| <a name="input_bind_paths"></a> [bind\_paths](#input\_bind\_paths) | List of bind mounts to pass as --bind arguments to apptainer exec. | `list(string)` | `[]` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Deployment name associated with the generated artifacts. | `string` | n/a | yes |
| <a name="input_display_name"></a> [display\_name](#input\_display\_name) | Human-readable name for the generated wrapper, modulefile, and manifest. | `string` | n/a | yes |
| <a name="input_entrypoint"></a> [entrypoint](#input\_entrypoint) | Optional command to execute inside the container before any caller-provided arguments. | `string` | `""` | no |
| <a name="input_entrypoint_args"></a> [entrypoint\_args](#input\_entrypoint\_args) | Default arguments passed after entrypoint and before user-supplied wrapper arguments. | `list(string)` | `[]` | no |
| <a name="input_env"></a> [env](#input\_env) | Environment variables exported by the generated wrapper before execution. | `map(string)` | `{}` | no |
| <a name="input_image_ref"></a> [image\_ref](#input\_image\_ref) | Artifact Registry OCI image reference without the docker:// prefix. | `string` | n/a | yes |
| <a name="input_install_apptainer"></a> [install\_apptainer](#input\_install\_apptainer) | If true, generate an install runner that attempts to install Apptainer when it is absent. | `bool` | `false` | no |
| <a name="input_install_root"></a> [install\_root](#input\_install\_root) | Absolute mounted path where the SIF, wrapper, modulefile, and manifest will be written. If unset, resolve the path from network\_storage. | `string` | `null` | no |
| <a name="input_manifest_subdir"></a> [manifest\_subdir](#input\_manifest\_subdir) | Relative subdirectory under install\_root where generated app manifests are written. | `string` | `"app-manifests"` | no |
| <a name="input_module_name"></a> [module\_name](#input\_module\_name) | Optional modulefile name. Defaults to app\_id. | `string` | `null` | no |
| <a name="input_module_version"></a> [module\_version](#input\_module\_version) | Optional modulefile version. If set, the modulefile path becomes <module\_name>/<module\_version>. | `string` | `null` | no |
| <a name="input_modulefile_subdir"></a> [modulefile\_subdir](#input\_modulefile\_subdir) | Relative subdirectory under install\_root where Tcl modulefiles are written. | `string` | `"modulefiles"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | Optional list of network storage mounts, following the standard Cluster<br/>Toolkit network\_storage contract (e.g. from a storage module used via<br/>`use:`). When install\_root is unset, the module resolves install\_root from<br/>the selected entry's local\_mount. | <pre>list(object({<br/>    server_ip               = string<br/>    remote_mount            = string<br/>    local_mount             = string<br/>    local_mount_owner       = optional(string)<br/>    local_mount_permissions = optional(string)<br/>    fs_type                 = string<br/>    mount_options           = string<br/>    client_install_runner   = optional(map(string))<br/>    mount_runner            = optional(map(string))<br/>  }))</pre> | `[]` | no |
| <a name="input_network_storage_index"></a> [network\_storage\_index](#input\_network\_storage\_index) | Index to select when network\_storage is provided as a list and install\_root is unset. | `number` | `0` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID associated with the deployment. | `string` | n/a | yes |
| <a name="input_pull_policy"></a> [pull\_policy](#input\_pull\_policy) | Container pull behavior. if\_missing skips staging when the target SIF already exists; always refreshes it. | `string` | `"if_missing"` | no |
| <a name="input_region"></a> [region](#input\_region) | Region associated with the deployment. | `string` | n/a | yes |
| <a name="input_run_args"></a> [run\_args](#input\_run\_args) | Default arguments passed directly to apptainer exec before the image path. | `list(string)` | `[]` | no |
| <a name="input_sif_subdir"></a> [sif\_subdir](#input\_sif\_subdir) | Relative subdirectory under install\_root where SIF images are staged. | `string` | `"containers"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_auth_runner"></a> [auth\_runner](#output\_auth\_runner) | Shell runner that authenticates Apptainer to Google Artifact Registry using gcloud. |
| <a name="output_install_root_resolved"></a> [install\_root\_resolved](#output\_install\_root\_resolved) | Resolved install root, either from install\_root or the selected network\_storage local\_mount. |
| <a name="output_install_runner"></a> [install\_runner](#output\_install\_runner) | Shell runner that optionally installs Apptainer when install\_apptainer is true. |
| <a name="output_manifest_path"></a> [manifest\_path](#output\_manifest\_path) | Resolved manifest output path. |
| <a name="output_manifest_runner"></a> [manifest\_runner](#output\_manifest\_runner) | Shell runner that writes the generated application manifest. |
| <a name="output_modulefile_path"></a> [modulefile\_path](#output\_modulefile\_path) | Resolved modulefile output path. |
| <a name="output_modulefile_runner"></a> [modulefile\_runner](#output\_modulefile\_runner) | Shell runner that writes the generated Tcl modulefile. |
| <a name="output_sif_path"></a> [sif\_path](#output\_sif\_path) | Resolved SIF output path. |
| <a name="output_stage_runner"></a> [stage\_runner](#output\_stage\_runner) | Shell runner that pulls the configured Artifact Registry image into a SIF file. |
| <a name="output_startup_runners"></a> [startup\_runners](#output\_startup\_runners) | Ordered list of runners suitable for direct use with modules/scripts/startup-script.<br/>The list includes installation, registry authentication, SIF staging, wrapper creation,<br/>modulefile creation, and manifest creation. |
| <a name="output_wrapper_path"></a> [wrapper\_path](#output\_wrapper\_path) | Resolved wrapper output path. |
| <a name="output_wrapper_runner"></a> [wrapper\_runner](#output\_wrapper\_runner) | Shell runner that writes the generated application wrapper script. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
