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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 4.0 |
| <a name="requirement_google-beta"></a> [google-beta](#requirement\_google-beta) | ~> 4.0 |
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 4.0 |
| <a name="provider_null"></a> [null](#provider\_null) | ~> 3.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_control_bucket"></a> [control\_bucket](#module\_control\_bucket) | terraform-google-modules/cloud-storage/google | ~> 4.0 |
| <a name="module_network"></a> [network](#module\_network) | ./network | n/a |
| <a name="module_pubsub"></a> [pubsub](#module\_pubsub) | terraform-google-modules/pubsub/google | ~> 5.0 |
| <a name="module_service_account"></a> [service\_account](#module\_service\_account) | terraform-google-modules/service-accounts/google | ~> 4.1 |

## Resources

| Name | Type |
|------|------|
| [google_compute_instance.server_vm](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance) | resource |
| [google_logging_project_sink.build_sink](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/logging_project_sink) | resource |
| [google_pubsub_subscription.cloud_build_logs_sub](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/pubsub_subscription) | resource |
| [google_pubsub_topic.cloud_build_logs](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/pubsub_topic) | resource |
| [google_pubsub_topic_iam_member.build_sink_pub](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/pubsub_topic_iam_member) | resource |
| [google_storage_bucket_object.config_file](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.deployment_file](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [null_resource.uploader](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_key"></a> [deployment\_key](#input\_deployment\_key) | Name to identify resources from this deployment | `string` | `""` | no |
| <a name="input_deployment_mode"></a> [deployment\_mode](#input\_deployment\_mode) | Use a tarball of this directory, or download from git to deploy the server. Must be either 'tarball' or 'git' | `string` | `"tarball"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Base "name" for the deployment. | `string` | n/a | yes |
| <a name="input_django_su_email"></a> [django\_su\_email](#input\_django\_su\_email) | DJango Admin SuperUser email | `string` | n/a | yes |
| <a name="input_django_su_password"></a> [django\_su\_password](#input\_django\_su\_password) | DJango Admin SuperUser password | `string` | n/a | yes |
| <a name="input_django_su_username"></a> [django\_su\_username](#input\_django\_su\_username) | DJango Admin SuperUser username | `string` | `"admin"` | no |
| <a name="input_extra_labels"></a> [extra\_labels](#input\_extra\_labels) | Extra labels to apply to created GCP resources. | `map(any)` | `{}` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP Project in which to deploy the HPC Frontend. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | GCP Region for HPC Frontend deployment. | `string` | n/a | yes |
| <a name="input_repo_branch"></a> [repo\_branch](#input\_repo\_branch) | git branch to checkout when deploying the HPC Frontend | `string` | `"main"` | no |
| <a name="input_repo_fork"></a> [repo\_fork](#input\_repo\_fork) | GitHub repository name in which to find the cluster-toolkit repo | `string` | `"GoogleCloudPlatform"` | no |
| <a name="input_server_instance_type"></a> [server\_instance\_type](#input\_server\_instance\_type) | Instance size to use from HPC Frontend webserver | `string` | `"e2-standard-2"` | no |
| <a name="input_static_ip"></a> [static\_ip](#input\_static\_ip) | Optional pre-configured static IP for HPC Frontend. | `string` | `""` | no |
| <a name="input_subnet"></a> [subnet](#input\_subnet) | Subnet in which to deploy HPC Frontend. | `string` | `""` | no |
| <a name="input_webserver_hostname"></a> [webserver\_hostname](#input\_webserver\_hostname) | DNS Hostname for the webserver | `string` | `""` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | GCP Zone for HPC Frontend deployment. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_server_ip"></a> [server\_ip](#output\_server\_ip) | Webserver IP Address |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
