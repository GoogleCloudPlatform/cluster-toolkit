# HPC Slurm C3 Blueprint for Google Cloud Dedicated

This blueprint provides an end-to-end Slurm cluster configuration on C3 instances designed specifically for [**Google Cloud Dedicated (GCD)**](https://cloud.google.com/products/dedicated).

It serves as the reference architecture for running high-performance computing (HPC) workloads on GCD without relying on Google Cloud public dependencies.

---

## Motivation & Architecture Comparison

Due to the sovereign nature of GCD, certain mechanisms that are available in the public Google Cloud (e.g. public Google APIs, Google OS Login IAM infrastructure,  marketplace VM images etc) are not present. In the following table we describe a number of differences from blueprints that target the public cloud and how the differences are handled.

| Standard GCP Blueprint | Google Cloud Dedicated (GCD) | How Differences Are Handled |
| :---- | :---- | :---- |
| **Public APIs** • `googleapis.com` | **Sovereign Universe Domains** • `*.apis-<location>.goog` | • **Sovereign API Routing**: Configures cluster deployment tools and client libraries to target sovereign regional endpoints and universe domains directly without relying on public API discovery. • **Offline Configuration**: Supports offline machine-type definitions so cluster deployment can proceed without querying public Google Cloud APIs. |
| **Google OS Login** • IAM-managed user directory & authentication | **POSIX / Non-OSLogin** • `enable_oslogin: false` • Automated metadata SSH keys | • **Local POSIX Accounts**: Disables OS Login and uses a shared NFS `/home` volume to maintain consistent POSIX UIDs/GIDs across all nodes. • **Automated SSH Configuration**: Attaches an automated startup script across all nodes to provision local user accounts from Compute Engine metadata (`ssh-keys`) and set up passwordless SSH for cluster-wide job execution (`srun`, MPI). |
| **Pre-baked Public Slurm Images** • External image repositories (`schedmd-slurm-gcp`) | **Self-Contained In-Project Packer Build** • `custom-slurm-image` | • **In-Project Image Baking**: Embeds a dedicated Packer build stage directly into the blueprint to build and bake Slurm images inside your project from a base OS image, eliminating dependencies on external cross-project image repositories. |
| **External Munge Authentication** • External Munge daemons & shared keys | **Native Slurm SAuth** • `enable_slurm_auth: true` • `slurm.key` over Controller NFS | • **Native SAuth over NFS**: Uses native Slurm authentication (`slurm.key`) generated on the controller and distributed to compute and login nodes over a shared NFS mount, removing the need for external Munge daemons. |
| **Commercial GCS Endpoints** • Commercial endpoints (`storage.googleapis.com`) | **Sovereign Endpoint Routing** | • **Dedicated Storage Routing**: Directs Cloud Storage requests to dedicated regional sovereign endpoints within the perimeter, ensuring storage operations run independently of commercial Google Cloud endpoints. |

---

## Blueprint Structure

```yaml
blueprint_name: hpc-c3-slurm-google-cloud-dedicated

deployment_groups:
  # Group 1: Setup vpc, slurm script
  - group: setup
    modules:
      - id: network             # pre-existing vpc
      - id: slurm-build-script  # slurm installation script

  # Group 2: Custom Slurm Image Build
  - group: build-slurm
    modules:
      - id: slurm-custom-image
        source: modules/packer/custom-image
        # Builds Rocky Linux 8 image with Slurm v6

  # Group 3: Cluster Provisioning
  - group: cluster
    modules:
      - id: homefs          # Shared NFS server for /home and /opt/apps
      - id: ssh_config      # Non-OSLogin user & passwordless SSH startup script
      - id: c3_nodeset      # C3 static compute nodes (use: [network, ssh_config])
      - id: c3_partition    # Slurm partition configuration
      - id: slurm_login     # Login VM with public IP (use: [network, ssh_config])
      - id: slurm_controller # Controller VM (enable_slurm_auth: true, enable_oslogin: false)
```

---

## Configuration

Edit `hpc-slurm-google-cloud-dedicated.yaml` to configure your project parameters:

```yaml
vars:
  project_id: <prefix>:my-dedicated-project
  deployment_name: hpc-slurm
  region: u-<location>
  zone: u-<location>-a
  universe_domain: apis-<location>.goog  # Set your sovereign universe domain (or omit for standard GCP)
  service_account_email: my-sa@developer.gserviceaccount.com
```

---

## Compact Placement & Physical Topology

You can configure Compute Engine **compact placement policies** (by default, this is enabled with a max_distance of 3) under `c3_nodeset.settings` to minimize network latency and tail jitter.

### 1. Changing Compact Placement max_distance Without Reservation

To let Cluster Toolkit automatically create and manage a compact placement policy, configure `enable_placement` and `placement_max_distance`:

```yaml
  - id: c3_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network, ssh_config]
    settings:
      node_count_static: 2
      machine_type: c3-standard-176
      # Enable auto-managed compact placement:
      enable_placement: true
      placement_max_distance: 2
```

### 2. Enabling Placement With Reservation

When attaching nodes to a pre-existing GCE reservation that already manages placement:

```yaml
  - id: c3_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network, ssh_config]
    settings:
      node_count_static: 16
      machine_type: c3-standard-176
      # Attach to reservation managing placement:
      reservation_name: c3-16node-reservation
```

To create a GCE reservation configured with a compact placement policy, refer to the Google Cloud documentation on [Creating a reservation that specifies a compact placement policy](https://cloud.google.com/compute/docs/instances/reservations-single-project#create-reservations-compact-placement).

Example `gcloud` commands:

```shell
# 1. Create a compact placement policy (e.g. Distance 2):
gcloud beta compute resource-policies create group-placement c3-d2-policy \
  --collocation=collocated \
  --max-distance=2 \
  --region=<region>

# 2. Create a reservation with the placement policy attached:
gcloud compute reservations create c3-16node-reservation \
  --vm-count=16 \
  --machine-type=c3-standard-176 \
  --zone=<zone> \
  --require-specific-reservation \
  --resource-policies=policy=c3-d2-policy
```

### 3. Check Physical Topology Across Compute Nodes

To inspect the physical topology and verify whether running instances are placed in the same rack (`subblock`) or optical block (`block`), run:

```shell
gcloud compute instances list \
  --project=<project_id> \
  --zone=<zone> \
  --format="table(name, zone, status, resourcePolicies.basename():label=PLACEMENT_POLICY, resourceStatus.physicalHostTopology.block, resourceStatus.physicalHostTopology.subblock, resourceStatus.physicalHostTopology.host)"
```

**Interpreting the Topology Columns:**

- **Distance = 1 (Intra-Rack)**: Identical `BLOCK` and identical `SUBBLOCK` across nodes.
- **Distance = 2 (Intra-Block)**: Identical `BLOCK` but different `SUBBLOCK` across nodes.
- **Distance = 3 (Inter-Block)**: Different `BLOCK` values across nodes.

---

## Quickstart Deployment

### 1\. Set Environment Variables

In dedicated sovereign environments, export `GHPC_MOCK_MACHINE_CONFIG` so `gcluster` can resolve machine type CPU/memory specifications offline without calling public GCP APIs:

```shell
export GHPC_MOCK_MACHINE_CONFIG='{"cpus": {"c3-standard-4": {"count": 4, "memoryMb": 16384}}, "gpus": {}, "tpus": {}}'
```

### 2\. Generate Deployment Configuration

```shell
./gcluster create community/examples/hpc-slurm-google-cloud-dedicated/hpc-slurm-google-cloud-dedicated.yaml \
  --vars "\
project_id=<prefix>:<proj_id>,\
deployment_name=<deploy_name>,\
region=u-<location>,\
zone=u-<location>-a,\
universe_domain=apis-<location>.goog,\
service_account_email=<account_id>-compute@developer.<prefix>-system.iam.gserviceaccount.com,\
source_image_project=<prefix>-system:rocky-linux-cloud,\
source_image_family=rocky-linux-8-optimized-gcp" \
  -w --force --validation-level=IGNORE
```

> **Note**: The `--validation-level=IGNORE` flag is required on Google Cloud Dedicated to bypass commercial Google Cloud pre-flight API checks, allowing deployment in custom universe domains and sovereign regions.

### 3\. Deploy Cluster

#### Option A: Full Deployment (First Time — Builds Custom Slurm Image \+ Cluster)

```shell
./gcluster deploy <deployment_name> --auto-approve
```

#### Option B: Fast Deployment (\~25 min saved if custom image is already built)

```shell
./gcluster deploy <deployment_name> --skip build-slurm --auto-approve
```

### 4\. Connect via SSH

```shell
gcloud compute ssh <deployment_name>-slurm-login-001 \
  --zone=<zone> \
  --project=<project_id>
```

### 5\. Verify Slurm Cluster Health

Once logged in, verify the scheduler and compute nodes:

```shell
# Check controller status:
scontrol ping
# Output: Slurmctld(primary) at <controller> is UP

# Check compute partitions:
sinfo
# Output: compute* up infinite 2 idle# <nodeset>-[0-1]

# Run a test job across nodes:
srun -N 2 hostname
```

---

## Teardown

```shell
./gcluster destroy <deployment_name> --auto-approve
```
