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
- optional Slurm auth cleanup for Native Auth hosts

## Session transport

Each user's Xvnc listens on a unix socket at
`/run/ghpc-remote-desktop/<uid>/vnc.sock`, in a directory owned by that user
with mode `0700`, and its TCP listener is disabled outright. The broker runs as
root, identifies the user from its own socket (see [Access](#access)), and then
relays the browser's websocket straight to that socket.

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

## Access

The broker listens only on a unix socket, `/run/ghpc-remote-desktop/broker.sock`.
It has no TCP port, so there is nothing to open in a firewall and nothing for
another VM to reach. A user forwards a local port to the socket over SSH:

```bash
gcloud compute ssh HOST --tunnel-through-iap -- -N -L 6080:/run/ghpc-remote-desktop/broker.sock
```

and browses to `http://localhost:6080/vnc.html`.

The broker asks the kernel which account opened each connection
(`SO_PEERCRED`). sshd opens a forwarded connection as the user who logged in, so
that is the OS Login account SSH authenticated. Root and accounts OS Login does
not manage are refused. Nothing in the request affects who the user is, so there
is no shared secret to provision. On a shared login node, any local user may
connect to the socket, but each only ever gets their own desktop.

## Outputs

- `startup_runners`
- `broker_socket_path`
- `vnc_backend`

## Example

```yaml
- id: login-desktop-runtime
  source: community/modules/remote-desktop/novnc-runtime
  settings:
    slurm_auth_mode: auto
    vnc_backend: tigervnc
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
| <a name="input_enable_gpu_acceleration"></a> [enable\_gpu\_acceleration](#input\_enable\_gpu\_acceleration) | Use hardware OpenGL for desktop sessions. Requires a GPU on the host and an<br/>image whose NVIDIA driver has graphics (not merely compute) support; falls<br/>back to software rendering otherwise. Requires vnc\_backend = "turbovnc".<br/>See community/examples/remote-desktop/README.md for which image to use. | `bool` | `false` | no |
| <a name="input_install_root"></a> [install\_root](#input\_install\_root) | Directory under which noVNC assets are installed. | `string` | `"/opt/ghpc-remote-desktop"` | no |
| <a name="input_max_user_sessions"></a> [max\_user\_sessions](#input\_max\_user\_sessions) | Maximum number of concurrent per-user desktop sessions the host VM will support. | `number` | `32` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. | <pre>list(object({<br/>    server_ip             = string<br/>    remote_mount          = string<br/>    local_mount           = string<br/>    fs_type               = string<br/>    mount_options         = string<br/>    client_install_runner = map(string)<br/>    mount_runner          = map(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_novnc_version"></a> [novnc\_version](#input\_novnc\_version) | noVNC release version to install. | `string` | `"1.7.0"` | no |
| <a name="input_session_idle_timeout_seconds"></a> [session\_idle\_timeout\_seconds](#input\_session\_idle\_timeout\_seconds) | Idle timeout after which a per-user desktop session is cleaned up. Set to 0 to keep idle sessions; a session whose desktop has exited is still cleaned up. | `number` | `43200` | no |
| <a name="input_session_resolution"></a> [session\_resolution](#input\_session\_resolution) | Desktop resolution used for each per-user XFCE session. | `string` | `"1920x1080"` | no |
| <a name="input_slurm_auth_mode"></a> [slurm\_auth\_mode](#input\_slurm\_auth\_mode) | Optional Slurm authentication mode for hosts that also participate in a Slurm cluster.<br/><br/>Supported values:<br/>- none:   no Slurm-specific handling<br/>- munge:  host is expected to use MUNGE authentication<br/>- native: host is expected to use Slurm Native Authentication and the runtime disables stale munge.service state<br/>- auto:   detect AuthType from slurm.conf at startup and apply the Native Auth cleanup only when needed | `string` | `"none"` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Optional additional startup script prepended before the desktop runtime setup. | `string` | `null` | no |
| <a name="input_vnc_backend"></a> [vnc\_backend](#input\_vnc\_backend) | VNC server backend used for per-user desktop sessions: tigervnc or turbovnc. | `string` | `"tigervnc"` | no |
| <a name="input_vnc_display_number"></a> [vnc\_display\_number](#input\_vnc\_display\_number) | First X display number reserved for per-user VNC sessions. Starts high to stay clear of users' own X servers on a shared host. | `number` | `50` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_broker_socket_path"></a> [broker\_socket\_path](#output\_broker\_socket\_path) | Unix socket the desktop broker serves on. Forward a local port to it over SSH to reach a desktop. |
| <a name="output_startup_runners"></a> [startup\_runners](#output\_startup\_runners) | Startup-script runners required to install and launch the noVNC desktop runtime. |
| <a name="output_vnc_backend"></a> [vnc\_backend](#output\_vnc\_backend) | VNC backend used by the noVNC desktop runtime. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
