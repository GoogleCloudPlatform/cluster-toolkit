## Description

Shared implementation behind the browser-desktop modules. Not intended to be
used directly from a blueprint; use
[novnc-runtime](../../remote-desktop/novnc-runtime/README.md) or
[novnc-desktop](../../remote-desktop/novnc-desktop/README.md), which are the
public interfaces onto it.

It emits the startup-script runners that install an XFCE desktop, a VNC backend,
the selected front end, and a per-user session broker. The broker establishes who
is asking from the peer credentials of its unix socket, checks the account
against OS Login, and starts exactly one desktop session per user.

### Why this is shared

The two front ends differ in how a desktop is *reached*, not in how one is
*created*. Identity, OS Login, session state, slot allocation, the user
environment and reaping are the same either way — that was roughly 70% of each
front end's broker before this module existed. Keeping two copies of that in
step is not a trade worth making.

### The broker package

`files/desktop_broker/` is a Python package, run as
`python3 -m desktop_broker.main --config /etc/ghpc-desktop-broker/config.json`:

```text
main.py         entry point: python3 -m desktop_broker.main --config ...
config.py       validated configuration; every invalid combination fails at
                startup rather than on a user's first request
errors.py       the one error type with an HTTP status and a safe message
identity.py     who is asking: the account of the process on the other end of
                the unix socket, from SO_PEERCRED
oslogin.py      whether OS Login knows the account and lets it log in,
                asked per user and re-checked per hand-off
sessions/       store.py (records, locks, slots), userenv.py (privileged work
                on a user's account), lifecycle.py (start/reuse/reap)
backends/       registry.py chooses between tigervnc and turbovnc; gpu.py
                detects what hardware OpenGL is actually available
novnc.py        serving the noVNC client and relaying RFB over a websocket
app.py          the only module that knows about every component
```

### No `__init__.py`, and no `__main__.py`

Two constraints shape how this package is laid out and delivered, and both are
easy to trip over:

1. **`gcluster` embeds these modules with `go:embed`, which silently skips any
   file whose name begins with an underscore.** An `__init__.py` would never
   reach a deployed host - the package would arrive with its subpackages'
   initialisers missing and fail to import. So there are none: this is a
   [PEP 420](https://peps.python.org/pep-0420/) implicit namespace package, and
   the entry point is `main.py`, invoked as `python3 -m desktop_broker.main`.
   Anything that would naturally live in an `__init__.py` lives in a named
   module instead - `backends/registry.py`.

2. **`modules/scripts/startup-script` keys its staged objects on
   `basename(destination)`.** Several files sharing a basename collide on that
   key and Terraform refuses the plan. The package is therefore written by a
   single runner, with each file embedded base64-encoded, rather than one `data`
   runner per file.

The install runner fails loudly if the package arrives incomplete, so a future
change that reintroduces an underscore-prefixed file produces a clear error
rather than an import failure at service start.

### Tests

`files/tests/` is a pytest suite covering configuration validation, identity
resolution, the OS Login profile and authorisation checks, session records
and slot allocation, the generated `xstartup`, and backend command construction.

```bash
cd community/modules/internal/desktop-broker/files && pytest tests/
```

One of these exists because a live host found what review did not:
`test_tigervnc_closes_its_tcp_listener` pins the `-rfbport -1` flag, whose
absence silently exposes every display on a local TCP port.

### Identity

The broker listens only on a unix socket, `/run/ghpc-remote-desktop/broker.sock`,
which any local user may connect to. On each request it asks the kernel which
account the connecting process runs as (`SO_PEERCRED`), refuses root and any
account OS Login does not manage, and starts or reuses that account's desktop.
Nothing the client sends affects the answer, so there is no shared secret and
no identity header.

Users reach the socket by SSH local forwarding, for example
`ssh -N -L 6080:/run/ghpc-remote-desktop/broker.sock HOST`. sshd opens the
forwarded connection as the user who logged in, so the broker sees the OS Login
account that SSH authenticated.

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [terraform_data.input_validation](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_enable_gpu_acceleration"></a> [enable\_gpu\_acceleration](#input\_enable\_gpu\_acceleration) | Use hardware OpenGL for desktop sessions where a usable GPU is present. | `bool` | `false` | no |
| <a name="input_install_root"></a> [install\_root](#input\_install\_root) | Directory under which the broker and front-end assets are installed. | `string` | `"/opt/ghpc-remote-desktop"` | no |
| <a name="input_max_user_sessions"></a> [max\_user\_sessions](#input\_max\_user\_sessions) | Maximum number of concurrent per-user desktop sessions. | `number` | `32` | no |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | An array of network attached storage mounts to be configured. | <pre>list(object({<br/>    server_ip             = string<br/>    remote_mount          = string<br/>    local_mount           = string<br/>    fs_type               = string<br/>    mount_options         = string<br/>    client_install_runner = map(string)<br/>    mount_runner          = map(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_novnc_version"></a> [novnc\_version](#input\_novnc\_version) | noVNC release version to install. Only used when front\_end is novnc. | `string` | `"1.7.0"` | no |
| <a name="input_session_idle_timeout_seconds"></a> [session\_idle\_timeout\_seconds](#input\_session\_idle\_timeout\_seconds) | Idle timeout after which a per-user desktop session is cleaned up. Set to 0 to keep idle sessions; a session whose desktop has exited is still cleaned up. | `number` | `43200` | no |
| <a name="input_session_resolution"></a> [session\_resolution](#input\_session\_resolution) | Desktop resolution used for each per-user session. | `string` | `"1920x1080"` | no |
| <a name="input_slurm_auth_mode"></a> [slurm\_auth\_mode](#input\_slurm\_auth\_mode) | Slurm authentication mode on the host: none, munge, native or auto. | `string` | `"none"` | no |
| <a name="input_startup_script"></a> [startup\_script](#input\_startup\_script) | Optional additional startup script prepended before the desktop runtime setup. | `string` | `null` | no |
| <a name="input_vnc_backend"></a> [vnc\_backend](#input\_vnc\_backend) | VNC server backend for per-user sessions: tigervnc or turbovnc. | `string` | `"tigervnc"` | no |
| <a name="input_vnc_display_number"></a> [vnc\_display\_number](#input\_vnc\_display\_number) | First X display number reserved for per-user VNC sessions. Starts high to stay clear of users' own X servers on a shared host. | `number` | `50` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_broker_socket_path"></a> [broker\_socket\_path](#output\_broker\_socket\_path) | Unix socket the broker serves on. Forward a local port to it over SSH to reach a desktop. |
| <a name="output_startup_runners"></a> [startup\_runners](#output\_startup\_runners) | Ordered startup-script runners that install the desktop runtime, the selected front end and the per-user broker. |
| <a name="output_vnc_backend"></a> [vnc\_backend](#output\_vnc\_backend) | VNC backend used for per-user desktop sessions. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
