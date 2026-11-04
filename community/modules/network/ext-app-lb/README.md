## Description

Creates the front end of a global external Application Load Balancer: its
address, certificate, URL map and forwarding rules. It routes each hostname to a
backend service created elsewhere, such as by
[iap-backend-service](../iap-backend-service/README.md), so one front end can
serve backends owned by several deployments.

This is the external, global counterpart of
[int-app-lb](../int-app-lb/README.md), which creates a regional internal load
balancer together with its own backends.

It wraps [terraform-google-lb-http's frontend module][lb-http-frontend].

[lb-http-frontend]: https://github.com/terraform-google-modules/terraform-google-lb-http/tree/main/modules/frontend

### Certificates

HTTPS is enabled when one of these is set, and HTTP then redirects to it
(`https_redirect`):

- `certificate_map`: a Certificate Manager map, such as
  [managed-certificate](../managed-certificate/README.md)'s. The only option
  for a wildcard certificate.
- `managed_ssl_certificate_domains`: a Google-managed certificate created here.
  It is issued only once every domain resolves to the load balancer.
- `ssl_certificates`: existing certificates, by self link.

Without any of them the load balancer serves HTTP only, which Identity-Aware
Proxy does not support.

The HTTPS proxy accepts TLS 1.2 and later, through an SSL policy created here
with the `MODERN` profile. Set `min_tls_version` to change that, or
`ssl_policy` to use an existing policy instead.

Choose between `certificate_map` and the other two when the load balancer is
created. Moving an existing load balancer from `ssl_certificates` or
`managed_ssl_certificate_domains` to `certificate_map` fails, because the
Google provider clears the proxy's certificates before attaching the map, which
the API refuses. Destroy the load balancer and deploy it again instead.

### Address

Pass a reserved global address, such as one from
[global-static-ip](../../../../modules/network/global-static-ip/README.md) or
`gcloud compute addresses create NAME --global`, so DNS records survive the load
balancer being recreated. Leave `address` null to reserve one with the load
balancer.

### Routes

Each route sends a hostname to a backend service. A URL map needs a default
backend service, so `routes` cannot be empty: destroy the load balancer while
there is nothing to route to. Requests for a hostname without a route go to the
route whose `host` is `*`, or without one, to the alphabetically first host's.

### Example

A wildcard certificate and one hostname per desktop:

```yaml
- id: desktop-certificate
  source: community/modules/network/managed-certificate
  settings:
    name: $(vars.deployment_name)-desktops
    domains: ["*.$(vars.desktop_domain)"]

- id: desktop-lb
  source: community/modules/network/ext-app-lb
  settings:
    name: $(vars.deployment_name)-desktops
    address: $(vars.desktop_address)
    certificate_map: $(desktop-certificate.certificate_map)
    routes:
    - host: login.$(vars.desktop_domain)
      backend_service: $(login-desktop-backend.backend_service_self_link)
    - host: viz.$(vars.desktop_domain)
      backend_service: $(viz-desktop-backend.backend_service_self_link)
```

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
| <a name="module_frontend"></a> [frontend](#module\_frontend) | GoogleCloudPlatform/lb-http/google//modules/frontend | ~> 14.3 |

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_ssl_policy.policy](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_ssl_policy) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_address"></a> [address](#input\_address) | A reserved global external IPv4 address for the load balancer: the address<br/>itself, not its name. Leave null to reserve one, which is released with the<br/>load balancer, so DNS records pointing at it must then be updated. | `string` | `null` | no |
| <a name="input_certificate_map"></a> [certificate\_map](#input\_certificate\_map) | ID of a Certificate Manager certificate map, such as managed-certificate's<br/>certificate\_map. Use it for a wildcard certificate. Must be known at plan<br/>time, as whether to serve HTTPS depends on it. | `string` | `null` | no |
| <a name="input_https_redirect"></a> [https\_redirect](#input\_https\_redirect) | Whether to redirect HTTP requests to HTTPS. Ignored without a certificate, as the load balancer then serves HTTP only. | `bool` | `true` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the resources. | `map(string)` | `{}` | no |
| <a name="input_managed_ssl_certificate_domains"></a> [managed\_ssl\_certificate\_domains](#input\_managed\_ssl\_certificate\_domains) | Domains for a Google-managed Compute Engine certificate, created here. It is<br/>issued once each domain resolves to the load balancer. It cannot cover a<br/>wildcard; use certificate\_map for one. | `list(string)` | `[]` | no |
| <a name="input_min_tls_version"></a> [min\_tls\_version](#input\_min\_tls\_version) | Lowest TLS version the HTTPS proxy accepts, in the SSL policy created when ssl\_policy is null. | `string` | `"TLS_1_2"` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the load balancer's forwarding rules, and prefix of its other resources. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the load balancer is created. Must be its backend services' project. | `string` | n/a | yes |
| <a name="input_routes"></a> [routes](#input\_routes) | Hostnames and the backend services that serve them, by self link, such as<br/>iap-backend-service's backend\_service\_self\_link. paths defaults to every<br/>path. Requests for any other hostname go to the route whose host is "*",<br/>or without one, to the alphabetically first host's backend service. | <pre>list(object({<br/>    host            = string<br/>    backend_service = string<br/>    paths           = optional(list(string), ["/*"])<br/>  }))</pre> | n/a | yes |
| <a name="input_ssl_certificates"></a> [ssl\_certificates](#input\_ssl\_certificates) | Self links of existing Compute Engine SSL certificates. | `list(string)` | `[]` | no |
| <a name="input_ssl_policy"></a> [ssl\_policy](#input\_ssl\_policy) | Self link of an existing SSL policy for the HTTPS proxy. Null creates one from min\_tls\_version. | `string` | `null` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_ip_address"></a> [ip\_address](#output\_ip\_address) | The load balancer's external IPv4 address. Point each route's hostname at it. |
| <a name="output_url_map"></a> [url\_map](#output\_url\_map) | Self link of the load balancer's URL map. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
