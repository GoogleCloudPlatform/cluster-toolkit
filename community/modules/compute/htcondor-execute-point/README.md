## Description

This module performs the following tasks:

- create an instance template from which execute points will be created
- create a managed instance group (MIG) for execute points
- create a Toolkit runner to configure the autoscaler to scale the MIG

It is expected to be used with the [htcondor-install] and [htcondor-configure]
modules.

[htcondor-install]: ../../scripts/htcondor-install/README.md
[htcondor-configure]: ../../scheduler/htcondor-configure/README.md

### Example

The following code snippet creates a pool of HTCondor execute points using
a startup script and network created in previous steps.

> **_NOTE:_** HTCondor does not appear to interoperate correctly with the user
> identities created by OS Login. Until this is resolved, we advise disabling
> OS Login on all HTCondor nodes, including execute points.

```yaml
- id: htcondor_execute_point
  source: community/modules/compute/htcondor-execute-point
  use:
  - network1
  - htcondor_configure_execute_point
  settings:
    metadata:
      central-manager: ((module.htcondor_cm.internal_ip[0]))
      enable-oslogin: "FALSE"
    service_account:
      email: $(htcondor_configure.execute_point_service_account)
      scopes:
      - cloud-platform
```

A full example can be found in the [examples README][htc-example].

[htc-example]: ../../../../examples/README.md#htcondor-poolyaml--

## Support

HTCondor is maintained by the [Center for High Throughput Computing][chtc] at
the University of Wisconsin-Madison. Support for HTCondor is available via:

- [Discussion lists](https://htcondor.org/mail-lists/)
- [HTCondor on GitHub](https://github.com/htcondor/htcondor/)
- [HTCondor manual](https://htcondor.readthedocs.io/en/latest/)

[chtc]: https://chtc.cs.wisc.edu/

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13.0 |

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_execute_point_instance_template"></a> [execute\_point\_instance\_template](#module\_execute\_point\_instance\_template) | terraform-google-modules/vm/google//modules/instance_template | ~> 7.8.0 |
| <a name="module_mig"></a> [mig](#module\_mig) | terraform-google-modules/vm/google//modules/mig | ~> 7.8.0 |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | HPC Toolkit deployment name. HTCondor cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enable or Disable OS Login with "ENABLE" or "DISABLE". Set to "INHERIT" to inherit project OS Login setting. | `string` | `"DISABLE"` | no |
| <a name="input_image"></a> [image](#input\_image) | HTCondor execute point VM image | <pre>object({<br>    family  = string,<br>    project = string<br>  })</pre> | <pre>{<br>  "family": "hpc-centos-7",<br>  "project": "cloud-hpc-image-public"<br>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to HTConodr execute points | `map(string)` | n/a | yes |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for HTCondor execute points | `string` | `"n2-standard-4"` | no |
| <a name="input_max_size"></a> [max\_size](#input\_max\_size) | Maximum size of the HTCondor execute point pool; set to constrain cost run-away. | `number` | `100` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata to add to HTCondor execute points | `map(string)` | `{}` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network HTCondor execute points will join | `string` | `"default"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured | <pre>list(object({<br>    server_ip     = string,<br>    remote_mount  = string,<br>    local_mount   = string,<br>    fs_type       = string,<br>    mount_options = string<br>  }))</pre> | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HTCondor execute points will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region in which HTCondor execute points will be created | `string` | n/a | yes |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | Service account to attach to HTCondor execute points | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | <pre>{<br>  "email": null,<br>  "scopes": [<br>    "https://www.googleapis.com/auth/cloud-platform"<br>  ]<br>}</pre> | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Startup script to run at boot-time for HTCondor execute points | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork HTCondor execute points will join | `string` | `null` | no |
| <a name="input_target_size"></a> [target\_size](#input\_target\_size) | Initial size of the HTCondor execute point pool; set to null (default) to avoid Terraform management of size. | `number` | `null` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | The default zone in which resources will be created | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_configure_autoscaler_runner"></a> [configure\_autoscaler\_runner](#output\_configure\_autoscaler\_runner) | Toolkit runner to configure the HTCondor autoscaler |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
