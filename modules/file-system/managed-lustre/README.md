## Description

This module creates a [Managed Lustre](https://cloud.google.com/managed-lustre)
instance. Managed Lustre is a high performance network file system that can be
mounted to one or more VMs.

For more information on this and other network storage options in the Cluster
Toolkit, see the extended [Network Storage documentation](../../../docs/network_storage.md).

### Supported Operating Systems

A Managed Lustre instance can be used with Slurm cluster or compute
VM running Ubuntu 22.04 or Rocky Linux 8 (including the HPC flavor).

### Managed Lustre Access

Managed Lustre must be enabled for your project by Google staff. Please contact
your sales representative for further steps.

### Example - New VPC

For Managed Lustre instance, the snippet below creates new VPC and configures
private-service-access for this newly created network.  Both items are required
to be passed to the Lustre module to ensure that they're built in order and
that the correct subnetwork has private service access.

```yaml
 - id: network
    source: modules/network/vpc

  - id: private_service_access
    source: modules/network/private-service-access
    use: [network]
    settings:
      prefix_length: 22

  - id: lustre
    source: modules/file-system/managed-lustre
    use: [network, private_service_access]
```

### Example - Slurm

When using Slurm you must take into consideration whether or not you are using
an official image from the `advanced-compute-images` project or building your own.
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
this Ansible playbook to correctly install the Lustre drivers into the image
(for Slurm-GCP versions greater than 6.10.0):

```yaml
- type: data
  destination: /var/tmp/slurm_vars.json
  content: |
    {
      "reboot": false,
      "install_cuda": false,
      "install_gcsfuse": true,
      "install_lustre": false,
      "install_managed_lustre": true,
      "install_nvidia_repo": true,
      "install_ompi": true,
      "allow_kernel_upgrades": false,
      "monitoring_agent": "cloud-ops",
    }
```

The `install_managed_lustre: true` line specifies that slurm-gcp should install
the correct modules within the slurm image.  This runner should be placed
ahead of the script that calls the ansible build of the slurm-gcp image.

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

### Example - GKE compatibility

By default the Managed Lustre instance that is deployed is not compatible with
GKE.  To enable the compatibility use the `gke_support_enabled: true` option.
This creates a file `/etc/modprobe/lnet.conf` that changes the listening port
to 6988.

```yaml
  - id: managed-lustre
    source: modules/file-system/managed-lustre
    use: [network, private_service_access]
    settings:
      name: lustre-instance
      local_mount: /lustre
      remote_mount: lustrefs
      size_gib: 18000
      gke_support_enabled: true
```

> [!WARNING]
>
> 1. VMs cannot connect to both GKE compatible and GKE incompatible lustre
> instances at the same time as they connect to different ports.  Lustre can
> only listen to one port at a time.
>
> 2. Setting `gke_support_enabled: true` will not affect Slurm nodes, GKE
> compatibility must be built into the Slurm image.
>
> 3. **Static IP Constraint & GKE CSI Support:** Managed Lustre instances do not guarantee static IP allocations. If the Lustre instance is recreated (due to scaling, location moves, etc.), its IP will change. Because GKE CSI PersistentVolume configurations are immutable, an IP change will cause GKE mount failures. You must manually delete and recreate GKE PersistentVolume configurations in such events.

### Example - Dynamic Tier

To deploy a Managed Lustre instance with Dynamic Tier enabled, you must use a **pre-existing VPC**. This is because the `DynamicTierCapacity` quota is scoped to a specific VPC network name, and you must request this quota before running the deployment.

For the Dynamic Tier, the minimum capacity is **472,000 GiB**.

```yaml
  - id: network
    source: modules/network/pre-existing-vpc
    settings:
      network_name: <existing_network_name>
      subnetwork_name: <existing_subnetwork_name>

  - id: private_service_access
    source: modules/network/private-service-access
    use: [network]
    settings:
      prefix_length: 22

  - id: lustre
    source: modules/file-system/managed-lustre
    use: [network, private_service_access]
    settings:
      name: lustre-instance
      local_mount: /lustre
      remote_mount: lustrefs
      size_gib: 472000 # Minimum size for Dynamic Tier
      enable_dynamic_tier: true
```

> [!NOTE]
> If your pre-existing VPC already has Private Service Access (PSA) configured, you can omit the `private-service-access` module from the blueprint and instead define the peering connection name manually in the `lustre` module settings:
>
> ```yaml
>       private_vpc_connection_peering: <peering_connection_name> # e.g. "servicenetworking.googleapis.com"
> ```

<p>

> [!IMPORTANT]
> **Dynamic Tier Prerequisites & Constraints:**
>
> 1. **VPC-Scoped Quota:** The Dynamic Tier requires requesting `DynamicTierCapacity` quota. This quota is scoped to a specific project, zone, and VPC name. Because of this, dynamically created VPCs are not recommended as you cannot request quota before the VPC is created.
>    * Refer to the official guide on how to [Request Additional Storage Capacity Quota](https://cloud.google.com/managed-lustre/docs/quotas#request_additional_storage_capacity_quota).
>    * You can request a quota increase using the following `gcloud` command:
>
>      ```bash
>      gcloud beta quotas preferences create \
>          --service=lustre.googleapis.com \
>          --project=YOUR_PROJECT_ID \
>          --quota-id=DynamicTierCapacity \
>          --preferred-value=PREFERRED_VALUE_IN_GIB \
>          --dimensions=zone=ZONE,network_name=VPC_NETWORK_NAME \
>          --justification="Requesting Dynamic Tier capacity for Cluster Toolkit deployment"
>      ```
>
> 2. **Minimum Size:** The minimum capacity is **472,000 GiB** (and must be in multiples of 472,000 GiB).

### Example - CMEK

To create a Managed Lustre instance with a Customer-Managed Encryption Key (CMEK), use the `kms_key` option.

```yaml
  - id: managed-lustre
    source: modules/file-system/managed-lustre
    use: [network, private_service_access]
    settings:
      name: lustre-instance
      local_mount: /lustre
      remote_mount: lustrefs
      size_gib: 18000
      kms_key: projects/<project_id>/locations/<location>/keyRings/<key_ring>/cryptoKeys/<key_name>
```

> [!IMPORTANT]
> When using CMEK, you must grant the Managed Lustre Service Account the `Cloud KMS CryptoKey Encrypter/Decrypter` role on the KMS key.
>
> The service account email follows the pattern: `service-<PROJECT_NUMBER>@gcp-sa-lustre.iam.gserviceaccount.com`.
>
> If the service account does not exist, you can create it using:
> `gcloud beta services identity create --service=lustre.googleapis.com --project=<PROJECT_ID>`
>
> You can grant the role using:
>
> ```bash
> gcloud kms keys add-iam-policy-binding <KEY_NAME> \
>     --location=<LOCATION> \
>     --keyring=<KEYRING_NAME> \
>     --member="serviceAccount:service-<PROJECT_NUMBER>@gcp-sa-lustre.iam.gserviceaccount.com" \
>     --role="roles/cloudkms.cryptoKeyEncrypterDecrypter" \
>     --project=<PROJECT_ID>
> ```

### Example - Importing data from GSC Bucket

One option with the Managed Lustre instance is to import data from a GSC bucket
upon the lustre instance creation.  To do this, use the `import_gcs_bucket_uri`
variable to dictate the bucket to pull data from.  The data will be imported
under the directory specified by `local_mount` (`/shared` if unspecified).

> [!NOTE]
>
> 1. This is a one way operation.  Once the data has been copied to the lustre
> instance it will not be updated with any changes made to the GCS bucket.
>
> 2. Once the lustre instance has been created in Terraform, the copy process
> will proceed in the background.  Data may not be appear in the mounted
> directory for a period of time after the deployment has completed (see below).

```yaml
- id: managed_lustre
  source: modules/file-system/managed-lustre
  use: [network, private_service_access]
  settings:
    name: lustre-instance
    local_mount: /lustre
    remote_mount: lustrefs
    size_gib: 18000
    import_gcs_bucket_uri: gs://<bucket_name>
```

> [!WARNING]
> Please follow [this guide](https://cloud.google.com/managed-lustre/docs/transfer-data#required_permissions)
> to set up the correct IAM permissions for importing data from GCS to lustre.
> Without this, the copy process may fail silently leaving an empty lustre
> instance.

If an import is requested, gcluster will output a json response similar to:

```json
{
  "name": "projects/<project_id>/locations/<location>/operations/<operation_id>",
  "metadata": {
    "@type": "type.googleapis.com/google.cloud.lustre.v1.ImportDataMetadata",
    "createTime": "<start time>",
    "target": "projects/<project_id>/locations/<location>/instances/<instance_name>",
    "requestedCancellation": false,
    "apiVersion": "v1"
  },
  "done": false
}
```

You can retrieve more information about the transfer using the following
command, substituting with values from the json response above:

```bash
gcloud lustre operations describe <operation_id> --location <location> --project <project_id>
```

This will provide information on if the transfer is complete or if any errors
have occurred. See more at
[Get operation](https://cloud.google.com/managed-lustre/docs/transfer-data#get_operation).

### Example - Multi-NIC (Slurm)

With `multinic.enabled`, clients use a second gVNIC as an extra LNet rail so a
single node can go beyond one NIC's bandwidth. At boot, a systemd unit
delivered through cloud-init:

* finds every non-primary NIC in the same VPC as nic0;
* adds a policy route for each (`ip rule from <nic IP> lookup lustre_table_<nic>`
  with a default route via that NIC's gateway);
* sets `rp_filter=2` on it;
* writes `options lnet networks="tcp0(<nic0>,<nicN>...)" <lnet_options>` to
  `/etc/modprobe.d/lustre.conf` before Lustre is mounted.

Requirements:

* The client image must run cloud-init. The setup is delivered as
  `metadata.user-data` through the `lnet_multinic_metadata` output. Images
  without cloud-init ignore it and stay single-rail.
* The second NIC must be in the **same VPC** as nic0 (a separate subnet is
  fine). NICs in other VPCs, such as GPU RDMA networks, are ignored.
* `multinic.client_tags` must match the nodeset's `tags`, otherwise the
  firewall rule for LNet callbacks (`tcp:988` and `tcp:1021-1023` from
  `psa_ip_ranges`; `tcp:6988` instead of 988 with `gke_support_enabled`)
  targets no instance.

Not supported on A3 High (`a3-highgpu-*`) and A3 Mega (`a3-megagpu-*`). These
machines have only one host NIC. `gcluster create` fails if the blueprint var `enable_multinic` is
`true` for these machine types.

To disable, set `enable_multinic = false`.
Existing nodes keep their configuration until they are recreated.

```yaml
- id: homefs
  source: modules/file-system/managed-lustre
  use: [network, private_service_access]
  settings:
    local_mount: /home
    size_gib: 72000
    per_unit_storage_throughput: 500
    multinic:
      enabled: true
      psa_ip_ranges: [$(private_service_access.cidr_range)]
      client_tags: [multinic-client]

- id: nodeset
  source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
  use: [network]
  settings:
    tags: [multinic-client]
    additional_networks:
    - network: null
      subnetwork: $(network.subnetworks["${vars.region}/<second-subnet-name>"].self_link)
      nic_type: GVNIC
      # ...remaining additional_networks fields
    metadata: $(homefs.lnet_multinic_metadata)
```

If the nodeset already sets `metadata.user-data`, merge instead of replacing it.
Either merge at the blueprint level (the later map wins, so put
`lnet_multinic_metadata` last):

```yaml
    metadata:
      $(merge({ user-data = "#cloud-config\ncreate_hostname_file: true\n" }, homefs.lnet_multinic_metadata))
```

or pass the existing cloud-config keys through `multinic.extra_cloud_config`.
`write_files` and `runcmd` passed there are kept, and the module's own entries
are appended after them.

The module's own runcmd starts the unit from `cloud-final` on first boot.
From the second boot onward, systemd orders it before remote mounts and
`google-startup-scripts.service`. If LNet was already loaded when the unit ran,
it logs a warning and the rails apply from the next boot. The module is never
unloaded, so an in-progress mount is not disrupted.

To verify on a client:

```bash
systemctl status multi-nic-lustre       # active (exited)
journalctl -u multi-nic-lustre          # "configured tcp0(...)" and no WARNING
cat /etc/modprobe.d/lustre.conf
cat /sys/module/lnet/parameters/networks /sys/module/lnet/parameters/lnet_numa_range /sys/module/lnet/parameters/lnet_peer_discovery_disabled
ip rule show                            # from <nicN IP> lookup lustre_table_<nicN>
lnetctl net show                        # one NI per rail
```

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
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.27.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.27.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_firewall.lnet_callback_ingress](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_firewall) | resource |
| [google_lustre_instance.lustre_instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/lustre_instance) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [google_compute_network_peering.private_peering](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_network_peering) | data source |
| [google_storage_bucket.lustre_import_bucket](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/storage_bucket) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used as name of the Lustre instance if no name is specified. | `string` | n/a | yes |
| <a name="input_description"></a> [description](#input\_description) | Description of the created Lustre instance. | `string` | `"Lustre Instance"` | no |
| <a name="input_enable_dynamic_tier"></a> [enable\_dynamic\_tier](#input\_enable\_dynamic\_tier) | Set to true to enable Dynamic Tier for the Lustre instance. | `bool` | `false` | no |
| <a name="input_gke_support_enabled"></a> [gke\_support\_enabled](#input\_gke\_support\_enabled) | Set to true to create Managed Lustre instance with GKE compatibility.<br/>Note: This does not work with Slurm, the Slurm image must be built with<br/>the correct compatibility. | `bool` | `false` | no |
| <a name="input_import_gcs_bucket_uri"></a> [import\_gcs\_bucket\_uri](#input\_import\_gcs\_bucket\_uri) | The name of the GCS bucket to import data from to managed lustre. Data will<br/>be imported to the local\_mount directory. Changing this value will not<br/>trigger a redeployment, to prevent data deletion. | `string` | `null` | no |
| <a name="input_kms_key"></a> [kms\_key](#input\_kms\_key) | The resource ID of a Customer-Managed Encryption Key (CMEK) to use for the Lustre instance. In the format: projects/<project\_id>/locations/<location>/keyRings/<key\_ring>/cryptoKeys/<key\_name> | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the Managed Lustre instance. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Local mount point for the Managed Lustre instance. | `string` | `"/shared"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | Mounting options for the file system. | `string` | `"defaults,_netdev"` | no |
| <a name="input_multinic"></a> [multinic](#input\_multinic) | Multi-NIC (LNet Multi-Rail) client configuration.<br/><br/>Requires a second NIC on the clients in the same VPC as nic0. At boot,<br/>every NIC in the same VPC as nic0 is used as a Lustre rail; NICs in other<br/>VPCs (such as GPU RDMA NICs) are ignored.<br/><br/>Requires an image that runs cloud-init; otherwise clients stay single-rail.<br/>client\_tags must match the client nodesets' tags. | <pre>object({<br/>    enabled = optional(bool, false)<br/>    # numa_range=1000000: hides CPU socket distance from LNet so peers spread<br/>    # over both rails instead of all picking the nearest NIC.<br/>    # lnet_peer_discovery_disabled=1: disables LNet dynamic peer discovery as<br/>    # required by Managed Lustre for multi-NIC client striping.<br/>    lnet_options = optional(string, "lnet_numa_range=1000000 lnet_peer_discovery_disabled=1")<br/>    table_base   = optional(number, 101)<br/>    rp_filter    = optional(number, 2)<br/>    # LNet servers open callback connections back to the client on tcp:988<br/>    # and tcp:1021-1023 (tcp:6988 instead of 988 with gke_support_enabled).<br/>    # Scope the ingress rule to the PSA tenant range and target the clients'<br/>    # network tag.<br/>    create_firewall = optional(bool, true)<br/>    psa_ip_ranges   = optional(list(string), [])<br/>    client_tags     = optional(list(string), [])<br/>    # Extra cloud-config keys merged into the emitted yaml. Needed where a<br/>    # blueprint already uses metadata.user-data<br/>    extra_cloud_config = optional(any, {})<br/>  })</pre> | `{}` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the Lustre instance | `string` | n/a | yes |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the instance is connected given in the format:<br/>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | Network self-link this instance will be on, required for checking private service access | `string` | n/a | yes |
| <a name="input_per_unit_storage_throughput"></a> [per\_unit\_storage\_throughput](#input\_per\_unit\_storage\_throughput) | Throughput of the instance in MB/s/TiB. Valid values are 125, 250, 500, 1000. If enable\_dynamic\_tier is false, this defaults to 500. | `number` | `null` | no |
| <a name="input_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#input\_private\_vpc\_connection\_peering) | The name of the VPC Network peering connection.<br/>If using new VPC, please use modules/network/private-service-access to create private-service-access and<br/>If using existing VPC with private-service-access enabled, set this manually." | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which Lustre instance will be created. | `string` | n/a | yes |
| <a name="input_remote_mount"></a> [remote\_mount](#input\_remote\_mount) | Remote mount point of the Managed Lustre instance | `string` | n/a | yes |
| <a name="input_size_gib"></a> [size\_gib](#input\_size\_gib) | Storage size of the Managed Lustre instance in GB. See https://cloud.google.com/managed-lustre/docs/create-instance for limitations | `number` | `36000` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Location for the Lustre instance. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_capacity_gib"></a> [capacity\_gib](#output\_capacity\_gib) | File share capacity in GiB. |
| <a name="output_install_managed_lustre_client"></a> [install\_managed\_lustre\_client](#output\_install\_managed\_lustre\_client) | Script for installing Managed Lustre client |
| <a name="output_lnet_multinic_metadata"></a> [lnet\_multinic\_metadata](#output\_lnet\_multinic\_metadata) | Instance metadata to merge into compute nodesets for LNet Multi-Rail<br/>configuration (empty when multinic is disabled). If multiple managed-lustre<br/>instances exist in a blueprint, wire this output once per nodeset.<br/>Delivered as cloud-init user-data: it has no effect on images that do not<br/>run cloud-init. |
| <a name="output_lustre_id"></a> [lustre\_id](#output\_lustre\_id) | An identifier for the resource with format `projects/{{project}}/locations/{{location}}/instances/{{name}}` |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a Managed Lustre instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
