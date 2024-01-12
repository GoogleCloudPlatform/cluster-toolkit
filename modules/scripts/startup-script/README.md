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

- `args`: (Optional) Arguments to be passed to `shell` or `ansible-local`
  runners. For `shell` runners, these will be passed as arguments to the script
  when it is executed. For `ansible-local` runners, they will be appended to
  a list of default arguments that invoke `ansible-playbook` on the localhost.
  Therefore`args` should not include any arguments that alter this behavior,
  such as `--connection`, `--inventory`, or `--limit`.

### Runner dependencies

`ansible-local` runners require Ansible to be installed in the VM before
running. To support other playbook runners in the HPC Toolkit, we install
version 2.11 of `ansible-core` as well as the larger package of collections
found in `ansible` version 4.10.0.

If an `ansible-local` runner is found in the list supplied to this module,
a script to install Ansible will be prepended to the list of runners. This
behavior can be disabled by setting `var.prepend_ansible_installer` to `false`.
This script will do the following at VM startup:

- Install system-wide python3 if not already installed using system package
  managers (yum, apt-get, etc)
- Install `python3-distutils` system-wide in debian and ubuntu based
  environments. This can be a missing dependency on system installations of
  python3 for installing and upgrading pip.
- Install system-wide pip3 if not already installed and upgrade pip3 if the
  version is not at least 18.0.
- Install and create a virtual environment located at `/usr/local/ghpc-venv`.
- Install ansible into this virtual environment if the current version of
  ansible is not version 2.11 or higher.

To use the virtual environment created by this script, you can activate it by
running the following command on the VM:

```shell
source /usr/local/ghpc-venv/bin/activate
```

You may also need to provide the correct python interpreter as the python3
binary in the virtual environment. This can be done by adding the following flag
when calling `ansible-playbook`:

```shell
-e ansible_python_interpreter=/usr/local/ghpc-venv/bin/activate
```

> **_NOTE:_** ansible-playbook and other ansible command line tools will only be
> accessible from the command line (and in your PATH variable) after activating
> this environment.

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

To view outputs from a Linux startup script, run:

```shell
sudo journalctl -u google-startup-scripts.service
```

### Monitoring Agent Installation

This `startup-script` module has several options for installing a Google
monitoring agent. There are two relevant settings: `install_stackdriver_agent`
and `install_cloud_ops_agent`.

The _Stackdriver Agent_ also called the _Legacy Cloud Monitoring Agent_ provides
better performance under some HPC workloads. While official documentation
recommends using the _Cloud Ops Agent_, it is recommended to use
`install_stackdriver_agent` when performance is important.

If an image or machine already has Cloud Ops Agent installed and you would like
to instead use the Stackdrier Agent, the following script will remove the Cloud
Ops Agent and install the Stackdriver Agent.

```bash
# Remove Cloud Ops Agent
sudo systemctl stop google-cloud-ops-agent.service
sudo systemctl disable google-cloud-ops-agent.service
curl -sSO https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh
sudo bash add-google-cloud-ops-agent-repo.sh --uninstall
sudo bash add-google-cloud-ops-agent-repo.sh --remove-repo

# Install Stackdriver Agent
curl -sSO https://dl.google.com/cloudagents/add-monitoring-agent-repo.sh
sudo bash add-monitoring-agent-repo.sh --also-install
curl -sSO https://dl.google.com/cloudagents/add-logging-agent-repo.sh
sudo bash add-logging-agent-repo.sh --also-install
sudo service stackdriver-agent start
```

### Example

```yaml
- id: startup
  source: ./modules/scripts/startup-script
  settings:
    runners:
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

- id: compute-cluster
  source: ./modules/compute/vm-instance
  use: [homefs, startup]
```

In the above example, a new GCS bucket is created to upload the startup-scripts.
But in the case where the user wants to reuse existing GCS bucket or folder,
they are able to do so by using the `gcs_bucket_path` as shown in the below example

