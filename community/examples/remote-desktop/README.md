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
- [hpc-slurm-remote-desktop-iap.yaml](./hpc-slurm-remote-desktop-iap.yaml): the
  same desktops, also reachable in a browser at an HTTPS hostname through a
  load balancer behind Identity-Aware Proxy. See
  [Reaching a desktop through a load balancer](#reaching-a-desktop-through-a-load-balancer).

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

# on the viz-desktop module:
deployment_groups:
- group: primary
  modules:
  - id: viz-desktop
    source: community/modules/remote-desktop/novnc-desktop
    settings:
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

## Reaching a desktop through a load balancer

`hpc-slurm-remote-desktop-iap.yaml` also publishes each desktop at its own HTTPS
hostname, so no SSH tunnel is needed. The SSH route above keeps working
alongside it.

A load balancer cannot reach a unix socket, so these brokers also listen on TCP
port 6080. A firewall rule admits Google's front ends to it, and the network's
rule for internal traffic admits its other VMs. On that port the user is
whoever Identity-Aware Proxy says they are, and a request without IAP's
assertion is refused. The broker
verifies the assertion IAP signs onto every request, checks it was minted for
this host's backend service, and maps the Google account to its OS Login
account. OS Login then decides whether that account may have a desktop, as it
does over SSH. There is still no shared secret, and no header is taken on
trust.

The blueprint has three parts:

- [iap-backend-service](../../modules/network/iap-backend-service/README.md)
  creates one backend service per desktop target.
- [modules/network/firewall-rules](../../../modules/network/firewall-rules/README.md)
  admits the load balancer to the brokers.
- [ext-app-lb](../../modules/network/ext-app-lb/README.md) provides the
  address, certificate and URL map. For one wildcard certificate rather than
  one per hostname, pass it the `certificate_map` of
  [managed-certificate](../../modules/network/managed-certificate/README.md).

The front end only needs each backend service's self link, so it can live in
another deployment and serve several clusters.

### Before deploying

1. Reserve a global address and note the IP:

   ```bash
   gcloud compute addresses create DEPLOYMENT_NAME-desktops --global
   gcloud compute addresses describe DEPLOYMENT_NAME-desktops --global \
     --format='value(address)'
   ```

2. In a DNS zone you control, point `login.DOMAIN` and `viz.DOMAIN` at that IP,
   or create one wildcard `*.DOMAIN` record. The Google-managed certificate is
   only issued once these names resolve to the load balancer.
3. Configure the
   [OAuth consent screen](https://console.cloud.google.com/auth/overview) once
   per project, so IAP can sign users in.
4. Grant each desktop user both `roles/compute.osLogin` and, through
   `desktop_iap_members`, IAP access. IAP admits them to the load balancer; OS
   Login decides whether they get a desktop.
5. Grant the desktop hosts' service account `roles/compute.viewer`, so each
   broker can look up its backend service. This blueprint runs them as the
   default compute service account, which otherwise has that only through the
   Editor role, and organization policy may keep it from being granted:

   ```bash
   gcloud projects add-iam-policy-binding PROJECT_ID \
     --member=serviceAccount:PROJECT_NUMBER-compute@developer.gserviceaccount.com \
     --role=roles/compute.viewer
   ```

Set `desktop_domain`, `desktop_address` (the IP itself) and
`desktop_iap_members` in the blueprint, then create and deploy it as above.

IAP's Google-managed OAuth client only admits users in your organization;
for anyone else, set `oauth2_client_id` and `oauth2_client_secret` on
`iap-backend-service`.

### Connecting

Browse to `https://login.DOMAIN/vnc.html` or `https://viz.DOMAIN/vnc.html`. The
certificate takes a while to become active after the first deploy. Until then,
every hostname fails TLS. Watch its progress with:

```bash
gcloud compute ssl-certificates list --global \
  --format='value(name,managed.status,managed.domainStatus)'
```

A user who reaches the same host over SSH and through the load balancer gets
the same desktop session either way, because both resolve to the same OS Login
account.

## Teardown Instructions

Replace `DEPLOYMENT_NAME` with the `deployment_name` used in the blueprint's
`vars` block.

```bash
./gcluster destroy DEPLOYMENT_NAME
```
