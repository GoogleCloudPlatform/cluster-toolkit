## Description

This module creates a [Google Cloud NetApp Volumes](https://cloud.google.com/netapp/volumes/docs/discover/overview) volume.

NetApp Volumes is a first-party Google service that provides NFS shared file systems to VMs. It offers advanced data management capabilities and highly scalable capacity and performance.
NetApp Volume provides:

- support for NFSv3 and NFSv4.1
- a [rich feature set](https://cloud.google.com/netapp/volumes/docs/discover/service-levels)
- scalable [performance](https://cloud.google.com/netapp/volumes/docs/performance/performance-benchmarks)
- [FlexCache](https://docs.cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/cache-ontap-volumes/overview): Caching of ONTAP-based volumes to provide high-throughput and low latency read access to compute clusters of on-premises data
- [Auto-tiering](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/manage-auto-tiering) of unused data to optimize cost

Support for NetApp Volumes is split into two modules.

- **netapp-storage-pool** provisions a [storage pool](https://cloud.google.com/netapp/volumes/docs/configure-and-use/storage-pools/overview). Storage pools are pre-provisioned storage capacity containers which host volumes. A pool also defines fundamental properties of all the volumes within, like the region, the attached network, the [service level](https://cloud.google.com/netapp/volumes/docs/discover/service-levels), CMEK encryption, Active Directory and LDAP settings.
- **netapp-volume** provisions a [volume](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/overview) inside an existing storage pool. A volume is a file system container shared using NFSv3 or NFSv4.1.

For more information on this and other network storage options in the Cluster
Toolkit, see the extended [Network Storage documentation](../../../docs/network_storage.md).

## Deletion policy

By default, `gcluster destroy` deletes volumes created by this module and all data is lost. Set `deletion_policy` in your blueprint to control destroy behavior:

1. **Omit or `null`** — Provider default. Terraform deletes the volume in Google Cloud.
2. **`DEFAULT` or `DELETE`** — Delete the volume in Google Cloud.
3. **`FORCE`** — Delete the volume even when nested snapshot resources exist.
4. **`PREVENT`** — Block Terraform from deleting the volume. Use this to protect production data from accidental `gcluster destroy`.
5. **`ABANDON`** — Remove the volume from Terraform state without deleting it in Google Cloud. The volume remains available for manual management or mounting through [pre-existing-network-storage](../pre-existing-network-storage/README.md).

Example:

```yaml
  - id: homefs
    source: modules/file-system/netapp-volume
    use: [netapp_pool]
    settings:
      volume_name: "homefs"
      capacity_gib: 1024
      local_mount: "/home"
      protocols: ["NFSV3"]
      deletion_policy: "PREVENT"
```

Storage pools can only be deleted when empty. This module does not expose `deletion_policy` for pools.

## Volumes overview

Volumes are file system containers shared using NFSv3 or NFSv4.1. Volumes live inside [storage pools](https://cloud.google.com/netapp/volumes/docs/configure-and-use/storage-pools/overview), which can be provisioned using the [netapp-storage-pool](../netapp-storage-pool/README.md) module. Volumes inherit settings from the pool and consume capacity from the pool.

### Inherited settings

When you use `use: [netapp_pool]`, the volume module inherits:

1. `netapp_storage_pool_id`
2. `service_level`
3. `type`
4. `allow_auto_tiering`
5. `scale_type`

### Volume location

The module sets volume location from the `locations/<location>` segment of `netapp_storage_pool_id`. When you use `use: [netapp_pool]`, Cluster Toolkit wires `netapp_storage_pool_id` from the pool automatically. Do not set location, region, or zone in the volume blueprint.

1. Zonal pool — location is the zone (for example, `us-east1-b`).
2. Regional pool — location is the region (for example, `us-east1`).

### Volume naming

Volume name rules depend on the storage pool service level. The module validates names at plan time.

1. **FLEX pools** — Use lowercase letters, numbers, and underscores only. The name must start with a lowercase letter and cannot end with an underscore. Hyphens are not allowed.
2. **STANDARD, PREMIUM, and EXTREME pools** — Hyphens are allowed. Underscores are not allowed.

## Volume examples

The following examples show the use of netapp-volume. They build on top of a storage pool which can be provisioned using the [netapp-storage-pool](../netapp-storage-pool/README.md) module.

### Example with minimal parameters

```yaml
  - id: home_volume
    source: modules/file-system/netapp-volume
    use: [netapp_pool]  # Create this pool using the netapp-storage-pool module
    settings:
      volume_name: "eda-home"
      capacity_gib: 1024               # Size up to available capacity in the pool
      local_mount: "/eda-home"         # Mount point at client when client uses USE directive
      protocols: ["NFSV3"]
    # Default export policy exports to "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16" and no_root_squash
    # netapp_storage_pool_id and service_level are inherited from netapp_pool
```

### Example with all parameters

```yaml
  - id: shared_volume
    source: modules/file-system/netapp-volume
    use: [netapp_pool]              # Create this pool using the netapp-storage-pool module
    settings:
      volume_name: "eda-shared"
      capacity_gib: 25000           # At least 15360 GiB (15 TiB) when large_capacity is true
      large_capacity: true
      local_mount: "/shared"        # Mount point at client when client uses USE directive
      mount_options: "rw"           # Allows customizing mount options for special workloads
      protocols: ["NFSV3","NFSV4"]  # List of protocols. ["NFSV3"], ["NFSV4"], or ["NFSV3", "NFSV4"]
      unix_permissions: "0770"      # Default permissions for root inode; supported on Flex Unified volumes
      # If no export policy is specified, a permissive default policy will be applied, which is:
      #  allowed_clients = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16" # RFC1918
      #  has_root_access = true      # no_root_squash enabled
      #  access_type = "READ_WRITE"
      export_policy_rules:
      - allowed_clients: "10.10.20.8,10.10.20.9"
        has_root_access: true       # no_root_squash enabled
        access_type: "READ_WRITE"
        nfsv3: false                # allow only NFSv4 for these hosts
        nfsv4: true
      - allowed_clients: "10.0.0.0/8"
        has_root_access: false      # no_root_squash disabled
        access_type: "READ_WRITE"      
        nfsv3: true                 # allow only NFSv3 for these hosts
        nfsv4: false
      tiering_policy:               # Enable auto-tiering. Requires auto-tiering enabled storage pool
        tier_action: "ENABLED"
        cooling_threshold_days: 31  # tier data blocks which have not been touched for 31 days
      deletion_policy: "DEFAULT"

      description: "Shared volume for EDA job"
      labels:
        owner: bob
```

### Example: Flex Unified large capacity volume

Requires a zonal pool with `scale_type: SCALE_TYPE_SCALEOUT`. Do not set `large_capacity` on Flex Unified volumes.

```yaml
  - id: flex_large_volume
    source: modules/file-system/netapp-volume
    use: [flex_pool_large]
    settings:
      volume_name: "flex_large_home"
      capacity_gib: 12288          # Minimum 4800 GiB for Flex large capacity
      local_mount: "/home"
      protocols: ["NFSV3"]
      large_capacity_config:
        constituent_count: 48      # Typical value for SCALE_TYPE_SCALEOUT pools
      tiering_policy:              # Requires allow_auto_tiering on the pool
        tier_action: "ENABLED"
        cooling_threshold_days: 31
        hot_tier_bypass_mode_enabled: false  # Set true only during data migration; disable after
```

## Protocol support

This module supports NFSv3 and NFSv4.1 only. SMB and iSCSI are not supported.

## ONTAP mode

This module provisions and manages volumes through Google Cloud APIs in Default mode only. It does not support volumes in ONTAP-mode pools.

If you need ONTAP-mode pools or volumes, provision them outside Cluster Toolkit and integrate them with the [pre-existing-network-storage](../pre-existing-network-storage/README.md) module. In ONTAP mode, volumes, snapshots, and policies are managed with ONTAP tools after the pool is created. See [Manage ONTAP mode](https://cloud.google.com/netapp/volumes/docs/ontap/manage-ontap).

## Large volumes

Standard, Premium, and Extreme service levels support [large capacity volumes](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/overview#large-capacity-volumes) of 15 TiB or larger. Flex Unified large capacity volumes use a separate configuration model.

### Minimum volume capacity

The module enforces these minimum `capacity_gib` values at plan time:

1. **STANDARD, PREMIUM, and EXTREME (regular volumes)** — 100 GiB
2. **STANDARD, PREMIUM, and EXTREME (large capacity volumes)** — 15 TiB (15360 GiB)
3. **Flex Unified** — 1 GiB
4. **Flex Unified large capacity (**`large_capacity_config`**)** — 4800 GiB

To create a large capacity volume:

1. **Standard, Premium, and Extreme** — Set `large_capacity: true` and `capacity_gib` to at least 15360.
2. **Flex Unified** — Use the `large_capacity_config` block with `constituent_count` in the blueprint. Requires a zonal pool with `scale_type: SCALE_TYPE_SCALEOUT`. Large capacity pools cannot be regional. The typical value is 48. Do not set `large_capacity` on Flex Unified volumes.

### Large capacity rules by service level

1. **Standard, Premium, and Extreme** — Set `large_capacity: true`. Set `capacity_gib` to at least 15360 (15 TiB). The module configures multiple NFS endpoints internally. Do not set `large_capacity_config`.
2. **Flex Unified** — Set `large_capacity_config` with `constituent_count`. Set `capacity_gib` to at least 4800. Do not set `large_capacity`. Requires a zonal pool with `scale_type: SCALE_TYPE_SCALEOUT`.
3. `large_capacity` **and** `large_capacity_config` **cannot be used together.**

### Mounting large capacity volumes

Large capacity volumes expose the same NFS export through multiple IP addresses . For best performance, spread clients evenly across all available IPs so load is distributed and aggregated throughput is higher. See [Connect large capacity volumes with multiple storage endpoints](https://cloud.google.com/netapp/volumes/docs/connect-clients/connect-large-capacity-volumes).

When you attach a volume with the `use:` directive, the module mount runner passes all `server_ips` to the mount script. Each client selects one endpoint from that list at mount time, which spreads clients across the available IPs.

Slurm integrations do not distribute clients across endpoints today. Slurm mounts use the first IP address only, so large capacity volumes do not get the full multi-endpoint performance benefit in Slurm clusters.

For volumes mounted through [pre-existing-network-storage](../pre-existing-network-storage/README.md), you must configure endpoints yourself (for example, round-robin DNS or manual client grouping per the Google Cloud documentation).

### Plan-time validation

The module enforces these rules at plan time:

1. Flex File pools (`service_level: FLEX` with `type: FILE`) are rejected.
2. `large_capacity` is rejected on Flex Unified pools; use `large_capacity_config` instead.
3. `large_capacity_config` requires a zonal pool with `scale_type: SCALE_TYPE_SCALEOUT`.
4. `tiering_policy` requires a pool with `allow_auto_tiering: true`.
5. `hot_tier_bypass_mode_enabled` is supported only when `service_level` is `FLEX`.

## Auto-tiering support

For storage pools with `allow_auto_tiering` enabled, you can enable auto-tiering on the volume using `tiering_policy`. For more information, see [Manage auto-tiering](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/manage-auto-tiering).

### Hot tier bypass mode

Set `hot_tier_bypass_mode_enabled` only for FLEX service levels. Use it when you are migrating data into a new volume.

When hot tier bypass mode is enabled, writes go directly to the cold tier instead of filling the hot tier. Frequently accessed data is promoted back to the hot tier based on client read activity.

1. Enable `hot_tier_bypass_mode_enabled` before you migrate data into the volume.
2. Disable `hot_tier_bypass_mode_enabled` after migration completes so normal tiering behavior resumes for ongoing workloads.

## Using existing volumes not created by Cluster Toolkit

NetApp Volumes volumes are regular NFS exports. Use the [pre-existing-network-storage](../pre-existing-network-storage/README.md) module to integrate volumes that Cluster Toolkit does not provision. This includes ONTAP-mode pools and volumes, FlexCache volumes, and any other NetApp export created outside these modules.

Example code:

```yaml
- id: homefs
  source: modules/file-system/pre-existing-network-storage
  settings:
    server_ip: ## Set server IP here ##
    remote_mount: nfsshare
    local_mount: /shared
    fs_type: nfs
```

This creates a resource in Cluster Toolkit which references the specified NFS export, which will be mounted at `/shared` by clients which mount if via USE directive.

Note that the `server_ip` must be known before deployment and this module does not allow to specify a list of IPs for large volumes. For large volumes it is recommended to use a DNS FQDN which hands out the volume IPs in round-robin fashion.

## FlexCache support

NetApp FlexCache technology accelerates data access, reduces WAN latency and lowers WAN bandwidth costs for read-intensive workloads, especially where clients need to access the same data repeatedly. When you create a FlexCache volume, you create a remote cache of an already existing (origin) volume that contains only the actively accessed data (hot data) of the origin volume.

The FlexCache support in Google Cloud NetApp Volumes allows you to provision a cache volume in your Google network to improve performance for hybrid cloud environments. A FlexCache volume can help you transition workloads to the hybrid cloud by caching data from an on-premises data center to cloud.

Deploying FlexCache volumes requires manual steps on the ONTAP origin side, which are not automated. Therefore this module has no support to deploy FlexCache volumes today. Deploy them manually and use the [pre-existing-network-storage](#using-existing-volumes-not-created-by-cluster-toolkit) instead.

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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.34.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.34.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_netapp_volume.netapp_volume](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/netapp_volume) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_allow_auto_tiering"></a> [allow\_auto\_tiering](#input\_allow\_auto\_tiering) | Whether the storage pool supports auto-tiering. Inherited from the pool when using use: [netapp\_pool]. | `bool` | `null` | no |
| <a name="input_capacity_gib"></a> [capacity\_gib](#input\_capacity\_gib) | The capacity of the volume in GiB. Minimum is 100 GiB for STANDARD, PREMIUM, and EXTREME; 15 TiB (15360 GiB) for STANDARD, PREMIUM, and EXTREME large capacity volumes; 1 GiB for Flex Unified; 4800 GiB for Flex Unified large capacity volumes. | `number` | `1024` | no |
| <a name="input_deletion_policy"></a> [deletion\_policy](#input\_deletion\_policy) | Controls Terraform destroy behavior. Omit to use the provider default (delete the volume in Google Cloud).<br/>DEFAULT or DELETE: delete the volume in Google Cloud.<br/>FORCE: delete the volume even when nested snapshot resources exist.<br/>PREVENT: block Terraform from deleting the volume.<br/>ABANDON: remove the volume from Terraform state without deleting it in Google Cloud. | `string` | `null` | no |
| <a name="input_description"></a> [description](#input\_description) | A description of the NetApp volume. | `string` | `""` | no |
| <a name="input_export_policy_rules"></a> [export\_policy\_rules](#input\_export\_policy\_rules) | Define NFS export policy. | <pre>list(object({<br/>    allowed_clients = optional(string)<br/>    has_root_access = optional(bool, false)<br/>    access_type     = optional(string, "READ_WRITE")<br/>    nfsv3           = optional(bool)<br/>    nfsv4           = optional(bool)<br/>  }))</pre> | <pre>[<br/>  {<br/>    "access_type": "READ_WRITE",<br/>    "allowed_clients": "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16",<br/>    "has_root_access": true<br/>  }<br/>]</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the NetApp volume. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_large_capacity"></a> [large\_capacity](#input\_large\_capacity) | If true, the volume will be created with large capacity for STANDARD/PREMIUM/EXTREME service levels.<br/>For FLEX service level, use large\_capacity\_config instead. | `bool` | `false` | no |
| <a name="input_large_capacity_config"></a> [large\_capacity\_config](#input\_large\_capacity\_config) | Configuration for a Flex Unified large capacity volume. Supported only for Flex Unified pools.<br/>Set constituent\_count in the blueprint. The typical value for current SCALE\_TYPE\_SCALEOUT pools is 48. | <pre>object({<br/>    constituent_count = number<br/>  })</pre> | `null` | no |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Mountpoint for this volume. | `string` | `"/shared"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | NFS mount options to mount file system. | `string` | `"rw,hard,rsize=262144,wsize=262144,tcp"` | no |
| <a name="input_netapp_storage_pool_id"></a> [netapp\_storage\_pool\_id](#input\_netapp\_storage\_pool\_id) | The ID of the NetApp storage pool to use for the volume. Volume location (region or zone) is parsed from this value.<br/>Inherited from the pool when using use: [netapp\_pool]. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which the NetApp volume will be created. | `string` | n/a | yes |
| <a name="input_protocols"></a> [protocols](#input\_protocols) | Access protocols for the volume. Only NFSv3 and NFSv4.1 (NFSV4) are supported. | `list(string)` | <pre>[<br/>  "NFSV3"<br/>]</pre> | no |
| <a name="input_scale_type"></a> [scale\_type](#input\_scale\_type) | Scale type of the storage pool. Inherited from the pool when using use: [netapp\_pool]. Flex-only; null for STANDARD, PREMIUM, and EXTREME pools. | `string` | `null` | no |
| <a name="input_service_level"></a> [service\_level](#input\_service\_level) | Service level of the storage pool used by this volume. Inherited from the pool when using use: [netapp\_pool]. | `string` | `null` | no |
| <a name="input_tiering_policy"></a> [tiering\_policy](#input\_tiering\_policy) | Define the tiering policy for the NetApp volume. Requires a pool with allow\_auto\_tiering enabled.<br/>hot\_tier\_bypass\_mode\_enabled (FLEX only): use during data migration so writes go to the cold tier instead of filling the hot tier; disable after migration completes. | <pre>object({<br/>    tier_action                  = optional(string)<br/>    cooling_threshold_days       = optional(number)<br/>    hot_tier_bypass_mode_enabled = optional(bool)<br/>  })</pre> | `null` | no |
| <a name="input_type"></a> [type](#input\_type) | Type of the storage pool used by this volume. Inherited from the pool when using use: [netapp\_pool]. Flex Unified pools use UNIFIED. Null for STANDARD, PREMIUM, and EXTREME pools. | `string` | `null` | no |
| <a name="input_unix_permissions"></a> [unix\_permissions](#input\_unix\_permissions) | UNIX permissions for root inode in the volume. | `string` | `"0770"` | no |
| <a name="input_volume_name"></a> [volume\_name](#input\_volume\_name) | The name of the volume. Needs to be unique within the storage pool.<br/>FLEX pools: lowercase letters, numbers, and underscores only; must start with a lowercase letter and cannot end with an underscore.<br/>STANDARD, PREMIUM, and EXTREME pools: hyphens are allowed; underscores are not allowed. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_capacity_gb"></a> [capacity\_gb](#output\_capacity\_gb) | Volume capacity in GiB. |
| <a name="output_install_nfs_client"></a> [install\_nfs\_client](#output\_install\_nfs\_client) | Script for installing NFS client |
| <a name="output_install_nfs_client_runner"></a> [install\_nfs\_client\_runner](#output\_install\_nfs\_client\_runner) | Runner to install NFS client using the startup-script module |
| <a name="output_mount_runner"></a> [mount\_runner](#output\_mount\_runner) | Runner to mount the file-system using an ansible playbook. The startup-script<br/>module will automatically handle installation of ansible.<br/>- id: example-startup-script<br/>  source: modules/scripts/startup-script<br/>  settings:<br/>    runners:<br/>    - $(your-fs-id.mount\_runner)<br/>... |
| <a name="output_netapp_volume_id"></a> [netapp\_volume\_id](#output\_netapp\_volume\_id) | An identifier for the resource with format `projects/{{project}}/locations/{{location}}/volumes/{{name}}` |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a NetApp Volumes volume. |
| <a name="output_server_ips"></a> [server\_ips](#output\_server\_ips) | List of IP addresses of the volume. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
