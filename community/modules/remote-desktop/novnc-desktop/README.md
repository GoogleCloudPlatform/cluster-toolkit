## Description

Creates a VM that serves per-user XFCE desktops through noVNC.

This is the standalone-host wrapper for
`community/modules/remote-desktop/novnc-runtime`. It follows the standard CTK
composition pattern:

- runtime module
- `modules/scripts/startup-script`
- `modules/compute/vm-instance`

## Use when

Use this module when the desktop should run on its own VM rather than on an
existing host such as a Slurm login node.

## Key inputs

- `instance_image`
- `machine_type`
- `network_self_link` / `subnetwork_self_link` or `network_interfaces`
- `network_storage`
- `desktop_endpoint_dir`
- `desktop_endpoint_name`
- `novnc_proxy_secret` or `novnc_proxy_secret_id` (exactly one)
- `novnc_identity_mode`
- `slurm_auth_mode`
- `vnc_backend`
- `enable_gpu_acceleration` (hardware OpenGL; needs `turbovnc` - see [internal/desktop-broker](../../internal/desktop-broker/README.md))

## Outputs

- `startup_script`
- `instance_name`
- `internal_ip`
- `external_ip`
- `novnc_listen_port`
- `novnc_proxy_secret` / `novnc_proxy_secret_id`
- `healthcheck_path`
- `iap_tunnel_command`

## Example

```yaml
- id: desktop
  source: community/modules/remote-desktop/novnc-desktop
  use: [network]
  settings:
    enable_public_ips: false
    vnc_backend: turbovnc
    novnc_proxy_secret_id: ghpc-desktop-proxy-secret
```

## Access

The supported production model is an authenticated reverse proxy in front of the
broker. One of `novnc_proxy_secret` or `novnc_proxy_secret_id` is required, and the secret must be sent in
`X-Cluster-Desktop-Secret` on every request.

`novnc_identity_mode` is `trusted_proxy`, the only mode implemented: the proxy
injects `X-Cluster-Desktop-Email` and the broker trusts it unverified. That is
only safe where the proxy is the sole route to the broker port. See the
[novnc-runtime README](../novnc-runtime/README.md) for what that implies.

For OFE-managed access, keep `enable_public_ips` disabled and use OFE as the
only public HTTPS entrypoint.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |

## Modules

