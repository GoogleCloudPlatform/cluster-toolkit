<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.84 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.84 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket_object.partition_config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/storage_bucket) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_has_tpu"></a> [has\_tpu](#input\_has\_tpu) | If set to true, the nodeset template's Pod spec will contain request/limit for TPU resource, open port 8740 for TPU communication and add toleration for google.com/tpu. | `bool` | `false` | no |
| <a name="input_nodeset_name"></a> [nodeset\_name](#input\_nodeset\_name) | The nodeset name | `string` | `"gkenodeset"` | no |
| <a name="input_partition_name"></a> [partition\_name](#input\_partition\_name) | The partition name | `string` | `"gke"` | no |
| <a name="input_slurm_bucket"></a> [slurm\_bucket](#input\_slurm\_bucket) | GCS Bucket of Slurm cluster file storage. | `any` | n/a | yes |
| <a name="input_slurm_bucket_dir"></a> [slurm\_bucket\_dir](#input\_slurm\_bucket\_dir) | Path directory within `bucket_name` for Slurm cluster file storage. | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
