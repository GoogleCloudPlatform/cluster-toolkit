## Description

This module creates a [Managed Lustre](https://cloud.google.com/managed-lustre)
instance. Managed Lustre is a high performance network file system that can be
mounted to one or more VMs.

For more information on this and other network storage options in the Cluster
Toolkit, see the extended [Network Storage documentation](../../../docs/network_storage.md).

### Supported Operating Systems

A Managed Lustre instance can be used with Slurm cluster or compute
VM running Ubuntu 20.04, 22.04 or Rocky Linux 8 (including the HPC flavor).

### Managed Lustre Access

Managed Lustre is available by invitation only. If you'd like to request access to Managed Lustre in your Google Cloud project, contact your sales representative.

### Example - New VPC

For Managed Lustre instance, the snippet below creates new VPC and configures
private-service-access for this newly created network.  Both items are required
to be passed to the Lustre module to ensure that they're built in order and
that the correct subnetwork has private service access.

```yaml
 - id: network
    source: modules/network/vpc

  - id: private_service_access
    source: community/modules/network/private-service-access
    use: [network]
    settings:
      prefix_length: 24

  - id: lustre
    source: modules/file-system/managed-lustre
    use: [network, private_service_access]
```

### Example - Slurm

When using Slurm you must take into consideration whether or not you are using
an official image from the `schedmd-slurm-public` project or building your own.
The Lustre client modules are pre-installed in the official images.  With the
official images, Lustre can be used as follows:

```yaml
- id: managed_lustre
  source: modules/file-system/managed-lustre
  use: [network, private_service_access]
  settings:
    name: lustre-instance
    local_mount: /lustre
    remote_mount: lustrefs
    size_gib: 18000

# Other modules: nodesets, partitions, login, etc.

- id: slurm_controller
  source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
  use:
  - network
  - lustre_partition
  - managed_lustre
  - slurm_login
  settings:
    machine_type: n2-standard-4
    enable_controller_public_ips: true
```

For custom images you must install the modules during the image build as the
Slurm cluster will not run the installation script like it does for the
standard VMs.

Assuming you have a startup script for the Slurm image building, you can add
this Ansible playbook to correctly install the Lustre drivers into the image:

```yaml
- type: ansible-local
  destination: install_managed_lustre.yml
  content: |
    ---
    - name: Install Managed Luster Client Modules
      hosts: all
      become: true
      vars:
        lustre_packages:
        - lustre-client-modules-{{ ansible_kernel }}
        - lustre-client-utils
      tasks:
      - name: Add gpg key for Lustre Client repo
        ansible.builtin.get_url:
          url: https://us-apt.pkg.dev/doc/repo-signing-key.gpg
          dest: /etc/apt/keyrings/lustre-client.asc
          mode: '0644'
          force: true
      - name: Add Lustre Client module repo
        ansible.builtin.apt_repository:
          repo: deb [ signed-by=/etc/apt/keyrings/lustre-client.asc ] https://us-apt.pkg.dev/projects/lustre-client-binaries lustre-client-ubuntu-{{ ansible_distribution_release }} main
      - name: Install Lustre packages
        ansible.builtin.apt:
          name: "{{ item }}"
          update_cache: true
        loop: "{{ lustre_packages }}"
```

### Example - Existing VPC

If you want to use existing network with private-service-access configured, you need
to manually provide `private_vpc_connection_peering` to the Managed Lustre module.
You can get this details from the Google Cloud Console UI in `VPC network peering`
section. Below is the example of using existing network and creating Managed Lustre.
If existing network is not configured with private-service-access, you can follow
[Configure private service access](https://cloud.google.com/vpc/docs/configure-private-services-access)
to set it up.

```yaml
  - id: network
    source: modules/network/pre-existing-vpc
    settings:
      network_name: <network_name> // Add network name
      subnetwork_name: <subnetwork_name> // Add subnetwork name

  - id: lustre
    source: modules/file-system/managed-lustre
    use: [network]
    settings:
      private_vpc_connection_peering: <private_vpc_connection_peering> # will look like "servicenetworking.googleapis.com"
```

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2025 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.27.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.27.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_lustre_instance.lustre_instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/lustre_instance) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [google_compute_network_peering.private_peering](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_network_peering) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used as name of the Lustre instance if no name is specified. | `string` | n/a | yes |
| <a name="input_description"></a> [description](#input\_description) | Description of the created Lustre instance. | `string` | `"Lustre Instance"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the Managed Lustre instance. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Local mount point for the Managed Lustre instance. | `string` | `"/shared"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | Mounting options for the file system. | `string` | `"defaults,_netdev"` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the Lustre instance | `string` | n/a | yes |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the instance is connected given in the format:<br/>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | Network self-link this instance will be on, required for checking private service access | `string` | n/a | yes |
| <a name="input_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#input\_private\_vpc\_connection\_peering) | The name of the VPC Network peering connection.<br/>If using new VPC, please use community/modules/network/private-service-access to create private-service-access and<br/>If using existing VPC with private-service-access enabled, set this manually." | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which Lustre instance will be created. | `string` | n/a | yes |
| <a name="input_remote_mount"></a> [remote\_mount](#input\_remote\_mount) | Remote mount point of the Managed Lustre instance | `string` | n/a | yes |
| <a name="input_size_gib"></a> [size\_gib](#input\_size\_gib) | Storage size of the Managed Lustre instance in GB. See https://cloud.google.com/managed-lustre/docs/create-instance for limitations | `number` | `18000` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Location for the Lustre instance. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_capacity_gib"></a> [capacity\_gib](#output\_capacity\_gib) | File share capacity in GiB. |
| <a name="output_install_managed_lustre_client"></a> [install\_managed\_lustre\_client](#output\_install\_managed\_lustre\_client) | Script for installing Managed Lustre client |
| <a name="output_lustre_id"></a> [lustre\_id](#output\_lustre\_id) | An identifier for the resource with format `projects/{{project}}/locations/{{location}}/instances/{{name}}` |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a Managed Lustre instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
