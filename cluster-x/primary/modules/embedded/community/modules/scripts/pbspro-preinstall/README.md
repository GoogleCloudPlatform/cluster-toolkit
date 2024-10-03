## Description

This module uploads PBS Pro RPM packages and, optionally, a license file to
Google Cloud Storage. This enables machines in a PBS cluster to rapidly
download and install PBS at boot or during the building of an image.

### Example

The following code snippet uses this module to upload RPM packages to Cloud
Storage and make them available as outputs for subsequent modules. It also
demonstrates how to give read-only access to service accounts that will be used
with PBS clusters. Explicit listing of service accounts is typically necessary
if the bucket is provisioned in one project and clusters in other projects.

```yaml
  - id: pbspro_setup
    source: community/modules/scripts/pbspro-preinstall
    settings:
      client_rpm:    "/path/to/pbs/packages/pbspro-client-2021.1.3.20220217134230-0.el7.x86_64.rpm"
      execution_rpm: "/path/to/pbs/packages/pbspro-execution-2021.1.3.20220217134230-0.el7.x86_64.rpm"
      server_rpm:    "/path/to/pbs/packages/pbspro-server-2021.1.3.20220217134230-0.el7.x86_64.rpm"
      bucket_viewers:
      - XXXXXXXXXXXX-compute@developer.gserviceaccount.com
    outputs:
    - client_rpm_url
    - execution_rpm_url
    - server_rpm_url
```

## Granting access to PBS Pro packages

This module can be used once to support many clusters by granting read-only
access to the bucket to other clusters. Begin by identifying the service
accounts used by cluster nodes. If you haven't actively chosen a service
account, the default Compute Engine service account is being used. In this case,
the service account is

```text
XXXXXXXXXXXX-compute@developer.gserviceaccount.com
```

where the `X` characters should be replaced by the project number of your
project:

```shell
gcloud projects describe example-project
```

