# EDA Slurm Blueprint & Workload Examples for Google Cloud Dedicated (GCD)

This directory provides a sovereign **[Cluster Toolkit (`gcluster`)](https://cloud.google.com/cluster-toolkit/docs)** blueprint ([`eda-gcd-cluster.yaml`](eda-gcd-cluster.yaml)) and end-to-end Electronic Design Automation (EDA) workload examples for **Google Cloud Dedicated (GCD)** environments (`*.apis-<location>.goog`):

1. **Sovereign GCD EDA Slurm Blueprint ([`eda-gcd-cluster.yaml`](eda-gcd-cluster.yaml))**: Provisions a custom Rocky Linux 9 Slurm v6 image via Packer, a Hyperdisk-backed shared NFS server (`/home`, `/tools`, `/library`, `/scratch`, `/opt/apps`), non-OSLogin POSIX SSH synchronization, and a `c3-standard-176` compute partition.
2. **Front-End RTL Verification ([`verilator-nvdla.def`](verilator-nvdla.def))**: Compiles and runs open-source SystemVerilog/SystemC verification regressions using **Verilator v5.052** and **RTLMeter (`NVDLA`)** on Slurm.
3. **Back-End Physical Design (`RTL-to-GDSII`)**: Runs synthesis, floorplanning, placement, clock-tree synthesis (CTS), routing, and final GDSII generation using the official **OpenROAD-flow-scripts (`docker.io/openroad/orfs`)** container with the built-in **ASAP7 (7nm)** PDK and **Ibex (32-bit RISC-V CPU)** design.

---

## 1. Architecture & Sovereign GCD Adaptations

Compared to standard commercial GCP blueprints, [`eda-gcd-cluster.yaml`](eda-gcd-cluster.yaml) adapts four core components for sovereign GCD operation:

| Component | Sovereign GCD Configuration (`eda-gcd-cluster.yaml`) | Purpose |
| :--- | :--- | :--- |
| **Universe Domain & Validators** | `universe_domain: $(vars.universe_domain)` with public GCP pre-flight `validators` skipped | Routes Terraform providers to sovereign regional endpoints (`apis-<location>.goog`) without querying unreachable public APIs. |
| **In-Project Image Baking (`build-slurm`)** | `modules/packer/custom-image` (`slurm-rocky9-eda`) | Builds Rocky Linux 9 with Slurm `6.12.3`, `apptainer`, `podman`, `numactl`, and `python3.11` inside your GCD project. Skippable via `--only setup,cluster` once built. |
| **Shared EDA Storage Tier (`homefs`)** | `community/modules/file-system/nfs-server` (`c3-standard-8` with a 2 TB `hyperdisk-balanced` disk at 10,000 provisioned IOPS and 400 MiB/s by default; set `nfs_machine_type`, `nfs_provisioned_iops`, and `nfs_provisioned_throughput` higher for better performance) | Exports `/home`, `/tools`, `/library`, `/scratch`, and `/opt/apps` across login, controller, and compute nodes. `/tools`, `/library`, `/scratch`, and `/opt/apps` use mode `1777` (world-writable with the sticky bit, like `/tmp`). |
| **Identity & Slurm Auth** | `enable_oslogin: false` + `enable_slurm_auth: true` | Synchronizes GCE metadata SSH keys to deterministic POSIX UIDs/GIDs on `/home` using `flock` and authenticates Slurm RPCs via native `slurm.key`. |

---

## 2. Deploying `eda-gcd-cluster.yaml`

### 2.1 Prerequisites & Environment Setup

1. **Authenticate to your GCD Universe via Workforce Identity Federation (WIF) and set environment variables**:

   ```bash
   export UNIVERSE_DOMAIN="<YOUR_GCD_UNIVERSE_DOMAIN>"  # e.g., apis-<location>.goog
   export GOOGLE_CLOUD_UNIVERSE_DOMAIN="${UNIVERSE_DOMAIN}"
   export CLOUDSDK_UNIVERSE_DOMAIN="${UNIVERSE_DOMAIN}"

   export PROJECT_ID="<YOUR_GCD_PROJECT_ID>"            # e.g., <partition>:<project-id>
   export SA_EMAIL="<ACCOUNT_ID>-compute@developer.<partition>-system.iam.gserviceaccount.com"
   export SOURCE_IMAGE_PROJECT="<partition>-system:rocky-linux-cloud"
   export REGION="<YOUR_GCD_REGION>"
   export ZONE="<YOUR_GCD_ZONE>"
   export DEPLOYMENT_NAME="eda-gcd-01"
   ```

2. **Export offline machine specifications so `gcluster` can expand the blueprint without querying public Compute APIs**:

   ```bash
   export GHPC_MOCK_MACHINE_CONFIG='{"cpus": {"c3-standard-4": {"count": 4, "memoryMb": 16384}, "c3-standard-176": {"count": 176, "memoryMb": 720896}}, "gpus": {}, "tpus": {}}'
   ```

3. **Network**: The blueprint creates a dedicated VPC network (`<deployment_name>-net`) with a private subnet (`10.128.0.0/20`), firewall rules that allow internal TCP, UDP and ICMP traffic (the `vpc` module's default rule for `10.0.0.0/9` plus a `<deployment_name>-allow-internal` rule for the cluster subnet), an SSH firewall rule limited to `authorized_ssh_ranges`, and a Cloud Router with Cloud NAT for outbound access from nodes without public IPs.

### 2.2 Create and Deploy the Cluster

Generate the deployment directory from the Cluster Toolkit root:

```bash
./gcluster create community/examples/hpc-slurm-google-cloud-dedicated/eda/eda-gcd-cluster.yaml \
  --vars deployment_name=${DEPLOYMENT_NAME},project_id=${PROJECT_ID},region=${REGION},zone=${ZONE},universe_domain=${UNIVERSE_DOMAIN},service_account_email=${SA_EMAIL},source_image_project=${SOURCE_IMAGE_PROJECT} \
  --vars 'authorized_ssh_ranges=[<YOUR_CLIENT_CIDR>/32]'
```

To allow SSH from more than one CIDR, set `authorized_ssh_ranges` in the blueprint's `vars` block instead (for example, `authorized_ssh_ranges: ["x.x.x.x/32", "y.y.y.0/24"]`).

#### Option A: First-Time Deployment (Build `slurm-rocky9-eda` Image + Deploy Cluster)

```bash
# 1. Deploy the VPC network and build the custom Slurm OS image with Packer (~20-25 mins, once per project):
./gcluster deploy ${DEPLOYMENT_NAME} --only setup,build-slurm --auto-approve

# 2. Deploy the shared EDA NFS server and Slurm cluster:
./gcluster deploy ${DEPLOYMENT_NAME} --only cluster --auto-approve
```

#### Option B: Fast Deployment (Skip Packer Build if `slurm-rocky9-eda` Already Exists)

The EDA image uses its own image family, `slurm-rocky9-eda`, because it bakes in `apptainer`, `podman`, `numactl`, and `python3.11`. A `slurm-rocky9` image built by the other GCD blueprints does not include these tools and cannot be used here.

```bash
./gcluster deploy ${DEPLOYMENT_NAME} --only setup,cluster --auto-approve
```

### 2.3 Connect to the Login Node & Verify Cluster Health

The login node is named `<cluster_name>-slurm-login-001`, where `<cluster_name>` is the Slurm cluster name derived from the deployment name (lowercased, non-alphanumeric characters removed, truncated to 10 characters; for example, `eda-gcd-01` becomes `edagcd01`). To find it:

```bash
gcloud compute instances list --project=${PROJECT_ID} --filter="name~slurm-login"
```

```bash
gcloud compute ssh <cluster_name>-slurm-login-001 \
  --zone=${ZONE} \
  --project=${PROJECT_ID}
```

Once logged in, verify the scheduler and shared EDA filesystems:

```bash
scontrol ping
sinfo
df -h /home /tools /library /scratch /opt/apps
```

---

## 3. Front-End RTL Verification Example: Verilator & RTLMeter (`NVDLA`)

This example uses **[Verilator v5.052](https://verilator.org)**, **GCC 14 (`gcc-toolset-14`)**, **Accellera SystemC 2.3.4**, and **[RTLMeter](https://github.com/verilator/rtlmeter)** packaged via [`verilator-nvdla.def`](verilator-nvdla.def) to compile and simulate NVIDIA's open-source Deep Learning Accelerator (**`NVDLA:default:conv`**).

### 3.1 Verify or Stage the Verilator Container (`/tools/containers/verilator-nvdla.sif`)

When the login node boots, `stage_verilator_container.sh` automatically copies [`verilator-nvdla.def`](verilator-nvdla.def) to `/tools/containers/verilator-nvdla.def` and starts a background build (`/tools/containers/build_verilator_sif.sh`) using node-local `/var/tmp`.

* **If your cluster has outbound proxy/mirror access to GitHub and Rocky Linux repos**:
  Monitor the automatic background build (~7–10 minutes on first boot):

  ```bash
  tail -f /tools/containers/build_verilator_sif.log
  ls -lh /tools/containers/verilator-nvdla.sif
  ```

* **If your GCD environment is strictly air-gapped (no outbound access to `github.com`)**:
  Build `verilator-nvdla.sif` on an internet-connected workstation from [`verilator-nvdla.def`](verilator-nvdla.def) and copy the `.sif` file to `/tools/containers/verilator-nvdla.sif`:

  ```bash
  # On an internet-connected workstation with Apptainer installed:
  APPTAINER_TMPDIR=/var/tmp apptainer build verilator-nvdla.sif \
    community/examples/hpc-slurm-google-cloud-dedicated/eda/verilator-nvdla.def

  # Copy the pre-built .sif image to the GCD login node's shared /tools volume:
  gcloud compute scp verilator-nvdla.sif \
    <cluster_name>-slurm-login-001:/tools/containers/verilator-nvdla.sif \
    --zone=${ZONE} --project=${PROJECT_ID}
  ```

Run the built-in smoke test on the login node to verify Verilator 5.052, GCC 14, SystemC 2.3.4, and RTLMeter:

```bash
apptainer run /tools/containers/verilator-nvdla.sif
apptainer exec /tools/containers/verilator-nvdla.sif rtlmeter show --cases | grep '^NVDLA:'
```

Expected output:

```text
Verilator 5.052 2026-09-05 rev v5.052
>>> ALL SMOKE TESTS PASSED SUCCESSFULLY!

NVDLA:default:anet                    hier
NVDLA:default:conv                    hier
NVDLA:default:gnet                    hier
NVDLA:default:hello                   hier, sanity
NVDLA:default:pool                    hier
NVDLA:default:relu                    hier
```

### 3.2 Step 1 — Compile the `NVDLA` Simulation Model Once

A Verilator workflow has two distinct phases:
1. **Compile (`verilate` + `cppbuild`)**: Translates SystemVerilog RTL to C++ and compiles ~600 C++ translation units with `make -j`. Run this **once** on a multi-core allocation and save the compiled binary to shared `/scratch/rtlmeter_models/`.
2. **Execute (`execute`)**: Runs single-core test regressions against the pre-compiled binary in seconds using node-local `/tmp` for temporary output files.

Create a working directory and submit the multi-core model build script (`00_build_model.sh`):

```bash
mkdir -p ~/verilator_nvdla && cd ~/verilator_nvdla

cat <<'EOF' > 00_build_model.sh
#!/usr/bin/env bash
#SBATCH --job-name=nvdla_build
#SBATCH --partition=compute
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=48
#SBATCH --mem-per-cpu=2G
#SBATCH --time=01:00:00
#SBATCH --output=/scratch/nvdla_build_%j.log

set -eo pipefail

CASE="${CASE:-NVDLA:default:conv}"
CASE_SLUG="$(printf '%s' "${CASE}" | tr -c 'A-Za-z0-9._-' '_')"
MODEL_DIR="${MODEL_DIR:-/scratch/rtlmeter_models/${CASE_SLUG}_t1}"
CONTAINER_SIF="${CONTAINER_SIF:-/tools/containers/verilator-nvdla.sif}"
RTLMETER_DIR="/opt/rtlmeter"

export APPTAINER_CLEANENV=1
export APPTAINERENV_PYTHONPATH="${RTLMETER_DIR}"

rm -rf "${MODEL_DIR}"
mkdir -p "${MODEL_DIR}"

apptainer exec \
  --bind /tools:/tools \
  --bind /scratch:/scratch \
  "${CONTAINER_SIF}" \
  "${RTLMETER_DIR}/rtlmeter" run \
    --cases "${CASE}" \
    --compileRoot "${MODEL_DIR}" \
    --nExecute 0

apptainer exec \
  --bind /tools:/tools \
  --bind /scratch:/scratch \
  "${CONTAINER_SIF}" \
  "${RTLMETER_DIR}/rtlmeter" report --steps "verilate cppbuild" "${MODEL_DIR}"
EOF

chmod +x 00_build_model.sh
BUILD_JID=$(sbatch --parsable 00_build_model.sh)
echo "Submitted NVDLA model build job: ${BUILD_JID}"
```

When the build job completes, `/scratch/nvdla_build_<JOBID>.log` should end with `@@@ All cases passed` followed by a per-step report. Reference results on `c3-standard-176`:

| Step | Elapsed time (s) | Peak memory (MB) |
| :--- | ---: | ---: |
| `verilate` | 283.63 | 7,375 |
| `cppbuild` | 218.40 | 1,402 |

### 3.3 Step 2 — Run Single-Core & Packed Multi-Core Simulations

> [!NOTE]
> Packing jobs onto shared nodes relies on `exclusive: false` on the `compute` partition in [`eda-gcd-cluster.yaml`](eda-gcd-cluster.yaml). The Slurm partition module defaults to `exclusive: true`, which sets `OverSubscribe=EXCLUSIVE` so each node runs only one job at a time. With `exclusive: false`, Slurm (`select/cons_tres` with `CR_Core_Memory`) allocates individual cores and memory, so jobs of different shapes, such as the 48-CPU model build, 1-CPU simulations, and the 16-CPU OpenROAD flow, can run on the same node whenever enough cores and memory are free.

Once `00_build_model.sh` completes (or chained automatically with `--dependency=afterok:${BUILD_JID}`), run either a single-core baseline simulation (`01_single_core.sh`) or a packed Slurm array regression (`02_packed_cores.sh`) that shares the compiled model from `/scratch` while isolating per-task runtime I/O in node-local `/tmp`. On exit, including on failure, each job copies its RTLMeter execute directory (logs and metrics) to `/scratch/nvdla_results/` and then removes its node-local directory:

```bash
cat <<'EOF' > 01_single_core.sh
#!/usr/bin/env bash
#SBATCH --job-name=nvdla_1core
#SBATCH --partition=compute
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=1
#SBATCH --mem-per-cpu=2G
#SBATCH --time=00:30:00
#SBATCH --output=/scratch/nvdla_1core_%j.log

set -eo pipefail

CASE="${CASE:-NVDLA:default:conv}"
CASE_SLUG="$(printf '%s' "${CASE}" | tr -c 'A-Za-z0-9._-' '_')"
MODEL_DIR="${MODEL_DIR:-/scratch/rtlmeter_models/${CASE_SLUG}_t1}"
CONTAINER_SIF="${CONTAINER_SIF:-/tools/containers/verilator-nvdla.sif}"
RTLMETER_DIR="/opt/rtlmeter"
SCRATCH_DIR="${SCRATCH_DIR:-/tmp}"
XR="${SCRATCH_DIR}/nvdla_1core_${SLURM_JOB_ID}"
RESULTS_DIR="/scratch/nvdla_results/1core_${SLURM_JOB_ID}"

export APPTAINER_CLEANENV=1
export APPTAINERENV_PYTHONPATH="${RTLMETER_DIR}"

# On exit (including failure), archive the RTLMeter execute directory (logs and
# metrics) to shared /scratch, then remove the node-local copy.
archive_and_clean() {
  rc=$?
  mkdir -p "${RESULTS_DIR}"
  cp -r "${XR}/." "${RESULTS_DIR}/" || true
  rm -rf "${XR}"
  exit "${rc}"
}

rm -rf "${XR}" && mkdir -p "${XR}"
trap archive_and_clean EXIT

apptainer exec \
  --bind /tools:/tools \
  --bind /scratch:/scratch \
  --bind "${SCRATCH_DIR}:${SCRATCH_DIR}" \
  "${CONTAINER_SIF}" \
  "${RTLMETER_DIR}/rtlmeter" run \
    --cases "${CASE}" \
    --compileRoot "${MODEL_DIR}" \
    --executeRoot "${XR}" \
    --nExecute 1

apptainer exec \
  --bind /tools:/tools \
  --bind /scratch:/scratch \
  --bind "${SCRATCH_DIR}:${SCRATCH_DIR}" \
  "${CONTAINER_SIF}" \
  "${RTLMETER_DIR}/rtlmeter" report --steps "execute" "${XR}"
EOF

cat <<'EOF' > 02_packed_cores.sh
#!/usr/bin/env bash
#SBATCH --job-name=nvdla_packed
#SBATCH --partition=compute
#SBATCH --array=1-88
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=1
#SBATCH --mem-per-cpu=2G
#SBATCH --time=00:30:00
#SBATCH --output=/scratch/nvdla_packed_%A_%a.log

set -eo pipefail

CASE="${CASE:-NVDLA:default:conv}"
CASE_SLUG="$(printf '%s' "${CASE}" | tr -c 'A-Za-z0-9._-' '_')"
MODEL_DIR="${MODEL_DIR:-/scratch/rtlmeter_models/${CASE_SLUG}_t1}"
CONTAINER_SIF="${CONTAINER_SIF:-/tools/containers/verilator-nvdla.sif}"
RTLMETER_DIR="/opt/rtlmeter"
SCRATCH_DIR="${SCRATCH_DIR:-/tmp}"
XR="${SCRATCH_DIR}/nvdla_packed_${SLURM_ARRAY_JOB_ID}_${SLURM_ARRAY_TASK_ID}"
RESULTS_DIR="/scratch/nvdla_results/packed_${SLURM_ARRAY_JOB_ID}_${SLURM_ARRAY_TASK_ID}"

export APPTAINER_CLEANENV=1
export APPTAINERENV_PYTHONPATH="${RTLMETER_DIR}"

# On exit (including failure), archive the RTLMeter execute directory (logs and
# metrics) to shared /scratch, then remove the node-local copy.
archive_and_clean() {
  rc=$?
  mkdir -p "${RESULTS_DIR}"
  cp -r "${XR}/." "${RESULTS_DIR}/" || true
  rm -rf "${XR}"
  exit "${rc}"
}

rm -rf "${XR}" && mkdir -p "${XR}"
trap archive_and_clean EXIT

apptainer exec \
  --bind /tools:/tools \
  --bind /scratch:/scratch \
  --bind "${SCRATCH_DIR}:${SCRATCH_DIR}" \
  "${CONTAINER_SIF}" \
  "${RTLMETER_DIR}/rtlmeter" run \
    --cases "${CASE}" \
    --compileRoot "${MODEL_DIR}" \
    --executeRoot "${XR}" \
    --nExecute 1
EOF

chmod +x 01_single_core.sh 02_packed_cores.sh

# Chain a 1-core smoke run and an 88-task packed regression behind the model build:
sbatch --dependency=afterok:${BUILD_JID} 01_single_core.sh
sbatch --dependency=afterok:${BUILD_JID} --array=1-88 02_packed_cores.sh
```

#### Reference Packed-Node Throughput on `c3-standard-176` (`NVDLA:default:conv`)

Packing independent single-core simulations across the 88 physical cores of a `c3-standard-176` node scales with **92.1% efficiency**. Wall time is the node occupancy measured by Slurm (first task `Start` to last task `End`) for `01_single_core.sh` (`k=1`) and the 88-task `02_packed_cores.sh` array (`k=88`), including container start-up and RTLMeter prepare/post-hook steps:

| Concurrent Tasks (`k`) | 1 | 88 (Full Node) |
| :--- | ---: | ---: |
| **Wall Time (s)** | 116 | **126** |
| **Throughput (jobs/hr)** | 31.0 | **2,514** |

---

## 4. Back-End Physical Design (`RTL-to-GDSII`) Example: OpenROAD (`ORFS`)

For back-end physical design (synthesis with Yosys, floorplanning, global/detailed placement, clock-tree synthesis, global/detailed routing, static timing signoff with OpenSTA, and GDSII stream-out with KLayout), we recommend using the official **[OpenROAD-flow-scripts (`ORFS`)](https://github.com/The-OpenROAD-Project/OpenROAD-flow-scripts)** container image (`docker.io/openroad/orfs`).

The official `openroad/orfs` container includes the open-source **ASAP7 (7nm predictive FinFET)** PDK and ready-to-run designs, so no external PDK or benchmark downloads are required.

### 4.1 Get the OpenROAD-flow-scripts Container

Obtain the official `openroad/orfs` container from [Docker Hub](https://hub.docker.com/r/openroad/orfs) (see [Build with Docker](https://openroad-flow-scripts.readthedocs.io/en/latest/user/BuildWithDocker.html) in the ORFS documentation) and convert it to an Apptainer image at `/tools/containers/openroad-orfs.sif`, or set `CONTAINER_SIF` to its location when submitting the job below.

### 4.2 Run the `asap7` (7nm) `ibex` RISC-V CPU Core RTL-to-GDSII Flow on Slurm

Inside `/OpenROAD-flow-scripts/flow`, `DESIGN_CONFIG=./designs/asap7/ibex/config.mk` targets the **lowRISC Ibex 32-bit RISC-V CPU core** (~18k cells, ~8–12 minutes end-to-end on 16 cores) on the **ASAP7 7nm** FinFET PDK.

For other supported PDKs and designs, see the [`flow/platforms`](https://github.com/The-OpenROAD-Project/OpenROAD-flow-scripts/tree/master/flow/platforms) and [`flow/designs`](https://github.com/The-OpenROAD-Project/OpenROAD-flow-scripts/tree/master/flow/designs) directories in the OpenROAD-flow-scripts GitHub repository.

Because the `.sif` container image is read-only, pass `WORK_HOME` pointing to a writable directory on node-local `/tmp` so `make` writes all intermediate `.odb`, `.def`, timing reports, and `.gds` files locally during the job, then archives the logs, reports, and results to `/scratch` when the job exits, including when the flow fails:

```bash
mkdir -p ~/openroad_asap7 && cd ~/openroad_asap7

cat <<'EOF' > run_openroad_asap7_ibex.sh
#!/usr/bin/env bash
#SBATCH --job-name=orfs_asap7_ibex
#SBATCH --partition=compute
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=16
#SBATCH --mem-per-cpu=2G
#SBATCH --time=01:00:00
#SBATCH --output=/scratch/orfs_asap7_ibex_%j.log

set -eo pipefail

CONTAINER_SIF="${CONTAINER_SIF:-/tools/containers/openroad-orfs.sif}"
DESIGN_CONFIG="${DESIGN_CONFIG:-./designs/asap7/ibex/config.mk}"
SCRATCH_DIR="${SCRATCH_DIR:-/tmp}"
LOCAL_WORK="${SCRATCH_DIR}/orfs_work_${SLURM_JOB_ID}"
OUT_DIR="/scratch/openroad_results/asap7_ibex_${SLURM_JOB_ID}"

# On exit (including when make fails), archive stage logs, reports, and results
# to shared /scratch, then remove the node-local work directory.
archive_and_clean() {
  rc=$?
  for d in logs reports results; do
    if [ -d "${LOCAL_WORK}/${d}" ]; then
      cp -r "${LOCAL_WORK}/${d}" "${OUT_DIR}/" || true
    fi
  done
  rm -rf "${LOCAL_WORK}"
  exit "${rc}"
}

rm -rf "${LOCAL_WORK}"
mkdir -p "${LOCAL_WORK}" "${OUT_DIR}"
trap archive_and_clean EXIT

export APPTAINER_CLEANENV=1

echo "=========================================================="
echo " OpenROAD-flow-scripts (ORFS) RTL-to-GDSII"
echo " Job ID:        ${SLURM_JOB_ID}"
echo " Node:          $(hostname)"
echo " Design Config: ${DESIGN_CONFIG}"
echo " Threads:       ${SLURM_CPUS_PER_TASK}"
echo " Local Work:    ${LOCAL_WORK}"
echo " Output Dir:    ${OUT_DIR}"
echo "=========================================================="

apptainer exec \
  --bind /tools:/tools \
  --bind /scratch:/scratch \
  --bind "${SCRATCH_DIR}:${SCRATCH_DIR}" \
  "${CONTAINER_SIF}" \
  bash -c "
    export PATH=/OpenROAD-flow-scripts/tools/install/OpenROAD/bin:/OpenROAD-flow-scripts/tools/install/yosys/bin:\$PATH
    cd /OpenROAD-flow-scripts/flow
    make DESIGN_CONFIG='${DESIGN_CONFIG}' \
         WORK_HOME='${LOCAL_WORK}' \
         NUM_CORES='${SLURM_CPUS_PER_TASK}'
  "

echo "=========================================================="
echo " Completed RTL-to-GDSII Flow!"
echo " Final Report: ${OUT_DIR}/reports/asap7/ibex/base/6_finish.rpt"
echo " Final GDSII:  ${OUT_DIR}/results/asap7/ibex/base/6_final.gds"
echo "=========================================================="
cat "${LOCAL_WORK}/reports/asap7/ibex/base/6_finish.rpt" || true
EOF

chmod +x run_openroad_asap7_ibex.sh
sbatch run_openroad_asap7_ibex.sh
```

To run the quick 1-minute `gcd` smoke test with the exact same script, override `DESIGN_CONFIG`:

```bash
sbatch --export=ALL,DESIGN_CONFIG=./designs/asap7/gcd/config.mk run_openroad_asap7_ibex.sh
```

---

## 5. Teardown

```bash
./gcluster destroy ${DEPLOYMENT_NAME} --auto-approve
```
