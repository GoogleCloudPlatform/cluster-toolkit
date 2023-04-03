`provision` module creates CloudBuilds triggers and schedules.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 4.58.0 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | ~> 4.58.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 4.58.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_daily_project_cleanup_schedule"></a> [daily\_project\_cleanup\_schedule](#module\_daily\_project\_cleanup\_schedule) | ./trigger-schedule | n/a |
| <a name="module_weekly_build_dependency_check_schedule"></a> [weekly\_build\_dependency\_check\_schedule](#module\_weekly\_build\_dependency\_check\_schedule) | ./trigger-schedule | n/a |
| <a name="module_weekly_builder_image_schedule"></a> [weekly\_builder\_image\_schedule](#module\_weekly\_builder\_image\_schedule) | ./trigger-schedule | n/a |

## Resources

| Name | Type |
|------|------|
| [google_cloudbuild_trigger.daily_project_cleanup](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.weekly_build_dependency_check](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.weekly_builder_image](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.zebug_fast_build_failure](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.zebug_fast_build_success](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | GCP region | `string` | `"us-central1"` | no |
| <a name="input_repo_uri"></a> [repo\_uri](#input\_repo\_uri) | URI of GitHub repo | `string` | `"https://github.com/GoogleCloudPlatform/hpc-toolkit"` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | GCP zone | `string` | `"us-central1-c"` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