```yaml
- id: startup
  source: ./modules/scripts/startup-script
  settings:
    gcs_bucket_path: gs://user-test-bucket/folder1/folder2
    install_stackdriver_agent: true

- id: compute-cluster
  source: ./modules/compute/vm-instance
  use: [startup]
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
| [google_storage_bucket_iam_binding.viewers](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_iam_binding) | resource |
| [google_storage_bucket_object.scripts](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [local_file.debug_file](https://registry.terraform.io/providers/hashicorp/local/latest/docs/resources/file) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_ansible_virtualenv_path"></a> [ansible\_virtualenv\_path](#input\_ansible\_virtualenv\_path) | Virtual environment path in which to install Ansible | `string` | `"/usr/local/ghpc-venv"` | no |
| <a name="input_bucket_viewers"></a> [bucket\_viewers](#input\_bucket\_viewers) | Additional service accounts or groups, users, and domains to which to grant read-only access to startup-script bucket (leave unset if using default Compute Engine service account) | `list(string)` | `[]` | no |
| <a name="input_configure_ssh_host_patterns"></a> [configure\_ssh\_host\_patterns](#input\_configure\_ssh\_host\_patterns) | If specified, it will automate ssh configuration by:<br>  - Defining a Host block for every element of this variable and setting StrictHostKeyChecking to 'No'.<br>  Ex: "hpc*", "hpc01*", "ml*"<br>  - The first time users log-in, it will create ssh keys that are added to the authorized keys list<br>  This requires a shared /home filesystem and relies on specifying the right prefix. | `list(string)` | `[]` | no |
| <a name="input_debug_file"></a> [debug\_file](#input\_debug\_file) | Path to an optional local to be written with 'startup\_script'. | `string` | `null` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used to name GCS bucket for startup scripts. | `string` | n/a | yes |
| <a name="input_gcs_bucket_path"></a> [gcs\_bucket\_path](#input\_gcs\_bucket\_path) | The GCS path for storage bucket and the object, starting with `gs://`. | `string` | `null` | no |
| <a name="input_http_proxy"></a> [http\_proxy](#input\_http\_proxy) | Web (http and https) proxy configuration for pip, apt, and yum/dnf | `string` | `""` | no |
| <a name="input_install_ansible"></a> [install\_ansible](#input\_install\_ansible) | Run Ansible installation script if either set to true or unset and runner of type 'ansible-local' are used. | `bool` | `null` | no |
| <a name="input_install_cloud_ops_agent"></a> [install\_cloud\_ops\_agent](#input\_install\_cloud\_ops\_agent) | Warning: Consider using `install_stackdriver_agent` for better performance. Run Google Ops Agent installation script if set to true. | `bool` | `false` | no |
| <a name="input_install_stackdriver_agent"></a> [install\_stackdriver\_agent](#input\_install\_stackdriver\_agent) | Run Google Stackdriver Agent installation script if set to true. Preferred over ops agent for performance. | `bool` | `false` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels for the created GCS bucket. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_prepend_ansible_installer"></a> [prepend\_ansible\_installer](#input\_prepend\_ansible\_installer) | DEPRECATED. Use `install_ansible=false` to prevent ansible installation. | `bool` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region to deploy to | `string` | n/a | yes |
| <a name="input_runners"></a> [runners](#input\_runners) | List of runners to run on remote VM.<br>    Runners can be of type ansible-local, shell or data.<br>    A runner must specify one of 'source' or 'content'.<br>    All runners must specify 'destination'. If 'destination' does not include a<br>    path, it will be copied in a temporary folder and deleted after running.<br>    Runners may also pass 'args', which will be passed as argument to shell runners only. | `list(map(string))` | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_compute_startup_script"></a> [compute\_startup\_script](#output\_compute\_startup\_script) | script to load and run all runners, as a string value. Targets the inputs for the slurm controller. |
| <a name="output_controller_startup_script"></a> [controller\_startup\_script](#output\_controller\_startup\_script) | script to load and run all runners, as a string value. Targets the inputs for the slurm controller. |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | script to load and run all runners, as a string value. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
