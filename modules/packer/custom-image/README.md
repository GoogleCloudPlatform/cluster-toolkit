# Custom Images in the Cluster Toolkit (formerly HPC Toolkit)

Please review the
[introduction to image building](../../../docs/image-building.md) for general
information on building custom images using the Toolkit.

## Introduction

This module uses [Packer](https://www.packer.io/) to create an image within an
Cluster Toolkit deployment. Packer operates by provisioning a short-lived VM in
Google Cloud on which it executes scripts to customize the boot disk for
repeated use. The VM's boot disk is specified from a source image that defaults
to the [HPC VM Image][hpcimage]. This Packer "template" supports customization
by the following approaches following a [recommended use](#recommended-use):

- [startup-script metadata][startup-metadata] from [raw string][sss] or
  [file][ssf]
- [Shell scripts][shell] uploaded from the Packer execution environment to the
  VM
- [Ansible playbooks][ansible] uploaded from the Packer execution environment to
  the VM

They can be specified independently of one another, so that anywhere from 1 to 3
solutions can be used simultaneously. In the case that 0 scripts are supplied,
the source boot disk is effectively copied to your project without
customization. This can be useful in scenarios where increased control over the
image maintenance lifecycle is desired or when policies restrict the use of
images to internal projects.

## Minimum requirements

### Outbound internet access

Most customization scripts require access to resources on the public internet.
This can be achieved by one of the following 2 approaches:

1. Using a public IP address on the VM

- Set [var.omit_external_ip](#input_omit_external_ip) to `false`

1. Configuring a VPC with a Cloud NAT in the region of the VM

- Use the [vpc] module which automates NAT creation

### Inbound internet access

Read [order of execution](#order-of-execution) below for a discussion of VM
customization solutions and their requirements for inbound SSH access.
[Environments without SSH access](#environments-without-ssh-access) should use
the metadata-based startup-script solution.

A simple way to enable inbound SSH access is to use the VPC module with
`allowed_ssh_ip_ranges` set to `0.0.0.0/0`.

### User or service account running Packer

The user or service account running Packer must have the permission to create
VMs in the selected VPC network and, if [use_iap](#input_use_iap) is set, must
have the "IAP-Secured Tunnel User" role. Recommended roles are:

- `roles/compute.instanceAdmin.v1`
- `roles/iap.tunnelResourceAccessor`

### VM service account roles

The service account attached to the temporary build VM created by Packer should
have the ability to write Cloud Logging entries so that you may inspect and
debug build logs. When using the metadata startup-script customization solution,
the service account attached to the temporary build VM created by Packer must
have the permission to modify its own metadata and to read from Cloud Storage
buckets. Recommended roles are:

- `roles/compute.instanceAdmin.v1`
- `roles/logging.logWriter`
- `roles/monitoring.metricWriter`
- `roles/storage.objectViewer`

It is recommended to create this service account as a separate step outside a
blueprint due to known delay in [IAM bindings propagation][iamprop].

## Example blueprints

A recommended pattern for building images with this module is to use the
terraform based [startup-script] module along with this packer custom-image
module. Below you can find links to several examples of this pattern, including
usage instructions.

### [Image Builder]

The [Image Builder] blueprint demonstrates a solution that builds an image
using:

- The [HPC VM Image][hpcimage] as a base upon which to customize
- A VPC network with firewall rules that allow IAP-based SSH tunnels
- A Toolkit runner that installs a custom script

Please review the [examples README] for usage instructions.

## Order of execution

The startup script specified in metadata executes in parallel with the other
supported methods. However, the remaining methods execute in a well-defined
order relative to one another.

1. All shell scripts will execute in the configured order
1. After shell scripts complete, all Ansible playbooks will execute in the
   configured order

> **_NOTE:_** if both [startup_script][sss] and [startup_script_file][ssf] are
> specified, then [startup_script_file][ssf] takes precedence.

## Recommended use

Because the [metadata startup script executes in parallel](#order-of-execution)
with the other solutions, conflicts can arise, especially when package managers
(`yum` or `apt`) lock their databases during package installation. Therefore, it
is recommended to choose one of the following approaches:

1. Specify _either_ [startup_script][sss] _or_ [startup_script_file][ssf] and do
   not specify [shell_scripts][shell] or [ansible_playbooks][ansible].
   - This can be especially useful in
     [environments that restrict SSH access](#environments-without-ssh-access)
1. Specify any combination of [shell_scripts][shell] and
   [ansible_playbooks][ansible] and do not specify [startup_script][sss] or
   [startup_script_file][ssf].

If any of the startup script approaches fail by returning a code other than 0,
Packer will determine that the build has failed and refuse to save the image.

## External access with SSH

The [shell scripts][shell] and [Ansible playbooks][ansible] customization
solutions both require SSH access to the VM from the Packer execution
environment. SSH access can be enabled one of 2 ways:

1. The VM is created without a public IP address and SSH tunnels are created
   using [Identity-Aware Proxy (IAP)][iaptunnel].
   - Allow [use_iap](#input_use_iap) to take on its default value of `true`
1. The VM is created with an IP address on the public internet and firewall
   rules allow SSH access from the Packer execution environment.
   - Set `omit_external_ip = false` (or `omit_external_ip: false` in a
     blueprint)
   - Add firewall rules that open SSH to the VM

The Packer template defaults to using to the 1st IAP-based solution because it
is more secure (no exposure to public internet) and because the [vpc] module
automatically sets up all necessary firewall rules for SSH tunneling and
outbound-only access to the internet through [Cloud NAT][cloudnat].

In either SSH solution, customization scripts should be supplied as files in the
[shell_scripts][shell] and [ansible_playbooks][ansible] settings.

## Environments without SSH access

Many network environments disallow SSH access to VMs. In these environments, the
[metadata-based startup scripts][startup-metadata] are appropriate because they
execute entirely independently of the Packer execution environment.

In this scenario, a single scripts should be supplied in the form of a string to
the [startup_script][sss] input variable. This solution integrates well with
Toolkit runners. Runners operate by using a single startup script whose behavior
is extended by downloading and executing a customizable set of runners from
Cloud Storage at startup.

> **_NOTE:_** Packer will attempt to use SSH if either [shell_scripts][shell] or
> [ansible_playbooks][ansible] are set to non-empty values. Leave them at their
> default, empty values to ensure access by SSH is disabled.

## Supplying startup script as a string

The [startup_script][sss] parameter accepts scripts formatted as strings. In
Packer and Terraform, multi-line strings can be specified using
[heredoc syntax](https://www.terraform.io/language/expressions/strings#heredoc-strings)
in an input [Packer variables file][pkrvars] (`*.pkrvars.hcl`) For example, the
following snippet defines a multi-line bash script followed by an integer
representing the size, in GiB, of the resulting image:

```hcl
startup_script = <<-EOT
  #!/bin/bash
  yum install -y epel-release
  yum install -y jq
  EOT

disk_size = 100
```

In a blueprint, the equivalent syntax is:

```yaml
...
    settings:
      startup_script: |
        #!/bin/bash
        yum install -y epel-release
        yum install -y jq
      disk_size: 100
...
```

## Monitoring startup script execution

When using startup script customization, Packer will print very limited output
to the console. For example:

```text
==> example.googlecompute.toolkit_image: Waiting for any running startup script to finish...
==> example.googlecompute.toolkit_image: Startup script not finished yet. Waiting...
==> example.googlecompute.toolkit_image: Startup script not finished yet. Waiting...
==> example.googlecompute.toolkit_image: Startup script, if any, has finished running.
```

### Debugging startup-script failures

> [!NOTE]
> There can be a delay in the propagation of the logs from the instance to
> Cloud Logging, so it may require waiting a few minutes to see the full logs.

If the Packer image build fails, the module will output a `gcloud` command
that can be used directly to review startup-script execution.

## License

Copyright 2022 Google LLC

Licensed under the Apache License, Version 2.0 (the "License"); you may not use
this file except in compliance with the License. You may obtain a copy of the
License at

```text
 http://www.apache.org/licenses/LICENSE-2.0
```

Unless required by applicable law or agreed to in writing, software distributed
under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the
specific language governing permissions and limitations under the License.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

No requirements.

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_accelerator_count"></a> [accelerator\_count](#input\_accelerator\_count) | Number of accelerator cards to attach to the VM; not necessary for families that always include GPUs (A2). | `number` | `null` | no |
| <a name="input_accelerator_type"></a> [accelerator\_type](#input\_accelerator\_type) | Type of accelerator cards to attach to the VM; not necessary for families that always include GPUs (A2). | `string` | `null` | no |
| <a name="input_ansible_playbooks"></a> [ansible\_playbooks](#input\_ansible\_playbooks) | A list of Ansible playbook configurations that will be uploaded to customize the VM image | <pre>list(object({<br/>    playbook_file   = string<br/>    galaxy_file     = string<br/>    extra_arguments = list(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_communicator"></a> [communicator](#input\_communicator) | Communicator to use for provisioners that require access to VM ("ssh" or "winrm") | `string` | `null` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Cluster Toolkit deployment name | `string` | n/a | yes |
| <a name="input_disk_size"></a> [disk\_size](#input\_disk\_size) | Size of disk image in GB | `number` | `null` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Type of persistent disk to provision | `string` | `"pd-balanced"` | no |
| <a name="input_enable_shielded_vm"></a> [enable\_shielded\_vm](#input\_enable\_shielded\_vm) | Enable the Shielded VM configuration (var.shielded\_instance\_config). | `bool` | `false` | no |
| <a name="input_image_family"></a> [image\_family](#input\_image\_family) | The family name of the image to be built. Defaults to `deployment_name` | `string` | `null` | no |
| <a name="input_image_name"></a> [image\_name](#input\_image\_name) | The name of the image to be built. If not supplied, it will be set to image\_family-$ISO\_TIMESTAMP | `string` | `null` | no |
| <a name="input_image_storage_locations"></a> [image\_storage\_locations](#input\_image\_storage\_locations) | Storage location, either regional or multi-regional, where snapshot content is to be stored and only accepts 1 value.<br/>See https://developer.hashicorp.com/packer/plugins/builders/googlecompute#image_storage_locations | `list(string)` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to apply to the short-lived VM | `map(string)` | `null` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | VM machine type on which to build new image | `string` | `"n2-standard-4"` | no |
| <a name="input_manifest_file"></a> [manifest\_file](#input\_manifest\_file) | File to which to write Packer build manifest | `string` | `"packer-manifest.json"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Instance metadata for the builder VM (use var.startup\_script or var.startup\_script\_file to set startup-script metadata) | `map(string)` | `{}` | no |
| <a name="input_network_project_id"></a> [network\_project\_id](#input\_network\_project\_id) | Project ID of Shared VPC network | `string` | `null` | no |
| <a name="input_omit_external_ip"></a> [omit\_external\_ip](#input\_omit\_external\_ip) | Provision the image building VM without a public IP address | `bool` | `true` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except the use of GPUs requires it to be `TERMINATE` | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which to create VM and image | `string` | n/a | yes |
| <a name="input_scopes"></a> [scopes](#input\_scopes) | DEPRECATED: use var.service\_account\_scopes | `set(string)` | `null` | no |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | The service account email to use. If null or 'default', then the default Compute Engine service account will be used. | `string` | `null` | no |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Service account scopes to attach to the instance. See<br/>https://cloud.google.com/compute/docs/access/service-accounts. | `set(string)` | <pre>[<br/>  "https://www.googleapis.com/auth/cloud-platform"<br/>]</pre> | no |
| <a name="input_shell_scripts"></a> [shell\_scripts](#input\_shell\_scripts) | A list of paths to local shell scripts which will be uploaded to customize the VM image | `list(string)` | `[]` | no |
| <a name="input_shielded_instance_config"></a> [shielded\_instance\_config](#input\_shielded\_instance\_config) | Shielded VM configuration for the instance (must set var.enabled\_shielded\_vm) | <pre>object({<br/>    enable_secure_boot          = bool<br/>    enable_vtpm                 = bool<br/>    enable_integrity_monitoring = bool<br/>  })</pre> | <pre>{<br/>  "enable_integrity_monitoring": true,<br/>  "enable_secure_boot": true,<br/>  "enable_vtpm": true<br/>}</pre> | no |
| <a name="input_source_image"></a> [source\_image](#input\_source\_image) | Source OS image to build from | `string` | `null` | no |
| <a name="input_source_image_family"></a> [source\_image\_family](#input\_source\_image\_family) | Alternative to source\_image. Specify image family to build from latest image in family | `string` | `"hpc-rocky-linux-8"` | no |
| <a name="input_source_image_project_id"></a> [source\_image\_project\_id](#input\_source\_image\_project\_id) | A list of project IDs to search for the source image. Packer will search the<br/>first project ID in the list first, and fall back to the next in the list,<br/>until it finds the source image. | `list(string)` | `null` | no |
| <a name="input_ssh_username"></a> [ssh\_username](#input\_ssh\_username) | Username to use for SSH access to VM | `string` | `"hpc-toolkit-packer"` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script (as raw string) used to build the custom Linux VM image (overridden by var.startup\_script\_file if both are set) | `string` | `null` | no |
| <a name="input_startup_script_file"></a> [startup\_script\_file](#input\_startup\_script\_file) | File path to local shell script that will be used to customize the Linux VM image (overrides var.startup\_script) | `string` | `null` | no |
| <a name="input_state_timeout"></a> [state\_timeout](#input\_state\_timeout) | The time to wait for instance state changes, including image creation | `string` | `"10m"` | no |
| <a name="input_subnetwork_name"></a> [subnetwork\_name](#input\_subnetwork\_name) | Name of subnetwork in which to provision image building VM | `string` | n/a | yes |
| <a name="input_tags"></a> [tags](#input\_tags) | Assign network tags to apply firewall rules to VM instance | `list(string)` | `null` | no |
| <a name="input_use_iap"></a> [use\_iap](#input\_use\_iap) | Use IAP proxy when connecting by SSH | `bool` | `true` | no |
| <a name="input_use_os_login"></a> [use\_os\_login](#input\_use\_os\_login) | Use OS Login when connecting by SSH | `bool` | `false` | no |
| <a name="input_windows_startup_ps1"></a> [windows\_startup\_ps1](#input\_windows\_startup\_ps1) | A list of strings containing PowerShell scripts which will customize a Windows VM image (requires WinRM communicator) | `list(string)` | `[]` | no |
| <a name="input_wrap_startup_script"></a> [wrap\_startup\_script](#input\_wrap\_startup\_script) | Wrap startup script with Packer-generated wrapper | `bool` | `true` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Cloud zone in which to provision image building VM | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->

[ansible]: #input_ansible_playbooks
[cloudnat]: https://cloud.google.com/nat/docs/overview
[examples readme]: ../../../examples/README.md#image-builderyaml-
[hpcimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm
[iamprop]: https://cloud.google.com/iam/docs/access-change-propagation
[iaptunnel]: https://cloud.google.com/iap/docs/using-tcp-forwarding
[image builder]: ../../../examples/image-builder.yaml
[logging-console]: https://console.cloud.google.com/logs/
[logging-read-docs]: https://cloud.google.com/sdk/gcloud/reference/logging/read
[pkrvars]: https://www.packer.io/guides/hcl/variables#from-a-file
[shell]: #input_shell_scripts
[ssf]: #input_startup_script_file
[sss]: #input_startup_script
[startup-metadata]: https://cloud.google.com/compute/docs/instances/startup-scripts/linux
[startup-script]: ../../../modules/scripts/startup-script
[vpc]: ../../network/vpc/README.md
