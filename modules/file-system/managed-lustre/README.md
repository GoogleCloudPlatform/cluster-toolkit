## Description

This module creates a [Managed Lustre](https://cloud.google.com/managed-lustre)
instance. Managed Lustre is a high performance network file system that can be
mounted to one or more compute VMs.

For more information on this and other network storage options in the Cluster
Toolkit, see the extended [Network Storage documentation](../../../docs/network_storage.md).

### Supported Operating Systems

A Managed Lustre instance can be used with Slurm cluster or compute
VM running Ubuntu 20.04, 22.04 or Rocky Linux 8 (including the HPC flavor).

> [!WARNING] The Ubuntu OS client kernel modules are only available for a
> limited number of kernels. To check compatibility please see
> [Artifact Registry](https://pantheon.corp.google.com/artifacts/browse/lustre-client-binaries/us?e=-13802955&hl=en&invt=Abuvpg&mods=logs_tg_prod&project=lustre-client-binaries)
> to check which kernels the packages currently support.

### Managed Lustre Access

Currently Managed Lustre is only available on allowlisted projects.  To set
this up, please work with your account representative.

### Example - New VPC

For Managed Lustre instance, Below snippet creates new VPC and configures
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

### Example - Existing VPC

If you want to use existing network with private-service-access configured, you need
to manually provide `private_vpc_connection_peering` to the parallelstore module.
You can get this details from the Google Cloud Console UI in `VPC network peering`
section. Below is the example of using existing network and creating parallelstore.
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

### Import data from GCS bucket

You can import data from your GCS bucket to Managed Lustre instance. Important
to note that data may not be available to the instance immediately. This
depends on latency and size of data. Below is the example of importing data
from  bucket.

```yaml
  - id: lustre
    source: modules/file-system/managed-lustre
    use: [network, private-service-access]
    settings:
      import_gcs_bucket_uri: gs://gcs-bucket/folder-path
      import_destination_path: /gcs/import/
```

Here you can replace `import_gcs_bucket_uri` with the uri of sub folder within
GCS bucket and `import_destination_path` with local directory within the
Managed Lustre instance.

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
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.27.0 |
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_lustre_instance.lustre_instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/lustre_instance) | resource |
| [null_resource.hydration](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [google_compute_subnetwork.private_subnetwork](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used as name of the Lustre instance if no name is specified. | `string` | n/a | yes |
| <a name="input_description"></a> [description](#input\_description) | Description of the created Lustre instance. | `string` | `"Lustre Instance"` | no |
| <a name="input_import_destination_path"></a> [import\_destination\_path](#input\_import\_destination\_path) | The name of local path to import data on Lustre instance from GCS bucket. | `string` | `null` | no |
| <a name="input_import_gcs_bucket_uri"></a> [import\_gcs\_bucket\_uri](#input\_import\_gcs\_bucket\_uri) | The name of the GCS bucket to import data from to the Lustre instance. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the Managed Lustre instance. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Local mount point for the Managed Lustre instance. | `string` | `"/shared"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | Mounting options for the file system. | `string` | `"defaults,_netdev"` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the Lustre instance | `string` | n/a | yes |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the instance is connected given in the format:<br/>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#input\_private\_vpc\_connection\_peering) | The name of the VPC Network peering connection.<br/>If using new VPC, please use community/modules/network/private-service-access to create private-service-access and<br/>If using existing VPC with private-service-access enabled, set this manually." | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which Lustre instance will be created. | `string` | n/a | yes |
| <a name="input_remote_mount"></a> [remote\_mount](#input\_remote\_mount) | Remote mount point of the Managed Lustre instance | `string` | n/a | yes |
| <a name="input_size_gib"></a> [size\_gib](#input\_size\_gib) | Storage size of the Managed Lustre instance in GB. See https://cloud.google.com/managed-lustre/docs/create-instance for limitations | `number` | `18000` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | Subnetwork self-link this instance will be on, required for checking private service access | `string` | n/a | yes |
| <a name="input_zone"></a> [zone](#input\_zone) | Location for the Lustre instance. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_capacity_gb"></a> [capacity\_gb](#output\_capacity\_gb) | File share capacity in GiB. |
| <a name="output_install_managed_lustre_client"></a> [install\_managed\_lustre\_client](#output\_install\_managed\_lustre\_client) | Script for installing NFS client |
| <a name="output_lustre_id"></a> [lustre\_id](#output\_lustre\_id) | An identifier for the resource with format `projects/{{project}}/locations/{{location}}/instances/{{name}}` |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a filestore instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