| Name | Source | Version |
| ---- | ------ | ------- |
| <a name="module_client_startup_script"></a> [client\_startup\_script](#module\_client\_startup\_script) | ../../../../modules/scripts/startup-script | n/a |
| <a name="module_instances"></a> [instances](#module\_instances) | ../../../../modules/compute/vm-instance | n/a |
| <a name="module_novnc_runtime"></a> [novnc\_runtime](#module\_novnc\_runtime) | ../novnc-runtime | n/a |

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_firewall.novnc_ingress](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_firewall) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_add_deployment_name_before_prefix"></a> [add\_deployment\_name\_before\_prefix](#input\_add\_deployment\_name\_before\_prefix) | If true, names are prefixed with deployment\_name for uniqueness. | `bool` | `true` | no |
| <a name="input_allowed_ingress_cidrs"></a> [allowed\_ingress\_cidrs](#input\_allowed\_ingress\_cidrs) | Private CIDR ranges allowed to reach the noVNC desktop broker listener. | `list(string)` | <pre>[<br/>  "10.0.0.0/8",<br/>  "172.16.0.0/12",<br/>  "192.168.0.0/16"<br/>]</pre> | no |
| <a name="input_auto_delete_boot_disk"></a> [auto\_delete\_boot\_disk](#input\_auto\_delete\_boot\_disk) | Controls if boot disk should be auto-deleted when instance is deleted. | `bool` | `true` | no |
| <a name="input_bandwidth_tier"></a> [bandwidth\_tier](#input\_bandwidth\_tier) | Bandwidth tier to use for the instance. | `string` | `"not_enabled"` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | Cluster Toolkit deployment name. | `string` | n/a | yes |
| <a name="input_desktop_endpoint_dir"></a> [desktop\_endpoint\_dir](#input\_desktop\_endpoint\_dir) | Optional directory where the runtime publishes DESKTOP\_* endpoint metadata for service discovery. | `string` | `null` | no |
| <a name="input_desktop_endpoint_name"></a> [desktop\_endpoint\_name](#input\_desktop\_endpoint\_name) | Endpoint metadata file name used when desktop\_endpoint\_dir is set. | `string` | `"desktop"` | no |
| <a name="input_disk_size_gb"></a> [disk\_size\_gb](#input\_disk\_size\_gb) | Size of disk for instances. | `number` | `100` | no |
| <a name="input_disk_type"></a> [disk\_type](#input\_disk\_type) | Disk type for instances. | `string` | `"pd-balanced"` | no |
| <a name="input_enable_gpu_acceleration"></a> [enable\_gpu\_acceleration](#input\_enable\_gpu\_acceleration) | Use hardware OpenGL for desktop sessions. Requires an instance\_image with an<br/>NVIDIA driver and a GPU on the machine type. See the novnc-runtime module. | `bool` | `false` | no |
| <a name="input_enable_oslogin"></a> [enable\_oslogin](#input\_enable\_oslogin) | Enable or Disable OS Login with ENABLE or DISABLE. | `string` | `"ENABLE"` | no |
| <a name="input_enable_public_ips"></a> [enable\_public\_ips](#input\_enable\_public\_ips) | If true, instances receive public IPs. | `bool` | `false` | no |
| <a name="input_guest_accelerator"></a> [guest\_accelerator](#input\_guest\_accelerator) | List of the type and count of accelerator cards attached to the instance. | <pre>list(object({<br/>    type  = string<br/>    count = number<br/>  }))</pre> | `[]` | no |
| <a name="input_install_root"></a> [install\_root](#input\_install\_root) | Directory under which noVNC assets are installed. | `string` | `"/opt/ghpc-remote-desktop"` | no |
| <a name="input_instance_count"></a> [instance\_count](#input\_instance\_count) | Number of instances. | `number` | `1` | no |
| <a name="input_instance_image"></a> [instance\_image](#input\_instance\_image) | Image used to build the noVNC desktop node.<br/><br/>Expected fields:<br/>name: The name of the image. Mutually exclusive with family.<br/>family: The image family to use. Mutually exclusive with name.<br/>project: The project where the image is hosted. | `map(string)` | <pre>{<br/>  "name": "debian-12-bookworm-v20250610",<br/>  "project": "debian-cloud"<br/>}</pre> | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. | `map(string)` | `{}` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | Machine type to use for desktop instance creation. | `string` | `"e2-standard-4"` | no |
| <a name="input_max_user_sessions"></a> [max\_user\_sessions](#input\_max\_user\_sessions) | Maximum number of concurrent per-user desktop sessions the desktop host will support. | `number` | `32` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Metadata provided as a map. | `map(string)` | `{}` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Optional name prefix for VM resources. | `string` | `"desktop"` | no |
| <a name="input_network_interfaces"></a> [network\_interfaces](#input\_network\_interfaces) | Explicit network interfaces. If set, network\_self\_link and subnetwork\_self\_link are ignored by the VM module. | <pre>list(object({<br/>    network            = string,<br/>    subnetwork         = string,<br/>    subnetwork_project = string,<br/>    network_ip         = string,<br/>    nic_type           = string,<br/>    stack_type         = string,<br/>    queue_count        = number,<br/>    access_config = list(object({<br/>      nat_ip                 = string,<br/>      public_ptr_domain_name = string,<br/>      network_tier           = string<br/>    })),<br/>    ipv6_access_config = list(object({<br/>      public_ptr_domain_name = string,<br/>      network_tier           = string<br/>    })),<br/>    alias_ip_range = list(object({<br/>      ip_cidr_range         = string,<br/>      subnetwork_range_name = string<br/>    }))<br/>  }))</pre> | `[]` | no |
| <a name="input_network_self_link"></a> [network\_self\_link](#input\_network\_self\_link) | The self link of the network to attach the VM. | `string` | `"default"` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. | <pre>list(object({<br/>    server_ip             = string<br/>    remote_mount          = string<br/>    local_mount           = string<br/>    fs_type               = string<br/>    mount_options         = string<br/>    client_install_runner = map(string)<br/>    mount_runner          = map(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_novnc_identity_mode"></a> [novnc\_identity\_mode](#input\_novnc\_identity\_mode) | How the desktop broker establishes which user a request belongs to. Only<br/>"trusted\_proxy" is supported: the identity is taken from request headers<br/>with no verification, so it is only safe where an authenticating proxy is<br/>the sole route to the broker. | `string` | `"trusted_proxy"` | no |
| <a name="input_novnc_listen_port"></a> [novnc\_listen\_port](#input\_novnc\_listen\_port) | Port exposed by the desktop broker for browser access to noVNC. | `number` | `6080` | no |
| <a name="input_novnc_proxy_secret"></a> [novnc\_proxy\_secret](#input\_novnc\_proxy\_secret) | Shared secret the desktop broker requires in the X-Cluster-Desktop-Secret<br/>header, supplied directly. Mutually exclusive with novnc\_proxy\_secret\_id.<br/><br/>Use this where the caller already owns the value and must hold the plaintext<br/>anyway - a front end that sends the header on every request gains nothing<br/>from a round trip through Secret Manager. Otherwise prefer the \_id form: a<br/>literal is carried in the startup script staged in Cloud Storage and appears<br/>in Terraform state. | `string` | `null` | no |
| <a name="input_novnc_proxy_secret_id"></a> [novnc\_proxy\_secret\_id](#input\_novnc\_proxy\_secret\_id) | Secret Manager secret ID holding the shared secret the desktop broker<br/>requires in the X-Cluster-Desktop-Secret header. Fetched on the instance at<br/>boot, so the value never enters Terraform state.<br/><br/>Create it with:<br/>  openssl rand -base64 48 \| gcloud secrets create SECRET\_ID --data-file=-<br/><br/>The instance service account needs roles/secretmanager.secretAccessor.<br/>Mutually exclusive with novnc\_proxy\_secret. | `string` | `null` | no |
| <a name="input_novnc_proxy_secret_version"></a> [novnc\_proxy\_secret\_version](#input\_novnc\_proxy\_secret\_version) | Version of novnc\_proxy\_secret\_id to read. | `string` | `"latest"` | no |
| <a name="input_novnc_version"></a> [novnc\_version](#input\_novnc\_version) | noVNC release version to install. | `string` | `"1.7.0"` | no |
| <a name="input_on_host_maintenance"></a> [on\_host\_maintenance](#input\_on\_host\_maintenance) | Describes maintenance behavior for the instance. | `string` | `"MIGRATE"` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which Google Cloud resources will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Default region for creating resources. | `string` | n/a | yes |
| <a name="input_secret_project_id"></a> [secret\_project\_id](#input\_secret\_project\_id) | Project holding the Secret Manager secrets below. Defaults to the deployment project. | `string` | `null` | no |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | Service account e-mail address to attach to the VM. | `string` | `null` | no |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Scopes to attach to the VM service account. | `set(string)` | <pre>[<br/>  "https://www.googleapis.com/auth/cloud-platform"<br/>]</pre> | no |
| <a name="input_session_idle_timeout_seconds"></a> [session\_idle\_timeout\_seconds](#input\_session\_idle\_timeout\_seconds) | Idle timeout after which a per-user desktop session is cleaned up. Set to 0 to disable cleanup. | `number` | `43200` | no |
| <a name="input_session_resolution"></a> [session\_resolution](#input\_session\_resolution) | Desktop resolution used for each per-user XFCE session. | `string` | `"1920x1080"` | no |
| <a name="input_slurm_auth_mode"></a> [slurm\_auth\_mode](#input\_slurm\_auth\_mode) | Optional Slurm authentication mode forwarded to novnc-runtime: none, munge, native, or auto. | `string` | `"none"` | no |
| <a name="input_spot"></a> [spot](#input\_spot) | Provision VMs using discounted Spot pricing. | `bool` | `false` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Optional additional startup script prepended before the desktop runtime setup. | `string` | `null` | no |
| <a name="input_subnetwork_self_link"></a> [subnetwork\_self\_link](#input\_subnetwork\_self\_link) | The self link of the subnetwork to attach the VM. | `string` | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Network tags applied to the VM. | `list(string)` | `[]` | no |
| <a name="input_threads_per_core"></a> [threads\_per\_core](#input\_threads\_per\_core) | Sets the number of threads per physical core. | `number` | `2` | no |
| <a name="input_vnc_backend"></a> [vnc\_backend](#input\_vnc\_backend) | VNC server backend used for per-user desktop sessions: tigervnc or turbovnc. | `string` | `"tigervnc"` | no |
| <a name="input_vnc_display_number"></a> [vnc\_display\_number](#input\_vnc\_display\_number) | First X display number reserved for per-user VNC sessions. | `number` | `1` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Default zone for creating resources. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_external_ip"></a> [external\_ip](#output\_external\_ip) | External IP addresses of created desktop instances, if enabled. |
| <a name="output_healthcheck_path"></a> [healthcheck\_path](#output\_healthcheck\_path) | Relative noVNC path that should respond when the broker is healthy. |
| <a name="output_iap_tunnel_command"></a> [iap\_tunnel\_command](#output\_iap\_tunnel\_command) | Example SSH tunnel command for low-level broker access through IAP. |
| <a name="output_instance_name"></a> [instance\_name](#output\_instance\_name) | Name of the first instance created, if any. |
| <a name="output_internal_ip"></a> [internal\_ip](#output\_internal\_ip) | Internal IP addresses of created desktop instances. |
| <a name="output_novnc_listen_port"></a> [novnc\_listen\_port](#output\_novnc\_listen\_port) | Port exposed by the desktop broker for noVNC browser access. |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | Script to load and run all desktop runtime runners. |
| <a name="output_vnc_backend"></a> [vnc\_backend](#output\_vnc\_backend) | VNC backend used by the desktop host. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
