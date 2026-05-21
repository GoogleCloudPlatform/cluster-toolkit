# Slurm Native Authentication Migration Guide

> [!WARNING]
> **DEPRECATION NOTICE**: Support for legacy MUNGE-based authentication (`auth/munge`) is currently **DEPRECATING**.
> It is scheduled for complete removal on **July 31, 2026** in favor of Slurm Native Authentication (`auth/slurm`).
> All new deployments will use Slurm Native Authentication by default. Please plan to migrate existing clusters using the destroy-and-recreate workflow detailed below.

This document serves as the canonical operational manual for migrating Slurm-GCP deployments within the Cluster Toolkit from legacy MUNGE authentication (`auth/munge`) to **Slurm Native Authentication** (`auth/slurm`).

## Architectural Context

Historically, cluster node communication relies on the MUNGE background daemon (`munged`) to encapsulate and validate credentials across network sockets. Native authentication eliminates this dependency entirely by incorporating self-contained token generation directly into the Slurm daemons (`slurmctld` and `slurmd`).

### The Key Distribution Lifecycle

1. **Bootstrap Generation**: Upon initial deployment bootstrap, the controller initialization logic generates a random token string stored in `/etc/slurm/slurm.key` with strict `0400` user permissions.
2. **Compute Node Distribution**: Compute and login nodes securely mount a read-only handler from the controller during their node initialization script sequence, copy the `slurm.key` locally, and unmount the temporary distribution share.

---

## Critical Notice: In-Place Upgrades Incompatibility

> [!CAUTION]
> **Mandatory Destroy-and-Recreate Lifecycle**
> Administrators **must not** attempt an in-place configuration update via `gcluster deploy -w` to toggle authentication models on an existing, active cluster.

### Why In-Place Upgrades Fail
Runtime configuration pushes triggered by deployment synchronizers (e.g., `slurmsync.py`) update the cluster definitions and invoke partial reconfigurations to apply parameter adjustments. However, key generation and distribution are isolated strictly to the **bootstrap-only** phase of node startup. Consequently:

- The new native tokens are never created or shared across existing instances.
- Restarting daemons expect `/etc/slurm/slurm.key` to establish communication.
- Because the keys are absent, all Slurm daemons enter persistent crash loops, resulting in immediate and total cluster-wide failure.

### Error Signatures Observed
If an in-place upgrade is mistakenly applied, system logs (`journalctl -u slurmctld` or `/var/log/slurm/slurmctld.log`) will exhibit the following distinct error signatures:

```text
error: Could not open node authentication key /etc/slurm/slurm.key: No such file or directory
fatal: slurm_auth_init: auth/slurm: auth_p_init: failed to initialize authentication plugin
```

---

## Canonical Migration Instructions

To successfully transition production and development environments to native authentication, administrators must execute a complete infrastructure replacement lifecycle.

### Step 1: Prepare Configuration Updates
Ensure your target blueprints or custom variables files specify the updated authentication parameter:

```yaml
# Example segment in blueprint template or cluster definition
settings:
  enable_slurm_auth: true
```

### Step 2: Data Preservation
Verify that all persistent state, critical application binaries, and user directories are safely stored on persistent network storage layers (e.g., Cloud Storage buckets or standalone Filestore instances) that are decoupled from the compute deployment lifecycle.

### Step 3: Execute Clean Destruction
Fully tear down the existing MUNGE-authenticated compute resources:

```bash
./gcluster destroy <deployment_name>
```

### Step 4: Re-Provision Resources
Deploy the fresh cluster resources. The initial bootstrap sequence will securely generate and propagate the native keys:

```bash
./gcluster deploy <deployment_name>
```

---

## Ecosystem Dependencies & Tooling Impact

Migrating to Slurm Native Authentication impacts several peripheral tools and automated pipelines that previously relied on the MUNGE daemon.

- **Workbench Tooling (`workbenchinfo.py`)**: The frontend workbench generation logic has been updated to support Native Auth by default. If you attach a legacy cluster to a newly created workbench, you must ensure your cluster specifies `enable_slurm_auth: false` in the database, otherwise the workbench will fail to mount the legacy `munge.key`.
- **CI/CD Integration Tests**: Automated testing playbooks (e.g., `slurm-integration-test.yml`) have been updated to dynamically support both authentication models. Test pipelines execute a unified wait condition checking for either `/etc/slurm/slurm.key` or `/var/run/munge/munge.socket.2`, ensuring 100% backward compatibility for legacy pipeline testing without timeouts.
- **External Submission Nodes**: Custom submission nodes residing on-premises or in hybrid networks must have the `/etc/slurm/slurm.key` securely distributed to them manually by administrators. Ensure file ownership is mapped precisely to the native Slurm user account with `0400` permissions.
- **Workload Identity**: Native authentication operates completely transparently alongside Google Cloud Workload Identity Federation, inheriting standard cryptographic boundaries cleanly.
