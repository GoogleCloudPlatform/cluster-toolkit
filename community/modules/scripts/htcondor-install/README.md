## Description

This module creates a Toolkit runner that will install HTCondor on RedHat 7 or
8 and its derivative operating systems. These include the CentOS 7 and Rocky
Linux 8 releases of the [HPC VM Image][hpcvmimage]. It may also function on
RedHat 9 and derivatives, however it is not yet supported. Please report any
[issues] on these 3 distributions or open a [discussion] to request support on
Debian or Ubuntu distributions.

[issues]: https://github.com/GoogleCloudPlatform/hpc-toolkit/issues
[discussion]: https://github.com/GoogleCloudPlatform/hpc-toolkit/discussions

It also exports a list of Google Cloud APIs which must be enabled prior to
provisioning an HTCondor Pool.

It is expected to be used with the [htcondor-configure] and
[htcondor-execute-point] modules.

[hpcvmimage]: https://cloud.google.com/compute/docs/instances/create-hpc-vm
[htcondor-configure]: ../../scheduler/htcondor-configure/README.md
[htcondor-execute-point]: ../../compute/htcondor-execute-point/README.md

### Example

The following code snippet uses this module to create startup scripts that
install the HTCondor software and adds custom configurations using
[htcondor-configure] and [htcondor-execute-point].

```yaml
- id: htcondor_install
  source: community/modules/scripts/htcondor-install

- id: htcondor_startup_central_manager
  source: modules/scripts/startup-script
  settings:
    runners:
    - $(htcondor_install.install_htcondor_runner)
    - $(htcondor_configure.central_manager_runner)

- id: htcondor_startup_access_point
  source: modules/scripts/startup-script
  settings:
    runners:
    - $(htcondor_install.install_htcondor_runner)
    - $(htcondor_install.install_autoscaler_deps_runner)
    - $(htcondor_install.install_autoscaler_runner)
    - $(htcondor_configure.access_point_runner)
    - $(htcondor_execute_point.configure_autoscaler_runner)
```

A full example can be found in the [examples README][htc-example].

[htc-example]: ../../../../examples/README.md#htc-htcondoryaml--

## Important note

All POSIX users and HTCondor jobs can act as the service account attached to
VMs within the pool. This enables the use of IAM restrictions via service
accounts but also allows users to access services to which system daemons need
access (e.g. to create Cloud Logging entries). If this is undesirable, one can
restrict access to the instance metadata server to the `root` and `condor`
users. This will allow system services to use the service account, but not
other POSIX users or HTCondor jobs. The firewall example below is appropriate
for CentOS 7.

```shell
firewall-cmd --direct --permanent --add-rule ipv4 filter OUTPUT_direct 1 \
    -m owner --uid-owner root -p tcp -d metadata.google.internal --dport 80 -j ACCEPT
firewall-cmd --direct --permanent --add-rule ipv4 filter OUTPUT_direct 2 \
    -m owner --uid-owner condor -p tcp -d metadata.google.internal --dport 80 -j ACCEPT
firewall-cmd --direct --permanent --add-rule ipv4 filter OUTPUT_direct 3 \
    -p tcp -d metadata.google.internal --dport 80 -j DROP
firewall-cmd --direct --permanent --add-rule ipv4 filter OUTPUT_direct 4 \
    -p tcp -d metadata.google.internal --dport 8080 -j DROP
firewall-cmd --permanent --zone=public --add-port=9618/tcp
firewall-cmd --reload
```

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

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_condor_version"></a> [condor\_version](#input\_condor\_version) | Yum/DNF-compatible version string; leave unset to default to 10.x series (examples: "10.5.1","10.*")) | `string` | `"10.*"` | no |
| <a name="input_enable_docker"></a> [enable\_docker](#input\_enable\_docker) | Install and enable docker daemon alongside HTCondor | `bool` | `true` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_gcp_service_list"></a> [gcp\_service\_list](#output\_gcp\_service\_list) | Google Cloud APIs required by HTCondor |
| <a name="output_install_autoscaler_deps_runner"></a> [install\_autoscaler\_deps\_runner](#output\_install\_autoscaler\_deps\_runner) | Toolkit Runner to install HTCondor autoscaler dependencies |
| <a name="output_install_autoscaler_runner"></a> [install\_autoscaler\_runner](#output\_install\_autoscaler\_runner) | Toolkit Runner to install HTCondor autoscaler |
| <a name="output_install_htcondor_runner"></a> [install\_htcondor\_runner](#output\_install\_htcondor\_runner) | Runner to install HTCondor using startup-scripts |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
