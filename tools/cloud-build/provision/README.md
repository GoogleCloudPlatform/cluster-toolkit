# Provisioning Cloud Build Triggers

`provision` module creates CloudBuilds triggers and schedules.

Usage:

```sh
cd tools/cloud-build/provision
terraform init
terraform plan
terraform apply
```

When prompted for gcs bucket, use `<team name>-dev-automation`.\
When prompted for project, use integration test project.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_external"></a> [external](#requirement\_external) | ~> 2.3.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 5.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_external"></a> [external](#provider\_external) | ~> 2.3.0 |
| <a name="provider_google"></a> [google](#provider\_google) | ~> 5.0 |

## Modules

| Name | Source | Version |
| ---- | ------ | ------- |
| <a name="module_daily_image_test_runner_schedule"></a> [daily\_image\_test\_runner\_schedule](#module\_daily\_image\_test\_runner\_schedule) | ./trigger-schedule | n/a |
| <a name="module_daily_project_cleanup_filestore_schedule"></a> [daily\_project\_cleanup\_filestore\_schedule](#module\_daily\_project\_cleanup\_filestore\_schedule) | ./trigger-schedule | n/a |
| <a name="module_daily_project_cleanup_schedule"></a> [daily\_project\_cleanup\_schedule](#module\_daily\_project\_cleanup\_schedule) | ./trigger-schedule | n/a |
| <a name="module_daily_project_cleanup_slurm_schedule"></a> [daily\_project\_cleanup\_slurm\_schedule](#module\_daily\_project\_cleanup\_slurm\_schedule) | ./trigger-schedule | n/a |
| <a name="module_daily_test_schedule"></a> [daily\_test\_schedule](#module\_daily\_test\_schedule) | ./trigger-schedule | n/a |
| <a name="module_daily_test_schedule_migrated"></a> [daily\_test\_schedule\_migrated](#module\_daily\_test\_schedule\_migrated) | ./trigger-schedule | n/a |
| <a name="module_weekly_build_dependency_check_schedule"></a> [weekly\_build\_dependency\_check\_schedule](#module\_weekly\_build\_dependency\_check\_schedule) | ./trigger-schedule | n/a |

## Resources

| Name | Type |
| ---- | ---- |
| [google_cloudbuild_trigger.daily_project_cleanup](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.daily_project_cleanup_filestore](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.daily_project_cleanup_slurm](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.daily_test](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.daily_test_migrated](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.image_build_test_runner](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.pr_go_build_test](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.pr_ofe_test](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.pr_ofe_venv](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.pr_test](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.pr_validation](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.weekly_build_dependency_check](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.zebug_fast_build_failure](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_cloudbuild_trigger.zebug_fast_build_success](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudbuild_trigger) | resource |
| [google_compute_reservation.c2standard60_us_west4_c](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_reservation) | resource |
| [google_compute_reservation.n1standard8_with_tesla_t4_europe_west1_d](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_reservation) | resource |
| [external_external.list_tests_midnight](https://registry.terraform.io/providers/hashicorp/external/latest/docs/data-sources/external) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_daily_tests_project_id"></a> [daily\_tests\_project\_id](#input\_daily\_tests\_project\_id) | The GCP project for daily tests | `string` | `"hpc-toolkit-dev-2"` | no |
| <a name="input_daily_tests_service_account"></a> [daily\_tests\_service\_account](#input\_daily\_tests\_service\_account) | The service account to run daily tests under. If null, the default Cloud Build service account is used. For projects enforcing BYOSA (like hpc-toolkit-dev-2), you must set this via environment variable, e.g. export TF\_VAR\_daily\_tests\_service\_account="projects/..." | `string` | `null` | no |
| <a name="input_kueue_migrated_tests"></a> [kueue\_migrated\_tests](#input\_kueue\_migrated\_tests) | List of tests migrated to Kueue | `list(string)` | <pre>[<br/>  "slurm-gcp-v6-rocky8",<br/>  "batch-mpi",<br/>  "htcondor",<br/>  "packer",<br/>  "monitoring",<br/>  "chrome-remote-desktop",<br/>  "chrome-remote-desktop-ubuntu",<br/>  "ansible-vm",<br/>  "e2e",<br/>  "hcls",<br/>  "slurm-gke",<br/>  "slurm-flex",<br/>  "ml-slurm",<br/>  "htc-slurm",<br/>  "hpc-build-slurm-image",<br/>  "hpc-enterprise-slurm",<br/>  "spack-gromacs",<br/>  "gcluster-dockerfile",<br/>  "gke",<br/>  "gke-inactive-reservation",<br/>  "ml-gke",<br/>  "ml-gke-e2e",<br/>  "gke-storage",<br/>  "gke-managed-hyperdisk",<br/>  "slurm-rapid-storage",<br/>  "gke-managed-lustre",<br/>  "pfs-managed-lustre-slurm",<br/>  "pfs-managed-lustre-vm",<br/>  "netapp-volumes",<br/>  "slurm-gcp-v6-reconfig-size",<br/>  "slurm-gcp-v6-simple-job-completion",<br/>  "slurm-gcp-v6-startup-scripts",<br/>  "slurm-gcp-v6-topology",<br/>  "slurm-gcp-v6-debian",<br/>  "slurm-gcp-v6-ubuntu",<br/>  "slurm-gcp-v6-ssd",<br/>  "gke-a2-highgpu-kueue-onspot",<br/>  "gke-a4-onspot",<br/>  "gke-g4-onspot",<br/>  "gke-h4d-onspot",<br/>  "ml-h4d-onspot-slurm",<br/>  "h4d-vm",<br/>  "ml-a3-highgpu-onspot-slurm",<br/>  "ml-a4-highgpu-onspot-slurm",<br/>  "ml-g4-onspot-slurm",<br/>  "gke-a3-highgpu-onspot",<br/>  "gke-a3-megagpu-onspot",<br/>  "gke-tpu-v6e",<br/>  "ml-a3-megagpu-onspot-slurm-ubuntu",<br/>  "gke-a3-ultragpu-onspot",<br/>  "ml-a3-ultragpu-onspot-slurm",<br/>  "ml-a3-ultragpu-onspot-jbvms",<br/>  "gke-a4x",<br/>  "ml-a4x-highgpu-slurm",<br/>  "batch",<br/>  "gke-g4-confidential",<br/>  "gke-tpu-v5e",<br/>  "gke-tpu-7x",<br/>  "gke-tpu-v5p"<br/>]</pre> | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID | `string` | `"hpc-toolkit-dev"` | no |
| <a name="input_region"></a> [region](#input\_region) | GCP region | `string` | `"us-central1"` | no |
| <a name="input_repo_uri"></a> [repo\_uri](#input\_repo\_uri) | URI of GitHub repo | `string` | `"https://github.com/GoogleCloudPlatform/cluster-toolkit"` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | GCP zone | `string` | `"us-central1-c"` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
