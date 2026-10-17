## Description

### Implementation

The desktop runtime, the per-user broker and the VNC installers live in
[internal/desktop-broker](../../internal/desktop-broker/README.md). This module
is the public interface onto it; the broker package and its pytest suite are
documented there.

Builds the startup-script runners for a per-user XFCE + noVNC desktop runtime.
This module does not create any VM resources.

Use it to layer the desktop runtime onto an existing host, such as a Slurm
login node.

## Includes

- `community/modules/internal/desktop-broker`
- noVNC
- the per-user desktop broker service
- optional endpoint metadata publishing
- optional Slurm auth cleanup for Native Auth hosts

## Session transport

Each user's Xvnc listens on a unix socket at
`/run/ghpc-remote-desktop/<uid>/vnc.sock`, in a directory owned by that user
with mode `0700`, and its TCP listener is disabled outright. The broker runs as
root, authenticates the request, and then relays the browser's websocket
straight to that socket.

There is no loopback TCP port for a display, which matters on a shared login
node: a TCP port on `127.0.0.1` is reachable by every local user, whereas the
socket is gated by the kernel on `connect()`.

A consequence worth knowing: an off-host VNC client running on another VM
cannot reach these displays at all. A client on the desktop host itself can,
since it can open the socket. Supporting a genuinely off-host consumer would
mean an authenticated TCP mode, which means extending the backend classes in
`files/desktop_broker/backends/`.

## Key inputs

- `startup_script`
  - optional script run before the desktop runtime setup
- `network_storage`
  - storage mounts required before the desktop starts
- `slurm_auth_mode`
  - `none`, `munge`, `native`, or `auto`
- `vnc_backend`
  - `tigervnc` or `turbovnc`
- `enable_gpu_acceleration`
  - hardware OpenGL via VirtualGL; requires `vnc_backend = "turbovnc"` and an
    image with a graphics-capable NVIDIA driver. Falls back to software
    otherwise. See `internal/desktop-broker`.
- `desktop_endpoint_dir`
  - optional directory for `<desktop_endpoint_name>.env`
- `desktop_endpoint_name`
  - logical endpoint name
- `novnc_proxy_secret` or `novnc_proxy_secret_id`
  - **exactly one required.** Shared secret for the
    `X-Cluster-Desktop-Secret` header; the broker refuses to start without
    one. Prefer the `_id` form, which keeps the value out of Terraform state.
- `novnc_identity_mode`
  - `trusted_proxy` (the only mode implemented)

## Identity

Only `trusted_proxy` is implemented. The broker checks the shared secret in
`X-Cluster-Desktop-Secret` and then takes the user's identity from
`X-Cluster-Desktop-Email`, with `X-Cluster-Desktop-Username` naming the POSIX
account and `X-Cluster-Desktop-Login-Uid` supplying the OS Login subject when
the caller knows it. None of that is verified.

That is only safe where an authenticating proxy is the **sole** route to the
broker port, because anything able to reach it and present the shared secret can
claim to be any user. The broker logs a warning at startup saying exactly this.

Reached over an SSH tunnel with no proxy in front, as the example blueprints do,
the identity headers have to be supplied by hand - so a browser alone cannot
open a desktop. Treat those blueprints as validation, not a production posture.

The identity layer is one module per trust model returning a single shape, so a
verified mode is a new file plus one entry in `_RESOLVERS`. Nothing else in the
broker changes.

## Outputs

- `startup_runners`
- `novnc_listen_port`
- `novnc_proxy_secret_id`
- `healthcheck_path`

## Example

