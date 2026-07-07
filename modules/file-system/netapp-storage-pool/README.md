## Description

This module creates a [Google Cloud NetApp Volumes](https://cloud.google.com/netapp/volumes/docs/discover/overview) storage pool.

NetApp Volumes is a first-party Google service that provides NFS shared file systems to VMs. It offers advanced data management capabilities and highly scalable capacity and performance.

NetApp Volume provides:

- support for NFSv3 and NFSv4.1
- a [rich feature set][service-levels]
- scalable [performance](https://cloud.google.com/netapp/volumes/docs/performance/performance-benchmarks)
- [FlexCache](https://docs.cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/cache-ontap-volumes/overview): Caching of ONTAP-based volumes to provide high-throughput and low latency read access to compute clusters of on-premises data
- [Auto-tiering](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/manage-auto-tiering) of unused data to optimize cost

Support for NetApp Volumes is split into two modules.

- **netapp-storage-pool** provisions a [storage pool](https://cloud.google.com/netapp/volumes/docs/configure-and-use/storage-pools/overview). Storage pools are pre-provisioned storage capacity containers which host volumes. A pool also defines fundamental properties of all the volumes within, like the region, the attached network, the [service level][service-levels], CMEK encryption, Active Directory and LDAP settings.
- **netapp-volume** provisions a [volume](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/overview) inside an existing storage pool. A volume is a file system container shared using NFSv3 or NFSv4.1.

For more information on this and other network storage options in the Cluster
Toolkit, see the extended [Network Storage documentation](../../../docs/network_storage.md).

### NetApp storage pool service levels

The netapp-storage-pool module supports the following NetApp Volumes [service levels][service-levels]:

- Standard: 16 KiBps throughput per provisioned KiB of volume capacity.
- Premium: 64 KiBps throughput per provisioned KiB of volume capacity. Optional [auto-tiering].
- Extreme: 128 KiBps throughput per provisioned KiB of volume capacity. Optional [auto-tiering].
- Flex Unified: Next-generation Flex service level. Supports zonal and regional pools, auto-tiering, and large capacity pools. Flex File and ONTAP mode are not supported by this module.

Check the [service level matrix][service-levels] for additional information on capability differences between service levels.

### Flex Unified defaults

When `service_level` is `FLEX`, the module sets these values automatically:

1. `mode` = `DEFAULT`
2. `type` = `UNIFIED`

You do not set `mode` or `type` in your blueprint. Flex File (`type: FILE`) and ONTAP mode are not supported. Use [pre-existing-network-storage][pre-existing] for ONTAP-mode pools.

### Region and zone settings

Always set `region` in your blueprint. Zone settings apply only to Flex Unified pools.

1. **Standard, Premium, and Extreme** — Set `region` only. Blueprint-level `zone` variables used for VMs are ignored.
2. **Zonal Flex Unified** — Set `region` and `zone`. The module verifies that `zone` is in `region`.
3. **Regional Flex Unified** — Set `region`, `zone` (active zone), and `replica_zone`. The module verifies that both zones are in `region`.
4. **Large capacity (SCALE_TYPE_SCALEOUT)** — Zonal Flex Unified only. Set `region` and `zone`. Do not set `replica_zone`.

### Minimum pool capacity

The module enforces these minimum `capacity_gib` values at plan time:

1. **STANDARD, PREMIUM, and EXTREME** — 2048 GiB
2. **Flex Unified** — 1024 GiB
3. **Flex Unified large capacity (`scale_type: SCALE_TYPE_SCALEOUT`)** — 6144 GiB

### Flex Unified auto-tiering

When `allow_auto_tiering` is `true` on a Flex Unified pool:

1. Set `hot_tier_size_gib` to the hot-tier capacity in GiB.
2. Optionally set `enable_hot_tier_auto_resize` to allow the hot tier to grow when it reaches 100%.

When `allow_auto_tiering` is `false` (the default), omit `hot_tier_size_gib` and
`enable_hot_tier_auto_resize`. The module fails at plan time if either is set.

Volumes in the pool can then enable auto-tiering with `tiering_policy` in the [netapp-volume](../netapp-volume/README.md) module.

### Flex Unified large capacity pools

To host Flex Unified large capacity volumes, create a zonal pool with:

1. `service_level: FLEX`
2. `region` and `zone` (omit `replica_zone`)
3. `scale_type: SCALE_TYPE_SCALEOUT`
4. `capacity_gib` of at least 6144

Pair this pool with volumes that set `large_capacity_config` in the [netapp-volume](../netapp-volume/README.md) module.

### Flex Unified custom performance

Flex Unified pools use custom performance implicitly. You can scale capacity, throughput, and IOPS independently.

1. Each pool includes 64 MiB/s throughput and 1,024 IOPS by default.
2. Set `total_throughput_mibps` to provision additional throughput in 1 MiB/s increments, up to 5 GiB/s for standard Flex pools.
3. Set `total_iops` to provision additional IOPS, up to 160,000 per pool. Omit `total_iops` to let Google Cloud calculate IOPS from `total_throughput_mibps` (16 IOPS per additional MiB/s).
4. All volumes in the pool share the pool throughput and IOPS.
5. Large capacity pools (`scale_type: SCALE_TYPE_SCALEOUT`) can reach higher throughput limits. See [Volume performance sizing](https://cloud.google.com/netapp/volumes/docs/plan-and-prepare/volume-performance-sizing).

```yaml
  - id: flex_pool_custom_perf
    source: modules/file-system/netapp-storage-pool
    use: [network, private_service_access]
    settings:
      pool_name: "flex-pool-perf"
      service_level: FLEX
      region: us-east1
      zone: us-east1-b
      capacity_gib: 5120
      total_throughput_mibps: 256
      total_iops: 4096
```

### Flex Unified zonal and regional pools

Flex Unified pools can be zonal or regional. The module selects the pool type from your zone settings.

#### Zonal vs regional rules

1. **Zonal pool** — Set `region` and `zone`. Omit `replica_zone`. Pool `location` is the zone name.
2. **Regional pool** — Set `region`, `zone` (active zone), and `replica_zone` (standby zone). Pool `location` is the region name. Volume access is served from the active zone. If the active zone fails, `replica_zone` becomes active.
3. **`zone` and `replica_zone` must be different** for regional pools.
4. **`zone` and `replica_zone` must be within `region`.**
5. **Large capacity pools (`scale_type: SCALE_TYPE_SCALEOUT`) must be zonal.** Set `zone` and omit `replica_zone`.
6. **Standard, Premium, and Extreme pools use `region` only.** Blueprint-level `zone` values (for example, for VM placement) are ignored and do not block pool creation.

#### Multiple pools in one blueprint

To provision both a zonal and a regional Flex pool, add one `netapp-storage-pool` module per pool. Use a unique module `id` and `pool_name` for each pool. Configure zone settings per module. Attach volumes with `use: [<pool_module_id>]`.

Example:

```yaml
vars:
  region: us-east1
  zone_a: us-east1-b
  zone_b: us-east1-c

deployment_groups:
- group: netapp-pools
  modules:
  - id: network
    source: modules/network/pre-existing-vpc
    settings:
      region: $(vars.region)
      network_name: $(vars.network)

  - id: flex_pool_zonal
    source: modules/file-system/netapp-storage-pool
    use: [network]
    settings:
      pool_name: $(vars.deployment_name)-flex-zonal
      service_level: FLEX
      region: $(vars.region)
      zone: $(vars.zone_a)
      capacity_gib: 4096
      total_throughput_mibps: 128
      total_iops: 2048

  - id: flex_pool_regional
    source: modules/file-system/netapp-storage-pool
    use: [network]
    settings:
      pool_name: $(vars.deployment_name)-flex-regional
      service_level: FLEX
      region: $(vars.region)
      zone: $(vars.zone_a)
      replica_zone: $(vars.zone_b)
      capacity_gib: 4096
      total_throughput_mibps: 128
      total_iops: 2048

- group: netapp-volumes
  modules:
  - id: zonal_homefs
    source: modules/file-system/netapp-volume
    use: [flex_pool_zonal]
    settings:
      volume_name: zonal_homefs
      capacity_gib: 1024
      local_mount: /zonal-home
      protocols: ["NFSV3"]

  - id: regional_homefs
    source: modules/file-system/netapp-volume
    use: [flex_pool_regional]
    settings:
      volume_name: regional_homefs
      capacity_gib: 1024
      local_mount: /regional-home
      protocols: ["NFSV3"]
```

#### Zone switch and Terraform state

If a regional pool fails over outside Terraform, update `zone` and `replica_zone` in your blueprint to match the current active and replica zones before the next apply. Otherwise Terraform can initiate an unwanted zone switch.

### ONTAP mode

These modules provision and manage NetApp Volumes through Google Cloud APIs in Default mode only. They do not support Flex Unified pools in ONTAP mode.

If you need ONTAP-mode pools or volumes, provision them outside Cluster Toolkit and integrate them with the [pre-existing-network-storage][pre-existing] module. In ONTAP mode, the pool is created through Google Cloud APIs, but volumes, snapshots, and policies inside the pool are managed with ONTAP tools. See [Manage ONTAP mode](https://cloud.google.com/netapp/volumes/docs/ontap/manage-ontap).

### Outputs for netapp-volume

When a volume module uses `use: [netapp_pool]`, Cluster Toolkit wires these pool outputs automatically:

1. `netapp_storage_pool_id`
2. `service_level`
3. `type`
4. `allow_auto_tiering`
5. `scale_type`

The volume module parses volume location from `netapp_storage_pool_id`. It does not inherit `region` or `zone`.

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

### Storage pool examples

#### Standard, Premium, or Extreme

```yaml
  - id: netapp_pool
    source: modules/file-system/netapp-storage-pool
    use: [network, private_service_access]
    settings:
      pool_name: "mypool"
      region: "us-west4"
      capacity_gib: 2048
      service_level: "EXTREME"
      allow_auto_tiering: true
      active_directory_policy: "projects/myproject/locations/us-west4/activeDirectories/my-ad"
      cmek_policy: "projects/myproject/locations/us-west4/kmsConfigs/my-cmek-policy"
      ldap_enabled: false
      description: "Demo storage pool"
      labels:
        owner: bob
```

#### Flex Unified zonal with auto-tiering

```yaml
  - id: flex_pool
    source: modules/file-system/netapp-storage-pool
    use: [network, private_service_access]
    settings:
      pool_name: "flex-pool"
      service_level: FLEX
      region: us-east1
      zone: us-east1-b
      capacity_gib: 4096
      total_throughput_mibps: 256
      total_iops: 4096
      allow_auto_tiering: true
      hot_tier_size_gib: 1024
      enable_hot_tier_auto_resize: true
      labels:
        owner: bob
```

#### Flex Unified large capacity (SCALEOUT)

```yaml
  - id: flex_pool_large
    source: modules/file-system/netapp-storage-pool
    use: [network, private_service_access]
    settings:
      pool_name: "flex-pool-large"
      service_level: FLEX
      region: us-east1
      zone: us-east1-b
      scale_type: SCALE_TYPE_SCALEOUT
      capacity_gib: 12288
      total_throughput_mibps: 1024
      total_iops: 16384
      allow_auto_tiering: true
      hot_tier_size_gib: 2048
      labels:
        owner: bob
```

### NetApp Volumes quota

Your project must have unused quota for NetApp Volumes in the region you will
provision the storage pool. This can be found by browsing to the [Quota tab within IAM & Admin](https://console.cloud.google.com/iam-admin/quotas) in the Cloud Console.
Please note that there are separate quota limits for Standard, Premium/Extreme, and Flex Unified service levels.

See also NetApp Volumes [default quotas](https://cloud.google.com/netapp/volumes/docs/quotas#netapp-volumes-default-quotas).

[service-levels]: https://cloud.google.com/netapp/volumes/docs/discover/service-levels
[auto-tiering]: https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/manage-auto-tiering
[pre-existing]: ../pre-existing-network-storage/README.md
[matrix]: ../../../docs/network_storage.md#compatibility-matrix

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2026 Google LLC

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
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.45.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.45.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_netapp_storage_pool.netapp_storage_pool](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/netapp_storage_pool) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [google_compute_network_peering.private_peering](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_network_peering) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_active_directory_policy"></a> [active\_directory\_policy](#input\_active\_directory\_policy) | The ID of the Active Directory policy to apply to the storage pool in the format:<br/>`projects/<project_id>/locations/<location>/activeDirectories/<name>` | `string` | `null` | no |
| <a name="input_allow_auto_tiering"></a> [allow\_auto\_tiering](#input\_allow\_auto\_tiering) | Whether to allow automatic tiering for the storage pool. | `bool` | `false` | no |
| <a name="input_capacity_gib"></a> [capacity\_gib](#input\_capacity\_gib) | The capacity of the storage pool in GiB. Minimum is 2048 GiB for STANDARD, PREMIUM, and EXTREME; 1024 GiB for Flex Unified; 6144 GiB for large capacity (SCALE\_TYPE\_SCALEOUT) pools. | `number` | `2048` | no |
| <a name="input_cmek_policy"></a> [cmek\_policy](#input\_cmek\_policy) | The ID of the Customer Managed Encryption Key (CMEK) policy to apply to the storage pool in the format:<br/>`projects/<project>/locations/<location>/kmsConfigs/<name>` | `string` | `null` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the deployment, used as name of the NetApp storage pool if no name is specified. | `string` | n/a | yes |
| <a name="input_description"></a> [description](#input\_description) | A description of the NetApp storage pool. | `string` | `""` | no |
| <a name="input_enable_hot_tier_auto_resize"></a> [enable\_hot\_tier\_auto\_resize](#input\_enable\_hot\_tier\_auto\_resize) | Whether hot-tier threshold will auto-increase when it reaches 100%. Flex-only. Requires allow\_auto\_tiering to be true. | `bool` | `null` | no |
| <a name="input_hot_tier_size_gib"></a> [hot\_tier\_size\_gib](#input\_hot\_tier\_size\_gib) | Total hot tier capacity for the storage pool in GiB. Flex-only. Requires allow\_auto\_tiering to be true. | `number` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the NetApp storage pool. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_ldap_enabled"></a> [ldap\_enabled](#input\_ldap\_enabled) | Whether to enable LDAP for the storage pool. | `bool` | `false` | no |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the NetApp storage pool is connected given in the format:<br/>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | Network self-link the pool will be on, required for checking private service access | `string` | n/a | yes |
| <a name="input_pool_name"></a> [pool\_name](#input\_pool\_name) | The name of the storage pool. Leave empty to generate name based on deployment name. | `string` | `null` | no |
| <a name="input_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#input\_private\_vpc\_connection\_peering) | The name of the private VPC connection peering. | `string` | `"sn-netapp-prod"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which the NetApp storage pool will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region for the storage pool. Required for all service levels. | `string` | n/a | yes |
| <a name="input_replica_zone"></a> [replica\_zone](#input\_replica\_zone) | Replica zone for regional Flex Unified pools. Must be within region when service\_level is FLEX. Omit for zonal Flex Unified pools. Ignored for STANDARD, PREMIUM, and EXTREME pools. | `string` | `null` | no |
| <a name="input_scale_type"></a> [scale\_type](#input\_scale\_type) | Scale type of the storage pool. Flex-only. Use SCALE\_TYPE\_SCALEOUT for large capacity pools. | `string` | `null` | no |
| <a name="input_service_level"></a> [service\_level](#input\_service\_level) | The service level of the storage pool. | `string` | `"PREMIUM"` | no |
| <a name="input_total_iops"></a> [total\_iops](#input\_total\_iops) | Total pool IOPS for Flex Unified custom performance. Omit to let Google Cloud calculate IOPS from total\_throughput\_mibps. | `number` | `null` | no |
| <a name="input_total_throughput_mibps"></a> [total\_throughput\_mibps](#input\_total\_throughput\_mibps) | Total pool throughput in MiB/s for Flex Unified custom performance. Omit to use the default 64 MiB/s. | `number` | `null` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone for zonal Flex Unified pools, or active zone for regional Flex Unified pools. Must be within region when service\_level is FLEX. Ignored for STANDARD, PREMIUM, and EXTREME pools. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_allow_auto_tiering"></a> [allow\_auto\_tiering](#output\_allow\_auto\_tiering) | Whether the storage pool supports auto-tiering enabled volumes. |
| <a name="output_capacity_gb"></a> [capacity\_gb](#output\_capacity\_gb) | Storage pool capacity in GiB. |
| <a name="output_mode"></a> [mode](#output\_mode) | Storage pool mode. Flex-only. |
| <a name="output_netapp_storage_pool_id"></a> [netapp\_storage\_pool\_id](#output\_netapp\_storage\_pool\_id) | An identifier for the resource with format `projects/{{project}}/locations/{{location}}/storagePools/{{name}}` |
| <a name="output_scale_type"></a> [scale\_type](#output\_scale\_type) | Scale type of the storage pool. Flex-only. |
| <a name="output_service_level"></a> [service\_level](#output\_service\_level) | Storage pool service level. |
| <a name="output_type"></a> [type](#output\_type) | Storage pool type. Flex Unified pools use UNIFIED. Omitted for STANDARD, PREMIUM, and EXTREME pools. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
