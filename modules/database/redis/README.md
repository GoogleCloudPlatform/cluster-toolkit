## Description

This module deploys a Google Cloud Memorystore for Redis instance.

## Example usage

```yaml
- id: redis
  source: modules/database/redis
  settings:
    project_id: "your-project-id"
    deployment_name: "my-deployment"
    environment: "dev"
    region: "us-central1"
    network_self_link: "projects/your-project-id/global/networks/your-network"
```

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
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_project_service.redis_api](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_service) | resource |
| [google_redis_instance.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/redis_instance) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_auth_enabled"></a> [auth\_enabled](#input\_auth\_enabled) | Indicates whether OSS Redis AUTH is enabled. | `bool` | `true` | no |
| <a name="input_connect_mode"></a> [connect\_mode](#input\_connect\_mode) | The connection mode of the Redis instance. | `string` | `"DIRECT_PEERING"` | no |
| <a name="input_deploy_redis"></a> [deploy\_redis](#input\_deploy\_redis) | Whether to deploy Redis. | `bool` | `true` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment. | `string` | n/a | yes |
| <a name="input_environment"></a> [environment](#input\_environment) | The environment name. | `string` | n/a | yes |
| <a name="input_memory_size_gb"></a> [memory\_size\_gb](#input\_memory\_size\_gb) | Redis memory size in GiB. | `number` | `2` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The VPC network to which the instance is connected. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID to deploy to. | `string` | n/a | yes |
| <a name="input_redis_version"></a> [redis\_version](#input\_redis\_version) | The version of Redis software. | `string` | `"REDIS_6_X"` | no |
| <a name="input_region"></a> [region](#input\_region) | The region to deploy Redis to. | `string` | n/a | yes |
| <a name="input_reserved_ip_range"></a> [reserved\_ip\_range](#input\_reserved\_ip\_range) | The name of the allocated IP range for the Private Service Access. | `string` | `null` | no |
| <a name="input_tier"></a> [tier](#input\_tier) | The service tier of the Redis instance. | `string` | `"BASIC"` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_auth_string"></a> [auth\_string](#output\_auth\_string) | The auth string (password) of the Redis instance. |
| <a name="output_redis_host"></a> [redis\_host](#output\_redis\_host) | The host of the Redis instance. |
| <a name="output_redis_port"></a> [redis\_port](#output\_redis\_port) | The port of the Redis instance. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
