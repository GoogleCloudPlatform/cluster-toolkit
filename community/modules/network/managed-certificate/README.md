## Description

Creates a Google-managed Certificate Manager certificate, authorised by DNS, and
a certificate map that serves it, for a global external Application Load
Balancer such as [ext-app-lb](../ext-app-lb/README.md).

DNS authorization lets the certificate cover a wildcard, such as
`*.desktops.example.com`, and be issued before any load balancer serves the
domain. For each domain, create the CNAME record in `dns_records`. The
certificate is issued once it resolves.

A wildcard covers one level of subdomain: `*.desktops.example.com` covers
`login.desktops.example.com`, not `desktops.example.com` or
`a.b.desktops.example.com`. A wildcard and its parent domain share one
authorization and one record.

### Map entries

The map has one entry per domain, matched by hostname, and no primary
(catch-all) entry. Identity-Aware Proxy refuses requests served through a
primary entry with a hostname and certificate mismatch. A client that does not
send a hostname (SNI) is therefore offered no certificate.

### Keep it apart from the load balancer

Recreating a DNS authorization changes its record, which then has to be updated
in DNS before the certificate can be renewed or reissued. Put this module in a
deployment group of its own when the load balancer is destroyed and recreated
more often than the certificate.

### Example

```yaml
- id: desktop-certificate
  source: community/modules/network/managed-certificate
  settings:
    name: $(vars.deployment_name)-desktops
    domains: ["*.$(vars.desktop_domain)"]
```

Pass `certificate_map` to [ext-app-lb](../ext-app-lb/README.md), and create the
records in `dns_records`.

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

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_certificate_manager_certificate.certificate](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/certificate_manager_certificate) | resource |
| [google_certificate_manager_certificate_map.map](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/certificate_manager_certificate_map) | resource |
| [google_certificate_manager_certificate_map_entry.domains](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/certificate_manager_certificate_map_entry) | resource |
| [google_certificate_manager_dns_authorization.domains](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/certificate_manager_dns_authorization) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_domains"></a> [domains](#input\_domains) | Domains the certificate covers, such as ["*.desktops.example.com"] or<br/>["login.example.com", "viz.example.com"]. A wildcard covers one level of<br/>subdomain only. | `list(string)` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the resources. | `map(string)` | `{}` | no |
| <a name="input_name"></a> [name](#input\_name) | Name of the certificate and its certificate map, and prefix of its DNS authorizations and map entries. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the certificate is created. Must be the load balancer's project. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_certificate_id"></a> [certificate\_id](#output\_certificate\_id) | ID of the certificate. |
| <a name="output_certificate_map"></a> [certificate\_map](#output\_certificate\_map) | ID of the certificate map, as a load balancer's target HTTPS proxy takes it. |
| <a name="output_dns_records"></a> [dns\_records](#output\_dns\_records) | The CNAME records that authorise the certificate, as {name, type, data}.<br/>Create them in the domains' DNS zones; the certificate is issued once they<br/>resolve. They change if the authorizations are recreated. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
