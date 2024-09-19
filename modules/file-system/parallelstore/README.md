## Description

This module creates [parallelstore](https://cloud.google.com/parallelstore)
instance. Parallelstore is Google Cloud's first party parallel file system
service based on [Intel DAOS](https://docs.daos.io/v2.2/)

### Supported Operating Systems

A parallelstore instance can be used with Slurm cluster or compute
VM running Ubuntu 22.04, debian 12 or HPC Rocky Linux 8.

### Parallelstore Quota

To get access to a private preview of Parallelstore APIs, your project needs to
be allowlisted. To set this up, please work with your account representative.

### Parallelstore mount options

After parallelstore instance is created, you can specify mount options depending
upon your workload. DAOS is configured to deliver the best user experience for
interactive workloads with aggressive caching. If you are running parallel
workloads concurrently accessing the sane files from multiple client nodes, it
is recommended to disable the writeback cache to avoid cross-client consistency
issues. You can specify different mount options as follows,

```yaml
  - id: parallelstore
    source: modules/file-system/parallelstore
    use: [network, ps_connect]
    settings:
      mount_options: "disable-wb-cache,thread-count=20,eq-count=8"
```

### Example - New VPC

For parallelstore instance, Below snippet creates new VPC and configures private-service-access
for this newly created network.

```yaml
 - id: network
    source: modules/network/vpc

  - id: private_service_access
    source: community/modules/network/private-service-access
    use: [network]
    settings:
      prefix_length: 24

  - id: parallelstore
    source: modules/file-system/parallelstore
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

  - id: parallelstore
    source: modules/file-system/parallelstore
    use: [network]
    settings:
      private_vpc_connection_peering: <private_vpc_connection_peering> # will look like "servicenetworking.googleapis.com"
```

### Import data from GCS bucket

You can import data from your GCS bucket to parallelstore instance. Important to
note that data may not be available to the instance immediately. This depends on
latency and size of data. Below is the example of importing data from  bucket.

```yaml
  - id: parallelstore
    source: modules/file-system/parallelstore
    use: [network]
    settings:
      import_gcs_bucket_uri: gs://gcs-bucket/folder-path
      import_destination_path: /gcs/import/
```

Here you can replace `import_gcs_bucket_uri` with the uri of sub folder within GCS
bucket and `import_destination_path` with local directory within parallelstore
instance.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2024 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 5.25.0 |
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 5.25.0 |
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_parallelstore_instance.instance](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_parallelstore_instance) | resource |
| [null_resource.hydration](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment. | `string` | n/a | yes |
| <a name="input_import_destination_path"></a> [import\_destination\_path](#input\_import\_destination\_path) | The name of local path to import data on parallelstore instance from GCS bucket. | `string` | `null` | no |
| <a name="input_import_gcs_bucket_uri"></a> [import\_gcs\_bucket\_uri](#input\_import\_gcs\_bucket\_uri) | The name of the GCS bucket to import data from to parallelstore. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to parallel store instance. | `map(string)` | `{}` | no |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | The mount point where the contents of the device may be accessed after mounting. | `string` | `"/parallelstore"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | Options describing various aspects of the parallelstore instance. | `string` | `"disable-wb-cache,thread-count=16,eq-count=8"` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of parallelstore instance. | `string` | `null` | no |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the instance is connected given in the format:<br/>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#input\_private\_vpc\_connection\_peering) | The name of the VPC Network peering connection.<br/>If using new VPC, please use community/modules/network/private-service-access to create private-service-access and<br/>If using existing VPC with private-service-access enabled, set this manually." | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created. | `string` | n/a | yes |
| <a name="input_size_gb"></a> [size\_gb](#input\_size\_gb) | Storage size of the parallelstore instance in GB. | `number` | `12000` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Location for parallelstore instance. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions to monitor import-data operation from GCS bucket to parallelstore. |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a parallelstore instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
