`trigger-schedule` is a helper module to schedule CloudBuild triggers, mimics default behaviour of CloudBuild UI schedule wizard.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 5.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 5.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_cloud_scheduler_job.schedule](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloud_scheduler_job) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_retry_count"></a> [retry\_count](#input\_retry\_count) | Number of times to retry a failed build | `number` | `1` | no |
| <a name="input_schedule"></a> [schedule](#input\_schedule) | Describes the schedule on which the job will be executed. | `string` | n/a | yes |
| <a name="input_time_zone"></a> [time\_zone](#input\_time\_zone) | Specifies the time zone to be used in interpreting schedule. | `string` | `"Asia/Kolkata"` | no |
| <a name="input_trigger"></a> [trigger](#input\_trigger) | View of google\_cloudbuild\_trigger resource | <pre>object({<br/>    name    = string<br/>    id      = string<br/>    project = string<br/>  })</pre> | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
