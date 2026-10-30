# Remote Desktop Examples

These blueprints deploy browser-based Linux desktops alongside a Slurm cluster,
using the [novnc-runtime](../../modules/remote-desktop/novnc-runtime/README.md)
and [novnc-desktop](../../modules/remote-desktop/novnc-desktop/README.md)
modules. Users reach an XFCE session over noVNC in a browser, with sessions
created per user from their OS Login identity.

## Architecture

Each blueprint deploys a Slurm cluster with one or both desktop types:

- **Login desktop**: `novnc-runtime` layered onto the Slurm login node, using
  TigerVNC. Suited to 2D and general interactive use.
- **Visualization desktop**: `novnc-desktop` on a dedicated VM, using TurboVNC.
  Suited to 3D, and the only backend supporting GPU acceleration on GCE.

Both hosts run a per-user broker that listens only on a unix socket,
`/run/ghpc-remote-desktop/broker.sock`, with no TCP port. You reach it by
forwarding a local port to that socket over SSH. The broker asks the kernel
which account opened the connection, and sshd opens it as the user who logged
in through OS Login, so you always get your own desktop. No shared secret or
identity header is involved, and on a shared login node another user who
connects to the socket gets their own desktop, not yours. Each display likewise
listens on a `0700` per-user unix socket, so other local users cannot reach it.

## Blueprints

- [hpc-slurm-remote-desktop.yaml](./hpc-slurm-remote-desktop.yaml): login and
  visualization desktops, reached over SSH through IAP.

## GPU desktops

The visualization desktop can use hardware OpenGL. Rather than ship a
near-identical second blueprint, apply these four changes to
`hpc-slurm-remote-desktop.yaml`:

```yaml
vars:
  # Debian is the only slurm-gcp family shipping an NVIDIA driver with graphics
  # (not merely compute) support. A compute driver ships no libEGL_nvidia and
  # cannot render.
  slurm_image:
    family: slurm-gcp-6-12-debian-12
    project: schedmd-slurm-public
  viz_machine_type: g2-standard-8

# on the viz-desktop module
      enable_gpu_acceleration: true
      on_host_maintenance: TERMINATE
```

`vnc_backend: turbovnc` is required and is already the visualization desktop's
setting. TurboVNC reaches the GPU through VirtualGL's EGL back end; TigerVNC
offloads GL only through `-rendernode`, which needs a DRM render node that GCE's
NVIDIA images never create. Without a usable GPU the session falls back to
software rendering rather than failing, and the broker logs why at startup.

## Getting Started

Before you start, make sure your prerequisites and dependencies are set up:
[Set up Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).

Set the following blueprint variables:

- `project_id`, `region` and `zone`

Each desktop user needs `roles/compute.osLogin` on the project to SSH in, and
`roles/iap.tunnelResourceAccessor` to do so through IAP.

## Deployment Instructions

> [!WARNING]
> Deploying these blueprints uses billable components of Google Cloud, including
> Compute Engine instances that run continuously until destroyed. GPU machine
> types are billed at a significantly higher rate.

Create and deploy a blueprint. Replace `BLUEPRINT` with the file you want, and
`DEPLOYMENT_NAME` with the `deployment_name` set in that blueprint's `vars`
block - for `hpc-slurm-remote-desktop.yaml` that is `hpc-slurm-remote-desktop`:

```bash
./gcluster create community/examples/remote-desktop/BLUEPRINT.yaml -w
./gcluster deploy DEPLOYMENT_NAME
```

## Verify

On each desktop host, confirm the broker is running:

```bash
systemctl status ghpc-desktop-broker
sudo journalctl -u ghpc-desktop-broker --no-pager
curl -s --unix-socket /run/ghpc-remote-desktop/broker.sock http://localhost/healthz   # -> ok
```

## Reaching a desktop in a browser

Forward a local port to the broker's socket on the host you want: the Slurm
login node for the login desktop, or the visualization VM:

```bash
gcloud compute ssh HOST --project PROJECT_ID --zone ZONE --tunnel-through-iap \
  -- -N -L 6080:/run/ghpc-remote-desktop/broker.sock
```

Then browse to `http://localhost:6080/vnc.html`. Your desktop starts on the
first visit and is reused when you reconnect. The visualization desktop module
prints this command for its VM as the `iap_tunnel_command` output.

There is nothing else to set up on your machine: no proxy and no headers. The
connection is yours because sshd opened it as your OS Login account.

## Teardown Instructions

Replace `DEPLOYMENT_NAME` with the `deployment_name` used in the blueprint's
`vars` block.

```bash
./gcluster destroy DEPLOYMENT_NAME
```
