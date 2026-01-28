## Description

This module creates a [Google Cloud NetApp Volumes](https://cloud.google.com/netapp/volumes/docs/discover/overview)
volume.

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

## Deletion protection
The netapp-volume module currently doesn't implement volume deletion protection. If you create a volume with Cluster Toolkit by using this module, Cluster Toolkit will also delete it when you run `gcluster destroy`. All the data in the volume will be gone. If you want to retain the volume instead, it is advised to [use existing volumes not created by Cluster Toolkit](#using-existing-volumes-not-created-by-cluster-toolkit).

## Volumes overview
Volumes are filesystem containers which can be shared using NFS or SMB filesharing protocols. Volumes *live* inside of [storage pools](https://cloud.google.com/netapp/volumes/docs/configure-and-use/storage-pools/overview), which can be provisioned using the [netapp-storage-pool] module. Volumes inherit fundamental settings from the pool. They *consume* capacity provided by the pool. You can create one or multiple volumes *inside* a pool.

[netapp-storage-pool]: ../netapp-storage-pool/README.md
[service-levels]: https://cloud.google.com/netapp/volumes/docs/discover/service-levels
[auto-tiering]: https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/manage-auto-tiering
[pre-existing]: ../pre-existing-network-storage/README.md
[matrix]: ../../../docs/network_storage.md#compatibility-matrix

## Volume examples
The following examples show the use of netapp-volume. They builds on top of an storage pool which can be provisioned using the [netapp-storage-pool][netapp-storage-pool] module.

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
      region: $(vars.region)
    # Default export policy exports to "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16" and no_root_squash
```

### Example with all parameters

```yaml
  - id: shared_volume
    source: modules/file-system/netapp-volume
    use: [netapp_pool]              # Create this pool using the netapp-storage-pool module
    settings:
      volume_name: "eda-shared"
      capacity_gib: 25000           # Size up to available capacity in the pool
      large_capacity: true
      local_mount: "/shared"        # Mount point at client when client uses USE directive
      mount_options: "rw"           # Allows customizing mount options for special workloads
      protocols: ["NFSV3","NFSV4"]  # List of protocols. ["NFSV3], ["NFSv4] or ["NFSV3, "NFSV4"]
      region: $(vars.region)
      unix_permissions: "0777"      # Specify default permissions for roo inode owned by root:root
      # If no export policy is specified, a permissive default policy will be applied, which is:
      #  allowed_clients = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16" # RFC1918
      #  has_root_access = true      # no_root_squash enabled
      #  access_type = "READ_WRITE"
      export_policy:
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

      description: "Shared volume for EDA job"
      labels:
        owner: bob
```

## Protocol support
Since Cluster Toolkit is currently built to provision Linux-based compute clusters, this module supports NFSv3 and NFSv4.1 only. SMB is blocked.

## Large volumes
Volumes larger than 15 TiB can be created as [Large Volumes](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/overview#large-capacity-volumes). Such volumes can grow up to 3 PiB and can scale read performance up to 29 GiBps. They provide six IP addresses to the volume. They are exported via the `server_ips` output. When connecting a large volume to a client using the USE directive, cluster toolkit currently uses the first IP only. This will be improved in the future.

This feature is allow-listed GA. To request allow-listing, see [Large Volumes](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/overview#large-capacity-volumes).

## Auto-tiering support
For auto-tiering enabled storage pools you can enable auto-tiering on the volume. For more information, see [manage auto-tiering](https://cloud.google.com/netapp/volumes/docs/configure-and-use/volumes/manage-auto-tiering).

## Using existing volumes not created by Cluster Toolkit
NetApp Volumes volumes are regular NFS exports. You can use the [pre-existing-network-storage] module to integrate them into Cluster Toolkit.

Example code:

```yaml
- id: homefs
  source: modules/file-system/pre-existing-network-storage
  settings:
    server_ip: ## Set server IP here ##
    remote_mount: nfsshare
    local_mount: /home
    fs_type: nfs
```

This creates a resource in Cluster Toolkit which references the specified NFS export, which will be mounted at `/home` by clients which mount if via USE directive.

Note that the `server_ip` must be known before deployment and this module does not allow
to specify a list of IPs for large volumes.

[pre-existing-network-storage]: ../pre-existing-network-storage/README.md

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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.5.7 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.45.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.45.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_netapp_volume.netapp_volume](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/netapp_volume) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_capacity_gib"></a> [capacity\_gib](#input\_capacity\_gib) | The capacity of the volume in GiB. | `number` | `1024` | no |
| <a name="input_description"></a> [description](#input\_description) | A description of the NetApp volume. | `string` | `""` | no |
| <a name="input_export_policy_rules"></a> [export\_policy\_rules](#input\_export\_policy\_rules) | Define NFS export policy. | <pre>list(object({<br/>    allowed_clients = optional(string)<br/>    has_root_access = optional(bool, false)<br/>    access_type     = optional(string, "READ_WRITE")<br/>    nfsv3           = optional(bool)<br/>    nfsv4           = optional(bool)<br/>  }))</pre> | <pre>[<br/>  {<br/>    "access_type": "READ_WRITE",<br/>    "allowed_clients": "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16",<br/>    "has_root_access": true<br/>  }<br/>]</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the NetApp volume. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_large_capacity"></a> [large\_capacity](#input\_large\_capacity) | If true, the volume will be created with large capacity.<br/>Large capacity volumes have 6 IP addresses and a minimal size of 15 TiB. | `bool` | `false` | no |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Mountpoint for this volume. | `string` | `"/shared"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | NFS mount options to mount file system. | `string` | `"rw,hard,rsize=65536,wsize=65536,tcp"` | no |
| <a name="input_netapp_storage_pool_id"></a> [netapp\_storage\_pool\_id](#input\_netapp\_storage\_pool\_id) | The ID of the NetApp storage pool to use for the volume. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which the NetApp storage pool will be created. | `string` | n/a | yes |
| <a name="input_protocols"></a> [protocols](#input\_protocols) | The protocols that the volume supports. Currently, only NFSv3 and NFSv4 is supported. | `list(string)` | <pre>[<br/>  "NFSV3"<br/>]</pre> | no |
| <a name="input_region"></a> [region](#input\_region) | Location for NetApp storage pool. | `string` | n/a | yes |
| <a name="input_tiering_policy"></a> [tiering\_policy](#input\_tiering\_policy) | Define the tiering policy for the NetApp volume. | <pre>object({<br/>    tier_action            = optional(string)<br/>    cooling_threshold_days = optional(number)<br/>  })</pre> | `null` | no |
| <a name="input_unix_permissions"></a> [unix\_permissions](#input\_unix\_permissions) | UNIX permissions for root inode in the volume. | `string` | `"0777"` | no |
| <a name="input_volume_name"></a> [volume\_name](#input\_volume\_name) | The name of the volume. Needs to be unique within the storage pool. | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_capacity_gb"></a> [capacity\_gb](#output\_capacity\_gb) | Volume capacity in GiB. |
| <a name="output_install_nfs_client"></a> [install\_nfs\_client](#output\_install\_nfs\_client) | Script for installing NFS client |
| <a name="output_install_nfs_client_runner"></a> [install\_nfs\_client\_runner](#output\_install\_nfs\_client\_runner) | Runner to install NFS client using the startup-script module |
| <a name="output_mount_runner"></a> [mount\_runner](#output\_mount\_runner) | Runner to mount the file-system using an ansible playbook. The startup-script<br/>module will automatically handle installation of ansible.<br/>- id: example-startup-script<br/>  source: modules/scripts/startup-script<br/>  settings:<br/>    runners:<br/>    - $(your-fs-id.mount\_runner)<br/>... |
| <a name="output_netapp_volume_id"></a> [netapp\_volume\_id](#output\_netapp\_volume\_id) | An identifier for the resource with format `projects/{{project}}/locations/{{location}}/volumes/{{name}}` |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a NetApp Volumes volume. |
| <a name="output_server_ips"></a> [server\_ips](#output\_server\_ips) | List of IP addresses of the volume. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
