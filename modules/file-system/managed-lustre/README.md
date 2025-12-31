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
| [google_storage_bucket.lustre_import_bucket](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/storage_bucket) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment, used as name of the Lustre instance if no name is specified. | `string` | n/a | yes |
| <a name="input_description"></a> [description](#input\_description) | Description of the created Lustre instance. | `string` | `"Lustre Instance"` | no |
| <a name="input_gke_support_enabled"></a> [gke\_support\_enabled](#input\_gke\_support\_enabled) | Set to true to create Managed Lustre instance with GKE compatibility.<br/>Note: This does not work with Slurm, the Slurm image must be built with<br/>the correct compatibility. | `bool` | `false` | no |
| <a name="input_import_gcs_bucket_uri"></a> [import\_gcs\_bucket\_uri](#input\_import\_gcs\_bucket\_uri) | The name of the GCS bucket to import data from to managed lustre. Data will<br/>be imported to the local\_mount directory. Changing this value will not<br/>trigger a redeployment, to prevent data deletion. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the Managed Lustre instance. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | Local mount point for the Managed Lustre instance. | `string` | `"/shared"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | Mounting options for the file system. | `string` | `"defaults,_netdev"` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the Lustre instance | `string` | n/a | yes |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to which the instance is connected given in the format:<br/>`projects/<project_id>/global/networks/<network_name>`" | `string` | n/a | yes |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | Network self-link this instance will be on, required for checking private service access | `string` | n/a | yes |
| <a name="input_per_unit_storage_throughput"></a> [per\_unit\_storage\_throughput](#input\_per\_unit\_storage\_throughput) | Throughput of the instance in MB/s/TiB. Valid values are 125, 250, 500, 1000. | `number` | `500` | no |
| <a name="input_private_vpc_connection_peering"></a> [private\_vpc\_connection\_peering](#input\_private\_vpc\_connection\_peering) | The name of the VPC Network peering connection.<br/>If using new VPC, please use modules/network/private-service-access to create private-service-access and<br/>If using existing VPC with private-service-access enabled, set this manually." | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which Lustre instance will be created. | `string` | n/a | yes |
| <a name="input_remote_mount"></a> [remote\_mount](#input\_remote\_mount) | Remote mount point of the Managed Lustre instance | `string` | n/a | yes |
| <a name="input_size_gib"></a> [size\_gib](#input\_size\_gib) | Storage size of the Managed Lustre instance in GB. See https://cloud.google.com/managed-lustre/docs/create-instance for limitations | `number` | `36000` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Location for the Lustre instance. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_capacity_gib"></a> [capacity\_gib](#output\_capacity\_gib) | File share capacity in GiB. |
| <a name="output_install_managed_lustre_client"></a> [install\_managed\_lustre\_client](#output\_install\_managed\_lustre\_client) | Script for installing Managed Lustre client |
| <a name="output_lustre_id"></a> [lustre\_id](#output\_lustre\_id) | An identifier for the resource with format `projects/{{project}}/locations/{{location}}/instances/{{name}}` |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a Managed Lustre instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