Supply all service accounts to the `bucket_viewers` setting as shown in the
[example](#example) above.

## Destroying a bucket with versioning enabled

By default, object versioning is enabled on the bucket so that PBS packages can
be recovered if they are overwritten or deleted. Buckets with object versioning
enabled cannot be deleted without enabling a special `force_destroy` flag which
indicates that the user is aware that they are deleting all version history of
the objects.

```shell
terraform apply -var force_destroy=true
terraform destroy
```

## Support

PBS Professional is licensed and supported by [Altair][pbspro]. This module is
maintained and supported by the Cluster Toolkit team in collaboration with Altair.

[pbspro]: https://www.altair.com/pbs-professional

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_pbspro_bucket"></a> [pbspro\_bucket](#module\_pbspro\_bucket) | terraform-google-modules/cloud-storage/google | ~> 5.0 |

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket_object.client_rpm](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.devel_rpm](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.execution_rpm](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.license_file](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.server_rpm](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_bucket_lifecycle_rules"></a> [bucket\_lifecycle\_rules](#input\_bucket\_lifecycle\_rules) | Additional lifecycle\_rules for specific buckets. Map of lowercase unprefixed name => list of lifecycle rules to configure. | <pre>list(object({<br/>    # Object with keys:<br/>    # - type - The type of the action of this Lifecycle Rule. Supported values: Delete and SetStorageClass.<br/>    # - storage_class - (Required if action type is SetStorageClass) The target Storage Class of objects affected by this Lifecycle Rule.<br/>    action = map(string)<br/><br/>    # Object with keys:<br/>    # - age - (Optional) Minimum age of an object in days to satisfy this condition.<br/>    # - created_before - (Optional) Creation date of an object in RFC 3339 (e.g. 2017-06-13) to satisfy this condition.<br/>    # - with_state - (Optional) Match to live and/or archived objects. Supported values include: "LIVE", "ARCHIVED", "ANY".<br/>    # - matches_storage_class - (Optional) Comma delimited string for storage class of objects to satisfy this condition. Supported values include: MULTI_REGIONAL, REGIONAL, NEARLINE, COLDLINE, STANDARD, DURABLE_REDUCED_AVAILABILITY.<br/>    # - num_newer_versions - (Optional) Relevant only for versioned objects. The number of newer versions of an object to satisfy this condition.<br/>    # - custom_time_before - (Optional) A date in the RFC 3339 format YYYY-MM-DD. This condition is satisfied when the customTime metadata for the object is set to an earlier date than the date used in this lifecycle condition.<br/>    # - days_since_custom_time - (Optional) The number of days from the Custom-Time metadata attribute after which this condition becomes true.<br/>    # - days_since_noncurrent_time - (Optional) Relevant only for versioned objects. Number of days elapsed since the noncurrent timestamp of an object.<br/>    # - noncurrent_time_before - (Optional) Relevant only for versioned objects. The date in RFC 3339 (e.g. 2017-06-13) when the object became nonconcurrent.<br/>    condition = map(string)<br/>  }))</pre> | <pre>[<br/>  {<br/>    "action": {<br/>      "type": "Delete"<br/>    },<br/>    "condition": {<br/>      "age": 14,<br/>      "num_newer_versions": 2<br/>    }<br/>  }<br/>]</pre> | no |
| <a name="input_bucket_viewers"></a> [bucket\_viewers](#input\_bucket\_viewers) | A list of additional accounts that can read packages from this bucket | `set(string)` | `[]` | no |
| <a name="input_client_rpm"></a> [client\_rpm](#input\_client\_rpm) | Absolute path to PBS Pro Client Host RPM file | `string` | n/a | yes |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Cluster Toolkit deployment name. Cloud resource names will include this value. | `string` | n/a | yes |
| <a name="input_devel_rpm"></a> [devel\_rpm](#input\_devel\_rpm) | Absolute path to PBS Pro Development RPM file | `string` | n/a | yes |
| <a name="input_execution_rpm"></a> [execution\_rpm](#input\_execution\_rpm) | Absolute path to PBS Pro Execution Host RPM file | `string` | n/a | yes |
| <a name="input_force_destroy"></a> [force\_destroy](#input\_force\_destroy) | Set to true if object versioning is enabled and you are certain that you want to destroy the bucket. | `bool` | `false` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the created bucket. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_license_file"></a> [license\_file](#input\_license\_file) | Path to PBS Pro license file | `string` | `null` | no |
| <a name="input_location"></a> [location](#input\_location) | Google Cloud Storage bucket location (defaults to var.region if not set) | `string` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which Google Cloud Storage bucket will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Default region for creating resources | `string` | n/a | yes |
| <a name="input_retention_policy"></a> [retention\_policy](#input\_retention\_policy) | Google Cloud Storage retention policy (to prevent accidental deletion) | `any` | `{}` | no |
| <a name="input_server_rpm"></a> [server\_rpm](#input\_server\_rpm) | Absolute path to PBS Pro Server Host RPM file | `string` | n/a | yes |
| <a name="input_storage_class"></a> [storage\_class](#input\_storage\_class) | Google Cloud Storage class | `string` | `"STANDARD"` | no |
| <a name="input_versioning"></a> [versioning](#input\_versioning) | Enable versioning of Google Cloud Storage objects (cannot be enabled with a retention policy) | `bool` | `false` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_bucket_name"></a> [bucket\_name](#output\_bucket\_name) | Bucket for PBS RPM packages |
| <a name="output_pbs_client_rpm_url"></a> [pbs\_client\_rpm\_url](#output\_pbs\_client\_rpm\_url) | gsutil URL of PBS client RPM package |
| <a name="output_pbs_devel_rpm_url"></a> [pbs\_devel\_rpm\_url](#output\_pbs\_devel\_rpm\_url) | gsutil URL of PBS development RPM package |
| <a name="output_pbs_execution_rpm_url"></a> [pbs\_execution\_rpm\_url](#output\_pbs\_execution\_rpm\_url) | gsutil URL of PBS execution host RPM package |
| <a name="output_pbs_license_file_url"></a> [pbs\_license\_file\_url](#output\_pbs\_license\_file\_url) | gsutil URL of PBS license file |
| <a name="output_pbs_server_rpm_url"></a> [pbs\_server\_rpm\_url](#output\_pbs\_server\_rpm\_url) | gsutil URL of PBS server host RPM package |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
