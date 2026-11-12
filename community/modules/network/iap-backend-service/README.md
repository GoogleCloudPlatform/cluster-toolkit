## Description

Creates a backend service behind Identity-Aware Proxy (IAP) for a global
external Application Load Balancer, and attaches desktop hosts to it.

The load balancer's front end (address, certificate and URL map) is not created
here: route a hostname to `backend_service_self_link` from one you manage, such
as [ext-app-lb](../ext-app-lb/README.md) in the same blueprint. Keeping the two
apart lets one front end serve the desktops of several deployments.

The backend service, health check and IAP access come from
[terraform-google-lb-http's backend module][lb-http-backend]. This module adds a
zonal network endpoint group, so the backend service does not depend on the
hosts. That matters because each host's desktop broker needs the backend
service's name to verify IAP assertions.

Pass the same `name` to the desktop module's `iap_backend_service`, and admit
Google's front ends (`130.211.0.0/22`, `35.191.0.0/16`) to the broker port with
[modules/network/firewall-rules](../../../../modules/network/firewall-rules).
Other VMs in the network may reach the port as well, under its internal traffic
rule; the broker refuses any request without a verified IAP assertion.

Each broker looks its backend service up by name, so the hosts' service account
needs `compute.backendServices.get`, such as through `roles/compute.viewer`.
The default compute service account has it only through the Editor role, which
organization policy may keep it from being granted.

IAP decides who reaches the hosts. OS Login, which the broker asks on every
hand-off, still decides who gets a desktop. Grant `iap_members` both
`roles/iap.httpsResourceAccessor` (done here) and `roles/compute.osLogin`.

[lb-http-backend]: https://github.com/terraform-google-modules/terraform-google-lb-http/tree/main/modules/backend

### Example

```yaml
- id: login-desktop-backend
  source: community/modules/network/iap-backend-service
  use: [network]
  settings:
    name: $(vars.login_desktop_backend)
    endpoints:
    - instance: $(slurm-controller.slurm_login_instances[0].name)
      ip_address: $(slurm-controller.slurm_login_instances[0].network_interface[0].network_ip)
    iap_members: $(vars.desktop_iap_members)
```

See
[hpc-slurm-remote-desktop-iap.yaml](../../../examples/remote-desktop/hpc-slurm-remote-desktop-iap.yaml)
for a complete blueprint.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.9.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.9.0 |

## Modules

| Name | Source | Version |
| ---- | ------ | ------- |
| <a name="module_backend"></a> [backend](#module\_backend) | GoogleCloudPlatform/lb-http/google//modules/backend | ~> 14.3 |

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_network_endpoint.desktops](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_network_endpoint) | resource |
| [google_compute_network_endpoint_group.desktops](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_network_endpoint_group) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_endpoints"></a> [endpoints](#input\_endpoints) | Desktop instances to serve, by name and primary internal IP, such as<br/>novnc-desktop's network\_endpoints. Every instance must be in zone. May be<br/>empty, for endpoints attached later. | <pre>list(object({<br/>    instance   = string<br/>    ip_address = string<br/>  }))</pre> | `[]` | no |
| <a name="input_health_check_path"></a> [health\_check\_path](#input\_health\_check\_path) | Path the load balancer's health check requests. The broker answers it without authentication. | `string` | `"/healthz"` | no |
| <a name="input_iap_members"></a> [iap\_members](#input\_iap\_members) | Principals allowed through Identity-Aware Proxy, such as<br/>"user:alice@example.com", "group:hpc@example.com" or "domain:example.com".<br/>They also need roles/compute.osLogin on the instances, or they get past IAP<br/>and are then refused a desktop. | `list(string)` | `[]` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the backend service, and of its endpoint group and health check.<br/>Pass the same name to the desktop modules' iap\_backend\_service: the brokers<br/>look the backend service up by it to verify IAP assertions. | `string` | n/a | yes |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | Network of the desktop instances. | `string` | n/a | yes |
| <a name="input_oauth2_client_id"></a> [oauth2\_client\_id](#input\_oauth2\_client\_id) | OAuth client for IAP. Leave null, with oauth2\_client\_secret, to use a Google-managed client, which admits only users in your organization. | `string` | `null` | no |
| <a name="input_oauth2_client_secret"></a> [oauth2\_client\_secret](#input\_oauth2\_client\_secret) | Secret of oauth2\_client\_id. | `string` | `null` | no |
| <a name="input_port"></a> [port](#input\_port) | TCP port the desktop brokers serve the load balancer on: the desktop modules' iap\_listen\_port. | `number` | `6080` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the backend service is created. Must be the load balancer's project. | `string` | n/a | yes |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | Subnetwork of the desktop instances. Optional when the network has one subnetwork in the zone's region. | `string` | `null` | no |
| <a name="input_timeout_sec"></a> [timeout\_sec](#input\_timeout\_sec) | Backend service timeout, which bounds how long a desktop's websocket can stay open. | `number` | `86400` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone of the desktop instances. Every instance in endpoints must be in it. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_backend_service_name"></a> [backend\_service\_name](#output\_backend\_service\_name) | Name of the backend service. Route a hostname to it in the load balancer's URL map. |
| <a name="output_backend_service_self_link"></a> [backend\_service\_self\_link](#output\_backend\_service\_self\_link) | Self link of the backend service, as a URL map's path matcher takes it. |
| <a name="output_network_endpoint_group"></a> [network\_endpoint\_group](#output\_network\_endpoint\_group) | Endpoint group the desktop instances are attached to. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
