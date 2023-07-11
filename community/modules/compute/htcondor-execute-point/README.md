## Description

This module performs the following tasks:

- create an instance template from which execute points will be created
- create a managed instance group (MIG) for execute points
- create a Toolkit runner to configure the autoscaler to scale the MIG

It is expected to be used with the [htcondor-install] and [htcondor-base]
modules.

[htcondor-install]: ../../scripts/htcondor-install/README.md
[htcondor-base]: ../../scheduler/htcondor-configure/README.md

### Known limitations

This module may be used exactly 1 or 2 times in a blueprint to create sets of
execute points in an HTCondor pool. If using 1 set, it may use either Spot or
On-demand pricing. If using 2 sets, one must use Spot and the other must
use On-demand pricing. If you do not follow this constraint, you will likely
receive an error while running `terraform apply` similar to that shown below.
Future development is planned to support more than 2 sets of VM configurations,
including all pricing options.

```text
│     │ var.runners is list of map of string with 7 elements
│
│ All startup-script runners must have a unique destination.
│
│ This was checked by the validation rule at modules/startup-script/variables.tf:72,3-13.
```

### How to run HTCondor jobs on Spot VMs

HTCondor access points provisioned by the Toolkit are specially configured to
add an attribute named `RequireSpot` to each [Job ClassAd][jobad]. When this
value is true, a job's `requirements` are automatically updated to require
that it run on a Spot VM. When this value is false, the `requirements` are
similarly updated to run only on On-Demand VMs. The default value of this
attribute is false. A job submit file may override this value as shown below.

```text
universe       = vanilla
executable     = /bin/echo
arguments      = "Hello, World!"
output         = out.\$(ClusterId).\$(ProcId)
error          = err.\$(ClusterId).\$(ProcId)
log            = log.\$(ClusterId).\$(ProcId)
request_cpus   = 1
request_memory = 100MB
+RequireSpot   = true
queue
```

[jobad]: https://htcondor.readthedocs.io/en/latest/users-manual/matchmaking-with-classads.html

### Example

A full example can be found in the [examples README][htc-example].

[htc-example]: ../../../../examples/README.md#htc-htcondoryaml--

The following code snippet creates a pool with 2 sets of HTCondor execute
points, one using On-demand pricing and the other using Spot pricing. They use
a startup script and network created in previous steps.

```yaml
- id: htcondor_execute_point
  source: community/modules/compute/htcondor-execute-point
  use:
  - network1
  - htcondor_configure_execute_point
  settings:
    service_account:
      email: $(htcondor_configure.execute_point_service_account)
      scopes:
      - cloud-platform

- id: htcondor_execute_point_spot
  source: community/modules/compute/htcondor-execute-point
  use:
  - network1
  - htcondor_configure_execute_point
  settings:
    service_account:
      email: $(htcondor_configure.execute_point_service_account)
      scopes:
      - cloud-platform

  - id: htcondor_startup_access_point
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(htcondor_install.install_htcondor_runner)
      - $(htcondor_install.install_autoscaler_deps_runner)
      - $(htcondor_install.install_autoscaler_runner)
      - $(htcondor_configure.access_point_runner)
      - $(htcondor_execute_point.configure_autoscaler_runner)
      - $(htcondor_execute_point_spot.configure_autoscaler_runner)

  - id: htcondor_access
    source: modules/compute/vm-instance
    use:
    - network1
    - htcondor_startup_access_point
    settings:
      name_prefix: access-point
      machine_type: c2-standard-4
      service_account:
        email: $(htcondor_configure.access_point_service_account)
        scopes:
        - cloud-platform
```

## Support

HTCondor is maintained by the [Center for High Throughput Computing][chtc] at
the University of Wisconsin-Madison. Support for HTCondor is available via:

- [Discussion lists](https://htcondor.org/mail-lists/)
- [HTCondor on GitHub](https://github.com/htcondor/htcondor/)
- [HTCondor manual](https://htcondor.readthedocs.io/en/latest/)

[chtc]: https://chtc.cs.wisc.edu/

## Known Issues

When using OS Login with "external users" (outside of the Google Cloud
organization), then Docker universe jobs will fail and cause the Docker daemon
to crash. This stems from the use of POSIX user ids (uid) outside the range
supported by Docker. Please consider disabling OS Login if this atypical
situation applies.

```yaml
vars:
  # add setting below to existing deployment variables
  enable_oslogin: DISABLE
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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.1 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_execute_point_instance_template"></a> [execute\_point\_instance\_template](#module\_execute\_point\_instance\_template) | terraform-google-modules/vm/google//modules/instance_template | ~> 8.0 |
| <a name="module_mig"></a> [mig](#module\_mig) | terraform-google-modules/vm/google//modules/mig | ~> 8.0 |

## Resources

| Name | Type |
|------|------|
| [google_compute_image.htcondor](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_image) | data source |
| [google_compute_zones.available](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_zones) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. HTCondor cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Boot disk size in GB | `number` | `100` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enable or Disable OS Login with "ENABLE" or "DISABLE". Set to "INHERIT" to inherit project OS Login setting. | `string` | `"ENABLE"` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | HTCondor execute point VM image | <pre>object({<br>    family  = string,<br>    project = string<br>  })</pre> | <pre>{<br>  "family": "hpc-rocky-linux-8",<br>  "project": "cloud-hpc-image-public"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to HTConodr execute points | `map(string)` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for HTCondor execute points | `string` | `"n2-standard-4"` | no |
| <a name="input_max_size"></a> [max\_size](#input\_max\_size) | Maximum size of the HTCondor execute point pool. | `number` | `100` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata to add to HTCondor execute points | `map(string)` | `{}` | no |
| <a name="input_min_idle"></a> [min\_idle](#input\_min\_idle) | Minimum number of idle VMs in the HTCondor pool (if pool reaches var.max\_size, this minimum is not guaranteed); set to ensure jobs beginning run more quickly. | `number` | `0` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network HTCondor execute points will join | `string` | `"default"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured | <pre>list(object({<br>    server_ip             = string,<br>    remote_mount          = string,<br>    local_mount           = string,<br>    fs_type               = string,<br>    mount_options         = string,<br>    client_install_runner = map(string)<br>    mount_runner          = map(string)<br>  }))</pre> | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HTCondor execute points will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region in which HTCondor execute points will be created | `string` | n/a | yes |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to HTCondor execute points | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | <pre>{<br>  "email": null,<br>  "scopes": [<br>    "https://www.googleapis.com/auth/cloud-platform"<br>  ]<br>}</pre> | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision VMs using discounted Spot pricing, allowing for preemption | `bool` | `false` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script to run at boot-time for Linux HTCondor execute points | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork HTCondor execute points will join | `string` | `null` | no |
| <a name="input_target_size"></a> [target\_size](#input\_target\_size) | Initial size of the HTCondor execute point pool; set to null (default) to avoid Terraform management of size. | `number` | `null` | no |
| <a name="input_windows_startup_ps1"></a> [windows\_startup\_ps1](#input\_windows\_startup\_ps1) | Startup script to run at boot-time for Windows-based HTCondor execute points | `list(string)` | `[]` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | The default zone in which resources will be created | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_configure_autoscaler_runner"></a> [configure\_autoscaler\_runner](#output\_configure\_autoscaler\_runner) | Toolkit runner to configure the HTCondor autoscaler |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
