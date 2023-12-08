## Description

This module creates a Google Kubernetes Engine
([GKE](https://cloud.google.com/kubernetes-engine)) cluster.

> **_NOTE:_** This is an experimental module and the functionality and
> documentation will likely be updated in the near future. This module has only
> been tested in limited capacity.

### Example

The following example creates a GKE cluster and a VPC designed to work with GKE.
See [VPC Network](#vpc-network) section for more information about network
requirements.

```yaml
  - id: network1
    source: modules/network/vpc
    settings:
      subnetwork_name: gke-subnet
      secondary_ranges:
        gke-subnet:
        - range_name: pods
          ip_cidr_range: 10.4.0.0/14
        - range_name: services
          ip_cidr_range: 10.0.32.0/20

  - id: gke_cluster
    source: community/modules/scheduler/gke-cluster
    use: [network1]
```

Also see a full [GKE example blueprint](../../../examples/hpc-gke.yaml).

### VPC Network

This module is configured to create a
[VPC-native cluster](https://cloud.google.com/kubernetes-engine/docs/concepts/alias-ips).
This means that alias IPs are used and that the subnetwork requires secondary
ranges for pods and services. In the example shown above these secondary ranges
are created in the VPC module. By default the `gke-cluster` module will look for
ranges with the names `pods` and `services`. These names can be configured using
the `pods_ip_range_name` and `services_ip_range_name` settings.

### Cluster Limitations

The current implementations has the following limitations:

- Autopilot is disabled
- Auto-provisioning of new node pools is disabled
- Network policies are not supported
- General addon configuration is not supported
- Only regional cluster is supported

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.51.0, < 5.0 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 4.65.0, < 5.0 |
| <a name="requirement_kubernetes"></a> [kubernetes](#requirement\_kubernetes) | ~> 2.23 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.51.0, < 5.0 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 4.65.0, < 5.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_workload_identity"></a> [workload\_identity](#module\_workload\_identity) | terraform-google-modules/kubernetes-engine/google//modules/workload-identity | 29.0.0 |

## Resources

| Name | Type |
|------|------|
| [google-beta_google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_container_cluster) | resource |
| [google-beta_google_container_node_pool.system_node_pools](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_container_node_pool) | resource |
| [google_project_iam_member.node_service_account_artifact_registry](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.node_service_account_gcr](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.node_service_account_log_writer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.node_service_account_metric_writer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.node_service_account_monitoring_viewer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.node_service_account_resource_metadata_writer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_client_config.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_config) | data source |
| [google_compute_default_service_account.default_sa](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_default_service_account) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_authenticator_security_group"></a> [authenticator\_security\_group](#input\_authenticator\_security\_group) | The name of the RBAC security group for use with Google security groups in Kubernetes RBAC. Group name must be in format gke-security-groups@yourdomain.com | `string` | `null` | no |
| <a name="input_autoscaling_profile"></a> [autoscaling\_profile](#input\_autoscaling\_profile) | (Beta) Optimize for utilization or availability when deciding to remove nodes. Can be BALANCED or OPTIMIZE\_UTILIZATION. | `string` | `"OPTIMIZE_UTILIZATION"` | no |
| <a name="input_configure_workload_identity_sa"></a> [configure\_workload\_identity\_sa](#input\_configure\_workload\_identity\_sa) | When true, a kubernetes service account will be created and bound using workload identity to the service account used to create the cluster. | `bool` | `false` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment. Used in the GKE cluster name by default and can be configured with `prefix_with_deployment_name`. | `string` | n/a | yes |
| <a name="input_enable_dataplane_v2"></a> [enable\_dataplane\_v2](#input\_enable\_dataplane\_v2) | Enables [Dataplane v2](https://cloud.google.com/kubernetes-engine/docs/concepts/dataplane-v2). This setting is immutable on clusters. | `bool` | `false` | no |
| <a name="input_enable_filestore_csi"></a> [enable\_filestore\_csi](#input\_enable\_filestore\_csi) | The status of the Filestore Container Storage Interface (CSI) driver addon, which allows the usage of filestore instance as volumes. | `bool` | `false` | no |
| <a name="input_enable_gcsfuse_csi"></a> [enable\_gcsfuse\_csi](#input\_enable\_gcsfuse\_csi) | The status of the GCSFuse Filestore Container Storage Interface (CSI) driver addon, which allows the usage of a gcs bucket as volumes. | `bool` | `false` | no |
| <a name="input_enable_master_global_access"></a> [enable\_master\_global\_access](#input\_enable\_master\_global\_access) | Whether the cluster master is accessible globally (from any region) or only within the same region as the private endpoint. | `bool` | `false` | no |
| <a name="input_enable_persistent_disk_csi"></a> [enable\_persistent\_disk\_csi](#input\_enable\_persistent\_disk\_csi) | The status of the Google Compute Engine Persistent Disk Container Storage Interface (CSI) driver addon, which allows the usage of a PD as volumes. | `bool` | `true` | no |
| <a name="input_enable_private_endpoint"></a> [enable\_private\_endpoint](#input\_enable\_private\_endpoint) | (Beta) Whether the master's internal IP address is used as the cluster endpoint. | `bool` | `true` | no |
| <a name="input_enable_private_ipv6_google_access"></a> [enable\_private\_ipv6\_google\_access](#input\_enable\_private\_ipv6\_google\_access) | The private IPv6 google access type for the VMs in this subnet. | `bool` | `true` | no |
| <a name="input_enable_private_nodes"></a> [enable\_private\_nodes](#input\_enable\_private\_nodes) | (Beta) Whether nodes have internal IP addresses only. | `bool` | `true` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | GCE resource labels to be applied to resources. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_maintenance_exclusions"></a> [maintenance\_exclusions](#input\_maintenance\_exclusions) | List of maintenance exclusions. A cluster can have up to three. | <pre>list(object({<br>    name            = string<br>    start_time      = string<br>    end_time        = string<br>    exclusion_scope = string<br>  }))</pre> | `[]` | no |
| <a name="input_maintenance_start_time"></a> [maintenance\_start\_time](#input\_maintenance\_start\_time) | Start time for daily maintenance operations. Specified in GMT with `HH:MM` format. | `string` | `"09:00"` | no |
| <a name="input_master_authorized_networks"></a> [master\_authorized\_networks](#input\_master\_authorized\_networks) | External network that can access Kubernetes master through HTTPS. Must be specified in CIDR notation. | <pre>list(object({<br>    cidr_block   = string<br>    display_name = string<br>  }))</pre> | `[]` | no |
| <a name="input_master_ipv4_cidr_block"></a> [master\_ipv4\_cidr\_block](#input\_master\_ipv4\_cidr\_block) | (Beta) The IP range in CIDR notation to use for the hosted master network. | `string` | `"172.16.0.32/28"` | no |
| <a name="input_min_master_version"></a> [min\_master\_version](#input\_min\_master\_version) | The minimum version of the master. If unset, the cluster's version will be set by GKE to the version of the most recent official release. | `string` | `null` | no |
| <a name="input_name_suffix"></a> [name\_suffix](#input\_name\_suffix) | Custom cluster name postpended to the `deployment_name`. See `prefix_with_deployment_name`. | `string` | `""` | no |
| <a name="input_network_id"></a> [network\_id](#input\_network\_id) | The ID of the GCE VPC network to host the cluster given in the format: `projects/<project_id>/global/networks/<network_name>`. | `string` | n/a | yes |
| <a name="input_pods_ip_range_name"></a> [pods\_ip\_range\_name](#input\_pods\_ip\_range\_name) | The name of the secondary subnet ip range to use for pods. | `string` | `"pods"` | no |
| <a name="input_prefix_with_deployment_name"></a> [prefix\_with\_deployment\_name](#input\_prefix\_with\_deployment\_name) | If true, cluster name will be prefixed by `deployment_name` (ex: <deployment\_name>-<name\_suffix>). | `bool` | `true` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID to host the cluster in. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | The region to host the cluster in. | `string` | n/a | yes |
| <a name="input_release_channel"></a> [release\_channel](#input\_release\_channel) | The release channel of this cluster. Accepted values are `UNSPECIFIED`, `RAPID`, `REGULAR` and `STABLE`. | `string` | `"UNSPECIFIED"` | no |
| <a name="input_service_account"></a> [service\_account](#input\_service\_account) | DEPRECATED: use service\_account\_email and scopes. | <pre>object({<br>    email  = string,<br>    scopes = set(string)<br>  })</pre> | `null` | no |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | Service account e-mail address to use with the system node pool | `string` | `null` | no |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Scopes to to use with the system node pool. | `set(string)` | <pre>[<br>  "https://www.googleapis.com/auth/cloud-platform"<br>]</pre> | no |
| <a name="input_services_ip_range_name"></a> [services\_ip\_range\_name](#input\_services\_ip\_range\_name) | The name of the secondary subnet range to use for services. | `string` | `"services"` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork to host the cluster in. | `string` | n/a | yes |
| <a name="input_system_node_pool_enabled"></a> [system\_node\_pool\_enabled](#input\_system\_node\_pool\_enabled) | Create a system node pool. | `bool` | `true` | no |
| <a name="input_system_node_pool_machine_type"></a> [system\_node\_pool\_machine\_type](#input\_system\_node\_pool\_machine\_type) | Machine type for the system node pool. | `string` | `"e2-standard-4"` | no |
| <a name="input_system_node_pool_name"></a> [system\_node\_pool\_name](#input\_system\_node\_pool\_name) | Name of the system node pool. | `string` | `"system"` | no |
| <a name="input_system_node_pool_node_count"></a> [system\_node\_pool\_node\_count](#input\_system\_node\_pool\_node\_count) | The total min and max nodes to be maintained in the system node pool. | <pre>object({<br>    total_min_nodes = number<br>    total_max_nodes = number<br>  })</pre> | <pre>{<br>  "total_max_nodes": 10,<br>  "total_min_nodes": 2<br>}</pre> | no |
| <a name="input_system_node_pool_taints"></a> [system\_node\_pool\_taints](#input\_system\_node\_pool\_taints) | Taints to be applied to the system node pool. | <pre>list(object({<br>    key    = string<br>    value  = any<br>    effect = string<br>  }))</pre> | <pre>[<br>  {<br>    "effect": "NO_SCHEDULE",<br>    "key": "components.gke.io/gke-managed-components",<br>    "value": true<br>  }<br>]</pre> | no |
| <a name="input_timeout_create"></a> [timeout\_create](#input\_timeout\_create) | Timeout for creating a node pool | `string` | `null` | no |
| <a name="input_timeout_update"></a> [timeout\_update](#input\_timeout\_update) | Timeout for updating a node pool | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_cluster_id"></a> [cluster\_id](#output\_cluster\_id) | An identifier for the resource with format projects/<project\_id>/locations/<region>/clusters/<name>. |
| <a name="output_gke_cluster_exists"></a> [gke\_cluster\_exists](#output\_gke\_cluster\_exists) | A static flag that signals to downstream modules that a cluster has been created. Needed by community/modules/scripts/kubernetes-operations. |
| <a name="output_instructions"></a> [instructions](#output\_instructions) | Instructions on how to connect to the created cluster. |
| <a name="output_k8s_service_account_name"></a> [k8s\_service\_account\_name](#output\_k8s\_service\_account\_name) | Name of k8s service account. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
