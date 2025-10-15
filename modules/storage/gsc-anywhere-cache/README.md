# GCS Anywhere Cache Module

## Description

This Terraform module configures Google Cloud Storage (GCS) Anywhere Caches for an **existing** GCS bucket. Anywhere Cache provides an SSD-backed zonal read cache, which can help reduce latency and network egress costs for read-heavy workloads by caching frequently accessed data closer to your compute instances.

For more details on the GCS Anywhere Cache feature, see the [official Google Cloud documentation](https://cloud.google.com/storage/docs/anywhere-cache).

This module uses the `google_storage_anywhere_cache` resource to manage cache instances for the specified bucket.

**Note:** Provisioning Anywhere Cache instances can take a significant amount of time (often 5-15+ minutes per zone).

## Usage

This module is intended to be used within a Cluster Toolkit blueprint. It assumes the GCS bucket specified in `bucket_name` already exists.

**Example Blueprint Configuration:**

```yaml

deployment_groups:
  - group: primary
    modules:
      # Anywhere Cache Configuration for an existing bucket
      - id: my_bucket_anywhere_caches
        source: "modules/storage/gsc-anywhere-cache" # Adjust path as needed
        settings:
          bucket_name: $(vars.data_bucket_name) # Replace with your DATA bucket name
          caches:
            - zone: "us-central1-a"
              ttl: "43200s"  # 12 hours
              admission_policy: "admit-on-first-miss"
            - zone: "us-central1-b"
              ttl: "86400s"  # 24 hours
              admission_policy: "admit-on-second-miss"
            - zone: "us-central1-c"
              # Using default ttl (24h) and admission_policy ("admit-on-first-miss")

## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 5.2.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_storage_anywhere_cache.cache_instances](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_anywhere_cache) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_bucket_name"></a> [bucket\_name](#input\_bucket\_name) | The name of the bucket. | `string` | n/a | yes |
| <a name="input_caches"></a> [caches](#input\_caches) | A list of Anywhere Cache configurations. | <pre>list(object({<br/>    zone             = string<br/>    ttl              = optional(string, "86400s")<br/>    admission_policy = optional(string, "admit-on-first-miss")<br/>  }))</pre> | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_cache_ids"></a> [cache\_ids](#output\_cache\_ids) | The IDs of the created Anywhere Cache instances. |

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.7.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 5.2.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 5.2.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_storage_anywhere_cache.cache_instances](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_anywhere_cache) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_bucket_name"></a> [bucket\_name](#input\_bucket\_name) | The name of the bucket. | `string` | n/a | yes |
| <a name="input_caches"></a> [caches](#input\_caches) | A list of Anywhere Cache configurations. | <pre>list(object({<br/>    zone             = string<br/>    ttl              = optional(string, "86400s")<br/>    admission_policy = optional(string, "admit-on-first-miss")<br/>  }))</pre> | `[]` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_cache_ids"></a> [cache\_ids](#output\_cache\_ids) | The IDs of the created Anywhere Cache instances. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