```yaml
- id: login-desktop-runtime
  source: community/modules/remote-desktop/novnc-runtime
  settings:
    slurm_auth_mode: auto
    vnc_backend: tigervnc
    desktop_endpoint_dir: /shared/ghpc-remote-desktop/endpoints
    desktop_endpoint_name: login
    novnc_proxy_secret_id: $(vars.desktop_proxy_secret)
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |

## Providers

No providers.

## Modules

| Name | Source | Version |
| ---- | ------ | ------- |
| <a name="module_desktop_broker"></a> [desktop\_broker](#module\_desktop\_broker) | ../../internal/desktop-broker | n/a |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_desktop_endpoint_dir"></a> [desktop\_endpoint\_dir](#input\_desktop\_endpoint\_dir) | Optional directory where the runtime publishes DESKTOP\_* endpoint metadata for service discovery. | `string` | `null` | no |
| <a name="input_desktop_endpoint_name"></a> [desktop\_endpoint\_name](#input\_desktop\_endpoint\_name) | Endpoint metadata file name used when desktop\_endpoint\_dir is set. | `string` | `"desktop"` | no |
| <a name="input_enable_gpu_acceleration"></a> [enable\_gpu\_acceleration](#input\_enable\_gpu\_acceleration) | Use hardware OpenGL for desktop sessions. Requires a GPU on the host and an<br/>image whose NVIDIA driver has graphics (not merely compute) support; falls<br/>back to software rendering otherwise. Requires vnc\_backend = "turbovnc".<br/>See community/examples/remote-desktop/README.md for which image to use. | `bool` | `false` | no |
| <a name="input_install_root"></a> [install\_root](#input\_install\_root) | Directory under which noVNC assets are installed. | `string` | `"/opt/ghpc-remote-desktop"` | no |
| <a name="input_max_user_sessions"></a> [max\_user\_sessions](#input\_max\_user\_sessions) | Maximum number of concurrent per-user desktop sessions the host VM will support. | `number` | `32` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. | <pre>list(object({<br/>    server_ip             = string<br/>    remote_mount          = string<br/>    local_mount           = string<br/>    fs_type               = string<br/>    mount_options         = string<br/>    client_install_runner = map(string)<br/>    mount_runner          = map(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_novnc_identity_mode"></a> [novnc\_identity\_mode](#input\_novnc\_identity\_mode) | How the desktop broker establishes which user a request belongs to. Only<br/>"trusted\_proxy" is supported: the identity is taken from request headers<br/>with no verification, so it is only safe where an authenticating proxy is<br/>the sole route to the broker. | `string` | `"trusted_proxy"` | no |
| <a name="input_novnc_listen_port"></a> [novnc\_listen\_port](#input\_novnc\_listen\_port) | Port exposed by the desktop broker for browser access to noVNC. | `number` | `6080` | no |
| <a name="input_novnc_proxy_secret"></a> [novnc\_proxy\_secret](#input\_novnc\_proxy\_secret) | Shared secret the desktop broker requires in the X-Cluster-Desktop-Secret<br/>header, supplied directly. Mutually exclusive with novnc\_proxy\_secret\_id.<br/><br/>Use this where the caller already owns the value and must hold the plaintext<br/>anyway - a front end that sends the header on every request gains nothing<br/>from a round trip through Secret Manager. Otherwise prefer the \_id form: a<br/>literal is carried in the startup script staged in Cloud Storage and appears<br/>in Terraform state. | `string` | `null` | no |
| <a name="input_novnc_proxy_secret_id"></a> [novnc\_proxy\_secret\_id](#input\_novnc\_proxy\_secret\_id) | Secret Manager secret ID holding the shared secret the desktop broker<br/>requires in the X-Cluster-Desktop-Secret header. Fetched on the instance at<br/>boot, so the value never enters Terraform state or the startup script staged<br/>in Cloud Storage.<br/><br/>Create it with:<br/>  openssl rand -base64 48 \| gcloud secrets create SECRET\_ID --data-file=-<br/><br/>The instance service account needs roles/secretmanager.secretAccessor.<br/>Mutually exclusive with novnc\_proxy\_secret. | `string` | `null` | no |
| <a name="input_novnc_proxy_secret_version"></a> [novnc\_proxy\_secret\_version](#input\_novnc\_proxy\_secret\_version) | Version of novnc\_proxy\_secret\_id to read. | `string` | `"latest"` | no |
| <a name="input_novnc_version"></a> [novnc\_version](#input\_novnc\_version) | noVNC release version to install. | `string` | `"1.7.0"` | no |
| <a name="input_secret_project_id"></a> [secret\_project\_id](#input\_secret\_project\_id) | Project holding the Secret Manager secrets below. Defaults to the instance's own project. | `string` | `null` | no |
| <a name="input_session_idle_timeout_seconds"></a> [session\_idle\_timeout\_seconds](#input\_session\_idle\_timeout\_seconds) | Idle timeout after which a per-user desktop session is cleaned up. Set to 0 to disable cleanup. | `number` | `43200` | no |
| <a name="input_session_resolution"></a> [session\_resolution](#input\_session\_resolution) | Desktop resolution used for each per-user XFCE session. | `string` | `"1920x1080"` | no |
| <a name="input_slurm_auth_mode"></a> [slurm\_auth\_mode](#input\_slurm\_auth\_mode) | Optional Slurm authentication mode for hosts that also participate in a Slurm cluster.<br/><br/>Supported values:<br/>- none:   no Slurm-specific handling<br/>- munge:  host is expected to use MUNGE authentication<br/>- native: host is expected to use Slurm Native Authentication and the runtime disables stale munge.service state<br/>- auto:   detect AuthType from slurm.conf at startup and apply the Native Auth cleanup only when needed | `string` | `"none"` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Optional additional startup script prepended before the desktop runtime setup. | `string` | `null` | no |
| <a name="input_vnc_backend"></a> [vnc\_backend](#input\_vnc\_backend) | VNC server backend used for per-user desktop sessions: tigervnc or turbovnc. | `string` | `"tigervnc"` | no |
| <a name="input_vnc_display_number"></a> [vnc\_display\_number](#input\_vnc\_display\_number) | First X display number reserved for per-user VNC sessions. | `number` | `1` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_healthcheck_path"></a> [healthcheck\_path](#output\_healthcheck\_path) | Unauthenticated broker path that responds once the broker is running. |
| <a name="output_novnc_listen_port"></a> [novnc\_listen\_port](#output\_novnc\_listen\_port) | Port exposed by the desktop broker for noVNC browser access. |
| <a name="output_startup_runners"></a> [startup\_runners](#output\_startup\_runners) | Startup-script runners required to install and launch the noVNC desktop runtime. |
| <a name="output_vnc_backend"></a> [vnc\_backend](#output\_vnc\_backend) | VNC backend used by the noVNC desktop runtime. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
