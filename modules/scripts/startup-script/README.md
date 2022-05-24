## Description

This module creates a startup script that will execute a list of runners in the
order they are specified. The runners are copied to a GCS bucket at deployment
time and then copied into the VM as they are executed after startup.

Each runner receives the following attributes:

- `destination`: (Required) The name of the file at the destination VM. If an
  absolute path is provided, the file will be copied to that path, otherwise
  the file will be created in a temporary folder and deleted once the startup
  script runs.
- `type`: (Required) The type of the runner, one of the following:
  - `shell`: The runner is a shell script and will be executed once copied to
    the destination VM.
  - `ansible-local`: The runner is an ansible playbook and will run on the VM
    with the following command line flags:

    ```shell
    ansible-playbook --connection=local --inventory=localhost, \
      --limit localhost <<DESTINATION>>
    ```

    > **_NOTE:_** Ansible must already be installed in this VM. This can be
    > done using another runner executed prior to this one. A pre-built ansible
    > installation shell runner is available as part of the
    > [startup-script module](./examples/install_ansible.sh).

  - `data`: The data or file specified will be copied to `<<DESTINATION>>`. No
    action will be performed after the data is staged. This data can be used by
    subsequent runners or simply made available on the VM for later use.
- `content`: (Optional) Content to be uploaded and, if `type` is
   either `shell` or `ansible-local`, executed. Must be defined if `source` is
   not.
- `source`: (Optional) A path to the file or data you want to upload. Must be
  defined if `content` is not. The source path is relative to the deployment
  group directory. Scripts distributed as part of modules should start with
  `modules/` followed by the name of the module used (not to be confused with
  the module ID) and the path to the script. The format is shown below:

    ```text
    source: ./modules/<<MODULE_NAME>>/<<SCRIPT_NAME>>
    ```

  For more examples with context, see the
  [example blueprint snippet](#example). To reference any other source file, an
  absolute path must be used.

- `args`: (Optional) Arguments to be passed to shell scripts.

  > **_NOTE:_** `args` will only be applied to runners of `type` `shell`.

### Staging the runners

Runners will be uploaded to a
[GCS bucket](https://cloud.google.com/storage/docs/creating-buckets). This
bucket will be created by this module and named as
`${var.deployment_name}-startup-scripts-${random_id}`. VMs using the startup
script created by this module will pull the runners content from a GCS bucket
and therefore must have access to GCS.

> **_NOTE:_** To ensure access to GCS, set the following OAuth scope on the
> instance using the startup scripts:
> `https://www.googleapis.com/auth/devstorage.read_only`.
>
> This is set as a default scope in the [vm-instance],
> [SchedMD-slurm-on-gcp-login-node] and [SchedMD-slurm-on-gcp-controller]
> modules

[vm-instance]: ../../compute/vm-instance/README.md
[SchedMD-slurm-on-gcp-login-node]: ../../../community/modules/scheduler/SchedMD-slurm-on-gcp-login-node/README.md
[SchedMD-slurm-on-gcp-controller]: ../../../community/modules/scheduler/SchedMD-slurm-on-gcp-controller/README.md

### Tracking startup script execution

For more information on how to use startup scripts on Google Cloud Platform,
please refer to
[this document](https://cloud.google.com/compute/docs/instances/startup-scripts/linux).

To debug startup scripts from a Linux VM created with startup script generated
by this module:

```shell
sudo DEBUG=1 google_metadata_script_runner startup
```

To view ouputs from a Linux startup script, run:

```shell
sudo journalctl -u google-startup-scripts.service
```

### Example

```yaml
- source: ./modules/scripts/startup-script
  kind: terraform
  id: startup
  settings:
    runners:
      - type: shell
        source: "modules/startup-script/examples/install_ansible.sh"
        destination: "install_ansible.sh"
      # Some modules such as filestore have runners as outputs for convenience:
      - $(homefs.install_nfs_client_runner)
      # These runners can still be created manually:
      # - type: shell
      #   destination: "modules/filestore/scripts/install_nfs_client.sh"
      #   source: "modules/filestore/scripts/install_nfs_client.sh"
      - type: ansible-local
        destination: "modules/filestore/scripts/mount.yaml"
        source: "modules/filestore/scripts/mount.yaml"
      - type: data
        source: /tmp/foo.tgz
        destination: /tmp/bar.tgz
      - type: shell
        destination: "decompress.sh"
        content: |
          #!/bin/sh
          echo $2
          tar zxvf /tmp/$1 -C /
        args: "bar.tgz 'Expanding file'"

- source: ./modules/compute/vm-instance
  kind: terraform
  id: compute-cluster
  use: [homefs, startup]
```

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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_local"></a> [local](#requirement\_local) | >= 2.0.0, < 2.2.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_local"></a> [local](#provider\_local) | >= 2.0.0, < 2.2.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket.configs_bucket](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket) | resource |
| [google_storage_bucket_object.scripts](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [local_file.debug_file](https://registry.terraform.io/providers/hashicorp/local/latest/docs/resources/file) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_debug_file"></a> [debug\_file](#input\_debug\_file) | Path to an optional local to be written with 'startup\_script'. | `string` | `null` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used to name GCS bucket for startup scripts. | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels for the created GCS bucket. List key, value pairs. | `any` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region to deploy to | `string` | n/a | yes |
| <a name="input_runners"></a> [runners](#input\_runners) | List of runners to run on remote VM.<br>    Runners can be of type ansible-local, shell or data.<br>    A runner must specify one of 'source' or 'content'.<br>    All runners must specify 'destination'. If 'destination' does not include a<br>    path, it will be copied in a temporary folder and deleted after running.<br>    Runners may also pass 'args', which will be passed as argument to shell runners only. | `list(map(string))` | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | script to load and run all runners, as a string value. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
