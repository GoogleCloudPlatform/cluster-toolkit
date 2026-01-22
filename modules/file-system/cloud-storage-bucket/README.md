## Description

This module creates a [Google Cloud Storage (GCS) bucket](https://cloud.google.com/storage).

For more information on this and other network storage options in the Cluster
Toolkit, see the extended [Network Storage documentation](../../../../docs/network_storage.md).

### Example

The following example will create a bucket named `simulation-results-xxxxxxxx`,
where `xxxxxxxx` is a randomly generated id.

```yaml
  - id: bucket
    source: modules/file-system/cloud-storage-bucket
    settings:
      name_prefix: simulation-results
      random_suffix: true
```

> **_NOTE:_** Use of `random_suffix` may cause the following error when used
> with other modules:
> `value depends on resource attributes that cannot be determined until apply`.
> To resolve this set `random_suffix` to `false` (default).

<!-- -->

> **_NOTE:_** Bucket namespace is shared by all users of Google Cloud so it is
> possible to have a bucket name clash with an existing bucket that is not in
> your project. To resolve this try to use a more unique name, or set the
> `random_suffix` variable to `true`.

## Naming of Bucket

There are potentially three parts to the bucket name. Each of these parts are
configurable in the blueprint.

1. A **custom prefix**, provided by the user in the blueprint \
Provide the custom prefix using the `name_prefix` setting.

1. The **deployment name**, included by default \
The deployment name can be excluded by setting `use_deployment_name_in_bucket_name: false`.

1. A **random id** suffix, excluded by default \
The random id can be included by setting `random_suffix: true`.

If none of these are provided (no `name_prefix`,
`use_deployment_name_in_bucket_name: false`, & `random_suffix: false`), then the
bucket name will default to `no-bucket-name-provided`.

Since bucket namespace is shared by all users of Google Cloud, it is more likely
to experience naming clashes than with other resources. In many cases, adding
the `random_suffix` will resolve the naming clash issue.

> **Warning**: If a bucket is created with a `random_suffix` and then used as
> the bucket for a startup script in the same deployment group this will cause a
> `not known at apply time` error in terraform. The solution is to either create
> the bucket in a separate deployment group or to remove the random suffix.

## Mounting

To mount the Cloud Storage bucket you must first ensure that the GCS Fuse client
has been installed and then call the proper `mount` command.

Both of these steps are automatically handled with the use of the `use` command
in a selection of Cluster Toolkit modules. See the [compatibility matrix][matrix] in
the network storage doc for a complete list of supported modules.

If mounting is not automatically handled as described above, the
`cloud-storage-bucket` module outputs runners that can be used with the
`startup-script` module to install the client and mount the file system. See the
following example:

```yaml
  - id: bucket
    source: modules/file-system/cloud-storage-bucket
    settings: {local_mount: /data}

  - id: mount-at-startup
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(bucket.client_install_runner)
      - $(bucket.mount_runner)
```

[matrix]: ../../../../docs/network_storage.md#compatibility-matrix

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | >= 6.9.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_google-beta"></a> [google-beta](#provider\_google-beta) | >= 6.9.0 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google-beta_google_storage_anywhere_cache.cache_instances](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_storage_anywhere_cache) | resource |
| [google-beta_google_storage_bucket.bucket](https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/google_storage_bucket) | resource |
| [google_storage_bucket_iam_binding.viewers](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_iam_binding) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_anywhere_cache"></a> [anywhere\_cache](#input\_anywhere\_cache) | Anywhere Cache configurations.<br/>When you create a cache for a bucket, the cache must be created in a zone within the location of your bucket.<br/>For example, if your bucket is located in the us-east1 region, you can create a cache in us-east1-b but not us-central1-c.<br/>If your bucket is located in the ASIA dual-region, you can create a cache in any zones that make up the asia-east1 and asia-southeast1 regions.<br/>This validation only works for single regions. | <pre>object({<br/>    zones            = list(string)<br/>    ttl              = optional(string, "86400s")<br/>    admission_policy = optional(string, "admit-on-first-miss")<br/>  })</pre> | `null` | no |
| <a name="input_anywhere_cache_create_timeout"></a> [anywhere\_cache\_create\_timeout](#input\_anywhere\_cache\_create\_timeout) | Timeout for Anywhere Cache creation operations. Can be set to a duration like '1h' or '30m'.<br/>The maximum documented creation time is 48 hours. Please refer to the official documentation for more details on timeouts:<br/>https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_anywhere_cache#timeouts | `string` | `"240m"` | no |
| <a name="input_autoclass"></a> [autoclass](#input\_autoclass) | Configure bucket autoclass setup<br/><br/>The autoclass config supports automatic transitions of objects in the bucket to appropriate storage classes based on each object's access pattern.<br/><br/>The terminal storage class defines that objects in the bucket eventually transition to if they are not read for a certain length of time. <br/>Supported values include: 'NEARLINE', 'ARCHIVE' (Default 'NEARLINE')<br/><br/>See Cloud documentation for more details:<br/><br/>https://cloud.google.com/storage/docs/autoclass | <pre>object({<br/>    enabled                = optional(bool, false)<br/>    terminal_storage_class = optional(string, null)<br/>  })</pre> | <pre>{<br/>  "enabled": false<br/>}</pre> | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Name of the HPC deployment; used as part of name of the GCS bucket. | `string` | n/a | yes |
| <a name="input_enable_hierarchical_namespace"></a> [enable\_hierarchical\_namespace](#input\_enable\_hierarchical\_namespace) | If true, enables hierarchical namespace for the bucket. This option must be configured during the initial creation of the bucket. | `bool` | `false` | no |
| <a name="input_enable_object_retention"></a> [enable\_object\_retention](#input\_enable\_object\_retention) | If true, enables retention policy at per object level for the bucket.<br/><br/>See Cloud documentation for more details:<br/><br/>https://cloud.google.com/storage/docs/object-lock | `bool` | `false` | no |
| <a name="input_enable_versioning"></a> [enable\_versioning](#input\_enable\_versioning) | If true, enables versioning for the bucket. | `bool` | `false` | no |
| <a name="input_force_destroy"></a> [force\_destroy](#input\_force\_destroy) | If true will destroy bucket with all objects stored within. | `bool` | `false` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the GCS bucket. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_lifecycle_rules"></a> [lifecycle\_rules](#input\_lifecycle\_rules) | List of config to manage data lifecycle rules for the bucket. For more details: https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket.html#nested_lifecycle_rule | <pre>list(object({<br/>    # Object with keys:<br/>    # - type - The type of the action of this Lifecycle Rule. Supported values: Delete and SetStorageClass.<br/>    # - storage_class - (Required if action type is SetStorageClass) The target Storage Class of objects affected by this Lifecycle Rule.<br/>    action = object({<br/>      type          = string<br/>      storage_class = optional(string)<br/>    })<br/><br/>    # Object with keys:<br/>    # - age - (Optional) Minimum age of an object in days to satisfy this condition.<br/>    # - send_age_if_zero - (Optional) While set true, num_newer_versions value will be sent in the request even for zero value of the field.<br/>    # - created_before - (Optional) Creation date of an object in RFC 3339 (e.g. 2017-06-13) to satisfy this condition.<br/>    # - with_state - (Optional) Match to live and/or archived objects. Supported values include: "LIVE", "ARCHIVED", "ANY".<br/>    # - matches_storage_class - (Optional) Comma delimited string for storage class of objects to satisfy this condition. Supported values include: MULTI_REGIONAL, REGIONAL, NEARLINE, COLDLINE, ARCHIVE, STANDARD, DURABLE_REDUCED_AVAILABILITY.<br/>    # - matches_prefix - (Optional) One or more matching name prefixes to satisfy this condition.<br/>    # - matches_suffix - (Optional) One or more matching name suffixes to satisfy this condition.<br/>    # - num_newer_versions - (Optional) Relevant only for versioned objects. The number of newer versions of an object to satisfy this condition.<br/>    # - custom_time_before - (Optional) A date in the RFC 3339 format YYYY-MM-DD. This condition is satisfied when the customTime metadata for the object is set to an earlier date than the date used in this lifecycle condition.<br/>    # - days_since_custom_time - (Optional) The number of days from the Custom-Time metadata attribute after which this condition becomes true.<br/>    # - days_since_noncurrent_time - (Optional) Relevant only for versioned objects. Number of days elapsed since the noncurrent timestamp of an object.<br/>    # - noncurrent_time_before - (Optional) Relevant only for versioned objects. The date in RFC 3339 (e.g. 2017-06-13) when the object became nonconcurrent.<br/>    condition = object({<br/>      age                        = optional(number)<br/>      send_age_if_zero           = optional(bool)<br/>      created_before             = optional(string)<br/>      with_state                 = optional(string)<br/>      matches_storage_class      = optional(string)<br/>      matches_prefix             = optional(string)<br/>      matches_suffix             = optional(string)<br/>      num_newer_versions         = optional(number)<br/>      custom_time_before         = optional(string)<br/>      days_since_custom_time     = optional(number)<br/>      days_since_noncurrent_time = optional(number)<br/>      noncurrent_time_before     = optional(string)<br/>    })<br/>  }))</pre> | `[]` | no |
| <a name="input_local_mount"></a> [local\_mount](#input\_local\_mount) | The mount point where the contents of the device may be accessed after mounting. | `string` | `"/mnt"` | no |
| <a name="input_mount_options"></a> [mount\_options](#input\_mount\_options) | Mount options to be put in fstab. Note: `implicit_dirs` makes it easier to work with objects added by other tools, but there is a performance impact. See: [more information](https://github.com/GoogleCloudPlatform/gcsfuse/blob/master/docs/semantics.md#implicit-directories) | `string` | `"defaults,_netdev,implicit_dirs"` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Name Prefix. | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | ID of project in which GCS bucket will be created. | `string` | n/a | yes |
| <a name="input_public_access_prevention"></a> [public\_access\_prevention](#input\_public\_access\_prevention) | Bucket public access can be controlled by setting a value of either `inherited` or `enforced`. <br/>When set to `enforced`, public access to the bucket is blocked.<br/>If set to `inherited`, the bucket's public access prevention depends on whether it is subject to the organization policy constraint for public access prevention.<br/><br/>See Cloud documentation for more details:<br/><br/>https://cloud.google.com/storage/docs/public-access-prevention | `string` | `null` | no |
| <a name="input_random_suffix"></a> [random\_suffix](#input\_random\_suffix) | If true, a random id will be appended to the suffix of the bucket name. | `bool` | `false` | no |
| <a name="input_region"></a> [region](#input\_region) | The region to deploy to | `string` | n/a | yes |
| <a name="input_retention_policy_period"></a> [retention\_policy\_period](#input\_retention\_policy\_period) | If defined, this will configure retention\_policy with retention\_period for the bucket, value must be in between 1 and 3155760000(100 years) seconds.<br/><br/>See Cloud documentation for more details:<br/><br/>https://cloud.google.com/storage/docs/bucket-lock | `number` | `null` | no |
| <a name="input_soft_delete_retention_duration"></a> [soft\_delete\_retention\_duration](#input\_soft\_delete\_retention\_duration) | If defined, this will configure soft\_delete\_policy with retention\_duration\_seconds for the bucket, value can be 0 or in between 604800(7 days) and 7776000(90 days).<br/>Setting a 0 duration disables soft delete, meaning any deleted objects will be permanently deleted.<br/><br/>See Cloud documentation for more details:<br/><br/>https://cloud.google.com/storage/docs/soft-delete | `number` | `null` | no |
| <a name="input_storage_class"></a> [storage\_class](#input\_storage\_class) | The storage class of the GCS bucket. | `string` | `"REGIONAL"` | no |
| <a name="input_uniform_bucket_level_access"></a> [uniform\_bucket\_level\_access](#input\_uniform\_bucket\_level\_access) | Allow uniform control access to the bucket. | `bool` | `true` | no |
| <a name="input_use_deployment_name_in_bucket_name"></a> [use\_deployment\_name\_in\_bucket\_name](#input\_use\_deployment\_name\_in\_bucket\_name) | If true, the deployment name will be included as part of the bucket name. This helps prevent naming clashes across multiple deployments. | `bool` | `true` | no |
| <a name="input_viewers"></a> [viewers](#input\_viewers) | A list of additional accounts that can read packages from this bucket | `set(string)` | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_anywhere_cache_ids"></a> [anywhere\_cache\_ids](#output\_anywhere\_cache\_ids) | The IDs of the created Anywhere Cache instances. |
| <a name="output_client_install_runner"></a> [client\_install\_runner](#output\_client\_install\_runner) | Runner that performs client installation needed to use gcs fuse. |
| <a name="output_gcs_bucket_name"></a> [gcs\_bucket\_name](#output\_gcs\_bucket\_name) | Bucket name. |
| <a name="output_gcs_bucket_path"></a> [gcs\_bucket\_path](#output\_gcs\_bucket\_path) | The gsutil bucket path with format of `gs://<bucket-name>`. |
| <a name="output_mount_runner"></a> [mount\_runner](#output\_mount\_runner) | Runner that mounts the cloud storage bucket with gcs fuse. |
| <a name="output_network_storage"></a> [network\_storage](#output\_network\_storage) | Describes a remote network storage to be mounted by fs-tab. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
