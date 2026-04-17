## Description

This module creates a [Google Cloud NetApp Volumes](https://cloud.google.com/netapp/volumes/docs/discover/overview)
storage pool.

NetApp Volumes is a first-party Google service that provides NFS and/or SMB shared file-systems to VMs. It offers advanced data management capabilities and highly scalable capacity and performance.
NetApp Volume provides:

- robust support for NFSv3, NFSv4.x and SMB 2.1 and 3.x
- a [rich feature set][service-levels]
- scalable [performance](https://cloud.google.com/netapp/volumes/docs/performance/performance-benchmarks)
- FlexCache: Caching of ONTAP-based volumes to provide high-throughput and low latency read access to compute clusters of on-premises data
- [Auto-tiering](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/manage-auto-tiering) of unused data to optimse cost

Support for NetApp Volumes is split into two modules.

- **netapp-storage-pool** provisions a [storage pool](https://cloud.google.com/netapp/volumes/docs/configure-and-use/storage-pools/overview). Storage pools are pre-provisioned storage capacity containers which host volumes. A pool also defines fundamental properties of all the volumes within, like the region, the attached network, the [service level][service-levels], CMEK encryption, Active Directory and LDAP settings.
- **netapp-volume** provisions a [volume](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/overview) inside an existing storage pool. A volume file-system container which is shared using NFS or SMB. It provides advanced data management capabilities.

For more information on this and other network storage options in the Cluster
Toolkit, see the extended [Network Storage documentation](../../../docs/network_storage.md).

### NetApp storage pool service levels

The netapp-storage-pool module currently supports the following NetApp Volumes [service levels][service-levels]:

- Standard: 16 KiBps throughput per provisioned KiB of volume capacity.
- Premium: 64 KiBps throughput per provisioned KiB of volume capacity. Optional [auto-tiering].
- Extreme: 128 KiBps throughput per provisioned KiB of volume capacity. Optional [auto-tiering].

Check the [service level matrix][service-levels] for additional information on capability differences between service levels. Flex service levels are currently not supported, but you can connect to existing Flex volumes using the [pre-existing-network-storage module][pre-existing].

### On-boarding NetApp Volumes
NetApp Volumes uses [Private Service Access](https://cloud.google.com/vpc/docs/private-services-access) (PSA) to connect volumes to your network. Before you create a storage pool, make sure to [connect NetApp Volumes to your network](https://cloud.google.com/netapp/volumes/docs/get-started/configure-access/networking).

Example of creating a storage pool using a new network:

```yaml
deployment_groups:
- group: primary
  modules:
  - id: network
    source: modules/network/vpc
    settings:
      region: $(vars.region)

  - id: private_service_access
    source: modules/network/private-service-access
    use: [network]
    settings:
      prefix_length: 24
      service_name: "netapp.servicenetworking.goog"
      deletion_policy: "ABANDON"

  - id: netapp_pool
    source: modules/file-system/netapp-storage-pool
    use: [network, private_service_access]
    settings:
      pool_name: $(vars.deployment_name)-eda-pool
      capacity_gib: 20000
      service_level: "EXTREME"
      region: $(vars.region)
```

Example of creating a storage pool using an existing network which was already PSA-peered with NetApp Volume:

```yaml
deployment_groups:
 - group: primary
  modules:
  - id: network
    source: modules/network/pre-existing-vpc
    settings:
      project_id: $(vars.project_id)
      region: $(vars.region)
      network_name: $(vars.network)

  - id: netapp_pool
    source: modules/file-system/netapp-storage-pool
    use: [network]
    settings:
      pool_name: "eda-pool"
      capacity_gib: 20000
      service_level: "EXTREME"
      region: $(vars.region)
```

### Storage pool example

The following example shows all available parameters in use:

```yaml
  - id: netapp_pool
    source: modules/file-system/netapp-storage-pool
    use: [network, private_service_access]
    settings:
      pool_name: "mypool"
      region: "us-west4"
      capacity_gib: 2048
      service_level: "EXTREME"
      active_directory_policy: "projects/myproject/locations/us-east4/activeDirectories/my-ad"
      cmek_policy: "projects/myproject/locations/us-east4/kmsConfigs/my-cmek-policy"
      ldap_enabled: false
      allow_auto_tiering: false
      description: "Demo storage pool"
      labels:
        owner: bob
```

### NetApp Volumes quota

Your project must have unused quota for NetApp Volumes in the region you will
provision the storage pool. This can be found by browsing to the [Quota tab within IAM & Admin](https://console.cloud.google.com/iam-admin/quotas) in the Cloud Console.
Please note that there are separate quota limits for Standard and Premium/Extreme service levels.

See also NetApp Volumes [default quotas](https://cloud.google.com/netapp/volumes/docs/quotas#netapp-volumes-default-quotas).

[service-levels]: https://cloud.google.com/netapp/volumes/docs/discover/service-levels
[auto-tiering]: https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/manage-auto-tiering
[pre-existing]: ../pre-existing-network-storage/README.md
[matrix]: ../../../docs/network_storage.md#compatibility-matrix

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.5.7 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.45.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.45.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_netapp_storage_pool.netapp_storage_pool](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/netapp_storage_pool) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [google_compute_network_peering.private_peering](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_network_peering) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_active_directory_policy"></a> [active\_directory\_policy](#input\_active\_directory\_policy) | The ID of the Active Directory policy to apply to the storage pool in the format:<br/>`projects/<project_id>/locations/<location>/activeDirectoryPolicies/<policy_id>` | `string` | `null` | no |
| <a name="input_allow_auto_tiering"></a> [allow\_auto\_tiering](#input\_allow\_auto\_tiering) | Whether to allow automatic tiering for the storage pool. | `bool` | `false` | no |
| <a name="input_capacity_gib"></a> [capacity\_gib](#input\_capacity\_gib) | The capacity of the storage pool in GiB. | `number` | `2048` | no |
| <a name="input_cmek_policy"></a> [cmek\_policy](#input\_cmek\_policy) | The ID of the Customer Managed Encryption Key (CMEK) policy to apply to the storage pool in the format:<br/>`projects/<project>/locations/<location>/kmsConfigs/<name>` | `string` | `null` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, used as name of the NetApp storage pool if no name is specified. | `string` | n/a | yes |
| <a name="input_description"></a> [description](#input\_description) | A description of the NetApp storage pool. | `string` | `""` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the NetApp storage pool. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_ldap_enabled"></a> [ldap\_enabled](#input\_ldap\_enabled) | Whether to enable LDAP for the storage pool. | `bool` | `false` | no |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the NetApp storage pool is connected given in the format:<br/>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | Network self-link the pool will be on, required for checking private service access | `string` | n/a | yes |
| <a name="input_pool_name"></a> [pool\_name](#input\_pool\_name) | The name of the storage pool. Leave empty to generate name based on deployment name. | `string` | `null` | no |
| <a name="input_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#input\_private\_vpc\_connection\_peering) | The name of the private VPC connection peering. | `string` | `"sn-netapp-prod"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which the NetApp storage pool will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Location for NetApp storage pool. | `string` | n/a | yes |
| <a name="input_service_level"></a> [service\_level](#input\_service\_level) | The service level of the storage pool. | `string` | `"PREMIUM"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_capacity_gb"></a> [capacity\_gb](#output\_capacity\_gb) | Storage pool capacity in GiB. |
| <a name="output_netapp_storage_pool_id"></a> [netapp\_storage\_pool\_id](#output\_netapp\_storage\_pool\_id) | An identifier for the resource with format `projects/{{project}}/locations/{{location}}/storagePools/{{name}}` |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
