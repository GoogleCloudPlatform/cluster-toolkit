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

## Description

This module creates an Internal Application (L7) Load Balancer in Google Cloud.
It provides advanced traffic management, support for regional backends (like Managed Instance Groups), and can be integrated with Identity-Aware Proxy (IAP) or custom SSL certificates.

This module is primarily intended for exposing the Slurm REST API (`slurmrestd`) in High-Availability setups, but is generic enough to route traffic for any internal L7 application.

### Example

```yaml
  - id: slurm_api_lb
    source: community/modules/network/int-app-lb
    use: [network]
    settings:
      project_id: $(vars.project_id)
      region: $(vars.region)
      backends:
      - $(slurm_controller.controller_instance_group)
      endpoints:
        slurm-restapi:
          port: 6842
          paths: ["/slurm/*", "/slurmdb/*"]
          health_check_type: "TCP"
      lb_name: $(vars.deployment_name)-api-lb
      ip_address: "10.80.0.100"
```

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 6.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | ~> 6.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_address.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_address) | resource |
| [google_compute_forwarding_rule.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_forwarding_rule) | resource |
| [google_compute_region_backend_service.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_region_backend_service) | resource |
| [google_compute_region_health_check.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_region_health_check) | resource |
| [google_compute_region_target_http_proxy.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_region_target_http_proxy) | resource |
| [google_compute_region_target_https_proxy.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_region_target_https_proxy) | resource |
| [google_compute_region_url_map.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_region_url_map) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_backends"></a> [backends](#input\_backends) | List of instance group self-links to use as backends for all endpoints. | `list(string)` | n/a | yes |
| <a name="input_connection_draining_timeout_sec"></a> [connection\_draining\_timeout\_sec](#input\_connection\_draining\_timeout\_sec) | Time (in seconds) to wait for connections to drain before dropping them during VM termination. | `number` | `30` | no |
| <a name="input_endpoints"></a> [endpoints](#input\_endpoints) | Map of endpoint names (used as named\_ports) to their listening port and URL paths. | <pre>map(object({<br/>    port                      = number<br/>    paths                     = list(string)<br/>    health_check_type         = optional(string, "HTTP")<br/>    health_check_request_path = optional(string, "/")<br/>  }))</pre> | n/a | yes |
| <a name="input_health_check_interval_sec"></a> [health\_check\_interval\_sec](#input\_health\_check\_interval\_sec) | How often (in seconds) to send a health check. | `number` | `10` | no |
| <a name="input_health_check_timeout_sec"></a> [health\_check\_timeout\_sec](#input\_health\_check\_timeout\_sec) | How long (in seconds) to wait before claiming failure. | `number` | `5` | no |
| <a name="input_healthy_threshold"></a> [healthy\_threshold](#input\_healthy\_threshold) | A consecutive number of successful checks before marking as healthy. | `number` | `2` | no |
| <a name="input_iap_config"></a> [iap\_config](#input\_iap\_config) | Settings for enabling Identity-Aware Proxy (IAP) on the backend service. | <pre>object({<br/>    oauth2_client_id     = string<br/>    oauth2_client_secret = string<br/>  })</pre> | `null` | no |
| <a name="input_ip_address"></a> [ip\_address](#input\_ip\_address) | The static internal IP address to assign to the forwarding rule. If null, one will be automatically assigned. | `string` | `null` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the resources. | `map(string)` | `{}` | no |
| <a name="input_lb_name"></a> [lb\_name](#input\_lb\_name) | The name for the Load Balancer resources. | `string` | `"internal-app-lb"` | no |
| <a name="input_log_config"></a> [log\_config](#input\_log\_config) | Logging configuration for this backend service. | <pre>object({<br/>    enable      = bool<br/>    sample_rate = optional(number, 1.0)<br/>  })</pre> | `null` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The network self-link to attach the load balancer to. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID to deploy to. | `string` | n/a | yes |
| <a name="input_protocol"></a> [protocol](#input\_protocol) | The protocol to use to talk to the backend. Can be HTTP, HTTPS, or HTTP2. | `string` | `"HTTP"` | no |
| <a name="input_region"></a> [region](#input\_region) | The region to deploy to. | `string` | n/a | yes |
| <a name="input_ssl_certificates"></a> [ssl\_certificates](#input\_ssl\_certificates) | List of SSL certificate self-links. If provided, the load balancer will be configured to use HTTPS. | `list(string)` | `[]` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The subnetwork self-link to attach the load balancer to. | `string` | n/a | yes |
| <a name="input_timeout_sec"></a> [timeout\_sec](#input\_timeout\_sec) | How long (in seconds) to wait for backend response. | `number` | `30` | no |
| <a name="input_unhealthy_threshold"></a> [unhealthy\_threshold](#input\_unhealthy\_threshold) | A consecutive number of failed checks before marking as unhealthy. | `number` | `3` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_forwarding_rule_id"></a> [forwarding\_rule\_id](#output\_forwarding\_rule\_id) | An identifier for the resource with format projects/{{project}}/regions/{{region}}/forwardingRules/{{name}} |
| <a name="output_forwarding_rule_ip"></a> [forwarding\_rule\_ip](#output\_forwarding\_rule\_ip) | The internal IP address of the forwarding rule. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
